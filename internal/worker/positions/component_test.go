package positions

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/model"
)

// fakeTicker - тикер, который тикает только когда тест пошлёт значение.
type fakeTicker struct {
	ch chan time.Time
}

func newFakeTicker() *fakeTicker {
	return &fakeTicker{ch: make(chan time.Time, 2)}
}

func (f *fakeTicker) C() <-chan time.Time { return f.ch }
func (f *fakeTicker) Stop()               {}

func (f *fakeTicker) tick() { f.ch <- time.Now() }

type fixture struct {
	gateway *MockpushGateway
	repo    *MockstaleRepository
	metrics *Mockmetrics
	ticker  *fakeTicker
	c       *Component
}

func newFixture(t *testing.T, cfg Config) fixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	f := fixture{
		gateway: NewMockpushGateway(ctrl),
		repo:    NewMockstaleRepository(ctrl),
		metrics: NewMockmetrics(ctrl),
		ticker:  newFakeTicker(),
	}
	f.metrics.EXPECT().StaleCheck(gomock.Any(), gomock.Any()).AnyTimes()
	f.metrics.EXPECT().RequestFinished(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	f.metrics.EXPECT().RequestDropped(gomock.Any()).AnyTimes()

	if cfg.Workers == 0 {
		cfg.Workers = 1
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 100
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = time.Second
	}
	cfg.CheckInterval = time.Second
	cfg.StaleAfter = time.Minute

	f.c = New(cfg, Deps{
		Repo:      f.repo,
		Gateway:   f.gateway,
		Metrics:   f.metrics,
		Log:       discardLogger(),
		NewTicker: func(time.Duration) Ticker { return f.ticker },
	})

	return f
}

func stopWithin(t *testing.T, c *Component, d time.Duration) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()

	return c.Stop(ctx)
}

func TestComponent_WorkersProcessTasksInParallelUpToTheirNumber(t *testing.T) {
	const workers = 3
	f := newFixture(t, Config{Workers: workers})

	var current, peak atomic.Int64
	started := make(chan struct{}, 6)
	release := make(chan struct{})

	f.gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
			n := current.Add(1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			started <- struct{}{}
			<-release
			current.Add(-1)

			return nil
		}).Times(6)

	for i := range 6 {
		require.Equal(t, offerQueued, f.c.queue.offer(taskAt(i)))
	}
	f.c.Start(context.Background())

	// Все три воркера заняты и ждут: четвёртая задача пока не стартовала.
	for range workers {
		<-started
	}
	require.EqualValues(t, workers, f.c.ActiveWorkers())

	close(release)
	require.NoError(t, stopWithin(t, f.c, 5*time.Second))

	require.EqualValues(t, workers, peak.Load(), "одновременно не больше POSITION_WORKERS запросов")
	require.EqualValues(t, 0, f.c.ActiveWorkers())
}

func TestComponent_GatewayErrorDoesNotKillTheWorker(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})

	gomock.InOrder(
		f.gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("push is down")),
		f.gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
	)

	f.c.queue.offer(taskAt(1))
	f.c.queue.offer(taskAt(2))
	f.c.Start(context.Background())

	// Единственный воркер обработал обе задачи: после ошибки он не упал.
	require.NoError(t, stopWithin(t, f.c, 5*time.Second))
}

func TestComponent_CoordinateBetweenQueueingAndProcessingCancelsTheRequest(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})
	task := taskAt(1)
	other := taskAt(2)

	// Вызов только по второй поездке: по первой пришла координата.
	f.gateway.EXPECT().RequestPosition(gomock.Any(), other.DriverID, other.TripID).Return(nil)

	f.c.queue.offer(task)
	f.c.queue.offer(other)
	f.c.Cancel(task.TripID)

	f.c.Start(context.Background())
	require.NoError(t, stopWithin(t, f.c, 5*time.Second))
}

func TestComponent_StopDrainsTheQueue(t *testing.T) {
	f := newFixture(t, Config{Workers: 2})

	var sent atomic.Int64
	f.gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
			sent.Add(1)
			return nil
		}).Times(5)

	for i := range 5 {
		f.c.queue.offer(taskAt(i))
	}
	f.c.Start(context.Background())

	require.NoError(t, stopWithin(t, f.c, 5*time.Second))
	require.EqualValues(t, 5, sent.Load(), "задачи не теряются при остановке")
	require.EqualValues(t, 0, f.c.QueueSize())
}

func TestComponent_StopGivesUpWhenTheBudgetIsOver(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})

	started := make(chan struct{})
	f.gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
			close(started)
			<-ctx.Done() // запрос висит, пока его не отменят

			return ctx.Err()
		}).Times(1)

	f.c.queue.offer(taskAt(1))
	f.c.queue.offer(taskAt(2)) // до неё дело не дойдёт: время на остановку выйдет
	f.c.Start(context.Background())
	<-started

	err := stopWithin(t, f.c, 50*time.Millisecond)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.EqualValues(t, 0, f.c.QueueSize(), "остаток очереди выброшен")
	// Горутин не остаётся: это проверяет goleak в TestMain.
}

func TestComponent_TickMovesStaleTripThroughTheWholePipeline(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})
	trip := model.StaleTrip{ID: uuid.New(), DriverID: uuid.New(), ActivityAt: base}

	f.repo.EXPECT().ListStale(gomock.Any(), time.Minute, 100, nil).Return([]model.StaleTrip{trip}, nil)

	called := make(chan struct{})
	f.gateway.EXPECT().RequestPosition(gomock.Any(), trip.DriverID, trip.ID).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
			close(called)
			return nil
		})

	f.c.Start(context.Background())
	f.ticker.tick()

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("поездка не дошла до Push Service")
	}

	require.NoError(t, stopWithin(t, f.c, 5*time.Second))
}

func TestComponent_ScansNeverOverlap(t *testing.T) {
	f := newFixture(t, Config{Workers: 1})

	var inFlight atomic.Int64
	var overlapped atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})

	f.repo.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, time.Duration, int, *model.StaleCursor) ([]model.StaleTrip, error) {
			if inFlight.Add(1) > 1 {
				overlapped.Store(true)
			}
			entered <- struct{}{}
			<-release
			inFlight.Add(-1)

			return nil, nil
		}).Times(2)

	f.c.Start(context.Background())

	f.ticker.tick()
	<-entered       // первый обход начался и ждёт
	f.ticker.tick() // второй тик пришёл во время первого обхода
	release <- struct{}{}
	<-entered // второй обход стартует только после конца первого
	release <- struct{}{}

	require.NoError(t, stopWithin(t, f.c, 5*time.Second))
	require.False(t, overlapped.Load(), "два обхода не должны идти параллельно")
}
