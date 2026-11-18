// Package positions - опрос устаревших поездок: сканер по тикеру, ограниченная
// очередь с приоритетом и пул воркеров, которые просят устройство водителя
// прислать координату через Push Service.
package positions

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/Kerefall/template/internal/worker/positions"

type Config struct {
	CheckInterval  time.Duration
	StaleAfter     time.Duration
	Workers        int
	QueueSize      int
	BatchSize      int
	RequestTimeout time.Duration
}

// Deps - зависимости компонента. NewTicker необязателен: по умолчанию time.Ticker.
type Deps struct {
	Repo      staleRepository
	Gateway   pushGateway
	Metrics   metrics
	Log       *slog.Logger
	NewTicker func(time.Duration) Ticker
}

type realTicker struct {
	*time.Ticker
}

func (t realTicker) C() <-chan time.Time {
	return t.Ticker.C
}

func newRealTicker(d time.Duration) Ticker {
	return realTicker{Ticker: time.NewTicker(d)}
}

// Component - сканер, очередь и воркеры как одно целое.
type Component struct {
	cfg     Config
	queue   *queue
	scanner *scanner
	gateway pushGateway
	metrics metrics
	log     *slog.Logger
	tracer  trace.Tracer

	active atomic.Int64

	scanCancel   context.CancelFunc
	workerCancel context.CancelFunc
	scanWG       sync.WaitGroup
	workerWG     sync.WaitGroup
}

func New(cfg Config, deps Deps) *Component {
	newTicker := deps.NewTicker
	if newTicker == nil {
		newTicker = newRealTicker
	}

	tracer := otel.Tracer(tracerName)
	q := newQueue(cfg.QueueSize)

	return &Component{
		cfg:     cfg,
		queue:   q,
		gateway: deps.Gateway,
		metrics: deps.Metrics,
		log:     deps.Log,
		tracer:  tracer,
		scanner: &scanner{
			repo:       deps.Repo,
			queue:      q,
			metrics:    deps.Metrics,
			log:        deps.Log,
			tracer:     tracer,
			staleAfter: cfg.StaleAfter,
			interval:   cfg.CheckInterval,
			batchSize:  cfg.BatchSize,
			newTicker:  newTicker,
		},
	}
}

// Start запускает воркеров и сканер. Воркеры стартуют при запуске сервиса и живут
// до Stop: горутина на задачу не создаётся.
//
// Контекст воркеров не наследует отмену ctx: при остановке они должны дорабатывать
// очередь, а не обрываться вместе с сигналом. Остановка идёт только через Stop.
func (c *Component) Start(ctx context.Context) {
	scanCtx, scanCancel := context.WithCancel(ctx)
	workerCtx, workerCancel := context.WithCancel(context.WithoutCancel(ctx))
	c.scanCancel = scanCancel
	c.workerCancel = workerCancel

	for range c.cfg.Workers {
		c.workerWG.Add(1)
		go func() {
			defer c.workerWG.Done()
			c.worker(workerCtx)
		}()
	}

	c.scanWG.Add(1)
	go func() {
		defer c.scanWG.Done()
		c.scanner.run(scanCtx)
	}()
}

// Stop останавливает компонент в фиксированном порядке:
//  1. сканер: новые задачи больше не появляются;
//  2. очередь закрывается, воркеры дорабатывают то, что в ней осталось;
//  3. если время ctx вышло раньше, остаток очереди выбрасывается, а запросы в
//     Push Service, которые ещё идут, отменяются. Тогда Stop возвращает ошибку.
//
// После возврата не остаётся ни одной горутины компонента.
func (c *Component) Stop(ctx context.Context) error {
	c.scanCancel()
	c.scanWG.Wait()

	c.queue.close()

	drained := make(chan struct{})
	go func() {
		c.workerWG.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		c.workerCancel()
		return nil
	case <-ctx.Done():
		c.queue.discard()
		c.workerCancel()
		<-drained

		return fmt.Errorf("position workers did not drain the queue in time: %w", ctx.Err())
	}
}

// Cancel вызывается, когда по поездке пришла координата: задача убирается из
// очереди, а если её уже взял воркер, то он не пойдёт в Push Service.
func (c *Component) Cancel(tripID uuid.UUID) {
	c.queue.cancel(tripID)
}

// QueueSize - сколько задач сейчас ждёт в очереди.
func (c *Component) QueueSize() int64 {
	return int64(c.queue.size())
}

// ActiveWorkers - сколько воркеров заняты прямо сейчас.
func (c *Component) ActiveWorkers() int64 {
	return c.active.Load()
}

func (c *Component) worker(ctx context.Context) {
	for {
		task, ok := c.queue.take()
		if !ok {
			return
		}

		c.process(ctx, task)
	}
}

func (c *Component) process(ctx context.Context, task Task) {
	c.active.Add(1)
	defer c.active.Add(-1)
	defer c.queue.done(task.TripID)
	defer func() {
		// Паника в обработке одной задачи не должна убивать воркера.
		if rec := recover(); rec != nil {
			c.log.ErrorContext(ctx, "position worker panic recovered",
				"trip_id", task.TripID.String(),
				"panic", fmt.Sprint(rec),
				"stack", string(debug.Stack()),
			)
		}
	}()

	// Спан обработки - новый корень, связанный со спаном сканера через link:
	// обработка асинхронна и переживает исходный обход.
	ctx, span := c.tracer.Start(ctx, "positions.process",
		trace.WithNewRoot(),
		trace.WithLinks(trace.Link{SpanContext: task.Link}),
	)
	defer span.End()

	// Пока задача лежала в очереди, по поездке могла прийти координата.
	if !c.queue.shouldSend(task.TripID) {
		c.metrics.RequestDropped(ctx)
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()

	start := time.Now()
	err := c.gateway.RequestPosition(reqCtx, task.DriverID, task.TripID)
	c.metrics.RequestFinished(ctx, err == nil, time.Since(start))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "position request failed")
		c.log.WarnContext(ctx, "position request failed",
			"trip_id", task.TripID.String(),
			"driver_id", task.DriverID.String(),
			"error", err,
		)
	}
}
