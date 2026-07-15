package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"process-output-mcp/outputstore"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	version       = "dev"
	store         *outputstore.OutputStore
	processStatus *ProcessStatus
)

// ProcessStatus tracks the state of the wrapped subprocess.
type ProcessStatus struct {
	mu       sync.RWMutex
	Running  bool   `json:"running"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error,omitempty"`
}

func (ps *ProcessStatus) SetRunning() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.Running = true
}

func (ps *ProcessStatus) SetExited(code int, err error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.Running = false
	ps.ExitCode = code
	if err != nil {
		ps.Error = err.Error()
	}
}

func (ps *ProcessStatus) Snapshot() ProcessStatus {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ProcessStatus{
		Running:  ps.Running,
		ExitCode: ps.ExitCode,
		Error:    ps.Error,
	}
}

func newMCPServer() *server.MCPServer {
	hooks := &server.Hooks{}

	hooks.AddBeforeAny(func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
		fmt.Printf("[hook] beforeAny: method=%s id=%v\n", method, id)
	})
	hooks.AddOnSuccess(func(ctx context.Context, id any, method mcp.MCPMethod, message any, result any) {
		fmt.Printf("[hook] onSuccess: method=%s id=%v\n", method, id)
	})
	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		fmt.Printf("[hook] onError: method=%s id=%v err=%v\n", method, id, err)
	})
	hooks.AddBeforeInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest) {
		fmt.Printf("[hook] beforeInitialize: id=%v\n", id)
	})
	hooks.AddAfterInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
		fmt.Printf("[hook] afterInitialize: id=%v\n", id)
	})
	hooks.AddBeforeCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest) {
		args, _ := json.Marshal(message.GetArguments())
		fmt.Printf("[hook] beforeCallTool: id=%v tool=%s args=%s\n", id, message.Params.Name, args)
	})
	hooks.AddAfterCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest, result any) {
		fmt.Printf("[hook] afterCallTool: id=%v tool=%s\n", id, message.Params.Name)
	})

	mcpServer := server.NewMCPServer(
		"process-output-mcp-server",
		"1.0.0",
		server.WithLogging(),
		server.WithHooks(hooks),
	)

	// Tool: get_output_lines_between
	mcpServer.AddTool(mcp.NewTool("get_output_lines_between",
		mcp.WithDescription("Get process output lines between two timestamps"),
		mcp.WithString("start_time",
			mcp.Required(),
			mcp.Description("Start timestamp in RFC3339 format (e.g. 2026-07-14T10:00:00Z)"),
		),
		mcp.WithString("end_time",
			mcp.Required(),
			mcp.Description("End timestamp in RFC3339 format (e.g. 2026-07-14T10:05:00Z)"),
		),
	), handleGetLinesBetween)

	// Tool: get_output_lines_from
	mcpServer.AddTool(mcp.NewTool("get_output_lines_from",
		mcp.WithDescription("Get all process output lines from a given timestamp until now"),
		mcp.WithString("start_time",
			mcp.Required(),
			mcp.Description("Start timestamp in RFC3339 format (e.g. 2026-07-14T10:00:00Z)"),
		),
	), handleGetLinesFrom)

	// Tool: get_latest_output
	mcpServer.AddTool(mcp.NewTool("get_latest_output",
		mcp.WithDescription("Get the most recent lines of process output"),
		mcp.WithNumber("lines",
			mcp.Description("Number of lines to retrieve (default: 50)"),
		),
	), handleGetLatestOutput)

	// Tool: get_process_status
	mcpServer.AddTool(mcp.NewTool("get_process_status",
		mcp.WithDescription("Get the current status of the wrapped process (running, exit code)"),
	), handleGetProcessStatus)

	// Tool: get_current_timestamp
	mcpServer.AddTool(mcp.NewTool("get_current_timestamp",
		mcp.WithDescription("Get the current server timestamp in RFC3339 format"),
	), handleGetCurrentTimestamp)

	return mcpServer
}

func handleGetLinesBetween(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	startStr, _ := args["start_time"].(string)
	endStr, _ := args["end_time"].(string)

	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return mcp.NewToolResultError("Invalid start_time format. Use RFC3339 (e.g. 2026-07-14T10:00:00Z)"), nil
	}

	endTime, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return mcp.NewToolResultError("Invalid end_time format. Use RFC3339 (e.g. 2026-07-14T10:00:00Z)"), nil
	}

	lines := store.GetLinesBetween(startTime, endTime)
	jsonData, err := json.Marshal(lines)
	if err != nil {
		return mcp.NewToolResultError("Failed to serialize output lines"), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

func handleGetLinesFrom(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	startStr, _ := args["start_time"].(string)

	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return mcp.NewToolResultError("Invalid start_time format. Use RFC3339 (e.g. 2026-07-14T10:00:00Z)"), nil
	}

	lines := store.GetLinesFrom(startTime)
	jsonData, err := json.Marshal(lines)
	if err != nil {
		return mcp.NewToolResultError("Failed to serialize output lines"), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

func handleGetLatestOutput(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	n := 50
	if val, ok := args["lines"].(float64); ok && val > 0 {
		n = int(val)
	}

	lines := store.GetLatestLines(n)
	jsonData, err := json.Marshal(lines)
	if err != nil {
		return mcp.NewToolResultError("Failed to serialize output lines"), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

func handleGetProcessStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	snapshot := processStatus.Snapshot()
	jsonData, err := json.Marshal(snapshot)
	if err != nil {
		return mcp.NewToolResultError("Failed to serialize process status"), nil
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

func handleGetCurrentTimestamp(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(time.Now().UTC().Format(time.RFC3339)), nil
}

func main() {
	port := flag.Int("p", 8070, "Port number for the MCP server")
	showVersion := flag.Bool("v", false, "Print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [-p port] <command>\n", os.Args[0])
		os.Exit(1)
	}

	store = outputstore.New()
	processStatus = &ProcessStatus{}

	cmd := exec.Command("bash", "-c", args[0])

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating stdout pipe: %v\n", err)
		os.Exit(1)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating stderr pipe: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting command: %v\n", err)
		os.Exit(1)
	}
	processStatus.SetRunning()

	var wg sync.WaitGroup
	wg.Add(2)

	// Capture stdout
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				store.AddLine(string(buf[:n]), false)
			}
			if err != nil {
				break
			}
		}
	}()

	// Capture stderr
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				store.AddLine(string(buf[:n]), true)
			}
			if err != nil {
				break
			}
		}
	}()

	// Start MCP server
	go func() {
		mcpServer := newMCPServer()
		httpServer := server.NewStreamableHTTPServer(mcpServer, server.WithStateLess(true))
		addr := fmt.Sprintf("127.0.0.1:%d", *port)
		log.Printf("Streamable HTTP MCP server listening on %s (endpoint: /mcp)", addr)
		if err := httpServer.Start(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for process output to finish
	wg.Wait()

	// Wait for the command to exit
	exitCode := 0
	var cmdErr error
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
		cmdErr = err
	}
	processStatus.SetExited(exitCode, cmdErr)
	log.Printf("Process exited with code %d", exitCode)

	// Keep the server running so clients can still query output
	select {}
}
