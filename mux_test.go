package larking

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewMux(t *testing.T) {
	m, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(
		http.MethodPost, "/unknown", strings.NewReader(`{"hello": "mux"}`),
	)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	m.ServeHTTP(w, r)

	res := w.Result()
	if res.StatusCode != http.StatusNotFound {
		t.Fatal(res.Status)
	}
}
