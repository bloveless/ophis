package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	sq "github.com/kballard/go-shellquote"

	"github.com/mark3labs/mcp-go/mcp"
)

// Constants for MCP parameter names and error messages
const (
	MCPCommandName   = "mcp"
	StartCommandName = "start"
	// PositionalArgsParam is the parameter name for positional arguments
	PositionalArgsParam = "args"
	FlagsParam          = "flags"
)

// Controller represents an MCP tool with its associated logic for execution and output handling.
type Controller struct {
	Tool    mcp.Tool `json:"tool"`
	Handler Handler
}

// Execute runs the tool command with the provided request.
func (c *Controller) Execute(ctx context.Context, request mcp.CallToolRequest) ([]byte, error) {
	// Get the executable path
	executablePath, err := os.Executable()
	if err != nil {
		slog.Error("failed to get executable path", "error", err)
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	// Build command arguments
	cmdArgs, err := c.buildCommandArgs(request)
	if err != nil {
		slog.Error("failed to build command arguments", "error", err)
		return nil, fmt.Errorf("failed to build command arguments: %w", err)
	}

	// Create exec.Cmd
	slog.Debug("executing command",
		"tool", c.Tool.Name,
		"executable", executablePath,
		"args", cmdArgs,
	)

	cmd := exec.CommandContext(ctx, executablePath, cmdArgs...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		// Log command exit error but include it in returned error
		slog.Debug("command failed",
			"tool", c.Tool.Name,
			"error", err,
			"exit_code", cmd.ProcessState.ExitCode(),
		)
	}
	return data, err
}

// buildCommandArgs builds the command line arguments from the tool and request.
func (c *Controller) buildCommandArgs(request mcp.CallToolRequest) ([]string, error) {
	message := request.GetArguments()

	// Start with the command path (e.g., "root_sub_command" -> ["root", "sub", "command"])
	// And remove the root command prefix
	args := strings.Split(c.Tool.Name, "_")[1:]
	slog.Debug("initial command arguments", "args", args)

	// Add flags
	if flagsValue, ok := message[FlagsParam]; ok {
		if flagMap, ok := flagsValue.(map[string]any); ok {
			flagArgs, err := buildFlagArgs(flagMap)
			if err != nil {
				return nil, fmt.Errorf("failed to build flag arguments: %w", err)
			}
			args = append(args, flagArgs...)
		}
	}

	// Add positional arguments
	if argsValue, ok := message[PositionalArgsParam]; ok {
		if argsStr, ok := argsValue.(string); ok && argsStr != "" {
			parsedArgs := parseArgumentString(argsStr)
			args = append(args, parsedArgs...)
		}
	}

	return args, nil
}

// buildFlagArgs converts a flag map to command line flag arguments.
func buildFlagArgs(flagMap map[string]any) ([]string, error) {
	var args []string

	for name, value := range flagMap {
		if name == "" {
			continue
		}

		// Convert value to string
		valueStr := ""
		if value != nil {
			// Special handling for boolean flags
			if boolVal, ok := value.(bool); ok {
				if boolVal {
					// For true boolean flags, just add the flag name
					args = append(args, fmt.Sprintf("--%s", name))
				}
				// For false boolean flags, don't add anything
				continue
			}

			valueStr = fmt.Sprintf("%v", value)
		}

		// Add flag with value (for non-boolean flags)
		if valueStr != "" {
			slog.Debug("adding flag argument", "flag_name", name, "input", value, "value", valueStr)
			args = append(args, fmt.Sprintf("--%s", name), valueStr)
		}
	}

	return args, nil
}

// parseArgumentString provides shell-like argument parsing with proper quote handling.
// It supports single quotes, double quotes, and backslash escaping.
//
// The parsing is done using the github.com/kballard/go-shellquote library which
// follows /bin/sh word-splitting rules. This allows MCP clients to pass complex
// arguments containing spaces, quotes, and special characters.
//
// Examples:
//   - `foo bar baz` -> ["foo", "bar", "baz"]
//   - `foo "bar baz"` -> ["foo", "bar baz"]
//   - `foo 'bar baz'` -> ["foo", "bar baz"]
//   - `foo bar\ baz` -> ["foo", "bar baz"]
//
// If parsing fails due to malformed input (e.g., unterminated quotes), the function
// falls back to simple space-based splitting to ensure robustness.
func parseArgumentString(argsStr string) []string {
	// Trim whitespace and handle empty string
	argsStr = strings.TrimSpace(argsStr)
	if argsStr == "" {
		return nil
	}

	// Use shellquote to properly parse the arguments
	args, err := sq.Split(argsStr)
	if err != nil {
		slog.Error("failed to parse argument string", "input", argsStr, "error", err)
		// If parsing fails, fall back to simple splitting
		// This ensures we don't completely fail on malformed input
		return strings.Fields(argsStr)
	}

	return args
}
