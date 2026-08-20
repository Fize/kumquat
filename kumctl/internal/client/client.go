package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL, Token string
	HTTP           *http.Client
}
type Envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}
type Operation struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

func New(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}
func (c *Client) Do(ctx context.Context, method, path string, body []byte, idempotencyKey string) (*Envelope, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api/v1"+path, reader)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out Envelope
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("invalid API response (%d): %s", resp.StatusCode, string(raw))
	}
	if resp.StatusCode >= 400 || out.Code != 0 {
		return nil, fmt.Errorf("API error %d: %s", out.Code, out.Message)
	}
	return &out, nil
}
func (c *Client) Wait(ctx context.Context, id string, interval time.Duration) (*Operation, error) {
	for {
		env, err := c.Do(ctx, http.MethodGet, "/operations/"+id, nil, "")
		if err != nil {
			return nil, err
		}
		var op Operation
		if err := json.Unmarshal(env.Data, &op); err != nil {
			return nil, err
		}
		switch op.State {
		case "succeeded":
			return &op, nil
		case "failed":
			return &op, errors.New(op.Error)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}
