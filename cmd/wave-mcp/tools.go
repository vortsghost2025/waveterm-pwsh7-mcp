package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aistore"
)

type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func defineTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "ping",
			Description: "Health check. Returns pong with server info.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_wave_env",
			Description: "Get Wave-relevant environment variables.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "run_readonly_command",
			Description: "Run a safe, read-only shell command. Only allowlisted commands are permitted.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The command to run (must be on the allowlist)",
					},
				},
				"required": []string{"command"},
			},
		},
		{
			Name:        "grep",
			Description: "Search file contents using a regular expression pattern. Returns matching file paths, line numbers, and line content. Skips .git, node_modules, dist, and other build artifacts.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Regular expression pattern to search for",
					},
					"include": map[string]any{
						"type":        "string",
						"description": "Comma-separated glob patterns to filter files (e.g. '*.go', '*.ts,*.tsx')",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute directory path to search in (defaults to current working directory)",
					},
					"max_results": map[string]any{
						"type":        "integer",
						"default":     50,
						"description": "Maximum results to return (default 50, max 500)",
					},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "glob",
			Description: "Find files matching a glob pattern. Returns file paths relative to the search directory. Skips .git, node_modules, dist, and other build artifacts.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Glob pattern to match file names (e.g. '*.go', '**/*.tsx')",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute directory path to search in (defaults to current working directory)",
					},
					"max_results": map[string]any{
						"type":        "integer",
						"default":     200,
						"description": "Maximum results to return (default 200, max 1000)",
					},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "read_text_file",
			Description: "Read a text file and return its contents with line numbers. Binary files are detected and rejected. Respects max_bytes limit to avoid returning huge files.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute path to the file to read",
					},
					"max_bytes": map[string]any{
						"type":        "integer",
						"default":     50000,
						"description": "Maximum bytes to return (default 50000, max 200000)",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "run_interactive_command",
			Description: "Run a command in a Wave terminal widget and return the output. The command is executed in a sub-process and its stdout/stderr are captured. Supports a configurable timeout. Only allowlisted commands are permitted for safety.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The command to run (must be on the allowlist)",
					},
					"timeout_ms": map[string]any{
						"type":        "integer",
						"default":     30000,
						"maximum":     120000,
						"minimum":     1000,
						"description": "Timeout in milliseconds (default 30000, max 120000)",
					},
				},
				"required": []string{"command"},
			},
		},
		{
			Name:        "read_dir",
			Description: "List the contents of a directory. Returns file/directory names with size, type, and modification time. Respects max_entries limit to avoid huge listings.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute path to the directory to list",
					},
					"max_entries": map[string]any{
						"type":        "integer",
						"default":     500,
						"description": "Maximum entries to return (default 500, max 10000)",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "write_text_file",
			Description: "Write a text file to the filesystem. Creates parent directories if needed. Backs up existing files before overwriting. Restricted to paths under the user's home or current working directory.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filename": map[string]any{
						"type":        "string",
						"description": "Absolute path to the file to write",
					},
					"contents": map[string]any{
						"type":        "string",
						"description": "The text content to write to the file",
					},
				},
				"required": []string{"filename", "contents"},
			},
		},
		{
			Name:        "edit_text_file",
			Description: "Edit a text file with atomic search-and-replace operations. Each edit specifies an old_str to find and a new_str to replace it with. All edits are validated before any changes are applied. Backs up the file before editing. Restricted to paths under the user's home or current working directory.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filename": map[string]any{
						"type":        "string",
						"description": "Absolute path to the file to edit",
					},
					"edits": map[string]any{
						"type":        "array",
						"description": "Array of edit operations, each with old_str and new_str",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"old_str": map[string]any{
									"type":        "string",
									"description": "The exact text to find and replace",
								},
								"new_str": map[string]any{
									"type":        "string",
									"description": "The text to replace old_str with",
								},
								"desc": map[string]any{
									"type":        "string",
									"description": "Optional description of the edit",
								},
							},
							"required": []string{"old_str", "new_str"},
						},
					},
				},
				"required": []string{"filename", "edits"},
			},
		},
		{
			Name:        "delete_text_file",
			Description: "Delete a text file from the filesystem. Creates a backup before deletion. Restricted to files under the user's home or current working directory.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filename": map[string]any{
						"type":        "string",
						"description": "Absolute path to the file to delete",
					},
				},
				"required": []string{"filename"},
			},
		},
		{
			Name:        "codebase_search",
			Description: "Search the codebase using a natural language query. Extracts keywords from the query and searches file contents for matches. Useful for finding code by concept rather than exact pattern.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Natural language search query",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute directory path to search in (defaults to current working directory)",
					},
					"max_results": map[string]any{
						"type":        "integer",
						"default":     50,
						"description": "Maximum results to return (default 50, max 500)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "term_get_scrollback",
			Description: "Read scrollback lines from a Wave terminal widget. Shells out to wsh termscrollback and strips ANSI escape sequences. Returns the last N lines of terminal output.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"widget_id": map[string]any{
						"type":        "string",
						"description": "The block/widget ID of the terminal (e.g. 'd84418c1')",
					},
					"count": map[string]any{
						"type":        "integer",
						"default":     200,
						"description": "Number of lines to return (default 200, max 1000)",
					},
				},
				"required": []string{"widget_id"},
			},
		},
		{
			Name:        "term_send_input",
			Description: "Send text input to a Wave terminal widget as if the user typed it. Optionally appends a newline (Enter key). Use this for interactive commands or when term_run_command is busy.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"widget_id": map[string]any{
						"type":        "string",
						"description": "The 8-char block/widget ID of the terminal",
					},
					"text": map[string]any{
						"type":        "string",
						"description": "The text to send to the terminal",
					},
					"enter": map[string]any{
						"type":        "boolean",
						"default":     false,
						"description": "Append a newline to simulate pressing Enter",
					},
				},
				"required":             []string{"widget_id", "text"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "term_list_widgets",
			Description: "List available terminal widgets in Wave. Returns widget IDs, view type, working directory, and shell state for each terminal block.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "note_put",
			Description: "Save a piece of information (note) to the AI agent's persistent memory store. Notes can be tagged, scoped, and given an optional TTL.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"body": map[string]any{
						"type":        "string",
						"description": "The main content of the note to save",
					},
					"scope": map[string]any{
						"type":        "string",
						"description": "Optional scope for organization",
					},
					"key": map[string]any{
						"type":        "string",
						"description": "Optional unique key for this note",
					},
					"tags": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Optional tags for search",
					},
					"ttlsec": map[string]any{
						"type":        "integer",
						"description": "Optional TTL in seconds",
					},
				},
				"required":             []string{"body"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "note_get",
			Description: "Retrieve a saved note by its ID or key.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "The ID of the note",
					},
					"key": map[string]any{
						"type":        "string",
						"description": "The unique key of the note",
					},
					"scope": map[string]any{
						"type":        "string",
						"description": "Optional scope",
					},
				},
				"additionalProperties": false,
			},
		},
		{
			Name:        "note_list",
			Description: "List saved notes with optional scope/tag/limit filters.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"scope":   map[string]any{"type": "string", "description": "Optional scope filter"},
					"tagglob": map[string]any{"type": "string", "description": "Optional tag glob pattern"},
					"limit":   map[string]any{"type": "integer", "default": 50, "description": "Maximum notes (default 50, max 200)"},
				},
				"additionalProperties": false,
			},
		},
		{
			Name:        "note_delete",
			Description: "Delete a saved note by its ID.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "The ID of the note to delete"},
				},
				"required":             []string{"id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "note_search",
			Description: "Search saved notes by text content using substring matching (body, key, tags).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Text to search for"},
					"scope": map[string]any{"type": "string", "description": "Optional scope"},
					"limit": map[string]any{"type": "integer", "default": 20, "description": "Maximum matches (default 20, max 100)"},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "tool_list",
			Description: "List all available tools with their names and descriptions.",
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			Name:        "tool_schema",
			Description: "Get the full definition for a specific tool.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "Tool name to look up"},
				},
				"required":             []string{"name"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "sys_info",
			Description: "Return system info about the Wave server host (hostname, user, OS, arch, CPUs, Go, optional Wave details).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hostname": map[string]any{"type": "boolean", "description": "Include hostname (default: true)"},
					"wave":     map[string]any{"type": "boolean", "description": "Include Wave details (default: true)"},
					"full":     map[string]any{"type": "boolean", "description": "Include all fields"},
				},
				"additionalProperties": false,
			},
		},
		{
			Name:        "sys_env",
			Description: "Return env vars from the Wave server process. Sensitive variables masked.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"names": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Optional list of env var names",
					},
				},
				"additionalProperties": false,
			},
		},
		{
			Name:        "term_search_scrollback",
			Description: "Search a terminal widget's scrollback for a pattern. Returns line numbers + snippets.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"widget_id":  map[string]any{"type": "string", "description": "8-char terminal widget ID"},
					"pattern":    map[string]any{"type": "string", "description": "Pattern (substring; regex if isregex=true)"},
					"isregex":    map[string]any{"type": "boolean", "description": "Treat as regex (default false)"},
					"maxmatches": map[string]any{"type": "integer", "default": 50, "description": "Max matches (default 50, max 200)"},
				},
				"required":             []string{"widget_id", "pattern"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "widget_clear_scrollback",
			Description: "Clear a terminal widget's scrollback buffer. Destructive.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"widget_id": map[string]any{"type": "string", "description": "8-char terminal widget ID"},
				},
				"required":             []string{"widget_id"},
				"additionalProperties": false,
			},
		},
	}
}

func handleToolCall(name string, args map[string]any) ToolCallResult {
	switch name {
	case "ping":
		return callPing()
	case "get_wave_env":
		return callGetWaveEnv()
	case "run_readonly_command":
		return callRunCommand(args)
	case "grep":
		return callGrep(args)
	case "glob":
		return callGlob(args)
	case "read_text_file":
		return callReadTextFile(args)
	case "run_interactive_command":
		return callRunInteractiveCommand(args)
	case "read_dir":
		return callReadDir(args)
	case "write_text_file":
		return callWriteTextFile(args)
	case "edit_text_file":
		return callEditTextFile(args)
	case "delete_text_file":
		return callDeleteTextFile(args)
	case "codebase_search":
		return callCodebaseSearch(args)
	case "term_get_scrollback":
		return callTermGetScrollback(args)
	case "term_send_input":
		return callTermSendInput(args)
	case "term_list_widgets":
		return callTermListWidgets(args)
	case "note_put":
		return callNotePut(args)
	case "note_get":
		return callNoteGet(args)
	case "note_list":
		return callNoteList(args)
	case "note_delete":
		return callNoteDelete(args)
	case "note_search":
		return callNoteSearch(args)
	case "tool_list":
		return ToolCallResult{Content: []ToolContent{{Type: "text", Text: callToolList()}}}
	case "tool_schema":
		return callToolSchema(args)
	case "sys_info":
		return callSysInfo(args)
	case "sys_env":
		return callSysEnv(args)
		return callSysEnv(args)
	case "term_search_scrollback":
		return callTermSearchScrollback(args)
	case "widget_clear_scrollback":
		return callWidgetClearScrollback(args)
	default:
		return ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", name)}},
		}
	}
}

func callPing() ToolCallResult {
	return ToolCallResult{
		Content: []ToolContent{{
			Type: "text",
			Text: fmt.Sprintf("pong — wave-mcp server running on %s/%s", runtime.GOOS, runtime.GOARCH),
		}},
	}
}

func callGetWaveEnv() ToolCallResult {
	var out strings.Builder
	prefixes := []string{"WAVE", "WAVETERM", "WCLOUD"}
	for _, e := range os.Environ() {
		for _, p := range prefixes {
			if strings.HasPrefix(e, p) {
				parts := strings.SplitN(e, "=", 2)
				val := ""
				if len(parts) > 1 {
					val = parts[1]
				}
				if strings.Contains(strings.ToUpper(parts[0]), "TOKEN") || strings.Contains(strings.ToUpper(parts[0]), "SECRET") || strings.Contains(strings.ToUpper(parts[0]), "KEY") {
					if len(val) > 8 {
						val = val[:4] + "..." + val[len(val)-4:]
					}
				}
				fmt.Fprintf(&out, "%s=%s\n", parts[0], val)
			}
		}
	}
	cmdPath, _ := exec.LookPath("wsh")
	fmt.Fprintf(&out, "WSH_PATH=%s\n", cmdPath)
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: out.String()}},
	}
}

func callRunCommand(args map[string]any) ToolCallResult {
	cmdRaw, ok := args["command"]
	if !ok {
		return ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "missing 'command' argument"}}}
	}
	cmdStr, ok := cmdRaw.(string)
	if !ok {
		return ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "'command' must be a string"}}}
	}
	if err := checkCommand(cmdStr); err != nil {
		return ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: err.Error()}}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		shell := "powershell"
		if pwshPath, err := exec.LookPath("pwsh"); err == nil {
			shell = pwshPath
		}
		cmd = exec.CommandContext(ctx, shell, "-NoProfile", "-Command", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := stdout.String()
	if stderr.Len() > 0 {
		if result != "" {
			result += "\n"
		}
		result += "stderr: " + stderr.String()
	}
	if err != nil {
		if result != "" {
			result += "\n"
		}
		result += fmt.Sprintf("error: %v", err)
	}
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: result}},
	}
}

var mcpSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"__pycache__":  true,
	".next":        true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".cache":       true,
	"venv":         true,
	".venv":        true,
	".bin":         true,
	"obj":          true,
	"bin":          true,
	"vendor":       true,
	"third_party":  true,
	"third-party":  true,
}

func mcpResolvePath(rawPath string) (string, error) {
	if rawPath == "" {
		return os.Getwd()
	}
	if rawPath == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rawPath = home
	} else if strings.HasPrefix(rawPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rawPath = filepath.Join(home, rawPath[2:])
	}
	if !filepath.IsAbs(rawPath) {
		return "", fmt.Errorf("path must be absolute: %s", rawPath)
	}
	info, err := os.Stat(rawPath)
	if err != nil {
		return "", fmt.Errorf("cannot access path %s: %v", rawPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", rawPath)
	}
	return rawPath, nil
}

func mcpIsBinary(buf []byte) bool {
	checkLen := len(buf)
	if checkLen > 8192 {
		checkLen = 8192
	}
	for i := 0; i < checkLen; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}

func mcpMatchInclude(name string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	base := filepath.Base(name)
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
	}
	return false
}

func mcpSplitPatterns(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func callGrep(args map[string]any) ToolCallResult {
	patternRaw, ok := args["pattern"]
	if !ok {
		return errResult("missing 'pattern' argument")
	}
	patternStr, ok := patternRaw.(string)
	if !ok {
		return errResult("'pattern' must be a string")
	}
	if patternStr == "" {
		return errResult("pattern cannot be empty")
	}

	pat, err := regexp.Compile(patternStr)
	if err != nil {
		return errResult(fmt.Sprintf("invalid regex pattern: %v", err))
	}

	pathRaw, _ := args["path"].(string)
	searchPath, err := mcpResolvePath(pathRaw)
	if err != nil {
		return errResult(err.Error())
	}

	maxResults := 50
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
		if maxResults < 1 {
			maxResults = 1
		}
		if maxResults > 500 {
			maxResults = 500
		}
	}

	include := ""
	if inc, ok := args["include"].(string); ok {
		include = inc
	}
	includePatterns := mcpSplitPatterns(include)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type match struct {
		file    string
		line    int
		content string
	}
	var matches []match
	count := 0

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.IsDir() {
			if mcpSkipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > 10*1024*1024 || info.Size() == 0 {
			return nil
		}
		if !mcpMatchInclude(path, includePatterns) {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		var chunk [8192]byte
		n, _ := f.Read(chunk[:])
		if mcpIsBinary(chunk[:n]) {
			return nil
		}
		f.Seek(0, 0)

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if ctx.Err() != nil {
				return ctx.Err()
			}
			text := scanner.Text()
			if pat.MatchString(text) {
				rel, _ := filepath.Rel(searchPath, path)
				if rel == "" {
					rel = path
				}
				contentText := text
				if len(contentText) > 500 {
					contentText = contentText[:500] + "..."
				}
				matches = append(matches, match{
					file:    filepath.ToSlash(rel),
					line:    lineNum,
					content: contentText,
				})
				count++
				if count >= maxResults {
					return filepath.SkipAll
				}
			}
		}
		return nil
	}

	if err := filepath.Walk(searchPath, walkFn); err != nil && ctx.Err() == nil {
	}

	if ctx.Err() != nil {
		return errResult("grep timed out after 30s")
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Found %d matches for %q in %s:\n\n", len(matches), patternStr, searchPath)
	for _, m := range matches {
		fmt.Fprintf(&out, "%s:%d: %s\n", m.file, m.line, m.content)
	}
	if count >= maxResults && len(matches) == maxResults {
		fmt.Fprintf(&out, "\n(results truncated at %d matches)\n", maxResults)
	}

	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: out.String()}},
	}
}

func callGlob(args map[string]any) ToolCallResult {
	patternRaw, ok := args["pattern"]
	if !ok {
		return errResult("missing 'pattern' argument")
	}
	patternStr, ok := patternRaw.(string)
	if !ok {
		return errResult("'pattern' must be a string")
	}
	if patternStr == "" {
		return errResult("pattern cannot be empty")
	}

	pathRaw, _ := args["path"].(string)
	searchPath, err := mcpResolvePath(pathRaw)
	if err != nil {
		return errResult(err.Error())
	}

	maxResults := 200
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
		if maxResults < 1 {
			maxResults = 1
		}
		if maxResults > 1000 {
			maxResults = 1000
		}
	}

	var results []string
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.IsDir() {
			if mcpSkipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ok, _ := filepath.Match(patternStr, info.Name())
		if !ok {
			return nil
		}
		rel, _ := filepath.Rel(searchPath, path)
		if rel == "" {
			rel = path
		}
		results = append(results, filepath.ToSlash(rel))
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		return nil
	}

	if err := filepath.Walk(searchPath, walkFn); err != nil && ctx.Err() == nil {
	}

	if ctx.Err() != nil {
		return errResult("glob timed out after 30s")
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Found %d files matching %q in %s:\n\n", len(results), patternStr, searchPath)
	for _, r := range results {
		fmt.Fprintf(&out, "%s\n", r)
	}
	if len(results) >= maxResults {
		fmt.Fprintf(&out, "\n(results truncated at %d files)\n", maxResults)
	}

	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: out.String()}},
	}
}

func callReadTextFile(args map[string]any) ToolCallResult {
	pathRaw, ok := args["path"]
	if !ok {
		return errResult("missing 'path' argument")
	}
	pathStr, ok := pathRaw.(string)
	if !ok {
		return errResult("'path' must be a string")
	}
	if pathStr == "" {
		return errResult("path cannot be empty")
	}

	resolvedPath := pathStr
	if strings.HasPrefix(resolvedPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return errResult(fmt.Sprintf("cannot resolve home dir: %v", err))
		}
		resolvedPath = filepath.Join(home, resolvedPath[2:])
	}
	if !filepath.IsAbs(resolvedPath) {
		return errResult("path must be absolute: " + pathStr)
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return errResult(fmt.Sprintf("cannot access path: %v", err))
	}
	if info.IsDir() {
		return errResult("path is a directory, not a file: " + pathStr)
	}

	maxBytes := 50000
	if mb, ok := args["max_bytes"].(float64); ok {
		maxBytes = int(mb)
		if maxBytes < 1000 {
			maxBytes = 1000
		}
		if maxBytes > 200000 {
			maxBytes = 200000
		}
	}

	f, err := os.Open(resolvedPath)
	if err != nil {
		return errResult(fmt.Sprintf("cannot open file: %v", err))
	}
	defer f.Close()

	var header [8192]byte
	n, _ := f.Read(header[:])
	if mcpIsBinary(header[:n]) {
		return errResult("file appears to be binary, cannot read as text")
	}
	f.Seek(0, 0)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var out strings.Builder
	lineNum := 0
	totalBytes := 0
	truncated := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		lineText := fmt.Sprintf("%d: %s\n", lineNum, line)
		if totalBytes+len(lineText) > maxBytes {
			truncated = true
			break
		}
		out.WriteString(lineText)
		totalBytes += len(lineText)
	}

	fmt.Fprintf(&out, "\n(%d lines, %d bytes", lineNum, info.Size())
	if truncated {
		fmt.Fprintf(&out, ", truncated at %d bytes", maxBytes)
	}
	fmt.Fprintf(&out, ")")

	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: out.String()}},
	}
}

func errResult(msg string) ToolCallResult {
	return ToolCallResult{
		IsError: true,
		Content: []ToolContent{{Type: "text", Text: msg}},
	}
}

func callNotePut(args map[string]any) ToolCallResult {
	bodyRaw, ok := args["body"]
	if !ok {
		return errResult("missing 'body' argument")
	}
	body, ok := bodyRaw.(string)
	if !ok {
		return errResult("'body' must be a string")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return errResult("body cannot be empty")
	}
	scope := ""
	if s, ok := args["scope"].(string); ok {
		scope = s
	}
	key := ""
	if k, ok := args["key"].(string); ok {
		key = k
	}
	workspaceId := ""
	if w, ok := args["workspaceid"].(string); ok {
		workspaceId = w
	}
	var tags []string
	if rawTags, ok := args["tags"].([]any); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok && s != "" {
				tags = append(tags, s)
			}
		}
	}
	ttlSec := 0
	if ttlRaw, ok := args["ttlsec"]; ok {
		if ttlFloat, ok := ttlRaw.(float64); ok && ttlFloat > 0 {
			ttlSec = int(ttlFloat)
		}
	}
	store := aistore.GetMemoryStore()
	opts := aistore.MemoryOpts{
		WorkspaceId: workspaceId,
		Scope:       scope,
		Key:         key,
		Tags:        tags,
		TtlSec:      ttlSec,
	}
	id, err := store.Put(context.Background(), opts, body)
	if err != nil {
		return errResult(fmt.Sprintf("failed to save note: %v", err))
	}
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Note saved with ID: %s", id)}},
	}
}

func callNoteGet(args map[string]any) ToolCallResult {
	workspaceId := ""
	if w, ok := args["workspaceid"].(string); ok {
		workspaceId = w
	}
	scope := ""
	if s, ok := args["scope"].(string); ok {
		scope = s
	}
	id := ""
	if i, ok := args["id"].(string); ok {
		id = i
	}
	key := ""
	if k, ok := args["key"].(string); ok {
		key = k
	}
	if id == "" && key == "" {
		return errResult("either 'id' or 'key' is required")
	}
	store := aistore.GetMemoryStore()
	var rec *aistore.MemoryRecord
	var err error
	if key != "" {
		rec, err = store.GetByKey(context.Background(), workspaceId, scope, key)
	} else {
		rec, err = store.Get(context.Background(), workspaceId, id)
	}
	if err != nil {
		return errResult(err.Error())
	}
	if rec == nil {
		return errResult("note not found")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "ID: %s\n", rec.Id)
	fmt.Fprintf(&out, "Body: %s\n", rec.Body)
	if rec.Key != "" {
		fmt.Fprintf(&out, "Key: %s\n", rec.Key)
	}
	if rec.Scope != "" {
		fmt.Fprintf(&out, "Scope: %s\n", rec.Scope)
	}
	if len(rec.Tags) > 0 {
		fmt.Fprintf(&out, "Tags: %s\n", strings.Join(rec.Tags, ", "))
	}
	fmt.Fprintf(&out, "Created: %s\n", time.UnixMilli(rec.CreatedAt).UTC().Format(time.RFC3339))
	if rec.TtlSec > 0 {
		expiresMs := rec.CreatedAt + int64(rec.TtlSec)*1000
		fmt.Fprintf(&out, "Expires: %s\n", time.UnixMilli(expiresMs).UTC().Format(time.RFC3339))
	}
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: out.String()}},
	}
}

func callNoteList(args map[string]any) ToolCallResult {
	workspaceId := ""
	if w, ok := args["workspaceid"].(string); ok {
		workspaceId = w
	}
	scope := ""
	if s, ok := args["scope"].(string); ok {
		scope = s
	}
	tagGlob := ""
	if tg, ok := args["tagglob"].(string); ok {
		tagGlob = tg
	}
	limit := 50
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
		if limit < 1 {
			limit = 1
		}
		if limit > 200 {
			limit = 200
		}
	}
	store := aistore.GetMemoryStore()
	records, cursor, err := store.List(context.Background(), aistore.MemoryListOpts{
		WorkspaceId: workspaceId,
		Scope:       scope,
		TagGlob:     tagGlob,
		Limit:       limit,
	})
	if err != nil {
		return errResult(err.Error())
	}
	if len(records) == 0 {
		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: "No notes found."}},
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Found %d notes:\n\n", len(records))
	for _, r := range records {
		preview := r.Body
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		fmt.Fprintf(&out, "- ID: %s\n", r.Id)
		if r.Key != "" {
			fmt.Fprintf(&out, "  Key: %s\n", r.Key)
		}
		if r.Scope != "" {
			fmt.Fprintf(&out, "  Scope: %s\n", r.Scope)
		}
		fmt.Fprintf(&out, "  Body: %s\n", preview)
		fmt.Fprintf(&out, "  Updated: %s\n", time.UnixMilli(r.UpdatedAt).UTC().Format(time.RFC3339))
		if len(r.Tags) > 0 {
			fmt.Fprintf(&out, "  Tags: %s\n", strings.Join(r.Tags, ", "))
		}
		out.WriteString("\n")
	}
	if cursor != "" {
		fmt.Fprintf(&out, "(more results available, cursor: %s)\n", cursor)
	}
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: out.String()}},
	}
}

func callNoteDelete(args map[string]any) ToolCallResult {
	idRaw, ok := args["id"]
	if !ok {
		return errResult("missing 'id' argument")
	}
	id, ok := idRaw.(string)
	if !ok {
		return errResult("'id' must be a string")
	}
	if id == "" {
		return errResult("'id' cannot be empty")
	}
	workspaceId := ""
	if w, ok := args["workspaceid"].(string); ok {
		workspaceId = w
	}
	store := aistore.GetMemoryStore()
	deleted, err := store.Delete(context.Background(), workspaceId, id)
	if err != nil {
		return errResult(err.Error())
	}
	if !deleted {
		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("No note found with ID: %s", id)}},
		}
	}
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Deleted note: %s", id)}},
	}
}

func callNoteSearch(args map[string]any) ToolCallResult {
	queryRaw, ok := args["query"]
	if !ok {
		return errResult("missing 'query' argument")
	}
	query, ok := queryRaw.(string)
	if !ok {
		return errResult("'query' must be a string")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return errResult("query cannot be empty")
	}
	workspaceId := ""
	if w, ok := args["workspaceid"].(string); ok {
		workspaceId = w
	}
	scope := ""
	if s, ok := args["scope"].(string); ok {
		scope = s
	}
	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
		if limit < 1 {
			limit = 1
		}
		if limit > 100 {
			limit = 100
		}
	}
	store := aistore.GetMemoryStore()
	matches, err := store.Search(context.Background(), workspaceId, scope, query, limit)
	if err != nil {
		return errResult(err.Error())
	}
	if len(matches) == 0 {
		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("No matches found for %q.", query)}},
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Found %d matches for %q:\n\n", len(matches), query)
	for _, m := range matches {
		fmt.Fprintf(&out, "- ID: %s\n", m.Id)
		if m.Scope != "" {
			fmt.Fprintf(&out, "  Scope: %s\n", m.Scope)
		}
		if m.Key != "" {
			fmt.Fprintf(&out, "  Key: %s\n", m.Key)
		}
		fmt.Fprintf(&out, "  Match: %s\n", m.Snippet)
		out.WriteString("\n")
	}
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: out.String()}},
	}
}

func callToolList() string {
	var out strings.Builder
	fmt.Fprintln(&out, "Available tools:")
	for _, t := range defineTools() {
		fmt.Fprintf(&out, "- %s: %s\n", t.Name, t.Description)
	}
	return out.String()
}

func callToolSchema(args map[string]any) ToolCallResult {
	nameRaw, ok := args["name"]
	if !ok {
		return errResult("missing 'name' argument")
	}
	name, ok := nameRaw.(string)
	if !ok {
		return errResult("'name' must be a string")
	}
	schemaBytes, err := json.MarshalIndent(getToolSchema(name), "", "  ")
	if err != nil {
		return errResult(err.Error())
	}
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: string(schemaBytes)}},
	}
}

func getToolSchema(name string) ToolDefinition {
	for _, t := range defineTools() {
		if t.Name == name {
			return t
		}
	}
	return ToolDefinition{}
}

func callSysInfo(args map[string]any) ToolCallResult {
	return ToolCallResult{
		Content: []ToolContent{{
			Type: "text",
			Text: "System info: OS=" + runtime.GOOS + " Arch=" + runtime.GOARCH + " CPUs=" + fmt.Sprintf("%d", runtime.NumCPU()) + " Go=" + runtime.Version(),
		}},
	}
}

func callSysEnv(args map[string]any) ToolCallResult {
	var names []string
	if raw, ok := args["names"].([]any); ok {
		for _, n := range raw {
			if s, ok := n.(string); ok && s != "" {
				names = append(names, s)
			}
		}
	}
	env := make(map[string]string)
	if len(names) == 0 {
		for _, kv := range os.Environ() {
			if eq := strings.IndexByte(kv, '='); eq > 0 {
				env[kv[:eq]] = kv[eq+1:]
			}
		}
	} else {
		for _, n := range names {
			if v, ok := os.LookupEnv(n); ok {
				env[n] = v
			}
		}
	}
	data, _ := json.Marshal(env)
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: string(data)}},
	}
}
