package session

import (
	"encoding/json"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/chronosarchive/chronosarchive/tools"
)

// buildToolDefinitions returns the tool list sent to the Anthropic API.
func buildToolDefinitions() []anthropic.ToolUnionParam {
	mkTool := func(name, desc string, props map[string]any, required []string) anthropic.ToolUnionParam {
		return anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        name,
				Description: param.NewOpt(desc),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: props,
					Required:   required,
				},
			},
		}
	}

	strProp := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	boolProp := func(desc string) map[string]any {
		return map[string]any{"type": "boolean", "description": desc}
	}
	intProp := func(desc string) map[string]any {
		return map[string]any{"type": "integer", "description": desc}
	}

	return []anthropic.ToolUnionParam{
		mkTool("read_file", "Read the full contents of a file at the given path.",
			map[string]any{"path": strProp("Path to the file (absolute or relative to project root)")},
			[]string{"path"},
		),
		mkTool("write_file", "Create or overwrite a file with the given content.",
			map[string]any{
				"path":    strProp("File path (absolute or relative to project root)"),
				"content": strProp("Full content to write"),
			},
			[]string{"path", "content"},
		),
		mkTool("edit_file", "Replace an exact unique string in a file (str_replace). The old_string must appear exactly once.",
			map[string]any{
				"path":       strProp("File path"),
				"old_string": strProp("Exact string to find (must be unique in the file)"),
				"new_string": strProp("Replacement string"),
			},
			[]string{"path", "old_string", "new_string"},
		),
		mkTool("list_directory", "List files and directories at a path.",
			map[string]any{
				"path":      strProp("Directory path (defaults to project root if omitted)"),
				"recursive": boolProp("If true, list recursively"),
			},
			nil,
		),
		mkTool("bash", "Run a shell command in the project directory. Use sparingly for build/test commands.",
			map[string]any{
				"command": strProp("Shell command to execute"),
				"timeout": intProp("Timeout in seconds (max 120, default 30)"),
			},
			[]string{"command"},
		),
		mkTool("grep", "Search for a regex pattern in files.",
			map[string]any{
				"pattern":   strProp("Regular expression pattern to search for"),
				"path":      strProp("File or directory to search (defaults to project root)"),
				"recursive": boolProp("Search subdirectories recursively"),
			},
			[]string{"pattern"},
		),
		mkTool("web_fetch", "Fetch the contents of a URL (GET). Returns status code and body.",
			map[string]any{
				"url": strProp("URL to fetch"),
			},
			[]string{"url"},
		),
		mkTool("http_request", "Make an HTTP request (any method) with optional headers and body.",
			map[string]any{
				"method":  strProp("HTTP method (GET, POST, PUT, PATCH, DELETE, etc.)"),
				"url":     strProp("Request URL"),
				"headers": map[string]any{"type": "object", "description": "Optional request headers as key-value pairs"},
				"body":    strProp("Optional request body"),
			},
			[]string{"method", "url"},
		),
		mkTool("create_directory", "Create a directory (and any missing parents) inside the project.",
			map[string]any{
				"path": strProp("Directory path to create (relative to project root)"),
			},
			[]string{"path"},
		),
		mkTool("move_file", "Move or rename a file or directory within the project.",
			map[string]any{
				"source":      strProp("Source path (relative to project root)"),
				"destination": strProp("Destination path (relative to project root)"),
			},
			[]string{"source", "destination"},
		),
		mkTool("delete_file", "Delete a file or directory. Set recursive=true to delete a non-empty directory.",
			map[string]any{
				"path":      strProp("Path to delete (relative to project root)"),
				"recursive": boolProp("If true, delete directory and all contents"),
			},
			[]string{"path"},
		),
		mkTool("web_search", "Search the web using DuckDuckGo and return a summary of results.",
			map[string]any{
				"query":       strProp("Search query"),
				"max_results": intProp("Maximum number of results to return (default 5)"),
			},
			[]string{"query"},
		),
	}
}

// executeTool dispatches a tool call to the appropriate implementation.
func (s *Session) executeTool(toolName string, rawInput json.RawMessage) (string, error) {
	projectPath := s.Config.ProjectPath

	switch toolName {
	case "read_file":
		return tools.ReadFile(projectPath, rawInput)
	case "write_file":
		return tools.WriteFile(projectPath, rawInput)
	case "edit_file":
		return tools.EditFile(projectPath, rawInput)
	case "list_directory":
		return tools.ListDir(projectPath, rawInput)
	case "bash":
		return tools.Bash(projectPath, rawInput)
	case "grep":
		return tools.Grep(projectPath, rawInput)
	case "web_fetch":
		return tools.WebFetch(rawInput)
	case "http_request":
		return tools.HTTPRequest(rawInput)
	case "create_directory":
		return tools.CreateDirectory(projectPath, rawInput)
	case "move_file":
		return tools.MoveFile(projectPath, rawInput)
	case "delete_file":
		return tools.DeleteFile(projectPath, rawInput)
	case "web_search":
		return tools.WebSearch(rawInput)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}
