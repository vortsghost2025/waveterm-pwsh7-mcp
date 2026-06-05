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
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/wavetermdev/waveterm/pkg/filebackup"
	"github.com/wavetermdev/waveterm/pkg/util/fileutil"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
)

// errJSONResult returns a ToolCallResult with IsError=true and a JSON
// body of the form {"error": msg}. Used by the new file/search handlers
// so test assertions can parse the error payload as JSON.
func errJSONResult(msg string) ToolCallResult {
	b, _ := json.Marshal(map[string]any{"error": msg})
	return ToolCallResult{
		IsError: true,
		Content: []ToolContent{{Type: "text", Text: string(b)}},
	}
}

// mcpExpandPath expands ~ to home dir and validates the path is within
// the user's home directory or current working directory.
func mcpExpandPath(rawPath string) (string, error) {
	if rawPath == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	expanded := rawPath
	if rawPath == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		expanded = home
	} else if strings.HasPrefix(rawPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		expanded = filepath.Join(home, rawPath[2:])
	}
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("path must be absolute: %s", rawPath)
	}
	cleanPath := filepath.Clean(expanded)
	if err := mcpIsPathSafe(cleanPath); err != nil {
		return "", err
	}
	return cleanPath, nil
}

// mcpIsPathSafe checks that absPath is under the user's home or cwd.
func mcpIsPathSafe(absPath string) error {
	absPath = filepath.Clean(absPath)
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %v", err)
	}
	home = filepath.Clean(home)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %v", err)
	}
	cwd = filepath.Clean(cwd)

	relHome, errRelHome := filepath.Rel(home, absPath)
	underHome := errRelHome == nil && relHome != ".." && !strings.HasPrefix(relHome, ".."+string(filepath.Separator))
	relCwd, errRelCwd := filepath.Rel(cwd, absPath)
	underCwd := errRelCwd == nil && relCwd != ".." && !strings.HasPrefix(relCwd, ".."+string(filepath.Separator))

	if !underHome && !underCwd {
		return fmt.Errorf("path %q is outside allowed directories (home: %s, cwd: %s)", absPath, home, cwd)
	}
	return nil
}

// isBlockedFile returns true if the path points to a sensitive file or directory.
func isBlockedFile(expandedPath string) (bool, string) {
	homeDir, _ := os.UserHomeDir()
	cleanPath := filepath.Clean(expandedPath)
	baseName := filepath.Base(cleanPath)

	exactPaths := []struct{ path, reason string }{
		{filepath.Join(homeDir, ".aws", "credentials"), "AWS credentials file"},
		{filepath.Join(homeDir, ".git-credentials"), "Git credentials file"},
		{filepath.Join(homeDir, ".netrc"), "netrc credentials file"},
		{filepath.Join(homeDir, ".pgpass"), "PostgreSQL password file"},
		{filepath.Join(homeDir, ".my.cnf"), "MySQL credentials file"},
		{filepath.Join(homeDir, ".kube", "config"), "Kubernetes config file"},
		{"/etc/shadow", "system password file"},
		{"/etc/sudoers", "system sudoers file"},
	}
	for _, ep := range exactPaths {
		if cleanPath == ep.path {
			return true, ep.reason
		}
	}

	dirPrefixes := []struct{ prefix, reason string }{
		{filepath.Join(homeDir, ".gnupg") + string(filepath.Separator), "GPG directory"},
		{filepath.Join(homeDir, ".password-store") + string(filepath.Separator), "password store directory"},
		{"/etc/sudoers.d/", "system sudoers directory"},
	}
	if runtime.GOOS == "darwin" {
		dirPrefixes = append(dirPrefixes,
			struct{ prefix, reason string }{"/Library/Keychains/", "macOS keychain directory"},
			struct{ prefix, reason string }{filepath.Join(homeDir, "Library", "Keychains") + string(filepath.Separator), "macOS keychain directory"})
	}
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			dirPrefixes = append(dirPrefixes,
				struct{ prefix, reason string }{filepath.Join(localAppData, "Microsoft", "Credentials") + string(filepath.Separator), "Windows credentials"})
		}
		if appData := os.Getenv("APPDATA"); appData != "" {
			dirPrefixes = append(dirPrefixes,
				struct{ prefix, reason string }{filepath.Join(appData, "Microsoft", "Credentials") + string(filepath.Separator), "Windows credentials"})
		}
	}
	for _, dp := range dirPrefixes {
		if strings.HasPrefix(cleanPath, dp.prefix) {
			return true, dp.reason
		}
	}

	if strings.Contains(cleanPath, filepath.Join(homeDir, ".secrets")) {
		return true, "secrets directory"
	}
	if strings.HasPrefix(baseName, "id_") && strings.Contains(cleanPath, ".ssh") {
		return true, "SSH private key"
	}
	if strings.Contains(baseName, "id_rsa") {
		return true, "SSH private key"
	}
	if strings.HasPrefix(baseName, "ssh_host_") && strings.Contains(baseName, "key") {
		return true, "SSH host key"
	}

	extensions := map[string]string{
		".pem": "certificate/key file", ".p12": "certificate file", ".key": "key file",
		".pfx": "certificate file", ".pkcs12": "certificate file",
		".keystore": "Java keystore file", ".jks": "Java keystore file",
	}
	if reason, exists := extensions[filepath.Ext(baseName)]; exists {
		return true, reason
	}
	if baseName == ".git-credentials" {
		return true, "Git credentials file"
	}
	return false, ""
}

// callReadDir lists a directory with size/type/mtime info.
func callReadDir(args map[string]any) ToolCallResult {
	pathRaw, ok := args["path"]
	if !ok {
		return errJSONResult("missing 'path' argument")
	}
	pathStr, ok := pathRaw.(string)
	if !ok {
		return errJSONResult("'path' must be a string")
	}
	if pathStr == "" {
		return errJSONResult("path cannot be empty")
	}
	maxEntries := 500
	if me, ok := args["max_entries"].(float64); ok {
		maxEntries = int(me)
		if maxEntries < 1 {
			maxEntries = 1
		}
		if maxEntries > 10000 {
			maxEntries = 10000
		}
	}
	resolvedPath, err := mcpExpandPath(pathStr)
	if err != nil {
		return errJSONResult(err.Error())
	}
	result, err := fileutil.ReadDir(resolvedPath, maxEntries)
	if err != nil {
		return errJSONResult(err.Error())
	}
	entries := make([]map[string]any, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = map[string]any{
			"name": e.Name, "dir": e.Dir, "symlink": e.Symlink,
			"size": e.Size, "mode": e.Mode, "modified": e.Modified, "modified_time": e.ModifiedTime,
		}
	}
	out := map[string]any{
		"path": result.Path, "absolute_path": result.AbsolutePath,
		"entries": entries, "entry_count": result.EntryCount, "total_entries": result.TotalEntries,
	}
	if result.Truncated {
		out["truncated"] = true
		out["truncated_message"] = fmt.Sprintf(
			"Directory listing truncated to %d entries (out of %d total). Increase max_entries to see more.",
			result.EntryCount, result.TotalEntries)
	}
	if result.ParentDir != "" {
		out["parent_dir"] = result.ParentDir
	}
	b, _ := json.Marshal(out)
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(b)}}}
}

// callWriteTextFile writes a text file to the filesystem with backup support.
func callWriteTextFile(args map[string]any) ToolCallResult {
	filenameRaw, ok := args["filename"]
	if !ok {
		return errJSONResult("missing 'filename' argument")
	}
	filename, ok := filenameRaw.(string)
	if !ok {
		return errJSONResult("'filename' must be a string")
	}
	contentsRaw, ok := args["contents"]
	if !ok {
		return errJSONResult("missing 'contents' argument")
	}
	contents, ok := contentsRaw.(string)
	if !ok {
		return errJSONResult("'contents' must be a string")
	}
	if contents == "" {
		return errJSONResult("contents cannot be empty")
	}
	if len(contents) > 100*1024 {
		return errJSONResult("file content too large (max 100KB)")
	}
	if utilfn.HasBinaryData([]byte(contents)) {
		return errJSONResult("contents appear to contain binary data")
	}
	resolvedPath, err := mcpExpandPath(filename)
	if err != nil {
		return errJSONResult(err.Error())
	}
	if blocked, reason := isBlockedFile(resolvedPath); blocked {
		return errJSONResult(fmt.Sprintf("access denied: %s", reason))
	}
	fileInfo, err := os.Lstat(resolvedPath)
	if err != nil && !os.IsNotExist(err) {
		return errJSONResult(fmt.Sprintf("cannot access path: %v", err))
	}
	if fileInfo != nil && fileInfo.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(resolvedPath)
		return errJSONResult(fmt.Sprintf("cannot write to symlink (target: %s). Write to the target file directly.", target))
	}
	dirPath := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return errJSONResult(fmt.Sprintf("cannot create directory: %v", err))
	}
	var backupPath string
	if fileInfo != nil && fileInfo.Mode().IsRegular() {
		bp, err := filebackup.MakeFileBackup(resolvedPath)
		if err != nil {
			return errJSONResult(fmt.Sprintf("cannot create backup: %v", err))
		}
		backupPath = bp
	}
	if err := os.WriteFile(resolvedPath, []byte(contents), 0644); err != nil {
		return errJSONResult(fmt.Sprintf("cannot write file: %v", err))
	}
	msg := fmt.Sprintf("Successfully wrote %s (%d bytes)", filename, len(contents))
	if backupPath != "" {
		msg += " (backup: " + backupPath + ")"
	}
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: msg}}}
}

// callEditTextFile edits a text file with atomic search-and-replace.
func callEditTextFile(args map[string]any) ToolCallResult {
	filenameRaw, ok := args["filename"]
	if !ok {
		return errJSONResult("missing 'filename' argument")
	}
	filename, ok := filenameRaw.(string)
	if !ok {
		return errJSONResult("'filename' must be a string")
	}
	editsRaw, ok := args["edits"]
	if !ok {
		return errJSONResult("missing 'edits' argument")
	}
	editsArr, ok := editsRaw.([]any)
	if !ok {
		return errJSONResult("'edits' must be an array")
	}
	if len(editsArr) == 0 {
		return errJSONResult("edits array cannot be empty")
	}
	resolvedPath, err := mcpExpandPath(filename)
	if err != nil {
		return errJSONResult(err.Error())
	}
	if blocked, reason := isBlockedFile(resolvedPath); blocked {
		return errJSONResult(fmt.Sprintf("access denied: %s", reason))
	}
	fileInfo, err := os.Lstat(resolvedPath)
	if err != nil {
		return errJSONResult(fmt.Sprintf("cannot access file: %v", err))
	}
	if !fileInfo.Mode().IsRegular() {
		return errJSONResult("path is not a regular file")
	}
	if fileInfo.Size() > 100*1024 {
		return errJSONResult(fmt.Sprintf("file too large for editing: %d bytes (max %d)", fileInfo.Size(), 100*1024))
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return errJSONResult(fmt.Sprintf("cannot read file: %v", err))
	}
	if utilfn.HasBinaryData(data) {
		return errJSONResult("file contains binary data, cannot edit")
	}
	edits := make([]fileutil.EditSpec, 0, len(editsArr))
	for i, e := range editsArr {
		eMap, ok := e.(map[string]any)
		if !ok {
			return errJSONResult(fmt.Sprintf("edit[%d] must be an object", i))
		}
		oldStr, ok := eMap["old_str"].(string)
		if !ok || oldStr == "" {
			return errJSONResult(fmt.Sprintf("edit[%d]: old_str must be a non-empty string", i))
		}
		newStr, ok := eMap["new_str"].(string)
		if !ok {
			return errJSONResult(fmt.Sprintf("edit[%d]: new_str must be a string", i))
		}
		desc, _ := eMap["desc"].(string)
		edits = append(edits, fileutil.EditSpec{OldStr: oldStr, NewStr: newStr, Desc: desc})
	}
	if _, err := fileutil.ApplyEdits(data, edits); err != nil {
		return errJSONResult(fmt.Sprintf("edit validation failed: %v", err))
	}
	bp, err := filebackup.MakeFileBackup(resolvedPath)
	if err != nil {
		return errJSONResult(fmt.Sprintf("cannot create backup: %v", err))
	}
	if err := fileutil.ReplaceInFile(resolvedPath, edits); err != nil {
		return errJSONResult(fmt.Sprintf("cannot edit file: %v", err))
	}
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Successfully edited %s with %d changes (backup: %s)", filename, len(edits), bp)}}}
}

// callDeleteTextFile deletes a text file after creating a backup.
func callDeleteTextFile(args map[string]any) ToolCallResult {
	filenameRaw, ok := args["filename"]
	if !ok {
		return errJSONResult("missing 'filename' argument")
	}
	filename, ok := filenameRaw.(string)
	if !ok {
		return errJSONResult("'filename' must be a string")
	}
	resolvedPath, err := mcpExpandPath(filename)
	if err != nil {
		return errJSONResult(err.Error())
	}
	if blocked, reason := isBlockedFile(resolvedPath); blocked {
		return errJSONResult(fmt.Sprintf("access denied: %s", reason))
	}
	fileInfo, err := os.Lstat(resolvedPath)
	if err != nil {
		return errJSONResult(fmt.Sprintf("cannot access file: %v", err))
	}
	if !fileInfo.Mode().IsRegular() {
		return errJSONResult("path is not a regular file (symlinks and special files cannot be deleted)")
	}
	if fileInfo.Size() > 100*1024 {
		return errJSONResult(fmt.Sprintf("file too large: %d bytes (max %d)", fileInfo.Size(), 100*1024))
	}
	bp, err := filebackup.MakeFileBackup(resolvedPath)
	if err != nil {
		return errJSONResult(fmt.Sprintf("cannot create backup: %v", err))
	}
	if err := os.Remove(resolvedPath); err != nil {
		return errJSONResult(fmt.Sprintf("cannot delete file: %v", err))
	}
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Successfully deleted %s (backup: %s)", filename, bp)}}}
}

// callCodebaseSearch searches the codebase using a natural language query.
func callCodebaseSearch(args map[string]any) ToolCallResult {
	queryRaw, ok := args["query"]
	if !ok {
		return errJSONResult("missing 'query' argument")
	}
	query, ok := queryRaw.(string)
	if !ok {
		return errJSONResult("'query' must be a string")
	}
	if query == "" {
		return errJSONResult("query cannot be empty")
	}
	pathRaw, _ := args["path"].(string)
	searchPath, err := mcpResolvePath(pathRaw)
	if err != nil {
		return errJSONResult(err.Error())
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
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return ToolCallResult{Content: []ToolContent{{Type: "text", Text: "Could not extract meaningful keywords from query"}}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type scoredMatch struct {
		file    string
		line    int
		content string
		score   int
	}
	var matches []scoredMatch
	count := 0
	seenKeys := make(map[string]bool)

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
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		var firstChunk [8192]byte
		n, _ := f.Read(firstChunk[:])
		if mcpIsBinary(firstChunk[:n]) {
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
			lowerText := strings.ToLower(text)
			lineScore := 0
			for _, kw := range keywords {
				if strings.Contains(lowerText, kw) {
					lineScore++
				}
			}
			if lineScore == 0 {
				continue
			}
			rel, _ := filepath.Rel(searchPath, path)
			if rel == "" {
				rel = path
			}
			key := fmt.Sprintf("%s:%d", rel, lineNum)
			if seenKeys[key] {
				continue
			}
			seenKeys[key] = true
			contentText := text
			if len(contentText) > 500 {
				contentText = contentText[:500] + "..."
			}
			matches = append(matches, scoredMatch{
				file: filepath.ToSlash(rel), line: lineNum,
				content: contentText, score: lineScore,
			})
			count++
			if count >= maxResults*2 {
				return filepath.SkipAll
			}
		}
		return nil
	}
	if err := filepath.Walk(searchPath, walkFn); err != nil && ctx.Err() == nil {
	}
	if ctx.Err() != nil {
		return errJSONResult("codebase_search timed out after 30s")
	}
	truncated := len(matches) > maxResults
	if truncated {
		matches = matches[:maxResults]
	}
	matchesJSON, _ := json.Marshal(matches)
	kwStr := strings.Join(keywords, ", ")
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: fmt.Sprintf(
		"Found %d matches for query %q (keywords: %s):\n%s",
		len(matches), query, kwStr, string(matchesJSON))}}}
}

// extractKeywords tokenizes a query and returns content-bearing keywords.
func extractKeywords(query string) []string {
	stopWords := map[string]bool{
		"the": true, "is": true, "at": true, "which": true, "on": true,
		"a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "with": true, "for": true, "to": true, "from": true,
		"of": true, "by": true, "be": true, "this": true, "that": true,
		"it": true, "are": true, "was": true, "were": true, "been": true,
		"being": true, "have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true, "could": true,
		"should": true, "may": true, "might": true, "shall": true, "can": true,
		"not": true, "no": true, "nor": true, "so": true, "if": true,
		"then": true, "than": true, "too": true, "very": true, "just": true,
		"about": true, "above": true, "after": true, "again": true, "all": true,
		"also": true, "any": true, "because": true, "before": true,
		"between": true, "both": true, "each": true, "few": true,
		"more": true, "most": true, "much": true, "must": true,
		"over": true, "same": true, "some": true, "such": true,
		"through": true, "under": true, "up": true,
		"what": true, "when": true, "where": true, "who": true, "why": true, "how": true,
		"find": true, "search": true, "look": true, "show": true,
		"get": true, "give": true, "tell": true, "me": true,
		"code": true, "file": true, "class": true,
		"method": true, "implement": true, "define": true, "definition": true,
		"handle": true, "used": true, "using": true, "use": true,
		"called": true, "need": true, "want": true,
		"like": true, "into": true, "make": true, "work": true, "doing": true,
	}
	var rawTokens []string
	var current strings.Builder
	prevWasLower := false
	for _, r := range query {
		if unicode.IsSpace(r) || r == '_' || r == '-' || r == '.' {
			if current.Len() > 0 {
				rawTokens = append(rawTokens, current.String())
				current.Reset()
			}
			prevWasLower = false
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			if current.Len() > 0 {
				rawTokens = append(rawTokens, current.String())
				current.Reset()
			}
			prevWasLower = false
			continue
		}
		if prevWasLower && unicode.IsUpper(r) {
			if current.Len() > 0 {
				rawTokens = append(rawTokens, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
		prevWasLower = unicode.IsLower(r)
	}
	if current.Len() > 0 {
		rawTokens = append(rawTokens, current.String())
	}
	seen := make(map[string]bool)
	var unique []string
	for _, t := range rawTokens {
		lower := strings.ToLower(t)
		if len(lower) <= 1 {
			continue
		}
		if stopWords[lower] {
			continue
		}
		if seen[lower] {
			continue
		}
		seen[lower] = true
		unique = append(unique, lower)
		if len(unique) >= 10 {
			break
		}
	}
	return unique
}

// callRunInteractiveCommand runs a command with a configurable timeout.
// Uses the same allowlist as run_readonly_command but supports longer timeouts
// and returns structured JSON output with exit code information.
func callRunInteractiveCommand(args map[string]any) ToolCallResult {
	cmdRaw, ok := args["command"]
	if !ok {
		return errJSONResult("missing 'command' argument")
	}
	cmdStr, ok := cmdRaw.(string)
	if !ok {
		return errJSONResult("'command' must be a string")
	}
	if cmdStr == "" {
		return errJSONResult("command cannot be empty")
	}
	if err := checkCommand(cmdStr); err != nil {
		return errJSONResult(err.Error())
	}

	timeoutMs := 30000
	if t, ok := args["timeout_ms"].(float64); ok {
		timeoutMs = int(t)
		if timeoutMs < 1000 {
			timeoutMs = 1000
		}
		if timeoutMs > 120000 {
			timeoutMs = 120000
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
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

	exitCode := 0
	timedOut := false
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			timedOut = true
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	result := map[string]any{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": exitCode,
		"timed_out": timedOut,
		"command":   cmdStr,
	}
	b, _ := json.Marshal(result)
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(b)}}}
}
