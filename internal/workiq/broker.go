package workiq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	brokerAddress = "127.0.0.1"
	brokerTimeout = 60 * time.Second
	brokerVersion = "1"
)

var errBrokerUnavailable = errors.New("workiq broker unavailable")

type brokerState struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

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
	defer os.Remove(lockPath)

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
	}); err != nil {
		return err
	}
	defer os.Remove(brokerStatePath())

	c, err := newDirect(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
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
	if err := c.call(ctx, brokerRequest{Method: "fetch", EntityURLs: entityURLs}, &response); err != nil {
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
	if err := c.call(ctx, brokerRequest{Method: "do_action", ActionURL: actionURL, JSONBody: raw}, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (c *brokerClient) call(ctx context.Context, request brokerRequest, response *brokerResponse) error {
	err := c.callOnce(ctx, c.currentState(), request, response)
	if !errors.Is(err, errBrokerUnavailable) {
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
	request.Token = state.Token
	if err := conn.SetDeadline(deadline(ctx)); err != nil {
		return fmt.Errorf("workiq broker: set deadline: %w", err)
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return fmt.Errorf("%w: write request: %v", errBrokerUnavailable, err)
	}
	if err := json.NewDecoder(conn).Decode(response); err != nil {
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
		if state, err := readBrokerState(); err == nil && brokerHealthy(ctx, state) {
			return state, nil
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
	var wg sync.WaitGroup
	defer wg.Wait()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("workiq broker: accept: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer conn.Close()
			handleBrokerConnection(ctx, conn, token, graph)
		}()
	}
}

func handleBrokerConnection(ctx context.Context, conn net.Conn, token string, graph brokerGraph) {
	var request brokerRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		return
	}
	response := brokerResponse{}
	switch {
	case request.Token != token:
		response.Error = "workiq broker: unauthorized request"
	case request.Method == "fetch":
		results, err := graph.Fetch(ctx, request.EntityURLs...)
		response.Results = results
		if err != nil {
			response.Error = err.Error()
		}
	case request.Method == "do_action":
		var body any
		if len(request.JSONBody) > 0 {
			if err := json.Unmarshal(request.JSONBody, &body); err != nil {
				response.Error = fmt.Sprintf("workiq broker: decode action: %v", err)
				break
			}
		}
		data, err := graph.DoAction(ctx, request.ActionURL, body)
		response.Data = data
		if err != nil {
			response.Error = err.Error()
		}
	default:
		response.Error = "workiq broker: unknown method"
	}
	_ = json.NewEncoder(conn).Encode(response)
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
	if state.Address == "" || state.Token == "" || state.PID < 1 || state.Version != brokerVersion {
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
		_ = os.Remove(lockPath)
	}
}

func brokerHealthy(ctx context.Context, state brokerState) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(checkCtx, "tcp", state.Address)
	if err != nil {
		_ = os.Remove(brokerStatePath())
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
