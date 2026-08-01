package workiq

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type fakeBrokerGraph struct {
	mu       sync.Mutex
	fetches  int
	actions  int
	active   int
	maxAlive int
}

func (f *fakeBrokerGraph) Fetch(ctx context.Context, urls ...string) ([]FetchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetches++
	return []FetchResult{{Data: json.RawMessage(`{"value":[]}`), StatusCode: 200}}, nil
}

func (f *fakeBrokerGraph) DoAction(ctx context.Context, url string, body any) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions++
	return json.RawMessage(`{"ok":true}`), nil
}

func TestBrokerClientHandlesConcurrentCalls(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	graph := &fakeBrokerGraph{}
	go func() {
		_ = serveBroker(ctx, listener, "test-token", graph)
	}()

	client := &brokerClient{state: brokerState{
		Address: listener.Addr().String(),
		Token:   "test-token",
		PID:     1,
		Version: brokerVersion,
	}}
	const requests = 12
	var wg sync.WaitGroup
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := client.fetch(context.Background(), []string{"/me/messages"})
			if err != nil {
				t.Errorf("fetch: %v", err)
				return
			}
			if len(results) != 1 || results[0].StatusCode != 200 {
				t.Errorf("unexpected results: %+v", results)
			}
		}()
	}
	wg.Wait()
	graph.mu.Lock()
	defer graph.mu.Unlock()
	if graph.fetches != requests {
		t.Errorf("fetches = %d, want %d", graph.fetches, requests)
	}
}

func TestBrokerRejectsInvalidToken(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = serveBroker(ctx, listener, "expected", &fakeBrokerGraph{})
	}()

	client := &brokerClient{state: brokerState{
		Address: listener.Addr().String(),
		Token:   "wrong",
		PID:     1,
		Version: brokerVersion,
	}}
	if _, err := client.fetch(context.Background(), []string{"/me/messages"}); err == nil {
		t.Fatal("expected invalid token error")
	}
}

func TestBrokerClientDoAction(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	graph := &fakeBrokerGraph{}
	go func() {
		_ = serveBroker(ctx, listener, "test-token", graph)
	}()

	client := &brokerClient{state: brokerState{
		Address: listener.Addr().String(),
		Token:   "test-token",
		PID:     1,
		Version: brokerVersion,
	}}
	data, err := client.doAction(context.Background(), "/me/messages/1/move", map[string]string{"destinationId": "archive"})
	if err != nil {
		t.Fatalf("doAction: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("data = %s", data)
	}
	graph.mu.Lock()
	defer graph.mu.Unlock()
	if graph.actions != 1 {
		t.Errorf("actions = %d, want 1", graph.actions)
	}
}

func TestBrokerUnavailableIsRetryable(t *testing.T) {
	client := &brokerClient{state: brokerState{
		Address: "127.0.0.1:1",
		Token:   "test-token",
		PID:     1,
		Version: brokerVersion,
	}}
	var response brokerResponse
	err := client.callOnce(context.Background(), client.currentState(), brokerRequest{Method: "fetch"}, &response)
	if !errors.Is(err, errBrokerUnavailable) {
		t.Fatalf("callOnce error = %v, want unavailable error", err)
	}
}

func TestBrokerCallCancelsAfterConnecting(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var request brokerRequest
		_ = json.NewDecoder(conn).Decode(&request)
		<-time.After(time.Second)
	}()

	client := &brokerClient{state: brokerState{
		Address: listener.Addr().String(),
		Token:   "test-token",
		PID:     1,
		Version: brokerVersion,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		var response brokerResponse
		result <- client.callOnce(ctx, client.currentState(), brokerRequest{Method: "fetch"}, &response)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("callOnce error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("callOnce did not stop after context cancellation")
	}
}

func TestBrokerShutdownClosesSilentClients(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveBroker(ctx, listener, "test-token", &fakeBrokerGraph{})
	}()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveBroker: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("broker did not stop with a silent client connected")
	}
}

func TestDeadlineWithoutContextDeadline(t *testing.T) {
	before := time.Now()
	got := deadline(context.Background())
	if got.Before(before.Add(brokerTimeout-time.Second)) || got.After(before.Add(brokerTimeout+time.Second)) {
		t.Errorf("deadline = %v, want approximately %v after now", got, brokerTimeout)
	}
}
