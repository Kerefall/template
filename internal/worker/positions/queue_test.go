package positions

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var base = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// taskAt создаёт задачу, которая молчит с момента base+offset минут.
func taskAt(offsetMinutes int) Task {
	return Task{
		TripID:     uuid.New(),
		DriverID:   uuid.New(),
		ActivityAt: base.Add(time.Duration(offsetMinutes) * time.Minute),
	}
}

func mustTake(t *testing.T, q *queue) Task {
	t.Helper()

	task, ok := q.take()
	require.True(t, ok)

	return task
}

func TestQueue_TakesTheLongestSilentFirst(t *testing.T) {
	q := newQueue(10)
	middle, oldest, newest := taskAt(5), taskAt(1), taskAt(9)

	// Кладём вперемешку.
	require.Equal(t, offerQueued, q.offer(middle))
	require.Equal(t, offerQueued, q.offer(newest))
	require.Equal(t, offerQueued, q.offer(oldest))

	require.Equal(t, oldest.TripID, mustTake(t, q).TripID)
	require.Equal(t, middle.TripID, mustTake(t, q).TripID)
	require.Equal(t, newest.TripID, mustTake(t, q).TripID)
}

func TestQueue_EqualTimestampsKeepArrivalOrder(t *testing.T) {
	q := newQueue(10)
	first, second := taskAt(3), taskAt(3)

	q.offer(first)
	q.offer(second)

	require.Equal(t, first.TripID, mustTake(t, q).TripID)
	require.Equal(t, second.TripID, mustTake(t, q).TripID)
}

func TestQueue_TripIsNotQueuedTwice(t *testing.T) {
	q := newQueue(10)
	task := taskAt(1)

	require.Equal(t, offerQueued, q.offer(task))
	require.Equal(t, offerDuplicate, q.offer(task), "поездка уже в очереди")

	taken := mustTake(t, q)
	require.Equal(t, offerDuplicate, q.offer(taken), "поездка обрабатывается воркером: тоже дубликат")

	q.done(taken.TripID)
	require.Equal(t, offerQueued, q.offer(taken), "после обработки поездку можно ставить снова")
}

func TestQueue_FullQueueEvictsTheFreshestForAnOlderTrip(t *testing.T) {
	q := newQueue(2)
	t2, t3, t1 := taskAt(2), taskAt(3), taskAt(1)

	q.offer(t2)
	q.offer(t3)

	// t1 молчит дольше самой свежей (t3): занимает её место.
	require.Equal(t, offerEvicted, q.offer(t1))
	require.Equal(t, 2, q.size())

	require.Equal(t, t1.TripID, mustTake(t, q).TripID)
	require.Equal(t, t2.TripID, mustTake(t, q).TripID)

	// Вытесненная поездка снова может быть поставлена: на неё нет отметки pending.
	require.Equal(t, offerQueued, q.offer(t3))
}

func TestQueue_FullQueueDropsAFresherTrip(t *testing.T) {
	q := newQueue(2)
	q.offer(taskAt(1))
	q.offer(taskAt(2))

	require.Equal(t, offerDropped, q.offer(taskAt(3)), "свежее всех в очереди: дропается сама")
	require.Equal(t, offerDropped, q.offer(taskAt(2)), "такая же свежая, как самая свежая: тоже дропается")
	require.Equal(t, 2, q.size())
}

func TestQueue_CancelRemovesQueuedTask(t *testing.T) {
	q := newQueue(10)
	a, b := taskAt(1), taskAt(2)
	q.offer(a)
	q.offer(b)

	q.cancel(a.TripID)

	require.Equal(t, 1, q.size())
	require.Equal(t, b.TripID, mustTake(t, q).TripID)
	require.Equal(t, offerQueued, q.offer(a), "отменённую поездку можно поставить снова")
}

func TestQueue_CancelMarksTaskAlreadyTakenByWorker(t *testing.T) {
	q := newQueue(10)
	task := taskAt(1)
	q.offer(task)

	taken := mustTake(t, q)
	require.True(t, q.shouldSend(taken.TripID))

	q.cancel(taken.TripID)

	require.False(t, q.shouldSend(taken.TripID), "после отмены воркер не должен идти в Push Service")
}

func TestQueue_CancelOfUnknownTripIsHarmless(t *testing.T) {
	q := newQueue(10)

	q.cancel(uuid.New())

	require.Equal(t, 0, q.size())
}

func TestQueue_TakeWaitsForATask(t *testing.T) {
	q := newQueue(10)
	task := taskAt(1)

	got := make(chan Task, 1)
	go func() {
		taken, _ := q.take()
		got <- taken
	}()

	// Пока задач нет, take ждёт.
	select {
	case <-got:
		t.Fatal("take вернулся, хотя очередь пуста")
	default:
	}

	q.offer(task)

	select {
	case taken := <-got:
		require.Equal(t, task.TripID, taken.TripID)
	case <-time.After(5 * time.Second):
		t.Fatal("take не проснулся после offer")
	}
}

func TestQueue_CloseLetsWorkersDrainThenStops(t *testing.T) {
	q := newQueue(10)
	task := taskAt(1)
	q.offer(task)

	q.close()

	require.Equal(t, task.TripID, mustTake(t, q).TripID, "очередь отдаёт то, что уже в ней лежит")
	_, ok := q.take()
	require.False(t, ok, "а потом сообщает, что работа закончилась")
	require.Equal(t, offerDropped, q.offer(taskAt(2)), "после закрытия новые задачи не принимаются")
}

func TestQueue_CloseWakesWaitingWorker(t *testing.T) {
	q := newQueue(10)

	finished := make(chan bool, 1)
	go func() {
		_, ok := q.take()
		finished <- ok
	}()

	q.close()

	select {
	case ok := <-finished:
		require.False(t, ok)
	case <-time.After(5 * time.Second):
		t.Fatal("take не проснулся после close")
	}
}

func TestQueue_DiscardDropsEverything(t *testing.T) {
	q := newQueue(10)
	q.offer(taskAt(1))
	q.offer(taskAt(2))

	q.discard()

	require.Equal(t, 0, q.size())
	_, ok := q.take()
	require.False(t, ok)
}
