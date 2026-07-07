package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// HTTPError is a non-retryable HTTP failure. Callers can inspect Status to
// treat expected conditions (e.g. 403 on traffic endpoints) as skippable.
type HTTPError struct {
	Status int
	URL    string
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.URL, e.Status, e.Body)
}

// Client talks to the GitHub GraphQL and REST APIs with bounded concurrency,
// retries with backoff on 202/5xx, and rate-limit awareness.
type Client struct {
	base  string
	token string
	http  *http.Client
	sem   chan struct{}
	sleep func(time.Duration) // injectable so tests don't wait
}

func NewClient(token string) *Client {
	return &Client{
		base:  "https://api.github.com",
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
		sem:   make(chan struct{}, 10),
		sleep: time.Sleep,
	}
}

// GraphQL runs a query and unmarshals the "data" payload into out.
// GraphQL-level errors are surfaced as real errors, never swallowed.
func (c *Client) GraphQL(query string, out any) error {
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return err
	}
	payload, err := c.request(http.MethodPost, c.base+"/graphql", body)
	if err != nil {
		return err
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return fmt.Errorf("graphql: decoding response: %w", err)
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("graphql: %s", env.Errors[0].Message)
	}
	if env.Data == nil {
		return fmt.Errorf("graphql: response had no data")
	}
	return json.Unmarshal(env.Data, out)
}

// REST performs a GET against the REST API and unmarshals into out.
// A 204 / empty body leaves out untouched.
func (c *Client) REST(path string, out any) error {
	payload, err := c.request(http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, out)
}

const maxAttempts = 10

func (c *Client) request(method, url string, body []byte) ([]byte, error) {
	backoff := 2 * time.Second
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			c.sleep(backoff)
			if backoff < 15*time.Second {
				backoff *= 2
			}
		}
		data, retry, err := c.attempt(method, url, body)
		if err == nil {
			return data, nil
		}
		if !retry {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("giving up after %d attempts: %w", maxAttempts, lastErr)
}

func (c *Client) attempt(method, url string, body []byte) (data []byte, retry bool, err error) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		return payload, false, nil
	case resp.StatusCode == http.StatusNoContent:
		return nil, false, nil
	case resp.StatusCode == http.StatusAccepted:
		// GitHub is computing stats in the background; retry.
		return nil, true, fmt.Errorf("%s: stats not ready (202)", url)
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		if s := resp.Header.Get("Retry-After"); s != "" {
			if secs, convErr := strconv.Atoi(s); convErr == nil {
				c.sleep(time.Duration(secs) * time.Second)
			}
			return nil, true, fmt.Errorf("%s: rate limited (%d)", url, resp.StatusCode)
		}
		return nil, false, &HTTPError{Status: resp.StatusCode, URL: url, Body: truncate(payload)}
	case resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	default:
		return nil, false, &HTTPError{Status: resp.StatusCode, URL: url, Body: truncate(payload)}
	}
}

func truncate(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}
