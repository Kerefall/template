package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	api "github.com/Kerefall/template/internal/generated"
)

// parseTripData разбирает и проверяет тело POST /api/v1/trips
// Тексты ошибок безопасны для клиента: они попадают в detail ответа 400
func parseTripData(body []byte) (api.TripData, error) {
	// Сначала смотрим на сырой JSON: из структуры не отличить отсутствующее
	// поле от нулевого значения, а по контракту все поля обязательны
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return api.TripData{}, errors.New("request body must be a valid JSON object")
	}

	for _, key := range []string{"user_id", "driver_id", "start_point", "end_point", "price"} {
		if !present(raw, key) {
			return api.TripData{}, fmt.Errorf("field %q is required", key)
		}
	}

	for _, key := range []string{"start_point", "end_point"} {
		var point map[string]json.RawMessage
		if err := json.Unmarshal(raw[key], &point); err != nil {
			return api.TripData{}, fmt.Errorf("field %q must be an object", key)
		}
		for _, coord := range []string{"latitude", "longitude"} {
			if !present(point, coord) {
				return api.TripData{}, fmt.Errorf("field %q.%s is required", key, coord)
			}
		}
	}

	// Неизвестные поля запрещены (additionalProperties: false в контракте)
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	var data api.TripData
	if err := dec.Decode(&data); err != nil {
		return api.TripData{}, errors.New("request body has unknown fields or fields of the wrong type")
	}

	if err := validateTripData(data); err != nil {
		return api.TripData{}, err
	}

	return data, nil
}

func present(m map[string]json.RawMessage, key string) bool {
	value, ok := m[key]
	return ok && string(value) != "null"
}

func validateTripData(d api.TripData) error {
	if uuid.UUID(d.UserId) == uuid.Nil {
		return errors.New("user_id must not be empty")
	}
	if uuid.UUID(d.DriverId) == uuid.Nil {
		return errors.New("driver_id must not be empty")
	}
	if err := validateCoordinates("start_point", d.StartPoint); err != nil {
		return err
	}
	if err := validateCoordinates("end_point", d.EndPoint); err != nil {
		return err
	}
	if d.Price < 0 {
		return errors.New("price must not be negative")
	}

	return nil
}

func validateCoordinates(name string, c api.Coordinates) error {
	if c.Latitude < -90 || c.Latitude > 90 {
		return fmt.Errorf("%s.latitude must be between -90 and 90", name)
	}
	if c.Longitude < -180 || c.Longitude > 180 {
		return fmt.Errorf("%s.longitude must be between -180 and 180", name)
	}

	return nil
}
