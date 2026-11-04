// Package httpjson provides bounded HTTP transport for local inference adapters.
package httpjson

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ai-venture/internal/locale"
)

const MaxResponseBytes = 1 << 20

type Client struct {
	base *url.URL
	http *http.Client
}

func ParseURL(address string) (*url.URL, error) {
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, locale.Errorf("invalid service address")
	}
	return u, nil
}

func New(address string, timeout time.Duration) (*Client, error) {
	u, err := ParseURL(address)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		return nil, locale.Errorf("positive HTTP timeout required")
	}
	return &Client{base: u, http: &http.Client{Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return locale.Errorf("inference service redirect refused")
		},
	}}, nil
}

func (c *Client) Do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		input = bytes.NewReader(data)
	}
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, method, u.String(), input)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		return response.StatusCode, nil, err
	}
	if len(data) > MaxResponseBytes {
		return response.StatusCode, nil, locale.Errorf("service response is too large")
	}
	return response.StatusCode, data, nil
}
