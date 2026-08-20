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
