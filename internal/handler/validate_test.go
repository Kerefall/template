package handler

import (
	"strings"
	"testing"
)

const validBody = `{
  "user_id": "5cb72c04-7650-45c9-a79b-bcdba0631e0c",
  "driver_id": "8860b315-ec86-42eb-a17c-7c163d721ff5",
  "start_point": {"latitude": 59.9398, "longitude": 30.3146},
  "end_point": {"latitude": 59.929, "longitude": 30.3626},
  "price": 1450
}`

func TestParseTripData(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"valid", validBody, false},
		{"zero price is allowed", strings.Replace(validBody, `"price": 1450`, `"price": 0`, 1), false},
		{"not json", `not json`, true},
		{"negative price", strings.Replace(validBody, `"price": 1450`, `"price": -1`, 1), true},
		{"fractional price", strings.Replace(validBody, `"price": 1450`, `"price": 14.5`, 1), true},
		{"null price", strings.Replace(validBody, `"price": 1450`, `"price": null`, 1), true},
		{"missing price", strings.Replace(validBody, `,
  "price": 1450`, ``, 1), true},
		{"unknown field", strings.Replace(validBody, `"price": 1450`, `"price": 1450, "id": "x"`, 1), true},
		{"bad user uuid", strings.Replace(validBody, `5cb72c04-7650-45c9-a79b-bcdba0631e0c`, `not-a-uuid`, 1), true},
		{"nil driver uuid", strings.Replace(validBody, `8860b315-ec86-42eb-a17c-7c163d721ff5`, `00000000-0000-0000-0000-000000000000`, 1), true},
		{"latitude out of range", strings.Replace(validBody, `59.9398`, `91`, 1), true},
		{"longitude out of range", strings.Replace(validBody, `30.3626`, `-181`, 1), true},
		{"missing longitude", strings.Replace(validBody, `, "longitude": 30.3626`, ``, 1), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTripData([]byte(tc.body))
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}
		})
	}
}
