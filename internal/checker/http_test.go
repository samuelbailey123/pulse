package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPChecker_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &HTTPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL:    srv.URL,
		Method: http.MethodGet,
	})

	require.Equal(t, StatusUp, result.Status)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.NoError(t, result.Error)
	assert.Positive(t, result.Latency)
	assert.False(t, result.Timestamp.IsZero())
}

func TestHTTPChecker_WrongStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &HTTPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL:    srv.URL,
		Method: http.MethodGet,
		Expect: config.Expectation{Status: http.StatusOK},
	})

	assert.Equal(t, StatusDown, result.Status)
	assert.Equal(t, http.StatusInternalServerError, result.StatusCode)
	assert.Error(t, result.Error)
}

func TestHTTPChecker_BodyContains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("healthy system running fine"))
	}))
	defer srv.Close()

	c := &HTTPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL:    srv.URL,
		Method: http.MethodGet,
		Expect: config.Expectation{BodyContains: "healthy"},
	})

	assert.Equal(t, StatusUp, result.Status)
	assert.NoError(t, result.Error)
}

func TestHTTPChecker_BodyNotContains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("degraded: something is wrong"))
	}))
	defer srv.Close()

	c := &HTTPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL:    srv.URL,
		Method: http.MethodGet,
		Expect: config.Expectation{BodyContains: "healthy"},
	})

	assert.Equal(t, StatusDown, result.Status)
	assert.Error(t, result.Error)
}

func TestHTTPChecker_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow server that outlasts the checker timeout.
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &HTTPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL:     srv.URL,
		Method:  http.MethodGet,
		Timeout: config.Duration{Duration: 100 * time.Millisecond},
	})

	assert.Equal(t, StatusDown, result.Status)
	assert.Error(t, result.Error)
}

func TestHTTPChecker_CustomHeaders(t *testing.T) {
	var receivedHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &HTTPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL:    srv.URL,
		Method: http.MethodGet,
		Headers: map[string]string{
			"X-Custom-Header": "pulse-check",
		},
	})

	require.Equal(t, StatusUp, result.Status)
	assert.Equal(t, "pulse-check", receivedHeader)
}

func TestHTTPChecker_DefaultAccepts2xx(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantStatus Status
	}{
		{"200 OK", http.StatusOK, StatusUp},
		{"201 Created", http.StatusCreated, StatusUp},
		{"204 No Content", http.StatusNoContent, StatusUp},
		{"301 Redirect", http.StatusMovedPermanently, StatusDown},
		{"404 Not Found", http.StatusNotFound, StatusDown},
		{"500 Server Error", http.StatusInternalServerError, StatusDown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer srv.Close()

			c := &HTTPChecker{}
			result := c.Check(context.Background(), config.Target{
				URL:    srv.URL,
				Method: http.MethodGet,
				// No Expect.Status set — relies on default 2xx logic.
			})

			assert.Equal(t, tc.wantStatus, result.Status, "status code %d", tc.statusCode)
		})
	}
}

func TestHTTPChecker_InvalidURL(t *testing.T) {
	c := &HTTPChecker{}
	result := c.Check(context.Background(), config.Target{
		// A control character makes url.Parse fail inside http.NewRequest.
		URL:    "http://\x00invalid",
		Method: http.MethodGet,
	})

	assert.Equal(t, StatusDown, result.Status)
	assert.Error(t, result.Error)
}

func TestHTTPChecker_MethodOverride(t *testing.T) {
	var receivedMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &HTTPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL:    srv.URL,
		Method: http.MethodPost,
	})

	require.Equal(t, StatusUp, result.Status)
	assert.Equal(t, http.MethodPost, receivedMethod)
}
