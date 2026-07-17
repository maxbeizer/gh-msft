// Package workiq provides a small Model Context Protocol (MCP) client that talks
// to the WorkIQ MCP server over stdio JSON-RPC.
//
// gh-msft performs no authentication of its own: the WorkIQ server (backed by the
// operating system's Microsoft SSO broker) owns authentication for the signed-in
// Microsoft 365 account. This client simply spawns "workiq mcp" and exposes the
// deterministic, non-LLM Graph proxy tools (fetch, do_action).
package workiq

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// FetchResult is one entry from the WorkIQ "fetch" tool response envelope. Each
// requested entity URL yields one FetchResult, in request order.
type FetchResult struct {
	Data       json.RawMessage `json:"data"`
	StatusCode int             `json:"statusCode"`
}

// Client is a synchronous MCP client bound to a single WorkIQ server process (or,
// in tests, to an arbitrary pair of pipes).
type Client struct {
	proc *exec.Cmd
	t    *stdioTransport
}

// commandFor returns the command and args used to launch the WorkIQ MCP server.
// It honors overrides for testability and non-default installs:
//   - WORKIQ_MCP_COMMAND: full command line, space-separated (e.g. "workiq mcp").
//   - WORKIQ_BIN: path to a workiq binary; "<bin> mcp" is used.
//
// Otherwise it falls back to "npx -y @microsoft/workiq@latest mcp", matching how
// the Copilot CLI launches WorkIQ.
func commandFor(env func(string) string) (string, []string) {
	if raw := env("WORKIQ_MCP_COMMAND"); strings.TrimSpace(raw) != "" {
		fields := strings.Fields(raw)
		return fields[0], fields[1:]
	}
	if bin := env("WORKIQ_BIN"); strings.TrimSpace(bin) != "" {
		return bin, []string{"mcp"}
	}
	return "npx", []string{"-y", "@microsoft/workiq@latest", "mcp"}
}

// New spawns the WorkIQ MCP server and performs the JSON-RPC initialize handshake.
// Close must be called to terminate the child process.
func New(ctx context.Context) (*Client, error) {
	name, args := commandFor(os.Getenv)
	cmd := exec.CommandContext(ctx, name, args...)
	// Send the child's stderr to the null device directly (not io.Discard): an
	// io.Writer stderr makes os/exec spawn a copy goroutine that Wait() blocks on
	// until every process holding the pipe exits. WorkIQ's grandchildren (node,
	// the native workiq binary) keep it open, which would hang Close forever.
	cmd.Stderr = nil
	setPgid(cmd) // put the child in its own group so Close can kill grandchildren

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("workiq: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("workiq: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("workiq: start %q: %w (is WorkIQ installed and are you signed in?)", name, err)
	}

	c := &Client{proc: cmd, t: newStdioTransport(stdin, stdout)}

	// A cold "npx -y @latest" launch (registry resolution) plus WorkIQ's remote
	// tool registration and SSO broker can occasionally stall. Bound the handshake
	// so we fail fast with a clear message instead of hanging forever.
	hctx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		hctx, cancel = context.WithTimeout(ctx, startupTimeout(os.Getenv))
		defer cancel()
	}
	if err := c.handshake(hctx); err != nil {
		_ = c.Close()
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("workiq: timed out starting %q; is WorkIQ installed and are you signed in? (set WORKIQ_STARTUP_TIMEOUT to adjust): %w", name, err)
		}
		return nil, err
	}
	return c, nil
}

// startupTimeout returns the handshake timeout, overridable via
// WORKIQ_STARTUP_TIMEOUT (Go duration, e.g. "90s"). Defaults to 60s.
func startupTimeout(env func(string) string) time.Duration {
	if raw := strings.TrimSpace(env("WORKIQ_STARTUP_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 60 * time.Second
}

// newWithPipes builds a client over caller-provided pipes and runs the handshake.
// It is used by tests to drive the client against a fake in-process server.
func newWithPipes(ctx context.Context, w io.Writer, r io.Reader) (*Client, error) {
	c := &Client{t: newStdioTransport(w, r)}
	if err := c.handshake(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) handshake(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "gh-msft", "version": "0.1.0"},
	}
	if _, err := c.t.call(ctx, "initialize", params); err != nil {
		return fmt.Errorf("workiq: initialize: %w", err)
	}
	if err := c.t.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("workiq: initialized notification: %w", err)
	}
	return nil
}

// Close terminates the underlying WorkIQ process group, if any. It kills the whole
// group (npx -> node -> native workiq) so no orphaned WorkIQ processes linger, and
// bounds the wait so Close never blocks the caller's exit.
func (c *Client) Close() error {
	if c.proc == nil || c.proc.Process == nil {
		return nil
	}
	terminate(c.proc)
	done := make(chan struct{})
	go func() {
		_ = c.proc.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	return nil
}

// toolResult mirrors the MCP tools/call result shape. WorkIQ returns tool
// payloads in structuredContent (a JSON object), leaving the text content array
// empty; older/other servers put a JSON string in content[].text. callTool
// handles both.
type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	IsError           bool            `json:"isError"`
}

// callTool invokes an MCP tool and returns its JSON payload as bytes. It prefers
// the concatenated text content, falling back to structuredContent (which is how
// WorkIQ actually returns fetch/do_action results).
func (c *Client) callTool(ctx context.Context, name string, args any) ([]byte, error) {
	raw, err := c.t.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, err
	}
	var res toolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("workiq: decode tool result: %w", err)
	}
	var sb strings.Builder
	for _, part := range res.Content {
		if part.Type == "text" {
			sb.WriteString(part.Text)
		}
	}
	payload := []byte(strings.TrimSpace(sb.String()))
	if len(payload) == 0 && len(res.StructuredContent) > 0 {
		payload = res.StructuredContent
	}
	if res.IsError {
		return nil, fmt.Errorf("workiq: tool %q reported error: %s", name, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

// fetchEnvelope is the JSON payload returned by the WorkIQ "fetch" tool.
type fetchEnvelope struct {
	Results []FetchResult `json:"results"`
	Note    string        `json:"note"`
}

// Fetch retrieves one or more WorkIQ/Graph entity paths (relative, e.g.
// "/me/messages"). Results are returned in request order. A non-2xx StatusCode on
// any result is surfaced as an error.
func (c *Client) Fetch(ctx context.Context, entityURLs ...string) ([]FetchResult, error) {
	if len(entityURLs) == 0 {
		return nil, errors.New("workiq: Fetch requires at least one entity URL")
	}
	text, err := c.callTool(ctx, "fetch", map[string]any{"entityUrls": entityURLs})
	if err != nil {
		return nil, err
	}
	var env fetchEnvelope
	if err := json.Unmarshal(text, &env); err != nil {
		return nil, fmt.Errorf("workiq: decode fetch envelope: %w", err)
	}
	for i, r := range env.Results {
		if r.StatusCode != 0 && (r.StatusCode < 200 || r.StatusCode >= 300) {
			return nil, fmt.Errorf("workiq: fetch %q returned status %d: %s", entityURLs[i], r.StatusCode, string(r.Data))
		}
	}
	return env.Results, nil
}

// DoAction invokes a WorkIQ/Graph action (POST-style), e.g. moving a message:
//
//	DoAction(ctx, "/me/messages/{id}/move", map[string]string{"DestinationId": "archive"})
//
// jsonBody may be nil for actions that take no body; it is sent as an empty object
// because the WorkIQ tool requires the field to be present.
func (c *Client) DoAction(ctx context.Context, actionURL string, jsonBody any) (json.RawMessage, error) {
	if strings.TrimSpace(actionURL) == "" {
		return nil, errors.New("workiq: DoAction requires an action URL")
	}
	if jsonBody == nil {
		jsonBody = map[string]any{}
	}
	text, err := c.callTool(ctx, "do_action", map[string]any{
		"actionUrl": actionURL,
		"jsonBody":  jsonBody,
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(text), nil
}

// stdioTransport implements newline-delimited JSON-RPC 2.0 over a writer/reader
// pair. Calls are synchronous and serialized by mu.
type stdioTransport struct {
	mu     sync.Mutex
	w      io.Writer
	r      *bufio.Reader
	nextID int
}

func newStdioTransport(w io.Writer, r io.Reader) *stdioTransport {
	return &stdioTransport{w: w, r: bufio.NewReaderSize(r, 1<<20)}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("json-rpc error %d: %s", e.Code, e.Message) }

// notify sends a JSON-RPC notification (no id, no response expected).
func (t *stdioTransport) notify(method string, params any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.write(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
}

// call sends a request and blocks until the matching response arrives, honoring
// ctx cancellation. Notifications and unrelated ids from the server are skipped.
func (t *stdioTransport) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.nextID++
	id := t.nextID
	if err := t.write(rpcRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params}); err != nil {
		return nil, err
	}

	type readResult struct {
		raw json.RawMessage
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		for {
			line, err := t.r.ReadBytes('\n')
			if err != nil && len(line) == 0 {
				done <- readResult{err: fmt.Errorf("read: %w", err)}
				return
			}
			trimmed := strings.TrimSpace(string(line))
			if trimmed == "" {
				if err != nil {
					done <- readResult{err: fmt.Errorf("read: %w", err)}
					return
				}
				continue
			}
			var resp rpcResponse
			if jerr := json.Unmarshal([]byte(trimmed), &resp); jerr != nil {
				// Ignore non-JSON or log-style lines the server may emit.
				continue
			}
			if resp.ID == nil || *resp.ID != id {
				continue // notification or unrelated response
			}
			if resp.Error != nil {
				done <- readResult{err: resp.Error}
				return
			}
			done <- readResult{raw: resp.Result}
			return
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case rr := <-done:
		return rr.raw, rr.err
	}
}

func (t *stdioTransport) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := t.w.Write(b); err != nil {
		return err
	}
	return nil
}
