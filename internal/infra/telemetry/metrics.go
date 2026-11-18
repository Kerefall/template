package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "github.com/Kerefall/template/internal/infra/telemetry"

// Результаты попыток запросить координату (tripgo.position_requests).
const (
	resultSuccess = "success"
	resultError   = "error"
	resultDropped = "dropped"
)

// Metrics - бизнес-метрики сервиса. Реализует интерфейсы metrics в пакетах
// usecase и worker, поэтому они про OpenTelemetry ничего не знают.
type Metrics struct {
	meter metric.Meter

	created         metric.Int64Counter
	completed       metric.Int64Counter
	positionsSaved  metric.Int64Counter
	staleChecks     metric.Int64Counter
	requests        metric.Int64Counter
	requestDuration metric.Float64Histogram
}

func newMetrics() (*Metrics, error) {
	m := &Metrics{meter: otel.Meter(meterName)}

	var err error
	counters := []struct {
		target *metric.Int64Counter
		name   string
		unit   string
		desc   string
	}{
		{&m.created, "tripgo.trips.created", "{trip}", "Created trips."},
		{&m.completed, "tripgo.trips.completed", "{trip}", "Completed trips."},
		{&m.positionsSaved, "tripgo.positions.saved", "{position}", "Saved route positions."},
		{&m.staleChecks, "tripgo.stale_checks", "{check}", "What the scanner did with a stale trip on a tick."},
		{&m.requests, "tripgo.position_requests", "{request}", "Outcomes of position requests to Push Service."},
	}
	for _, c := range counters {
		*c.target, err = m.meter.Int64Counter(c.name, metric.WithUnit(c.unit), metric.WithDescription(c.desc))
		if err != nil {
			return nil, fmt.Errorf("create counter %s: %w", c.name, err)
		}
	}

	m.requestDuration, err = m.meter.Float64Histogram(
		"tripgo.position_requests.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of Push Service calls."),
	)
	if err != nil {
		return nil, fmt.Errorf("create position request duration histogram: %w", err)
	}

	return m, nil
}

func (m *Metrics) TripCreated(ctx context.Context) {
	m.created.Add(ctx, 1)
}

func (m *Metrics) TripCompleted(ctx context.Context) {
	m.completed.Add(ctx, 1)
}

func (m *Metrics) PositionSaved(ctx context.Context) {
	m.positionsSaved.Add(ctx, 1)
}

// StaleCheck считает, что сканер сделал с поездкой: queued, duplicate или dropped.
func (m *Metrics) StaleCheck(ctx context.Context, result string) {
	m.staleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

// RequestFinished учитывает завершённый вызов Push Service: счётчик и длительность.
func (m *Metrics) RequestFinished(ctx context.Context, success bool, d time.Duration) {
	result := resultError
	if success {
		result = resultSuccess
	}

	attrs := metric.WithAttributes(attribute.String("result", result))
	m.requests.Add(ctx, 1, attrs)
	m.requestDuration.Record(ctx, d.Seconds(), attrs)
}

// RequestDropped учитывает задачу, которую отбросили без вызова Push Service
// (по поездке пришла координата).
func (m *Metrics) RequestDropped(ctx context.Context) {
	m.requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", resultDropped)))
}

// RegisterPositionGauges регистрирует метрики, значение которых читается в момент
// сбора: размер очереди и число занятых воркеров.
func (m *Metrics) RegisterPositionGauges(queueSize, workersActive func() int64) error {
	gauges := []struct {
		name string
		unit string
		desc string
		read func() int64
	}{
		{"tripgo.position_requests.queue.size", "{task}", "Tasks waiting in the queue.", queueSize},
		{"tripgo.position_requests.workers.active", "{worker}", "Workers busy right now.", workersActive},
	}

	for _, g := range gauges {
		read := g.read
		_, err := m.meter.Int64ObservableGauge(g.name,
			metric.WithUnit(g.unit),
			metric.WithDescription(g.desc),
			metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
				o.Observe(read())
				return nil
			}),
		)
		if err != nil {
			return fmt.Errorf("create gauge %s: %w", g.name, err)
		}
	}

	return nil
}
