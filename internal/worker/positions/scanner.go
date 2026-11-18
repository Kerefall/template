package positions

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Kerefall/template/internal/model"
)

// Значения лейбла result у tripgo.stale_checks.
const (
	resultQueued    = "queued"
	resultDuplicate = "duplicate"
	resultDropped   = "dropped"
)

// scanner - одна горутина с тикером. На каждом тике забирает из базы устаревшие
// поездки порциями и ставит их в очередь, не блокируясь.
type scanner struct {
	repo       staleRepository
	queue      *queue
	metrics    metrics
	log        *slog.Logger
	tracer     trace.Tracer
	staleAfter time.Duration
	interval   time.Duration
	batchSize  int
	newTicker  func(time.Duration) Ticker
}

// run крутится, пока не отменён контекст. Обход выполняется в этой же горутине,
// поэтому два обхода параллельно не бывает: тик, пришедший во время обхода,
// будет обработан только после его конца.
func (s *scanner) run(ctx context.Context) {
	ticker := s.newTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			s.scan(ctx)
		}
	}
}

// scan выполняет один обход: читает порции и ставит поездки в очередь.
// Если очередь заполнилась, обход заканчивается: остальные поездки всё равно
// не поместились бы, а они попадут в выборку на следующем тике.
func (s *scanner) scan(ctx context.Context) {
	ctx, span := s.tracer.Start(ctx, "positions.scan")
	defer span.End()

	link := span.SpanContext()

	var cursor *model.StaleCursor
	for ctx.Err() == nil {
		batch, err := s.repo.ListStale(ctx, s.staleAfter, s.batchSize, cursor)
		if err != nil {
			// При остановке сервиса запрос обрывается отменой контекста: это не ошибка.
			if ctx.Err() == nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "list stale trips failed")
				s.log.ErrorContext(ctx, "list stale trips failed", "error", err)
			}

			return
		}

		full := s.enqueue(ctx, batch, link)
		if full || len(batch) < s.batchSize {
			return
		}

		last := batch[len(batch)-1]
		cursor = &model.StaleCursor{ActivityAt: last.ActivityAt, ID: last.ID}
	}
}

// enqueue ставит порцию в очередь и учитывает результат каждой постановки.
// Возвращает true, если очередь оказалась заполнена.
func (s *scanner) enqueue(ctx context.Context, batch []model.StaleTrip, link trace.SpanContext) bool {
	var dropped int

	for _, t := range batch {
		task := Task{TripID: t.ID, DriverID: t.DriverID, ActivityAt: t.ActivityAt, Link: link}

		switch s.queue.offer(task) {
		case offerQueued:
			s.metrics.StaleCheck(ctx, resultQueued)
		case offerEvicted:
			// Новая поездка заняла место более свежей: свежая дропнута.
			s.metrics.StaleCheck(ctx, resultQueued)
			s.metrics.StaleCheck(ctx, resultDropped)
			dropped++
		case offerDuplicate:
			s.metrics.StaleCheck(ctx, resultDuplicate)
		case offerDropped:
			s.metrics.StaleCheck(ctx, resultDropped)
			dropped++
		}
	}

	if dropped > 0 {
		s.log.WarnContext(ctx, "position request queue is full, trips dropped",
			"dropped", dropped,
			"queue_capacity", s.queue.capacity,
		)
	}

	return dropped > 0
}
