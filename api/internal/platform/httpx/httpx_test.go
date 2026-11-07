package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	cases := []struct {
		name, body string
		ok         bool
		status     int
	}{
		{"valid", `{"name":"x"}`, true, 0},
		{"unknown field", `{"name":"x","admin":true}`, false, 400},
		{"trailing data", `{"name":"x"}{}`, false, 400},
		{"too big", `{"name":"` + strings.Repeat("a", 100) + `"}`, false, 413},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/", strings.NewReader(c.body))
			r.Body = http.MaxBytesReader(w, r.Body, 50)
			var v in
			if got := DecodeJSON(w, r, &v); got != c.ok {
				t.Fatalf("ok = %v, want %v", got, c.ok)
			}
			if !c.ok {
				if w.Code != c.status {
					t.Errorf("status = %d, want %d", w.Code, c.status)
				}
				var e ErrorBody
				if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || e.Error.Code == "" {
					t.Errorf("error body not in standard format: %s", w.Body)
				}
			}
		})
	}
}
