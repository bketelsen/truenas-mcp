package truenas

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/truenas/api_client_golang/truenas_api"
)

// Caller is the interface for making TrueNAS JSON-RPC calls.
// *Client satisfies this interface.
type Caller interface {
	Call(method string, params ...interface{}) (json.RawMessage, error)
}

// Client wraps the TrueNAS WebSocket JSON-RPC client.
type Client struct {
	mu     sync.Mutex
	api    connection
	dial   func() (connection, error)
	closed bool
}

type connection interface {
	Call(string, int64, interface{}) (json.RawMessage, error)
	Ping() (string, error)
	Close() error
}

// Connect establishes a WebSocket connection to TrueNAS and authenticates with an API key.
func Connect(host, apiKey string, tlsInsecure bool) (*Client, error) {
	dial := func() (connection, error) { return connectAPI(host, apiKey, tlsInsecure) }
	api, err := dial()
	if err != nil {
		return nil, err
	}
	return &Client{api: api, dial: dial}, nil
}

func connectAPI(host, apiKey string, tlsInsecure bool) (connection, error) {
	url := fmt.Sprintf("wss://%s/api/current", host)

	// TrueNAS appliances often use self-signed certificates, but production clients
	// should fail closed unless the operator explicitly opts out of verification.
	verifySSL := !tlsInsecure
	api, err := truenas_api.NewClient(url, verifySSL)
	if err != nil {
		return nil, fmt.Errorf("connecting to TrueNAS at %s: %w", host, err)
	}

	if err := api.Login("", "", apiKey); err != nil {
		_ = api.Close()
		return nil, fmt.Errorf("authenticating with TrueNAS: %w", err)
	}

	return api, nil
}

// Close cleanly shuts down the WebSocket connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.api != nil {
		_ = c.api.Close()
		c.api = nil
	}
}

// Call invokes a TrueNAS JSON-RPC method and returns the result as raw JSON.
// It handles the envelope parsing and error extraction.
func (c *Client) Call(method string, params ...interface{}) (json.RawMessage, error) {
	// The upstream client does not serialize WebSocket writes. Also keep
	// connection replacement and shutdown exclusive with outstanding calls.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, fmt.Errorf("TrueNAS client is closed")
	}
	// Probe before sending the requested operation. Reconnect only here:
	// retrying an operation after a lost response could duplicate a write.
	if c.api != nil {
		if _, err := c.api.Ping(); err != nil {
			_ = c.api.Close()
			c.api = nil
		}
	}
	if c.api == nil {
		api, err := c.dial()
		if err != nil {
			return nil, fmt.Errorf("reconnecting to TrueNAS: %w", err)
		}
		c.api = api
	}
	if len(params) == 0 {
		params = []interface{}{}
	}

	raw, err := c.api.Call(method, 30, params)
	if err != nil {
		_ = c.api.Close()
		c.api = nil
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
