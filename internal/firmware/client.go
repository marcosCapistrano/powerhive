package firmware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 5 * time.Second
	apiPrefix             = "/api/v1"
)

// Client wraps the firmware HTTP API for a single miner controller.
type Client struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
}

// Option mutates the client during construction.
type Option func(*Client)

// WithHTTPClient allows configuring a custom http.Client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		c.httpClient = h
	}
}

// WithAPIKey sets a default API key header on the client.
func WithAPIKey(key string) Option {
	return func(c *Client) {
		c.apiKey = strings.TrimSpace(key)
	}
}

// WithBaseURL overrides the derived base URL (handy for tests).
func WithBaseURL(base string) Option {
	return func(c *Client) {
		c.baseURL = base
	}
}

// NewClient builds a firmware client for the supplied miner address.
func NewClient(addr string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("miner address is required")
	}

	client := &Client{}

	for _, opt := range opts {
		opt(client)
	}

	if client.baseURL == "" {
		baseURL, err := deriveBaseURL(addr)
		if err != nil {
			return nil, err
		}
		client.baseURL = baseURL
	}

	if client.httpClient == nil {
		client.httpClient = &http.Client{
			Timeout: defaultRequestTimeout,
		}
	}

	return client, nil
}

// SetAPIKey updates the default API key header.
func (c *Client) SetAPIKey(key string) {
	c.apiKey = strings.TrimSpace(key)
}

// Unlock exchanges an unlock password for a short-lived bearer token.
func (c *Client) Unlock(ctx context.Context, password string) (string, error) {
	var res UnlockResponse
	if err := c.do(ctx, http.MethodPost, "/unlock", requestOptions{
		body: map[string]string{"pw": password},
	}, &res); err != nil {
		return "", err
	}
	if strings.TrimSpace(res.Token) == "" {
		return "", fmt.Errorf("unlock succeeded but token is empty")
	}
	return res.Token, nil
}

// ListAPIKeys fetches the current API keys using a bearer token.
func (c *Client) ListAPIKeys(ctx context.Context, bearer string) ([]APIKey, error) {
	var keys []APIKey
	if err := c.do(ctx, http.MethodGet, "/apikeys", requestOptions{
		bearer: bearer,
	}, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// CreateAPIKey registers a new API key using a bearer token.
func (c *Client) CreateAPIKey(ctx context.Context, bearer, key, description string) error {
	payload := map[string]string{
		"key":         key,
		"description": description,
	}
	return c.do(ctx, http.MethodPost, "/apikeys", requestOptions{
		bearer: bearer,
		body:   payload,
	}, nil)
}

// DeleteAPIKey removes an API key using a bearer token.
func (c *Client) DeleteAPIKey(ctx context.Context, bearer, key string) error {
	payload := map[string]string{"key": key}
	return c.do(ctx, http.MethodPost, "/apikeys/delete", requestOptions{
		bearer: bearer,
		body:   payload,
	}, nil)
}

// Info returns general miner metadata. No auth required.
func (c *Client) Info(ctx context.Context) (InfoResponse, error) {
	var info InfoResponse
	err := c.do(ctx, http.MethodGet, "/info", requestOptions{}, &info)
	return info, err
}

// Model fetches the current model metadata. No auth required.
func (c *Client) Model(ctx context.Context) (ModelResponse, error) {
	var model ModelResponse
	err := c.do(ctx, http.MethodGet, "/model", requestOptions{}, &model)
	return model, err
}

// Status retrieves the lightweight miner state.
func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	var status StatusResponse
	err := c.do(ctx, http.MethodGet, "/status", requestOptions{}, &status)
	return status, err
}

// Summary pulls the richer miner summary (requires API key or bearer).
func (c *Client) Summary(ctx context.Context) (SummaryResponse, error) {
	var summary SummaryResponse
	err := c.do(ctx, http.MethodGet, "/summary", requestOptions{}, &summary)
	return summary, err
}

// PerfSummary retrieves the current preset and related autotune data.
func (c *Client) PerfSummary(ctx context.Context) (PerfSummaryResponse, error) {
	var perf PerfSummaryResponse
	err := c.do(ctx, http.MethodGet, "/perf-summary", requestOptions{}, &perf)
	return perf, err
}

// Chains returns per-chip telemetry for each hashboard.
func (c *Client) Chains(ctx context.Context) ([]ChainTelemetry, error) {
	var chains []ChainTelemetry
	err := c.do(ctx, http.MethodGet, "/chains", requestOptions{}, &chains)
	return chains, err
}

// AutotunePresets fetches available performance presets.
func (c *Client) AutotunePresets(ctx context.Context, bearer string) ([]AutotunePreset, error) {
	var presets []AutotunePreset
	err := c.do(ctx, http.MethodGet, "/autotune/presets", requestOptions{
		bearer: bearer,
	}, &presets)
	return presets, err
}

// SetPreset changes the current performance preset using an API key.
// Uses POST /settings with minimal payload to change only the preset.
// Returns SaveConfigResult indicating if a restart/reboot is required.
func (c *Client) SetPreset(ctx context.Context, apiKey, preset string) (*SaveConfigResult, error) {
	payload := SetPresetRequest{
		Miner: MinerConfig{
			Overclock: OverclockSettings{
				Preset: preset,
			},
		},
	}

	var result SaveConfigResult
	if err := c.do(ctx, http.MethodPost, "/settings", requestOptions{
		apiKey: apiKey,
		body:   payload,
	}, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) RestartMining(ctx context.Context, apiKey string) error {
	if err := c.do(ctx, http.MethodPost, "/restart", requestOptions{
		apiKey: apiKey,
		body:   nil,
	}, nil); err != nil {
		return err
	}

	return nil
}

type requestOptions struct {
	body   any
	bearer string
	apiKey string
}

func (c *Client) do(ctx context.Context, method, endpoint string, opts requestOptions, out any) error {
	if c == nil {
		return fmt.Errorf("nil client")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	var body io.Reader
	if opts.body != nil {
		data, err := json.Marshal(opts.body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return fmt.Errorf("create request %s %s: %w", method, endpoint, err)
	}

	if opts.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if bearer := strings.TrimSpace(opts.bearer); bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	apiKey := strings.TrimSpace(opts.apiKey)
	if apiKey == "" {
		apiKey = c.apiKey
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("firmware %s %s: %d %s", method, endpoint, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode %s response: %w", endpoint, err)
	}

	return nil
}

func deriveBaseURL(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", fmt.Errorf("address is empty")
	}

	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}

	u, err := url.Parse(addr)
	if err != nil {
		return "", fmt.Errorf("parse miner address %q: %w", addr, err)
	}

	u.Path = path.Join(u.Path, apiPrefix)
	u.RawQuery = ""
	u.Fragment = ""

	return strings.TrimRight(u.String(), "/"), nil
}
