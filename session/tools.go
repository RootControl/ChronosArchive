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
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}
