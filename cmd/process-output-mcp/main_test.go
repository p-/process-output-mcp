package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"process-output-mcp/outputstore"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupTestEnv initializes the package globals and returns an in-process MCP client.
func setupTestEnv(t *testing.T) *client.Client {
	t.Helper()

	store = outputstore.New()
	processStatus = &ProcessStatus{}

	mcpServer := newMCPServer()

	c, err := client.NewInProcessClient(mcpServer)
	if err != nil {
		t.Fatalf("failed to create in-process client: %v", err)
	}

	ctx := context.Background()
	_, err = c.Initialize(ctx, mcp.InitializeRequest{})
	if err != nil {
		t.Fatalf("failed to initialize client: %v", err)
	}

	return c
}

func TestGetLinesBetween_ReturnsMatchingLines(t *testing.T) {
	c := setupTestEnv(t)
	ctx := context.Background()

	start := time.Now().UTC()
	time.Sleep(5 * time.Millisecond)

	store.AddLine("hello world\n", false)
	store.AddLine("error msg\n", true)

	time.Sleep(5 * time.Millisecond)
	end := time.Now().UTC()

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_output_lines_between",
			Arguments: map[string]any{
				"start_time": start.Format(time.RFC3339Nano),
				"end_time":   end.Format(time.RFC3339Nano),
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	var lines []outputstore.OutputLine
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &lines); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0].Content != "hello world\n" || lines[0].IsStdErr {
		t.Errorf("first line mismatch: %+v", lines[0])
	}
	if lines[1].Content != "error msg\n" || !lines[1].IsStdErr {
		t.Errorf("second line mismatch: %+v", lines[1])
	}
}

func TestGetLinesBetween_InvalidTimestamp(t *testing.T) {
	c := setupTestEnv(t)
	ctx := context.Background()

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_output_lines_between",
			Arguments: map[string]any{
				"start_time": "not-a-timestamp",
				"end_time":   "also-bad",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid timestamp")
	}
}

func TestGetLatestOutput_DefaultCount(t *testing.T) {
	c := setupTestEnv(t)
	ctx := context.Background()

	// Add 5 lines
	for i := 0; i < 5; i++ {
		store.AddLine(fmt.Sprintf("line %d\n", i), false)
	}

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_latest_output",
			Arguments: map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	var lines []outputstore.OutputLine
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &lines); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines (fewer than default 50), got %d", len(lines))
	}
}

func TestGetLatestOutput_CustomCount(t *testing.T) {
	c := setupTestEnv(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		store.AddLine(fmt.Sprintf("line %d\n", i), false)
	}

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_latest_output",
			Arguments: map[string]any{
				"lines": float64(3),
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	var lines []outputstore.OutputLine
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &lines); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0].Content != "line 7\n" {
		t.Errorf("expected 'line 7\\n', got %q", lines[0].Content)
	}
}

func TestGetProcessStatus_Running(t *testing.T) {
	c := setupTestEnv(t)
	ctx := context.Background()

	processStatus.SetRunning()

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_process_status",
			Arguments: map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	var status ProcessStatus
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &status); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !status.Running {
		t.Error("expected running=true")
	}
	if status.ExitCode != 0 {
		t.Errorf("expected exitCode=0, got %d", status.ExitCode)
	}
}

func TestGetProcessStatus_Exited(t *testing.T) {
	c := setupTestEnv(t)
	ctx := context.Background()

	processStatus.SetRunning()
	processStatus.SetExited(42, fmt.Errorf("signal: killed"))

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_process_status",
			Arguments: map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	var status ProcessStatus
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &status); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if status.Running {
		t.Error("expected running=false")
	}
	if status.ExitCode != 42 {
		t.Errorf("expected exitCode=42, got %d", status.ExitCode)
	}
	if status.Error != "signal: killed" {
		t.Errorf("expected error='signal: killed', got %q", status.Error)
	}
}

func TestGetCurrentTimestamp_ReturnsValidRFC3339(t *testing.T) {
	c := setupTestEnv(t)
	ctx := context.Background()

	before := time.Now().UTC().Truncate(time.Second)

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_current_timestamp",
			Arguments: map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	after := time.Now().UTC().Truncate(time.Second).Add(time.Second)

	text := result.Content[0].(mcp.TextContent).Text
	ts, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatalf("returned timestamp is not valid RFC3339: %q", text)
	}

	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not between %v and %v", ts, before, after)
	}
}

func TestGetCurrentTimestamp_CompatibleWithGetLinesBetween(t *testing.T) {
	c := setupTestEnv(t)
	ctx := context.Background()

	// Get a timestamp, then wait to ensure the next second starts
	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_current_timestamp",
			Arguments: map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	startTime := result.Content[0].(mcp.TextContent).Text

	time.Sleep(1100 * time.Millisecond)
	store.AddLine("test line\n", false)
	time.Sleep(1100 * time.Millisecond)

	// Get a timestamp after adding lines
	result, err = c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_current_timestamp",
			Arguments: map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	endTime := result.Content[0].(mcp.TextContent).Text

	// Use the timestamps with get_output_lines_between
	result, err = c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_output_lines_between",
			Arguments: map[string]any{
				"start_time": startTime,
				"end_time":   endTime,
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	var lines []outputstore.OutputLine
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &lines); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0].Content != "test line\n" {
		t.Errorf("unexpected content: %q", lines[0].Content)
	}
}

func TestHTTPIntegration(t *testing.T) {
	store = outputstore.New()
	processStatus = &ProcessStatus{}
	processStatus.SetRunning()

	store.AddLine("http test line\n", false)

	mcpServer := newMCPServer()
	httpServer := server.NewStreamableHTTPServer(mcpServer, server.WithStateLess(true))

	ts := httptest.NewServer(httpServer)
	defer ts.Close()

	c, err := client.NewStreamableHttpClient(ts.URL + "/mcp")
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}

	ctx := context.Background()
	_, err = c.Initialize(ctx, mcp.InitializeRequest{})
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	// Test get_latest_output via HTTP
	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_latest_output",
			Arguments: map[string]any{
				"lines": float64(10),
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	var lines []outputstore.OutputLine
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &lines); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0].Content != "http test line\n" {
		t.Errorf("unexpected content: %q", lines[0].Content)
	}

	// Test get_process_status via HTTP
	result, err = c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get_process_status",
			Arguments: map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	var status ProcessStatus
	text = result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &status); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !status.Running {
		t.Error("expected running=true via HTTP")
	}
}
