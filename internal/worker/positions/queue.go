package positions

import (
	"container/heap"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// Task - поездка, по которой нужно запросить координату.
type Task struct {
	TripID   uuid.UUID
	DriverID uuid.UUID
	// ActivityAt - время последней координаты (или started_at): чем оно раньше,
	// тем дольше поездка молчит и тем раньше её нужно опросить.
	ActivityAt time.Time
	// Link - спан сканера, поставившего задачу. Обработка воркером связывается
	// с ним через span link, а не через родителя: она идёт асинхронно.
	Link trace.SpanContext
}

type offerResult int

const (
	// offerQueued - задача поставлена в очередь.
	offerQueued offerResult = iota
	// offerEvicted - задача поставлена, вытеснив более свежую.
	offerEvicted
	// offerDuplicate - поездка уже в очереди или обрабатывается.
	offerDuplicate
	// offerDropped - очередь полна, а задача не старше самой свежей в ней.
	offerDropped
)

type entry struct {
	task      Task
	seq       uint64
	oldestIdx int
	freshIdx  int
	// queued - задача лежит в очереди; false, если её уже взял воркер.
	queued bool
	// cancelled - по поездке пришла координата, пока воркер уже взял задачу.
	cancelled bool
}

// older: a молчит дольше, чем b. При равном времени раньше поставленная.
func older(a, b *entry) bool {
	if a.task.ActivityAt.Equal(b.task.ActivityAt) {
		return a.seq < b.seq
	}

	return a.task.ActivityAt.Before(b.task.ActivityAt)
}

// oldestHeap держит наверху поездку, которая молчит дольше всех: её берёт воркер.
type oldestHeap []*entry

func (h oldestHeap) Len() int           { return len(h) }
func (h oldestHeap) Less(i, j int) bool { return older(h[i], h[j]) }
func (h oldestHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].oldestIdx = i
	h[j].oldestIdx = j
}

func (h *oldestHeap) Push(x any) {
	e := x.(*entry) //nolint:forcetypeassert // heap.Push вызывается только с *entry
	e.oldestIdx = len(*h)
	*h = append(*h, e)
}

func (h *oldestHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	e.oldestIdx = -1

	return e
}

// freshestHeap держит наверху самую свежую поездку: её вытесняют при переполнении.
type freshestHeap []*entry

func (h freshestHeap) Len() int           { return len(h) }
func (h freshestHeap) Less(i, j int) bool { return older(h[j], h[i]) }
func (h freshestHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].freshIdx = i
	h[j].freshIdx = j
}

func (h *freshestHeap) Push(x any) {
	e := x.(*entry) //nolint:forcetypeassert // heap.Push вызывается только с *entry
	e.freshIdx = len(*h)
	*h = append(*h, e)
}

func (h *freshestHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	e.freshIdx = -1

	return e
}

// queue - ограниченная очередь с приоритетом: первой отдаёт поездку, которая молчит
// дольше всех. Она же хранит множество pending: поездки, которые сейчас в очереди
// или обрабатываются воркером. По нему работает дедупликация, и оно же защищает
// от повторной постановки, пока предыдущий запрос в Push Service ещё идёт.
//
// Все операции выполняются под одним мьютексом, поэтому между проверкой
// «поездка уже здесь?» и постановкой нет окна для гонки.
type queue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	capacity int
	seq      uint64
	oldest   oldestHeap
	freshest freshestHeap
	pending  map[uuid.UUID]*entry
	closed   bool
}

func newQueue(capacity int) *queue {
	q := &queue{
		capacity: capacity,
		pending:  make(map[uuid.UUID]*entry),
	}
	q.notEmpty = sync.NewCond(&q.mu)

	return q
}

// offer ставит задачу в очередь без блокировки.
func (q *queue) offer(t Task) offerResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return offerDropped
	}
	if _, ok := q.pending[t.TripID]; ok {
		return offerDuplicate
	}

	result := offerQueued
	if len(q.oldest) >= q.capacity {
		freshest := q.freshest[0]
		// Новая поездка молчит дольше самой свежей в очереди - она занимает её место.
		// Иначе дропается сама новая.
		if !t.ActivityAt.Before(freshest.task.ActivityAt) {
			return offerDropped
		}
		q.removeLocked(freshest)
		result = offerEvicted
	}

	e := &entry{task: t, seq: q.seq, queued: true}
	q.seq++
	heap.Push(&q.oldest, e)
	heap.Push(&q.freshest, e)
	q.pending[t.TripID] = e
	q.notEmpty.Signal()

	return result
}

// take ждёт задачу и отдаёт самую давнюю. Взятая задача остаётся в pending, пока
// воркер не вызовет done. Если очередь закрыта и пуста, возвращает false.
func (q *queue) take() (Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.oldest) == 0 && !q.closed {
		q.notEmpty.Wait()
	}
	if len(q.oldest) == 0 {
		return Task{}, false
	}

	e := heap.Pop(&q.oldest).(*entry) //nolint:forcetypeassert // в куче только *entry
	heap.Remove(&q.freshest, e.freshIdx)
	e.queued = false

	return e.task, true
}

// shouldSend проверяет перед отправкой, не пришла ли по поездке координата
// после того, как воркер взял задачу.
func (q *queue) shouldSend(tripID uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	e, ok := q.pending[tripID]

	return ok && !e.cancelled
}

// done снимает поездку с учёта после обработки: её снова можно ставить в очередь.
func (q *queue) done(tripID uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.pending, tripID)
}

// cancel вызывается, когда по поездке пришла координата. Задачу в очереди
// убираем совсем, а у уже взятой воркером ставим отметку, и воркер не пойдёт в Push Service.
func (q *queue) cancel(tripID uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()

	e, ok := q.pending[tripID]
	if !ok {
		return
	}
	if e.queued {
		q.removeLocked(e)
		return
	}
	e.cancelled = true
}

// close запрещает новые задачи и будит воркеров: они дорабатывают то, что уже
// в очереди, и выходят.
func (q *queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.notEmpty.Broadcast()
}

// discard закрывает очередь и выбрасывает всё, что в ней осталось.
// Нужна, когда время на остановку вышло.
func (q *queue) discard() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	for _, e := range q.oldest {
		delete(q.pending, e.task.TripID)
	}
	q.oldest = nil
	q.freshest = nil
	q.notEmpty.Broadcast()
}

func (q *queue) size() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.oldest)
}

func (q *queue) removeLocked(e *entry) {
	heap.Remove(&q.oldest, e.oldestIdx)
	heap.Remove(&q.freshest, e.freshIdx)
	delete(q.pending, e.task.TripID)
}
