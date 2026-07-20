// Command mcp is a minimal example: expose the QR encoder to an AI agent over
// the Model Context Protocol. It speaks newline-framed JSON-RPC 2.0 on
// stdin/stdout and advertises one tool, generate_qr, that turns text into a QR
// code — ASCII by default, or a base64 PNG. It implements just enough of MCP
// (initialize, tools/list, tools/call) to be driven by a conforming client.
//
//	go run ./example/mcp        # then speak JSON-RPC on stdin
//
// Register it with an MCP client (e.g. an AI agent) as a stdio server; the agent
// then calls generate_qr({"text": "...", "level": "H", "png": true}).
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"

	"github.com/netstar-labs/qr"
)

var levels = map[string]qr.Level{
	"": qr.Medium, "L": qr.Low, "M": qr.Medium, "Q": qr.Quartile, "H": qr.High,
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 64*1024), 4<<20) // room for a large text argument
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = out.Encode(errorResp(nil, -32700, "parse error"))
			continue
		}
		resp, isNotification := dispatch(&req)
		if isNotification {
			continue // notifications (no id) get no reply
		}
		_ = out.Encode(resp)
	}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      any     `json:"id"`
	Result  any     `json:"result,omitempty"`
	Error   *errObj `json:"error,omitempty"`
}

type errObj struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func okResp(id, result any) response {
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResp(id any, code int, msg string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &errObj{Code: code, Message: msg}}
}

// dispatch handles one request. A request with no id is a notification and gets
// no response (the second return is true).
func dispatch(req *request) (response, bool) {
	var id any
	notification := len(req.ID) == 0
	if !notification {
		_ = json.Unmarshal(req.ID, &id)
	}
	switch req.Method {
	case "initialize":
		return okResp(id, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "qr", "version": "1"},
		}), notification
	case "notifications/initialized":
		return response{}, true
	case "tools/list":
		return okResp(id, map[string]any{"tools": []any{tool}}), notification
	case "tools/call":
		return callTool(id, req.Params), notification
	default:
		if notification {
			return response{}, true
		}
		return errorResp(id, -32601, "method not found: "+req.Method), false
	}
}

// tool is the descriptor returned by tools/list.
var tool = map[string]any{
	"name":        "generate_qr",
	"description": "Encode text as a QR code. Returns ASCII art by default, or a base64 PNG when png=true.",
	"inputSchema": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text":  map[string]any{"type": "string", "description": "content to encode"},
			"level": map[string]any{"type": "string", "enum": []string{"L", "M", "Q", "H"}, "description": "error-correction level (default M)"},
			"png":   map[string]any{"type": "boolean", "description": "return a base64 PNG instead of ASCII"},
		},
		"required": []string{"text"},
	},
}

func callTool(id any, params json.RawMessage) response {
	var p struct {
		Name      string `json:"name"`
		Arguments struct {
			Text  string `json:"text"`
			Level string `json:"level"`
			PNG   bool   `json:"png"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return errorResp(id, -32602, "invalid params")
	}
	if p.Name != "generate_qr" {
		return errorResp(id, -32602, "unknown tool: "+p.Name)
	}
	level, ok := levels[p.Arguments.Level]
	if !ok {
		return okResp(id, textContent("level must be L, M, Q, or H", true))
	}
	code, err := qr.Encode(p.Arguments.Text, level)
	if err != nil {
		// A tool-level failure is a result with isError, not a protocol error.
		return okResp(id, textContent("qr error: "+err.Error(), true))
	}
	if p.Arguments.PNG {
		var buf bytes.Buffer
		if err := code.PNG(&buf, 8, 4); err != nil {
			return okResp(id, textContent("qr error: "+err.Error(), true))
		}
		return okResp(id, map[string]any{"content": []any{map[string]any{
			"type":     "image",
			"data":     base64.StdEncoding.EncodeToString(buf.Bytes()),
			"mimeType": "image/png",
		}}})
	}
	return okResp(id, textContent(code.String(), false))
}

func textContent(s string, isErr bool) map[string]any {
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": s}},
		"isError": isErr,
	}
}
