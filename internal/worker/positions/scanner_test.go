package positions

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/model"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// staleTrips создаёт n поездок, молчащих всё дольше: от самой свежей к самой давней
// порядок в выдаче базы обратный, поэтому возвращаем по возрастанию ActivityAt.
func staleTrips(n int) []model.StaleTrip {
	trips := make([]model.StaleTrip, n)
	for i := range trips {
		trips[i] = model.StaleTrip{
			ID:         uuid.New(),
			DriverID:   uuid.New(),
			ActivityAt: base.Add(time.Duration(i) * time.Second),
		}
	}

	return trips
}

// pagedRepo отдаёт срезы поездок так же, как ListStale: порциями по limit,
// продолжая после курсора.
func pagedRepo(t *testing.T, trips []model.StaleTrip, calls *[]*model.StaleCursor) *MockstaleRepository {
	t.Helper()

	repo := NewMockstaleRepository(gomock.NewController(t))
	repo.EXPECT().
		ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ time.Duration, limit int, after *model.StaleCursor) ([]model.StaleTrip, error) {
			*calls = append(*calls, after)

			start := 0
			if after != nil {
				for start < len(trips) && !trips[start].ActivityAt.After(after.ActivityAt) {
					start++
				}
			}
			end := min(start+limit, len(trips))

			return trips[start:end], nil
		}).
		AnyTimes()

	return repo
}

// countingMetrics считает результаты tripgo.stale_checks.
func countingMetrics(t *testing.T) (*Mockmetrics, map[string]int) {
	t.Helper()

	counts := make(map[string]int)
	m := NewMockmetrics(gomock.NewController(t))
	m.EXPECT().StaleCheck(gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, result string) { counts[result]++ }).
		AnyTimes()

	return m, counts
}

// testBatchSize - размер порции во всех тестах сканера.
const testBatchSize = 100

func newScanner(repo staleRepository, m metrics, queueSize int) *scanner {
	return &scanner{
		repo:       repo,
		queue:      newQueue(queueSize),
		metrics:    m,
		log:        discardLogger(),
		tracer:     otel.Tracer("test"),
		staleAfter: time.Minute,
		interval:   time.Second,
		batchSize:  testBatchSize,
	}
}

func TestScanner_QueuesStaleTrips(t *testing.T) {
	var calls []*model.StaleCursor
	m, counts := countingMetrics(t)
	s := newScanner(pagedRepo(t, staleTrips(3), &calls), m, 10)

	s.scan(context.Background())

	require.Equal(t, 3, s.queue.size())
	require.Equal(t, map[string]int{resultQueued: 3}, counts)
}

func TestScanner_DoesNotQueueATripTwice(t *testing.T) {
	var calls []*model.StaleCursor
	m, counts := countingMetrics(t)
	s := newScanner(pagedRepo(t, staleTrips(2), &calls), m, 10)

	s.scan(context.Background()) // первый тик
	s.scan(context.Background()) // второй тик, воркеры ещё не разобрали очередь

	require.Equal(t, 2, s.queue.size(), "в очереди по-прежнему две задачи")
	require.Equal(t, map[string]int{resultQueued: 2, resultDuplicate: 2}, counts)
}

func TestScanner_FullQueueGivesDroppedAndDoesNotBlock(t *testing.T) {
	var calls []*model.StaleCursor
	m, counts := countingMetrics(t)
	s := newScanner(pagedRepo(t, staleTrips(5), &calls), m, 2)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.scan(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("сканер заблокировался на записи в полную очередь")
	}

	require.Equal(t, 2, s.queue.size())
	require.Equal(t, map[string]int{resultQueued: 2, resultDropped: 3}, counts)
}

func TestScanner_ReadsInBatches(t *testing.T) {
	var calls []*model.StaleCursor
	trips := staleTrips(250)
	m, counts := countingMetrics(t)
	s := newScanner(pagedRepo(t, trips, &calls), m, 250)

	s.scan(context.Background())

	require.Len(t, calls, 3, "250 поездок порциями по 100: три запроса")
	require.Nil(t, calls[0])
	require.Equal(t, trips[99].ID, calls[1].ID, "вторая порция продолжается после сотой поездки")
	require.Equal(t, trips[199].ID, calls[2].ID)
	require.Equal(t, 250, counts[resultQueued])
	require.Equal(t, 250, s.queue.size())
}

func TestScanner_StopsReadingWhenTheQueueIsFull(t *testing.T) {
	var calls []*model.StaleCursor
	m, counts := countingMetrics(t)
	s := newScanner(pagedRepo(t, staleTrips(250), &calls), m, 50)

	s.scan(context.Background())

	require.Len(t, calls, 1, "очередь заполнилась на первой порции: второго запроса нет")
	require.Equal(t, 50, counts[resultQueued])
	require.Equal(t, 50, counts[resultDropped])
}

func TestScanner_RepositoryErrorEndsThePassWithoutPanic(t *testing.T) {
	repo := NewMockstaleRepository(gomock.NewController(t))
	repo.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, context.DeadlineExceeded)
	m, counts := countingMetrics(t)
	s := newScanner(repo, m, 10)

	s.scan(context.Background())

	require.Empty(t, counts)
	require.Equal(t, 0, s.queue.size())
}
