package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeDB struct{ err error }

func (db fakeDB) PingContext(context.Context) error { return db.err }

func TestHealthEndpoints(t *testing.T) {
	tests := []struct {
		name string
		path string
		err  error
		want int
	}{
		{"live when unavailable", "/healthz", errors.New("offline"), http.StatusOK},
		{"ready", "/readyz", nil, http.StatusOK},
		{"unavailable", "/readyz", errors.New("offline"), http.StatusServiceUnavailable},
		{"unknown", "/unknown", nil, http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			healthHandler(fakeDB{err: tc.err}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
		})
	}
}
