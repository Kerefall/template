package handler

import (
	"context"
	"io"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kerefall/template/internal/domain"
	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/repository"
)

const maxBodyBytes = 1 << 20

func toAPITrip(t repository.Trip) api.Trip {
	return api.Trip{
		Id:         openapi_types.UUID(t.ID),
		UserId:     openapi_types.UUID(t.UserID),
		DriverId:   openapi_types.UUID(t.DriverID),
		StartPoint: api.Coordinates{Latitude: t.StartLatitude, Longitude: t.StartLongitude},
		EndPoint:   api.Coordinates{Latitude: t.EndLatitude, Longitude: t.EndLongitude},
		Price:      t.Price,
		Status:     api.TripStatus(t.Status),
		StartedAt:  t.StartedAt,
		FinishedAt: t.FinishedAt,
	}
}

func (h Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeInvalidRequest(w, r, "Request body could not be read")
		return
	}

	data, err := parseTripData(body)
	if err != nil {
		writeInvalidRequest(w, r, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.QueryTimeout)
	defer cancel()

	trip := repository.Trip{
		ID:             uuid.New(),
		UserID:         uuid.UUID(data.UserId),
		DriverID:       uuid.UUID(data.DriverId),
		StartLatitude:  data.StartPoint.Latitude,
		StartLongitude: data.StartPoint.Longitude,
		EndLatitude:    data.EndPoint.Latitude,
		EndLongitude:   data.EndPoint.Longitude,
		Price:          data.Price,
	}

	// Поездка и запись в журнале статусов создаются атомарно.
	var created repository.Trip
	err = h.Tx.Do(ctx, func(ctx context.Context) error {
		var err error

		created, err = h.Trips.CreateTrip(ctx, trip)
		if err != nil {
			return err
		}

		return h.Trips.AddStatusHistory(ctx, created.ID, "", domain.StatusActive, "trip created")
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.Log.Info("trip created", "trip_id", created.ID.String(), "driver_id", created.DriverID.String())

	w.Header().Set("Location", "/api/v1/trips/"+created.ID.String())
	writeJSON(w, http.StatusCreated, toAPITrip(created))
}

func (h Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	ctx, cancel := context.WithTimeout(r.Context(), h.QueryTimeout)
	defer cancel()

	trip, err := h.Trips.GetTrip(ctx, uuid.UUID(tripId))
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(trip))
}

func (h Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	ctx, cancel := context.WithTimeout(r.Context(), h.QueryTimeout)
	defer cancel()

	id := uuid.UUID(tripId)

	// Смена статуса и запись в журнал - одна транзакция.
	var finished repository.Trip
	err := h.Tx.Do(ctx, func(ctx context.Context) error {
		var err error

		finished, err = h.Trips.FinishTrip(ctx, id)
		if err != nil {
			return err
		}

		return h.Trips.AddStatusHistory(ctx, id, domain.StatusActive, domain.StatusCompleted, "trip finished")
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.Log.Info("trip completed", "trip_id", id.String())

	writeJSON(w, http.StatusOK, toAPITrip(finished))
}
