package workiq

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCommandFor(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantName string
		wantArgs []string
	}{
		{
			name:     "default npx",
			env:      map[string]string{},
			wantName: "npx",
			wantArgs: []string{"-y", "@microsoft/workiq@latest", "mcp"},
		},
		{
			name:     "WORKIQ_BIN override",
			env:      map[string]string{"WORKIQ_BIN": "/opt/workiq"},
			wantName: "/opt/workiq",
			wantArgs: []string{"mcp"},
		},
		{
			name:     "WORKIQ_MCP_COMMAND override wins",
			env:      map[string]string{"WORKIQ_MCP_COMMAND": "workiq mcp --flag", "WORKIQ_BIN": "/ignored"},
			wantName: "workiq",
			wantArgs: []string{"mcp", "--flag"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := func(k string) string { return tt.env[k] }
			name, args := commandFor(env)
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if strings.Join(args, " ") != strings.Join(tt.wantArgs, " ") {
				t.Errorf("args = %v, want %v", args, tt.wantArgs)
			}
		})
	}
}

// fakeServer scripts JSON-RPC responses over a pipe pair, emulating "workiq mcp".
// It always answers initialize, and dispatches tools/call to the provided handler.
type fakeServer struct {
	toIn   io.Writer // client writes requests here (server reads)
	fromIn io.Reader
	toOut  io.Writer // server writes responses here (client reads)
	handle func(name string, args json.RawMessage) (toolResult, *rpcError)
}

func startFakeServer(t *testing.T, handle func(name string, args json.RawMessage) (toolResult, *rpcError)) (clientW io.Writer, clientR io.Reader) {
	t.Helper()
	// clientToServer: client writes, server reads.
	csr, csw := io.Pipe()
	// serverToClient: server writes, client reads.
	scr, scw := io.Pipe()

	fs := &fakeServer{toOut: scw, fromIn: csr, handle: handle}
	go fs.run(t)

	t.Cleanup(func() {
		_ = csw.Close()
		_ = scw.Close()
	})
	return csw, scr
}

func (fs *fakeServer) run(t *testing.T) {
	dec := json.NewDecoder(fs.fromIn)
	for {
		var req rpcRequest
		if err := dec.Decode(&req); err != nil {
			return
		}
		if req.ID == nil {
			continue // notification, no response
		}
		switch req.Method {
		case "initialize":
			fs.reply(*req.ID, json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{}}`), nil)
		case "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(toBytes(req.Params), &params)
			res, rerr := fs.handle(params.Name, params.Arguments)
			if rerr != nil {
				fs.reply(*req.ID, nil, rerr)
				continue
			}
			b, _ := json.Marshal(res)
			fs.reply(*req.ID, b, nil)
		default:
			fs.reply(*req.ID, json.RawMessage(`{}`), nil)
		}
	}
}

func toBytes(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (fs *fakeServer) reply(id int, result json.RawMessage, rerr *rpcError) {
	resp := rpcResponse{JSONRPC: "2.0", ID: &id, Result: result, Error: rerr}
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	_, _ = fs.toOut.Write(b)
}

func textResult(s string) toolResult {
	return toolResult{Content: []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{{Type: "text", Text: s}}}
}

func TestFetchSuccess(t *testing.T) {
	var gotName string
	var gotArgs json.RawMessage
	handle := func(name string, args json.RawMessage) (toolResult, *rpcError) {
		gotName, gotArgs = name, args
		env := `{"results":[{"data":{"value":[{"subject":"hi"}]},"statusCode":200}],"note":"ok"}`
		return textResult(env), nil
	}
	w, r := startFakeServer(t, handle)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c, err := newWithPipes(ctx, w, r)
	if err != nil {
		t.Fatalf("newWithPipes: %v", err)
	}
	results, err := c.Fetch(ctx, "/me/messages")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotName != "fetch" {
		t.Errorf("tool name = %q, want fetch", gotName)
	}
	if !strings.Contains(string(gotArgs), "/me/messages") {
		t.Errorf("args missing entity url: %s", gotArgs)
	}
	if len(results) != 1 || results[0].StatusCode != 200 {
		t.Fatalf("unexpected results: %+v", results)
	}
	if !strings.Contains(string(results[0].Data), "hi") {
		t.Errorf("data = %s", results[0].Data)
	}
}

func TestFetchNon200IsError(t *testing.T) {
	handle := func(name string, args json.RawMessage) (toolResult, *rpcError) {
		env := `{"results":[{"data":{"error":"forbidden"},"statusCode":403}]}`
		return textResult(env), nil
	}
	w, r := startFakeServer(t, handle)
	ctx := context.Background()
	c, err := newWithPipes(ctx, w, r)
	if err != nil {
		t.Fatalf("newWithPipes: %v", err)
	}
	if _, err := c.Fetch(ctx, "/me/messages"); err == nil {
		t.Fatal("expected error for status 403, got nil")
	}
}

func TestToolJSONRPCError(t *testing.T) {
	handle := func(name string, args json.RawMessage) (toolResult, *rpcError) {
		return toolResult{}, &rpcError{Code: -32000, Message: "boom"}
	}
	w, r := startFakeServer(t, handle)
	ctx := context.Background()
	c, err := newWithPipes(ctx, w, r)
	if err != nil {
		t.Fatalf("newWithPipes: %v", err)
	}
	_, err = c.Fetch(ctx, "/me/messages")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected json-rpc error to surface, got %v", err)
	}
}

func TestToolIsErrorFlag(t *testing.T) {
	handle := func(name string, args json.RawMessage) (toolResult, *rpcError) {
		res := textResult("something went wrong")
		res.IsError = true
		return res, nil
	}
	w, r := startFakeServer(t, handle)
	ctx := context.Background()
	c, err := newWithPipes(ctx, w, r)
	if err != nil {
		t.Fatalf("newWithPipes: %v", err)
	}
	if _, err := c.Fetch(ctx, "/x"); err == nil {
		t.Fatal("expected error when isError=true")
	}
}

func TestDoActionSendsActionURLAndBody(t *testing.T) {
	var gotArgs json.RawMessage
	handle := func(name string, args json.RawMessage) (toolResult, *rpcError) {
		if name != "do_action" {
			t.Errorf("tool name = %q, want do_action", name)
		}
		gotArgs = args
		return textResult(`{"ok":true}`), nil
	}
	w, r := startFakeServer(t, handle)
	ctx := context.Background()
	c, err := newWithPipes(ctx, w, r)
	if err != nil {
		t.Fatalf("newWithPipes: %v", err)
	}
	raw, err := c.DoAction(ctx, "/me/messages/abc/move", map[string]string{"DestinationId": "archive"})
	if err != nil {
		t.Fatalf("DoAction: %v", err)
	}
	if !strings.Contains(string(gotArgs), "/me/messages/abc/move") {
		t.Errorf("actionUrl missing: %s", gotArgs)
	}
	if !strings.Contains(string(gotArgs), "archive") {
		t.Errorf("jsonBody missing DestinationId: %s", gotArgs)
	}
	if !strings.Contains(string(raw), "ok") {
		t.Errorf("unexpected result: %s", raw)
	}
}

func TestFetchRequiresURL(t *testing.T) {
	handle := func(name string, args json.RawMessage) (toolResult, *rpcError) {
		return textResult("{}"), nil
	}
	w, r := startFakeServer(t, handle)
	ctx := context.Background()
	c, err := newWithPipes(ctx, w, r)
	if err != nil {
		t.Fatalf("newWithPipes: %v", err)
	}
	if _, err := c.Fetch(ctx); err == nil {
		t.Fatal("expected error when no entity URLs provided")
	}
}
