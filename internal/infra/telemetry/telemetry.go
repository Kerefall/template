// Package telemetry - инициализация OpenTelemetry (трейсы и метрики по OTLP)
// и логгера slog с trace_id и span_id.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Version - версия сборки, попадает в service.version.
var Version = "dev"

const metricsInterval = 10 * time.Second

type Telemetry struct {
	Metrics        *Metrics
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
}

// Setup настраивает глобальные провайдеры трейсов и метрик. Адрес коллектора,
// протокол и сэмплирование берутся из стандартных переменных OTEL_*.
// Если коллектор недоступен, сервис работает дальше: ошибки экспорта
// только попадают в лог.
func Setup(ctx context.Context, log *slog.Logger) (*Telemetry, error) {
	otel.SetErrorHandler(errorHandler{log: log})

	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", "trip-service"),
			attribute.String("service.version", Version),
			attribute.String("service.namespace", "tripgo"),
			attribute.String("deployment.environment", "local"),
		),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	traceExporter, err := newTraceExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	metricExporter, err := newMetricExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(metricsInterval))),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	metrics, err := newMetrics()
	if err != nil {
		return nil, err
	}

	return &Telemetry{
		Metrics:        metrics,
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
	}, nil
}

// errorHandler пишет ошибки экспорта телеметрии в лог.
type errorHandler struct {
	log *slog.Logger
}

func (h errorHandler) Handle(err error) {
	h.log.Warn("telemetry export failed", "error", err)
}

// Shutdown отправляет накопленные данные и останавливает экспорт.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	return errors.Join(
		t.tracerProvider.Shutdown(ctx),
		t.meterProvider.Shutdown(ctx),
	)
}

func useGRPC() bool {
	return strings.EqualFold(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"), "grpc")
}

func newTraceExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	if useGRPC() {
		return otlptracegrpc.New(ctx)
	}

	return otlptracehttp.New(ctx)
}

func newMetricExporter(ctx context.Context) (sdkmetric.Exporter, error) {
	if useGRPC() {
		return otlpmetricgrpc.New(ctx)
	}

	return otlpmetrichttp.New(ctx)
}
