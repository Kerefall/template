package create

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Kerefall/template/internal/domain/trip"
	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/domainerror"
	"github.com/Kerefall/template/internal/model"
)

// parseInput разбирает тело запроса. Здесь проверяется только форма JSON

func parseInput(body []byte) (model.CreateTripInput, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return model.CreateTripInput{}, domainerror.InvalidRequest("request body must be a valid JSON object")
	}

	for _, key := range []string{"user_id", "driver_id", "start_point", "end_point", "price"} {
		if !present(raw, key) {
			return model.CreateTripInput{}, domainerror.InvalidRequest(fmt.Sprintf("field %q is required", key))
		}
	}

	for _, key := range []string{"start_point", "end_point"} {
		var point map[string]json.RawMessage
		if err := json.Unmarshal(raw[key], &point); err != nil {
			return model.CreateTripInput{}, domainerror.InvalidRequest(fmt.Sprintf("field %q must be an object", key))
		}
		for _, coord := range []string{"latitude", "longitude"} {
			if !present(point, coord) {
				return model.CreateTripInput{}, domainerror.InvalidRequest(fmt.Sprintf("field \"%s.%s\" is required", key, coord))
			}
		}
	}

	// Неизвестные поля запрещены (additionalProperties: false в контракте)
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	var data api.TripData
	if err := dec.Decode(&data); err != nil {
		return model.CreateTripInput{}, domainerror.InvalidRequest(
			"request body has unknown fields or fields of the wrong type")
	}

	return model.CreateTripInput{
		UserID:   data.UserId,
		DriverID: data.DriverId,
		Start:    trip.Point{Latitude: data.StartPoint.Latitude, Longitude: data.StartPoint.Longitude},
		End:      trip.Point{Latitude: data.EndPoint.Latitude, Longitude: data.EndPoint.Longitude},
		Price:    data.Price,
	}, nil
}

func present(m map[string]json.RawMessage, key string) bool {
	value, ok := m[key]

	return ok && string(value) != "null"
}

func toResponse(t trip.Trip) api.Trip {
	return api.Trip{
		Id:         t.ID,
		UserId:     t.UserID,
		DriverId:   t.DriverID,
		StartPoint: api.Coordinates{Latitude: t.Start.Latitude, Longitude: t.Start.Longitude},
		EndPoint:   api.Coordinates{Latitude: t.End.Latitude, Longitude: t.End.Longitude},
		Price:      t.Price,
		Status:     api.TripStatus(t.Status),
		StartedAt:  t.StartedAt,
		FinishedAt: t.FinishedAt,
	}
}
