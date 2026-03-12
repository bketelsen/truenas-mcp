package truenas

import (
	"encoding/json"
	"fmt"

	"github.com/truenas/api_client_golang/truenas_api"
)

// Client wraps the TrueNAS WebSocket JSON-RPC client.
type Client struct {
	api *truenas_api.Client
}

// Connect establishes a WebSocket connection to TrueNAS and authenticates with an API key.
func Connect(host, apiKey string) (*Client, error) {
	url := fmt.Sprintf("wss://%s/api/current", host)

	// verifySSL=false to support self-signed certs common on NAS devices
	api, err := truenas_api.NewClient(url, false)
	if err != nil {
		return nil, fmt.Errorf("connecting to TrueNAS at %s: %w", host, err)
	}

	if err := api.Login("", "", apiKey); err != nil {
		api.Close()
		return nil, fmt.Errorf("authenticating with TrueNAS: %w", err)
	}

	return &Client{api: api}, nil
}

// Close cleanly shuts down the WebSocket connection.
func (c *Client) Close() {
	if c.api != nil {
		c.api.Close()
	}
}

// API returns the underlying TrueNAS API client for making calls.
func (c *Client) API() *truenas_api.Client {
	return c.api
}

// Call invokes a TrueNAS JSON-RPC method and returns the result as raw JSON.
// It handles the envelope parsing and error extraction.
func (c *Client) Call(method string, params ...interface{}) (json.RawMessage, error) {
	if len(params) == 0 {
		params = []interface{}{}
	}

	raw, err := c.api.Call(method, 30, params)
	if err != nil {
		return nil, fmt.Errorf("calling %s: %w", method, err)
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parsing response from %s: %w", method, err)
	}

	if errData, exists := envelope["error"]; exists && string(errData) != "null" {
		return nil, fmt.Errorf("TrueNAS API error from %s: %s", method, string(errData))
	}

	result, ok := envelope["result"]
	if !ok {
		return nil, fmt.Errorf("no result in response from %s", method)
	}

	return result, nil
}
