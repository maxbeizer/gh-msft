package workiq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	brokerAddress              = "127.0.0.1"
	brokerTimeout              = 60 * time.Second
	brokerVersion              = "1"
	maxBrokerConnections       = 64
	maxBrokerRequestSize       = 1 << 20
	unauthenticatedReadTimeout = 10 * time.Second
)

var errBrokerUnavailable = errors.New("workiq broker unavailable")

type brokerState struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
	ID      string `json:"id"`
	Error   string `json:"error,omitempty"`
}

type brokerStartupError struct{ message string }

func (e *brokerStartupError) Error() string { return "workiq broker startup: " + e.message }

type brokerRequest struct {
	Token      string          `json:"token"`
	Method     string          `json:"method"`
	EntityURLs []string        `json:"entityUrls,omitempty"`
	ActionURL  string          `json:"actionUrl,omitempty"`
	JSONBody   json.RawMessage `json:"jsonBody,omitempty"`
}

type brokerResponse struct {
	Results []FetchResult   `json:"results,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type brokerGraph interface {
	Fetch(context.Context, ...string) ([]FetchResult, error)
	DoAction(context.Context, string, any) (json.RawMessage, error)
}

type brokerClient struct {
	mu    sync.RWMutex
	state brokerState
}

// RunBroker serves a long-lived WorkIQ process for local gh-msft invocations.
// It is intentionally called only by the binary's private __broker entry point.
func RunBroker(ctx context.Context) error {
	if strings.TrimSpace(os.Getenv("WORKIQ_BROKER_CHILD")) == "" {
		return errors.New("workiq broker must be started by gh-msft")
	}
	dir, err := brokerDir()
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, "broker.lock")
	instanceID := ""
	defer func() {
		if instanceID == "" {
			_ = os.Remove(lockPath)
			return
		}
		removeOwnedFile(lockPath, instanceID)
	}()

	c, err := newDirect(ctx)
	if err != nil {
		_ = writeBrokerState(brokerState{Version: brokerVersion, Error: err.Error()})
		return err
	}
	defer c.Close()
	instanceID, err = newBrokerToken()
	if err != nil {
		return err
	}
	if err := os.WriteFile(lockPath, []byte(instanceID), 0600); err != nil {
		return fmt.Errorf("workiq broker: identify lock: %w", err)
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(brokerAddress, "0"))
	if err != nil {
		return fmt.Errorf("workiq broker: listen: %w", err)
	}
	defer listener.Close()
	token, err := newBrokerToken()
	if err != nil {
		return err
	}
	if err := writeBrokerState(brokerState{
		Address: listener.Addr().String(),
		Token:   token,
		PID:     os.Getpid(),
		Version: brokerVersion,
		ID:      instanceID,
	}); err != nil {
		return err
	}
	defer removeOwnedFile(brokerStatePath(), instanceID)
	return serveBroker(ctx, listener, token, c)
}

func newBrokerClient(ctx context.Context) (*Client, error) {
	state, err := awaitBroker(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{broker: &brokerClient{state: state}}, nil
}

func (c *brokerClient) fetch(ctx context.Context, entityURLs []string) ([]FetchResult, error) {
	var response brokerResponse
	if err := c.call(ctx, brokerRequest{Method: "fetch", EntityURLs: entityURLs}, &response, true); err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (c *brokerClient) doAction(ctx context.Context, actionURL string, body any) (json.RawMessage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("workiq broker: encode action: %w", err)
	}
	var response brokerResponse
	if err := c.call(ctx, brokerRequest{Method: "do_action", ActionURL: actionURL, JSONBody: raw}, &response, false); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (c *brokerClient) call(ctx context.Context, request brokerRequest, response *brokerResponse, retry bool) error {
	err := c.callOnce(ctx, c.currentState(), request, response)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !retry || !errors.Is(err, errBrokerUnavailable) {
		return err
	}
	state, restartErr := awaitBroker(ctx)
	if restartErr != nil {
		return fmt.Errorf("%w: %v", err, restartErr)
	}
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	return c.callOnce(ctx, state, request, response)
}

func (c *brokerClient) currentState() brokerState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *brokerClient) callOnce(ctx context.Context, state brokerState, request brokerRequest, response *brokerResponse) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", state.Address)
	if err != nil {
		return fmt.Errorf("%w: %v", errBrokerUnavailable, err)
	}
	defer conn.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	request.Token = state.Token
	if err := conn.SetDeadline(deadline(ctx)); err != nil {
		return fmt.Errorf("workiq broker: set deadline: %w", err)
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: write request: %v", errBrokerUnavailable, err)
	}
	if err := json.NewDecoder(conn).Decode(response); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: read response: %v", errBrokerUnavailable, err)
	}
	if response.Error != "" {
		if strings.Contains(strings.ToLower(response.Error), "accept the eula") {
			return fmt.Errorf("%w: %s", ErrEULANotAccepted, response.Error)
		}
		return errors.New(response.Error)
	}
	return nil
}

func awaitBroker(ctx context.Context) (brokerState, error) {
	deadline := time.Now().Add(brokerTimeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	for {
		if state, err := readBrokerState(); err == nil {
			if brokerHealthy(ctx, state) {
				return state, nil
			}
		} else {
			var startupErr *brokerStartupError
			if errors.As(err, &startupErr) {
				_ = os.Remove(brokerStatePath())
				return brokerState{}, startupErr
			}
		}
		recoverStaleBrokerLock()
		if err := startBroker(); err != nil && !errors.Is(err, os.ErrExist) {
			return brokerState{}, err
		}
		if time.Now().After(deadline) {
			return brokerState{}, errors.New("workiq broker did not become ready; set WORKIQ_DIRECT_PROCESS=1 to bypass it")
		}
		select {
		case <-ctx.Done():
			return brokerState{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func startBroker() error {
	dir, err := brokerDir()
	if err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "broker.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_ = lock.Close()
	_ = os.Remove(brokerStatePath())
	executable, err := os.Executable()
	if err != nil {
		_ = os.Remove(lock.Name())
		return fmt.Errorf("workiq broker: locate executable: %w", err)
	}
	cmd := exec.Command(executable, "__broker")
	cmd.Env = append(os.Environ(), "WORKIQ_BROKER_CHILD=1")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		_ = os.Remove(lock.Name())
		return fmt.Errorf("workiq broker: start: %w", err)
	}
	return nil
}

func serveBroker(ctx context.Context, listener net.Listener, token string, graph brokerGraph) error {
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var connectionsMu sync.Mutex
	connections := make(map[net.Conn]struct{})
	defer wg.Wait()
	go func() {
		<-serveCtx.Done()
		_ = listener.Close()
		connectionsMu.Lock()
		defer connectionsMu.Unlock()
		for conn := range connections {
			_ = conn.Close()
		}
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("workiq broker: accept: %w", err)
		}
		connectionsMu.Lock()
		if len(connections) >= maxBrokerConnections {
			connectionsMu.Unlock()
			_ = conn.Close()
			continue
		}
		connections[conn] = struct{}{}
		connectionsMu.Unlock()
		if serveCtx.Err() != nil {
			_ = conn.Close()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer conn.Close()
			defer func() {
				connectionsMu.Lock()
				delete(connections, conn)
				connectionsMu.Unlock()
			}()
			if handleBrokerConnection(serveCtx, conn, token, graph) {
				// A timed-out stdio request may still hold the transport mutex.
				// Retire this broker so the next command starts a clean WorkIQ child.
				cancel()
				_ = listener.Close()
			}
		}()
	}
}

func handleBrokerConnection(ctx context.Context, conn net.Conn, token string, graph brokerGraph) bool {
	if err := conn.SetReadDeadline(time.Now().Add(unauthenticatedReadTimeout)); err != nil {
		return false
	}
	var request brokerRequest
	if err := json.NewDecoder(io.LimitReader(conn, maxBrokerRequestSize)).Decode(&request); err != nil {
		return false
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return false
	}
	response := brokerResponse{}
	timedOut := false
	operationCtx, cancel := context.WithTimeout(ctx, brokerTimeout)
	defer cancel()
	peerGone := make(chan struct{})
	go func() {
		_, _ = conn.Read(make([]byte, 1))
		close(peerGone)
	}()
	go func() {
		select {
		case <-peerGone:
			cancel()
		case <-operationCtx.Done():
		}
	}()
	switch {
	case request.Token != token:
		response.Error = "workiq broker: unauthorized request"
	case request.Method == "fetch":
		if operationCtx.Err() != nil {
			response.Error = operationCtx.Err().Error()
			break
		}
		results, err := graph.Fetch(operationCtx, request.EntityURLs...)
		response.Results = results
		if err != nil {
			response.Error = err.Error()
			timedOut = errors.Is(err, context.DeadlineExceeded)
		}
	case request.Method == "do_action":
		var body any
		if len(request.JSONBody) > 0 {
			if err := json.Unmarshal(request.JSONBody, &body); err != nil {
				response.Error = fmt.Sprintf("workiq broker: decode action: %v", err)
				break
			}
		}
		if operationCtx.Err() != nil {
			response.Error = operationCtx.Err().Error()
			break
		}
		data, err := graph.DoAction(operationCtx, request.ActionURL, body)
		response.Data = data
		if err != nil {
			response.Error = err.Error()
			timedOut = errors.Is(err, context.DeadlineExceeded)
		}
	default:
		response.Error = "workiq broker: unknown method"
	}
	_ = json.NewEncoder(conn).Encode(response)
	return timedOut
}

func brokerDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("workiq broker: find cache directory: %w", err)
	}
	dir = filepath.Join(dir, "gh-msft")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("workiq broker: create cache directory: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return "", fmt.Errorf("workiq broker: secure cache directory: %w", err)
	}
	return dir, nil
}

func brokerStatePath() string {
	dir, err := brokerDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "broker.json")
}

func writeBrokerState(state brokerState) error {
	path := brokerStatePath()
	if path == "" {
		return errors.New("workiq broker: state path unavailable")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "broker-*.json")
	if err != nil {
		return fmt.Errorf("workiq broker: create state file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("workiq broker: secure state file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("workiq broker: write state file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("workiq broker: close state file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("workiq broker: publish state file: %w", err)
	}
	return nil
}

func readBrokerState() (brokerState, error) {
	data, err := os.ReadFile(brokerStatePath())
	if err != nil {
		return brokerState{}, err
	}
	var state brokerState
	if err := json.Unmarshal(data, &state); err != nil {
		return brokerState{}, err
	}
	if state.Error != "" {
		return brokerState{}, &brokerStartupError{message: state.Error}
	}
	if state.Address == "" || state.Token == "" || state.PID < 1 || state.Version != brokerVersion || state.ID == "" {
		return brokerState{}, errors.New("invalid broker state")
	}
	return state, nil
}

func recoverStaleBrokerLock() {
	dir, err := brokerDir()
	if err != nil {
		return
	}
	lockPath := filepath.Join(dir, "broker.lock")
	info, err := os.Stat(lockPath)
	if err == nil && time.Since(info.ModTime()) > brokerTimeout {
		if state, stateErr := readBrokerState(); stateErr == nil && brokerHealthy(context.Background(), state) {
			return
		}
		_ = os.Remove(lockPath)
	}
}

func removeOwnedFile(path, instanceID string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var state brokerState
	if json.Unmarshal(data, &state) == nil {
		if state.ID == instanceID {
			_ = os.Remove(path)
		}
		return
	}
	if string(data) == instanceID {
		_ = os.Remove(path)
	}
}

func brokerHealthy(ctx context.Context, state brokerState) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(checkCtx, "tcp", state.Address)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func newBrokerToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("workiq broker: generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func deadline(ctx context.Context) time.Time {
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return time.Now().Add(brokerTimeout)
}
