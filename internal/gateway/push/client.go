// Package push - HTTP-клиент Push Service.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Форма запроса к Push Service. Сверяется с contracts/openapi/push-service.openapi.yaml:
// если там другой путь или другие имена полей, правится только этот блок.
const (
	pushPath     = "/api/v1/push"
	kindPosition = "REQUEST_POSITION"
)

type request struct {
	Kind        string      `json:"kind"`
	RecipientID uuid.UUID   `json:"recipient_id"`
	Data        requestData `json:"data"`
}

type requestData struct {
	TripID uuid.UUID `json:"trip_id"`
}

var (
	// ErrTransport - запрос не дошёл до сервиса или не получил ответ (сеть, таймаут).
	ErrTransport = errors.New("push service transport error")
	// ErrClientStatus - Push Service ответил кодом 4xx: запрос плохой, повтор не поможет.
	ErrClientStatus = errors.New("push service rejected the request")
	// ErrServerStatus - Push Service ответил кодом 5xx: проблема на его стороне.
	ErrServerStatus = errors.New("push service is failing")
)

// Client - гейтвей в Push Service. Делает одну попытку, без ретраев.
type Client struct {
	baseURL string
	http    *http.Client
	tracer  trace.Tracer
}

// New создаёт клиента со своим http.Client и таймаутами. timeout ограничивает
// весь запрос целиком, в том числе ожидание ответа.
func New(baseURL string, timeout time.Duration) *Client {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: timeout}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
	}

	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout, Transport: transport},
		tracer:  otel.Tracer("github.com/Kerefall/template/internal/gateway/push"),
	}
}

// Close закрывает простаивающие соединения клиента.
func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

// RequestPosition просит устройство водителя прислать координату по поездке.
func (c *Client) RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error {
	ctx, span := c.tracer.Start(ctx, "push.request_position",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("push.kind", kindPosition)),
	)
	defer span.End()

	err := c.send(ctx, driverID, tripID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "push request failed")
	}

	return err
}

func (c *Client) send(ctx context.Context, driverID, tripID uuid.UUID) error {
	body, err := json.Marshal(request{
		Kind:        kindPosition,
		RecipientID: driverID,
		Data:        requestData{TripID: tripID},
	})
	if err != nil {
		return fmt.Errorf("marshal push request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+pushPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransport, err)
	}
	// Тело закрываем всегда, а перед этим вычитываем остаток: так соединение
	// возвращается в пул и переиспользуется.
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return fmt.Errorf("%w: status %d", ErrClientStatus, resp.StatusCode)
	default:
		return fmt.Errorf("%w: status %d", ErrServerStatus, resp.StatusCode)
	}
}
