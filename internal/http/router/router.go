package router

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/domainerror"
	"github.com/Kerefall/template/internal/http/middleware"
)

// Handlers - ручки сервиса. Контейнер собирает их и передаёт сюда.
type Handlers struct {
	CreateTrip func(w http.ResponseWriter, r *http.Request)
	GetTrip    func(w http.ResponseWriter, r *http.Request, id uuid.UUID)
	FinishTrip func(w http.ResponseWriter, r *http.Request, id uuid.UUID)
	Health     func(w http.ResponseWriter, r *http.Request)
	Ready      func(w http.ResponseWriter, r *http.Request)
}

// server реализует сгенерированный интерфейс и передаёт вызовы хендлерам.
type server struct {
	api.Unimplemented
	handlers Handlers
}

func (s server) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	s.handlers.CreateTrip(w, r)
}

func (s server) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.handlers.GetTrip(w, r, tripID)
}

func (s server) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.handlers.FinishTrip(w, r, tripID)
}

func (s server) Health(w http.ResponseWriter, r *http.Request) {
	s.handlers.Health(w, r)
}

func (s server) Ready(w http.ResponseWriter, r *http.Request) {
	s.handlers.Ready(w, r)
}

// New создаёт роутер. Порядок middleware: Recover -> Tracing -> Logging -> Metrics.
func New(handlers Handlers, log *slog.Logger) (http.Handler, error) {
	router := chi.NewRouter()

	// Middleware срабатывают до роутинга, а шаблон маршрута нужен им заранее
	// (для имени спана и лейблов метрик), поэтому находим его сами.
	routeOf := func(r *http.Request) string {
		rctx := chi.NewRouteContext()
		if router.Match(rctx, r.Method, r.URL.Path) {
			return rctx.RoutePattern()
		}

		return ""
	}

	metrics, err := middleware.Metrics(routeOf)
	if err != nil {
		return nil, fmt.Errorf("create metrics middleware: %w", err)
	}

	router.Use(
		middleware.Recover(log),
		middleware.Tracing(routeOf),
		middleware.Logging(log, routeOf),
		metrics,
	)

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		domainerror.Write(w, r, nil, domainerror.ErrRouteNotFound)
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		domainerror.Write(w, r, nil, domainerror.ErrMethodNotAllowed)
	})

	api.HandlerWithOptions(server{handlers: handlers}, api.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: bindError,
	})

	return router, nil
}

// bindError вызывается сгенерированным кодом, когда не удалось разобрать
// параметры запроса (например, tripId не UUID).
func bindError(w http.ResponseWriter, r *http.Request, err error) {
	detail := "request validation failed"

	var formatErr *api.InvalidParamFormatError
	if errors.As(err, &formatErr) {
		detail = fmt.Sprintf("invalid value of parameter %q", formatErr.ParamName)
	}

	domainerror.Write(w, r, nil, domainerror.InvalidRequest(detail))
}
