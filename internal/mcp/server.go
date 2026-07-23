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
	out     io.Writer
	tools   map[string]ToolHandler
	version string
}

// ToolHandler is a function that handles a tool call.
type ToolHandler func(args json.RawMessage) (json.RawMessage, error)

// jsonSchemaProperty defines a single property in a JSON Schema.
type jsonSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// toolSchema defines the schema for a tool's input parameters.
type toolSchema struct {
	Description string                        `json:"description"`
	Properties  map[string]jsonSchemaProperty `json:"properties"`
	Required    []string                      `json:"required"`
}

// toolRegistry maps tool names to their schemas.
var toolRegistry = map[string]toolSchema{
	"scan": {
		Description: "Run upgrade_status:analyze on a Drupal project",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
		},
		Required: []string{"project_path"},
	},
	"autofix": {
		Description: "Run drupal-rector on custom modules and themes",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
		},
		Required: []string{"project_path"},
	},
	"contrib_check": {
		Description: "Check Drupal.org for D11 compatibility of a module",
		Properties: map[string]jsonSchemaProperty{
			"module_machine_name": {Type: "string", Description: "Module machine name"},
		},
		Required: []string{"module_machine_name"},
	},
	"issue_patches": {
		Description: "Extract patch/diff/MR links from Drupal.org issues",
		Properties: map[string]jsonSchemaProperty{
			"issue_nid":     {Type: "string", Description: "Issue node ID"},
			"module_name":   {Type: "string", Description: "Module machine name"},
		},
		Required: []string{},
	},
	"apply_patch": {
		Description: "Download and apply a patch to the project",
		Properties: map[string]jsonSchemaProperty{
			"patch_url":    {Type: "string", Description: "URL of the patch file"},
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
		},
		Required: []string{"patch_url", "project_path"},
	},
	"validate": {
		Description: "Re-run scan and return error state",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"scope":        {Type: "string", Description: "Scope filter (optional)"},
			"module":       {Type: "string", Description: "Module name filter (optional)"},
			"file":         {Type: "string", Description: "File path filter (optional)"},
		},
		Required: []string{"project_path"},
	},
	"create_patch": {
		Description: "Generate a patch from rector fixes",
		Properties: map[string]jsonSchemaProperty{
			"module_name":          {Type: "string", Description: "Module machine name"},
			"deprecation_details":  {Type: "string", Description: "Deprecation details"},
		},
		Required: []string{"module_name"},
	},
	"detect_env": {
		Description: "Detect the development environment",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"force_detect": {Type: "boolean", Description: "Force re-detection"},
		},
		Required: []string{"project_path"},
	},
	"composer_require": {
		Description: "Run composer require with environment awareness",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"package":      {Type: "string", Description: "Composer package name"},
			"dev":          {Type: "boolean", Description: "Install as dev dependency"},
			"no_update":    {Type: "boolean", Description: "Skip composer update"},
		},
		Required: []string{"project_path", "package"},
	},
	"drush_exec": {
		Description: "Execute drush commands with environment awareness",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"command":      {Type: "string", Description: "Drush command"},
			"args":         {Type: "array", Description: "Command arguments"},
			"format":       {Type: "string", Description: "Output format (json, table, etc.)"},
		},
		Required: []string{"project_path", "command"},
	},
	"contrib_upgrade_path": {
		Description: "Get upgrade path for a contrib module",
		Properties: map[string]jsonSchemaProperty{
			"module_machine_name":    {Type: "string", Description: "Module machine name"},
			"current_drupal_version": {Type: "string", Description: "Current Drupal version"},
			"target_drupal_version":  {Type: "string", Description: "Target Drupal version"},
		},
		Required: []string{"module_machine_name", "current_drupal_version", "target_drupal_version"},
	},
	"upgrade_scan": {
		Description: "Run upgrade scan with environment setup",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"scope":        {Type: "string", Description: "Scope filter"},
			"module":       {Type: "string", Description: "Module name filter"},
		},
		Required: []string{"project_path"},
	},
	"patch_status": {
		Description: "Check if a patch is applied",
		Properties: map[string]jsonSchemaProperty{
			"project_path":     {Type: "string", Description: "Absolute path to the Drupal project"},
			"patch_url":        {Type: "string", Description: "URL of the patch"},
			"composer_package": {Type: "string", Description: "Composer package name"},
		},
		Required: []string{"project_path", "patch_url", "composer_package"},
	},
	"patch_rollback": {
		Description: "Rollback a patch",
		Properties: map[string]jsonSchemaProperty{
			"project_path":     {Type: "string", Description: "Absolute path to the Drupal project"},
			"patch_url":        {Type: "string", Description: "URL of the patch"},
			"composer_package": {Type: "string", Description: "Composer package name"},
		},
		Required: []string{"project_path", "patch_url", "composer_package"},
	},
	"generate_report": {
		Description: "Generate upgrade report",
		Properties: map[string]jsonSchemaProperty{
			"project_path":      {Type: "string", Description: "Absolute path to the Drupal project"},
			"report_type":       {Type: "string", Description: "Report type (json, markdown, both)"},
			"include_scan_data": {Type: "boolean", Description: "Include scan data in report"},
			"include_patch_list": {Type: "boolean", Description: "Include patch list in report"},
		},
		Required: []string{"project_path"},
	},
	"module_info": {
		Description: "Get module metadata from Drupal.org",
		Properties: map[string]jsonSchemaProperty{
			"module_machine_name":  {Type: "string", Description: "Module machine name"},
			"include_maintainers":  {Type: "boolean", Description: "Include maintainer info"},
			"include_dependencies": {Type: "boolean", Description: "Include dependency info"},
		},
		Required: []string{"module_machine_name"},
	},
	"drupal_version_matrix": {
		Description: "Get Drupal/PHP version compatibility matrix",
		Properties: map[string]jsonSchemaProperty{
			"drupal_version": {Type: "string", Description: "Drupal version"},
			"php_version":    {Type: "string", Description: "PHP version"},
		},
		Required: []string{},
	},
	"core_upgrade_check": {
		Description: "Check if core upgrade is available",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
		},
		Required: []string{"project_path"},
	},
	"core_upgrade_apply": {
		Description: "Apply core upgrade",
		Properties: map[string]jsonSchemaProperty{
			"project_path":   {Type: "string", Description: "Absolute path to the Drupal project"},
			"target_version": {Type: "string", Description: "Target Drupal version"},
			"dry_run":        {Type: "boolean", Description: "Dry run mode"},
		},
		Required: []string{"project_path", "target_version"},
	},
	"patch_reconcile": {
		Description: "Reconcile patches with upstream",
		Properties: map[string]jsonSchemaProperty{
			"module_machine_name": {Type: "string", Description: "Module machine name"},
			"current_patch_url":   {Type: "string", Description: "Current patch URL"},
		},
		Required: []string{"module_machine_name", "current_patch_url"},
	},
}

// NewServer creates a new MCP server writing to out.
func NewServer(out io.Writer, version string) *Server {
	return &Server{
		out:     out,
		tools:   defaultTools(),
		version: version,
	}
}

// RegisterTool overrides or adds a tool handler by name.
func (s *Server) RegisterTool(name string, handler ToolHandler) {
	s.tools[name] = handler
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
		result := fmt.Sprintf(`{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"drup","version":"%s"}}`, s.version)
		return s.sendResult(req.ID, json.RawMessage(result))
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
		
		// Look up schema from registry
		schema, hasSchema := toolRegistry[name]
		
		tool := map[string]interface{}{
			"name": name,
		}
		
		if hasSchema {
			tool["description"] = schema.Description
			
			// Build properties map
			properties := make(map[string]interface{})
			for propName, propDef := range schema.Properties {
				properties[propName] = map[string]interface{}{
					"type":        propDef.Type,
					"description": propDef.Description,
				}
			}
			
			tool["inputSchema"] = map[string]interface{}{
				"type":       "object",
				"properties": properties,
				"required":   schema.Required,
			}
		} else {
			// Fallback for tools not in registry
			tool["description"] = fmt.Sprintf("Tool: %s", name)
			tool["inputSchema"] = map[string]interface{}{
				"type": "object",
			}
		}
		
		tools = append(tools, tool)
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
