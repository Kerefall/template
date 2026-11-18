package save

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/position"
	"github.com/Kerefall/template/internal/domain/trip"
	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/domainerror"
	"github.com/Kerefall/template/internal/model"
)

// parseInput разбирает тело запроса. Здесь проверяется только форма JSON, а диапазоны
// координат и «время из будущего» проверяет домен.
func parseInput(tripID uuid.UUID, body []byte) (model.SavePositionInput, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return model.SavePositionInput{}, domainerror.InvalidRequest("request body must be a valid JSON object")
	}

	for _, key := range []string{"coordinates", "recorded_at"} {
		if !present(raw, key) {
			return model.SavePositionInput{}, domainerror.InvalidRequest(fmt.Sprintf("field %q is required", key))
		}
	}

	var coordinates map[string]json.RawMessage
	if err := json.Unmarshal(raw["coordinates"], &coordinates); err != nil {
		return model.SavePositionInput{}, domainerror.InvalidRequest(`field "coordinates" must be an object`)
	}
	for _, key := range []string{"latitude", "longitude"} {
		if !present(coordinates, key) {
			return model.SavePositionInput{}, domainerror.InvalidRequest(fmt.Sprintf("field \"coordinates.%s\" is required", key))
		}
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	var data api.PositionData
	if err := dec.Decode(&data); err != nil {
		// Сюда попадают и неизвестные поля, и recorded_at не в формате RFC 3339.
		return model.SavePositionInput{}, domainerror.InvalidRequest(
			"request body has unknown fields or fields of the wrong type (recorded_at must be an RFC 3339 date-time)",
		)
	}

	return model.SavePositionInput{
		TripID:     tripID,
		Point:      trip.Point{Latitude: data.Coordinates.Latitude, Longitude: data.Coordinates.Longitude},
		RecordedAt: data.RecordedAt,
	}, nil
}

func present(m map[string]json.RawMessage, key string) bool {
	value, ok := m[key]

	return ok && string(value) != "null"
}

func toResponse(p position.Position) api.TripPosition {
	return api.TripPosition{
		Id:          p.ID,
		TripId:      p.TripID,
		Coordinates: api.Coordinates{Latitude: p.Point.Latitude, Longitude: p.Point.Longitude},
		RecordedAt:  p.RecordedAt,
	}
}
