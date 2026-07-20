package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ToolCallParams is the params for a tools/call request.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Server is the MCP stdio server.
type Server struct {
	out   io.Writer
	tools map[string]ToolHandler
}

// ToolHandler is a function that handles a tool call.
type ToolHandler func(args json.RawMessage) (json.RawMessage, error)

// NewServer creates a new MCP server writing to out.
func NewServer(out io.Writer) *Server {
	return &Server{
		out:   out,
		tools: defaultTools(),
	}
}

// Run starts the server, reading from stdin and writing to stdout.
func (s *Server) Run() error {
	return s.run(os.Stdin)
}

func (s *Server) run(in io.Reader) error {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Bytes()
		if err := s.handleRaw(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) handleRaw(data []byte) error {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return s.sendError(nil, -32700, "Parse error")
	}
	return s.handleRequest(req)
}

func (s *Server) handleRequest(req JSONRPCRequest) error {
	switch req.Method {
	case "initialize":
		return s.sendResult(req.ID, json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{"tools":{}}}`))
	case "tools/list":
		return s.handleListTools(req.ID)
	case "tools/call":
		return s.handleToolCall(req.ID, req.Params)
	default:
		return s.sendError(req.ID, -32601, "Method not found")
	}
}

func (s *Server) handleListTools(id interface{}) error {
	tools := []map[string]interface{}{}
	for name, handler := range s.tools {
		_ = handler
		tools = append(tools, map[string]interface{}{
			"name":        name,
			"description": fmt.Sprintf("Tool: %s", name),
		})
	}

	result, _ := json.Marshal(map[string]interface{}{"tools": tools})
	return s.sendResult(id, result)
}

func (s *Server) handleToolCall(id interface{}, params json.RawMessage) error {
	var p ToolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return s.sendError(id, -32602, "Invalid params")
	}

	handler, ok := s.tools[p.Name]
	if !ok {
		return s.sendError(id, -32601, fmt.Sprintf("Tool not found: %s", p.Name))
	}

	result, err := handler(p.Arguments)
	if err != nil {
		return s.sendError(id, -32603, err.Error())
	}

	return s.sendResult(id, result)
}

func (s *Server) sendResult(id interface{}, result json.RawMessage) error {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	return s.writeResponse(resp)
}

func (s *Server) sendError(id interface{}, code int, message string) error {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	return s.writeResponse(resp)
}

func (s *Server) writeResponse(resp JSONRPCResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(s.out, string(data))
	return err
}
