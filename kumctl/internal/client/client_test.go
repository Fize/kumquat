package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMutationHeadersAndWait(t *testing.T) {
	calls := 0
	c := New("http://api.example", "token")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		payload := `{"code":0,"message":"success","data":{"id":"op_1","state":"succeeded"}}`
		if r.URL.Path == "/api/v1/applications" {
			if got := r.Header.Get("Idempotency-Key"); got != "same-key" {
				t.Errorf("key=%q", got)
			}
			payload = `{"code":0,"message":"accepted","data":{"id":"op_1","state":"pending"}}`
		} else {
			calls++
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: http.Header{}}, nil
	})}
	env, err := c.Do(context.Background(), http.MethodPost, "/applications", []byte(`{}`), "same-key")
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Data) == 0 {
		t.Fatal("missing data")
	}
	if _, err = c.Wait(context.Background(), "op_1", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestDoRejectsHTTPAndEnvelopeErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		payload string
		want    string
	}{
		{name: "http status", status: http.StatusBadRequest, payload: `{"code":0,"message":"bad request"}`, want: "API error HTTP 400: bad request"},
		{name: "api code", status: http.StatusOK, payload: `{"code":403,"message":"forbidden"}`, want: "API error 403: forbidden"},
		{name: "invalid json", status: http.StatusOK, payload: `not-json`, want: "invalid API response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("http://api.example", "")
			c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.payload)), Header: http.Header{}}, nil
			})}
			if _, err := c.Do(context.Background(), http.MethodGet, "/modules", nil, ""); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestWaitReturnsFailureAndHonorsContext(t *testing.T) {
	t.Run("failure with no message", func(t *testing.T) {
		c := New("http://api.example", "")
		c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"id":"op","state":"failed"}}`)), Header: http.Header{}}, nil
		})}
		_, err := c.Wait(context.Background(), "op", time.Millisecond)
		if err == nil || err.Error() != "operation failed" {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("context timeout", func(t *testing.T) {
		c := New("http://api.example", "")
		c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"id":"op","state":"pending"}}`)), Header: http.Header{}}, nil
		})}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := c.Wait(ctx, "op", time.Second)
		if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Fatalf("error = %v", err)
		}
	})
}
