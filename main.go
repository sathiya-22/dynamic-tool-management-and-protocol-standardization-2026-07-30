package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/santhosh-tekuri/jsonschema/v5"
	_ "github.com/santhosh-tekuri/jsonschema/v5/httploader" // Required for schema $ref resolution
)

// ToolSchema represents the JSON schema for tool arguments
type ToolSchema map[string]interface{}

// Tool represents a registered tool
type Tool struct {
	Name        string   `json:"name" validate:"required"`
	Description string   `json:"description"`
	Endpoint    string   `json:"endpoint" validate:"required,url"` // The actual tool's API endpoint
	Schema      ToolSchema `json:"schema" validate:"required"`       // JSON schema for the tool's arguments
}

// ToolCall represents a request to execute a tool
type ToolCall struct {
	ToolName  string                 `json:"tool_name" validate:"required"`
	Arguments map[string]interface{} `json:"arguments" validate:"required"`
}

// ToolRegistry manages the lifecycle of tools
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	// Compiler to compile JSON schemas for validation
	schemaCompiler *jsonschema.Compiler
}

func NewToolRegistry() *ToolRegistry {
	compiler := jsonschema.NewCompiler()
	// Optionally, add custom format checkers or load external schemas
	// For now, default compiler is sufficient.
	return &ToolRegistry{
		tools:          make(map[string]Tool),
		schemaCompiler: compiler,
	}
}

// RegisterTool adds a new tool to the registry
func (tr *ToolRegistry) RegisterTool(tool Tool) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if _, exists := tr.tools[tool.Name]; exists {
		return fmt.Errorf("tool '%s' already registered", tool.Name)
	}

	// Compile the schema upon registration to catch errors early
	schemaData, err := json.Marshal(tool.Schema)
	if err != nil {
		return fmt.Errorf("failed to marshal tool schema for compilation: %w", err)
	}
	// Use a unique URI for each schema, e.g., based on tool name
	if _, err := tr.schemaCompiler.Compile(fmt.Sprintf("tool://%s/schema", tool.Name), bytes.NewReader(schemaData)); err != nil {
		return fmt.Errorf("failed to compile schema for tool '%s': %w", tool.Name, err)
	}

	tr.tools[tool.Name] = tool
	log.Printf("Tool '%s' registered: %s", tool.Name, tool.Endpoint)
	return nil
}

// GetTool retrieves a tool by name
func (tr *ToolRegistry) GetTool(name string) (Tool, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	tool, exists := tr.tools[name]
	return tool, exists
}

// ListTools returns all registered tools
func (tr *ToolRegistry) ListTools() []Tool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	var toolList []Tool
	for _, tool := range tr.tools {
		toolList = append(toolList, tool)
	}
	return toolList
}

// RemoveTool removes a tool from the registry
func (tr *ToolRegistry) RemoveTool(name string) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if _, exists := tr.tools[name]; !exists {
		return fmt.Errorf("tool '%s' not found", name)
	}
	delete(tr.tools, name)
	// Also remove the compiled schema from the compiler's cache if necessary,
	// though the compiler's current design doesn't provide a direct way to unload.
	// For this prototype, we'll rely on garbage collection or restart.
	log.Printf("Tool '%s' unregistered", name)
	return nil
}

// ATRPServer handles HTTP requests for the registry
type ATRPServer struct {
	registry *ToolRegistry
	validate *validator.Validate
}

func NewATRPServer(registry *ToolRegistry) *ATRPServer {
	return &ATRPServer{
		registry: registry,
		validate: validator.New(),
	}
}

func (s *ATRPServer) handleRegisterTool(w http.ResponseWriter, r *http.Request) {
	var tool Tool
	if err := json.NewDecoder(r.Body).Decode(&tool); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.validate.Struct(tool); err != nil {
		http.Error(w, fmt.Sprintf("Invalid tool definition: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if err := s.registry.RegisterTool(tool); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Tool registered successfully", "name": tool.Name})
}

func (s *ATRPServer) handleListTools(w http.ResponseWriter, r *http.Request) {
	tools := s.registry.ListTools()
	json.NewEncoder(w).Encode(tools)
}

func (s *ATRPServer) handleRemoveTool(w http.ResponseWriter, r *http.Request) {
	toolName := r.URL.Path[len("/tools/"):]
	if toolName == "" {
		http.Error(w, "Tool name required", http.StatusBadRequest)
		return
	}

	if err := s.registry.RemoveTool(toolName); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("Tool '%s' removed successfully", toolName)})
}

func (s *ATRPServer) handleExecuteTool(w http.ResponseWriter, r *http.Request) {
	var toolCall ToolCall
	if err := json.NewDecoder(r.Body).Decode(&toolCall); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.validate.Struct(toolCall); err != nil {
		http.Error(w, fmt.Sprintf("Invalid tool call payload: %s", err.Error()), http.StatusBadRequest)
		return
	}

	tool, exists := s.registry.GetTool(toolCall.ToolName)
	if !exists {
		http.Error(w, fmt.Sprintf("Tool '%s' not found", toolCall.ToolName), http.StatusNotFound)
		return
	}

	// Perform JSON schema validation here on toolCall.Arguments against tool.Schema.
	schema, err := s.registry.schemaCompiler.GetSchema(fmt.Sprintf("tool://%s/schema", tool.Name))
	if err != nil {
		log.Printf("Error retrieving compiled schema for tool '%s': %v", tool.Name, err)
		http.Error(w, fmt.Sprintf("Internal server error: failed to get schema for tool '%s'", tool.Name), http.StatusInternalServerError)
		return
	}

	if err = schema.Validate(toolCall.Arguments); err != nil {
		http.Error(w, fmt.Sprintf("Tool arguments validation failed for '%s': %s", tool.Name, err.Error()), http.StatusBadRequest)
		return
	}

	log.Printf("Forwarding tool call for '%s' to %s with args: %v", tool.Name, tool.Endpoint, toolCall.Arguments)

	// Simulate forwarding the request. In a real scenario, you would make an
	// HTTP request to tool.Endpoint with toolCall.Arguments as the body.
	// For this prototype, we just return a success message.
	responsePayload := map[string]interface{}{
		"message":      fmt.Sprintf("Tool call for '%s' forwarded successfully", tool.Name),
		"tool_endpoint": tool.Endpoint,
		"arguments_sent": toolCall.Arguments,
		"status":       "pending_execution_at_tool_endpoint", // Indicate it's been sent
	}
	json.NewEncoder(w).Encode(responsePayload)
}

func main() {
	registry := NewToolRegistry()
	server := NewATRPServer(registry)

	http.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			server.handleRegisterTool(w, r)
		case http.MethodGet:
			server.handleListTools(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/tools/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			server.handleRemoveTool(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			server.handleExecuteTool(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	port := ":8080"
	log.Printf("Agent Tool Registry & Protocol (ATR&P) server starting on port %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
