package aiusechat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

const (
	GrepTimeout          = 30 * time.Second
	SearchDefaultResults = 50
	GlobMaxResults       = 200
	SearchMaxFileSize    = 10 * 1024 * 1024
	SearchMaxLineLen     = 500
)

var skipSearchDirs = map[string]bool{
	".git":             true,
	"node_modules":     true,
	"__pycache__":      true,
	".next":            true,
	"dist":             true,
	"build":            true,
	"target":           true,
	".cache":           true,
	"venv":             true,
	".venv":            true,
	".bin":             true,
	"obj":              true,
	"bin":              true,
	"lib":              true,
	"include":          true,
	"share":            true,
	"coverage":         true,
	".nyc_output":      true,
	".turbo":           true,
	"out":              true,
	"debug":            true,
	"release":          true,
	"cmake-build-":     true,
	".gradle":          true,
	".idea":            true,
	".vscode":          true,
	".vs":              true,
	".terraform":       true,
	"vendor":           true,
	"third_party":      true,
	"third-party":      true,
}

type grepParams struct {
	Pattern    string `json:"pattern"`
	Include    string `json:"include,omitempty"`
	Path       string `json:"path,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

type globParams struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

type codebaseSearchParams struct {
	Query      string `json:"query"`
	Path       string `json:"path,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

type grepMatchResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

func expandAndVerifyPath(rawPath string) (string, error) {
	expanded, err := wavebase.ExpandHomeDir(rawPath)
	if err != nil {
		return "", fmt.Errorf("failed to expand path: %w", err)
	}
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("path must be absolute, got relative: %s", rawPath)
	}
	info, err := os.Stat(expanded)
	if err != nil {
		return "", fmt.Errorf("cannot access path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", expanded)
	}
	return expanded, nil
}

func resolveSearchPath(rawPath string) (string, error) {
	if rawPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("cannot determine working directory: %w", err)
		}
		return cwd, nil
	}
	return expandAndVerifyPath(rawPath)
}

func parseGrepInput(input any) (*grepParams, error) {
	result := &grepParams{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.Pattern == "" {
		return nil, fmt.Errorf("missing pattern parameter")
	}
	if result.MaxResults <= 0 {
		result.MaxResults = SearchDefaultResults
	}
	var err error
	result.Path, err = resolveSearchPath(result.Path)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseGlobInput(input any) (*globParams, error) {
	result := &globParams{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.Pattern == "" {
		return nil, fmt.Errorf("missing pattern parameter")
	}
	if result.MaxResults <= 0 {
		result.MaxResults = GlobMaxResults
	}
	var err error
	result.Path, err = resolveSearchPath(result.Path)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseCodebaseSearchInput(input any) (*codebaseSearchParams, error) {
	result := &codebaseSearchParams{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.Query == "" {
		return nil, fmt.Errorf("missing query parameter")
	}
	if result.MaxResults <= 0 {
		result.MaxResults = SearchDefaultResults
	}
	var err error
	result.Path, err = resolveSearchPath(result.Path)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func isSearchDir(name string) bool {
	if name == "" {
		return false
	}
	return !skipSearchDirs[strings.ToLower(name)]
}

func isBinaryChunk(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func splitIncludePatterns(include string) []string {
	if include == "" {
		return nil
	}
	var patterns []string
	for _, part := range strings.Split(include, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			patterns = append(patterns, part)
		}
	}
	return patterns
}

func matchesIncludePatterns(name string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	base := filepath.Base(name)
	for _, pat := range patterns {
		if ok, _ := filepath.Match(pat, base); ok {
			return true
		}
	}
	return false
}

func verifyGrepInput(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
	params, err := parseGrepInput(input)
	if err != nil {
		return err
	}
	if _, err := regexp.Compile(params.Pattern); err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}
	return nil
}

func verifyGlobInput(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
	_, err := parseGlobInput(input)
	return err
}

func verifyCodebaseSearchInput(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
	_, err := parseCodebaseSearchInput(input)
	return err
}

func grepCallback(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
	params, err := parseGrepInput(input)
	if err != nil {
		return nil, err
	}

	pat, err := regexp.Compile(params.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), GrepTimeout)
	defer cancel()

	includePatterns := splitIncludePatterns(params.Include)
	var matches []grepMatchResult
	matchCount := 0
	truncated := false

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.IsDir() {
			if !isSearchDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > SearchMaxFileSize || info.Size() == 0 {
			return nil
		}
		if !matchesIncludePatterns(path, includePatterns) {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		var firstChunk [8192]byte
		n, _ := f.Read(firstChunk[:])
		if isBinaryChunk(firstChunk[:n]) {
			return nil
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil
		}

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
				relPath, _ := filepath.Rel(params.Path, path)
				if relPath == "" {
					relPath = path
				}
				contentText := text
				if len(contentText) > SearchMaxLineLen {
					contentText = contentText[:SearchMaxLineLen] + "..."
				}
				matches = append(matches, grepMatchResult{
					File:    filepath.ToSlash(relPath),
					Line:    lineNum,
					Content: contentText,
				})
				matchCount++
				if matchCount >= params.MaxResults {
					truncated = true
					return filepath.SkipAll
				}
			}
		}
		return nil
	}

	if err := filepath.Walk(params.Path, walkFn); err != nil && ctx.Err() == nil {
		// Some error other than timeout
	}

	if ctx.Err() != nil {
		return nil, fmt.Errorf("search timed out after %v", GrepTimeout)
	}

	result := map[string]any{
		"matches": matches,
		"count":   len(matches),
	}
	if truncated {
		result["truncated"] = true
	}
	return result, nil
}

func globCallback(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
	params, err := parseGlobInput(input)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), GrepTimeout)
	defer cancel()

	var results []string
	matchCount := 0
	truncated := false

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.IsDir() {
			if !isSearchDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ok, _ := filepath.Match(params.Pattern, info.Name())
		if !ok {
			return nil
		}
		relPath, _ := filepath.Rel(params.Path, path)
		if relPath == "" {
			relPath = path
		}
		results = append(results, filepath.ToSlash(relPath))
		matchCount++
		if matchCount >= params.MaxResults {
			truncated = true
			return filepath.SkipAll
		}
		return nil
	}

	if err := filepath.Walk(params.Path, walkFn); err != nil && ctx.Err() == nil {
	}

	if ctx.Err() != nil {
		return nil, fmt.Errorf("search timed out after %v", GrepTimeout)
	}

	result := map[string]any{
		"files": results,
		"count": len(results),
	}
	if truncated {
		result["truncated"] = true
	}
	return result, nil
}

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
		"between": true, "both": true, "each": true, "few": true, "more": true,
		"most": true, "much": true, "must": true, "over": true, "same": true,
		"some": true, "such": true, "through": true, "under": true, "up": true,
		"what": true, "when": true, "where": true, "who": true, "why": true,
		"how": true, "find": true, "search": true, "look": true, "show": true,
		"get": true, "give": true, "tell": true, "me": true,
		"code": true, "file": true, "function": true, "class": true, "method": true,
		"implement": true, "implementation": true, "define": true, "definition": true,
		"handle": true, "used": true, "using": true, "use": true, "call": true,
		"called": true, "need": true, "want": true, "like": true, "into": true,
		"make": true, "work": true, "doing": true,
	}

	query = strings.ToLower(query)
	var words []string
	var current strings.Builder
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				word := current.String()
				if len(word) > 1 && !stopWords[word] {
					words = append(words, word)
				}
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		word := current.String()
		if len(word) > 1 && !stopWords[word] {
			words = append(words, word)
		}
	}

	var unique []string
	seen := make(map[string]bool)
	for _, w := range words {
		if !seen[w] {
			seen[w] = true
			unique = append(unique, w)
		}
	}
	if len(unique) > 10 {
		unique = unique[:10]
	}
	return unique
}

func codebaseSearchCallback(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
	params, err := parseCodebaseSearchInput(input)
	if err != nil {
		return nil, err
	}

	keywords := extractKeywords(params.Query)
	if len(keywords) == 0 {
		return map[string]any{
			"results":   []any{},
			"count":     0,
			"query":     params.Query,
			"keywords":  []string{},
			"message":   "Could not extract meaningful keywords from query",
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), GrepTimeout)
	defer cancel()

	type scoredMatch struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Content string `json:"content"`
		Score   int    `json:"-"`
	}

	var matches []scoredMatch
	matchCount := 0
	seenKeys := make(map[string]bool)

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.IsDir() {
			if !isSearchDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > SearchMaxFileSize || info.Size() == 0 {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		var firstChunk [8192]byte
		n, _ := f.Read(firstChunk[:])
		if isBinaryChunk(firstChunk[:n]) {
			return nil
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil
		}

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

			relPath, _ := filepath.Rel(params.Path, path)
			if relPath == "" {
				relPath = path
			}
			key := fmt.Sprintf("%s:%d", relPath, lineNum)
			if seenKeys[key] {
				continue
			}
			seenKeys[key] = true

			contentText := text
			if len(contentText) > SearchMaxLineLen {
				contentText = contentText[:SearchMaxLineLen] + "..."
			}

			matches = append(matches, scoredMatch{
				File:    filepath.ToSlash(relPath),
				Line:    lineNum,
				Content: contentText,
				Score:   lineScore,
			})
			matchCount++
			if matchCount >= params.MaxResults*2 {
				return filepath.SkipAll
			}
		}
		return nil
	}

	if err := filepath.Walk(params.Path, walkFn); err != nil && ctx.Err() == nil {
	}

	if ctx.Err() != nil {
		return nil, fmt.Errorf("search timed out after %v", GrepTimeout)
	}

	results := make([]scoredMatch, 0, len(matches))
	truncated := len(matches) > params.MaxResults
	for _, m := range matches {
		if len(results) >= params.MaxResults {
			break
		}
		results = append(results, m)
	}

	return map[string]any{
		"results":   results,
		"count":     len(results),
		"query":     params.Query,
		"keywords":  keywords,
		"truncated": truncated,
		"message":   fmt.Sprintf("Found %d matches for query %q", len(results), params.Query),
	}, nil
}

func GetGrepToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "grep",
		DisplayName: "Grep",
		Description: "Search file contents using a regular expression pattern. Returns matching file paths, line numbers, and line content. By default searches the current working directory tree (skipping .git, node_modules, dist, and other build artifacts). Use the 'include' parameter to filter by file extension (e.g., '*.go' or '*.ts,*.tsx'). Use the 'path' parameter to search a specific directory.",
		ToolLogName: "gen:grep",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Regular expression pattern to search for in file contents",
				},
				"include": map[string]any{
					"type":        "string",
					"description": "Optional comma-separated glob patterns to filter files by name (e.g., '*.go', '*.ts,*.tsx,*.js')",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Optional absolute path to search in (defaults to current working directory)",
				},
				"max_results": map[string]any{
					"type":         "integer",
					"minimum":      1,
					"maximum":      500,
					"default":      SearchDefaultResults,
					"description":  "Maximum number of results to return",
				},
			},
			"required":             []string{"pattern", "include", "path", "max_results"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			params, err := parseGrepInput(input)
			if err != nil {
				return fmt.Sprintf("error: %v", err)
			}
			return fmt.Sprintf("searching for %q in %s", params.Pattern, params.Path)
		},
		ToolAnyCallback: grepCallback,
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: verifyGrepInput,
	}
}

func GetGlobToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "glob",
		DisplayName: "Glob",
		Description: "Find files by name pattern (glob). Returns file paths matching the given glob pattern. By default searches the current working directory tree (skipping .git, node_modules, dist, and other build artifacts). Use the 'path' parameter to search a specific directory.",
		ToolLogName: "gen:glob",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern to match file names against (e.g., '*.go', '**/*.tsx', '*.{ts,tsx}')",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Optional absolute path to search in (defaults to current working directory)",
				},
				"max_results": map[string]any{
					"type":         "integer",
					"minimum":      1,
					"maximum":      1000,
					"default":      GlobMaxResults,
					"description":  "Maximum number of results to return",
				},
			},
			"required":             []string{"pattern", "path", "max_results"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			params, err := parseGlobInput(input)
			if err != nil {
				return fmt.Sprintf("error: %v", err)
			}
			return fmt.Sprintf("finding files matching %q in %s", params.Pattern, params.Path)
		},
		ToolAnyCallback: globCallback,
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: verifyGlobInput,
	}
}

func GetCodebaseSearchToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "codebase_search",
		DisplayName: "Codebase Search",
		Description: "Search the codebase using a natural language query. Extracts keywords from the query and searches file contents for matches. Returns relevant file paths, line numbers, and matching content. Best for finding code related to a specific concept, feature, or function without needing to construct exact regex patterns.",
		ToolLogName: "gen:codebasesearch",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural language description of the code you're looking for (e.g., 'handle user authentication', 'database connection code', 'error handling middleware')",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Optional absolute path to search in (defaults to current working directory)",
				},
				"max_results": map[string]any{
					"type":         "integer",
					"minimum":      1,
					"maximum":      500,
					"default":      SearchDefaultResults,
					"description":  "Maximum number of results to return",
				},
			},
			"required":             []string{"query", "path", "max_results"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			params, err := parseCodebaseSearchInput(input)
			if err != nil {
				return fmt.Sprintf("error: %v", err)
			}
			return fmt.Sprintf("searching codebase for %q in %s", params.Query, params.Path)
		},
		ToolAnyCallback: codebaseSearchCallback,
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: verifyCodebaseSearchInput,
	}
}
