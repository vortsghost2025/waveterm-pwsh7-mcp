package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

type JSONRPCRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      any         `json:"id"`
	Result  any         `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[wave-mcp] ")
	audit := NewAuditLogger()
	defer audit.Close()
	log.Print("wave-mcp server starting (stdio MCP)")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			log.Printf("invalid JSON-RPC: %v", err)
			continue
		}
		resp := handleMessage(req, audit)
		if resp != nil {
			respBytes, _ := json.Marshal(resp)
			fmt.Println(string(respBytes))
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("stdin error: %v", err)
	}
}

func handleMessage(req JSONRPCRequest, audit *AuditLogger) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return handleInitialize(req)
	case "notifications/initialized":
		return nil
	case "ping":
		return &JSONRPCResponse{Jsonrpc: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return handleToolsList(req)
	case "tools/call":
		return handleToolsCall(req, audit)
	default:
		return &JSONRPCResponse{
			Jsonrpc: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

func handleInitialize(req JSONRPCRequest) *JSONRPCResponse {
	return &JSONRPCResponse{
		Jsonrpc: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"protocolVersion": "1.0",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "wave-mcp",
				"version": "0.1.0",
			},
		},
	}
}

func handleToolsList(req JSONRPCRequest) *JSONRPCResponse {
	return &JSONRPCResponse{
		Jsonrpc: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"tools": defineTools(),
		},
	}
}

func handleToolsCall(req JSONRPCRequest, audit *AuditLogger) *JSONRPCResponse {
	var params struct {
		Name   string         `json:"name"`
		Args   map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			Jsonrpc: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: "invalid params"},
		}
	}
	result := handleToolCall(params.Name, params.Args)
	resultText := ""
	if len(result.Content) > 0 {
		resultText = result.Content[0].Text
	}
	audit.Log(params.Name, params.Args, resultText, nil)
	return &JSONRPCResponse{
		Jsonrpc: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}
