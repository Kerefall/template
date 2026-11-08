package postgres

import "testing"

const tripsTable = "trips"

func TestParseSQL(t *testing.T) {
	cases := []struct {
		sql            string
		wantOperation  string
		wantCollection string
	}{
		{`SELECT id, status FROM trips WHERE id = $1`, "SELECT", tripsTable},
		{`SELECT id FROM trips WHERE id = $1 FOR UPDATE`, "SELECT", tripsTable},
		{`INSERT INTO trip_status_history (trip_id) VALUES ($1)`, "INSERT", "trip_status_history"},
		{`UPDATE trips SET status = $1 WHERE id = $2`, "UPDATE", tripsTable},
		{`begin`, "BEGIN", ""},
		{`;`, "other", ""},
	}

	for _, tc := range cases {
		operation, collection := parseSQL(tc.sql)
		if operation != tc.wantOperation || collection != tc.wantCollection {
			t.Errorf("parseSQL(%q) = (%q, %q), want (%q, %q)",
				tc.sql, operation, collection, tc.wantOperation, tc.wantCollection)
		}
	}
}
