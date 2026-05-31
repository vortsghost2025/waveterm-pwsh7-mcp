package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
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
				"type":     "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_wave_env",
			Description: "Get Wave-relevant environment variables.",
			InputSchema: map[string]any{
				"type":     "object",
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", cmdStr)
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
	if rawPath == "~" || strings.HasPrefix(rawPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rawPath = filepath.Join(home, rawPath[1:])
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

func errResult(msg string) ToolCallResult {
	return ToolCallResult{
		IsError: true,
		Content: []ToolContent{{Type: "text", Text: msg}},
	}
}


