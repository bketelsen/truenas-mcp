package truenas

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

type fakeConnection struct {
	pingErr error
	callErr error
	calls   []string
	closed  bool
}

func (f *fakeConnection) Ping() (string, error) { return "pong", f.pingErr }
func (f *fakeConnection) Close() error          { f.closed = true; return nil }
func (f *fakeConnection) Call(method string, _ int64, _ interface{}) (json.RawMessage, error) {
	f.calls = append(f.calls, method)
	return json.RawMessage(`{"result":true}`), f.callErr
}

func TestCallReconnectsBeforeOperation(t *testing.T) {
	stale := &fakeConnection{pingErr: errors.New("websocket: close sent")}
	fresh := &fakeConnection{}
	dials := 0
	c := &Client{api: stale, dial: func() (connection, error) { dials++; return fresh, nil }}
	result, err := c.Call("system.info")
	if err != nil || string(result) != "true" {
		t.Fatalf("result=%s err=%v", result, err)
	}
	if !stale.closed || len(stale.calls) != 0 || len(fresh.calls) != 1 || dials != 1 {
		t.Fatalf("unexpected reconnection: stale=%+v fresh=%+v dials=%d", stale, fresh, dials)
	}
}

func TestCallDoesNotReplayFailedWrite(t *testing.T) {
	failed := &fakeConnection{callErr: errors.New("response lost")}
	fresh := &fakeConnection{}
	dials := 0
	c := &Client{api: failed, dial: func() (connection, error) { dials++; return fresh, nil }}
	if _, err := c.Call("pool.dataset.create", "example"); err == nil {
		t.Fatal("expected transport error")
	}
	if len(failed.calls) != 1 || dials != 0 || !failed.closed {
		t.Fatal("failed operation retried or connection retained")
	}
	if _, err := c.Call("system.info"); err != nil {
		t.Fatal(err)
	}
	if dials != 1 || len(fresh.calls) != 1 || fresh.calls[0] != "system.info" {
		t.Fatal("failed write was replayed")
	}
}

func TestCallReconnectFailureDoesNotSendOperation(t *testing.T) {
	stale := &fakeConnection{pingErr: errors.New("disconnected")}
	c := &Client{api: stale, dial: func() (connection, error) { return nil, errors.New("authentication failed") }}
	if _, err := c.Call("pool.dataset.create"); err == nil {
		t.Fatal("expected reconnection error")
	}
	if len(stale.calls) != 0 {
		t.Fatal("sent operation on failed connection")
	}
}

func TestClosedClientDoesNotReconnect(t *testing.T) {
	c := &Client{api: &fakeConnection{}, dial: func() (connection, error) { t.Fatal("reconnected closed client"); return nil, nil }}
	c.Close()
	c.Close()
	if _, err := c.Call("system.info"); err == nil {
		t.Fatal("expected closed error")
	}
}

func TestConcurrentCallsSerializeConnectionAccess(t *testing.T) {
	fresh := &fakeConnection{}
	c := &Client{api: &fakeConnection{pingErr: errors.New("closed")}, dial: func() (connection, error) { return fresh, nil }}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := c.Call("system.info"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(fresh.calls) != 20 {
		t.Fatalf("calls=%d", len(fresh.calls))
	}
}
