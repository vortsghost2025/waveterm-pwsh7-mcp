package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func homeDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return home
}

func cwdDir(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return cwd
}

// --------------- mcpExpandPath ---------------

func TestMcpExpandPath(t *testing.T) {
	home := homeDir(t)
	cwd := cwdDir(t)

	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantSub string
	}{
		{
			name:    "empty path rejected",
			input:   "",
			wantErr: true,
		},
		{
			name:    "tilde expands to home",
			input:   "~/testdir",
			wantSub: filepath.Join(home, "testdir"),
		},
		{
			name:    "bare tilde expands to home",
			input:   "~",
			wantSub: home,
		},
		{
			name:    "relative path rejected",
			input:   "relative/path",
			wantErr: true,
		},
		{
			name:    "cwd-relative allowed",
			input:   filepath.Join(cwd, "subdir"),
		},
		{
			name:    "path outside home and cwd rejected",
			input:   "/opt/forbidden",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mcpExpandPath(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got path %q", tc.input, got)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if tc.wantSub != "" && got != tc.wantSub {
				t.Fatalf("expected %q, got %q", tc.wantSub, got)
			}
		})
	}
}

// --------------- mcpIsPathSafe ---------------

func TestMcpIsPathSafe(t *testing.T) {
	home := homeDir(t)
	cwd := cwdDir(t)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name: "path under home",
			path: filepath.Join(home, "documents"),
		},
		{
			name: "path under cwd",
			path: filepath.Join(cwd, "subdir"),
		},
		{
			name:    "path outside both",
			path:    "/opt/forbidden",
			wantErr: true,
		},
		{
			name:    "root path outside",
			path:    "/",
			wantErr: true,
		},
		{
			name: "home itself",
			path: home,
		},
		{
			name: "cwd itself",
			path: cwd,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := mcpIsPathSafe(tc.path)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q", tc.path)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.path, err)
			}
		})
	}
}

// --------------- isBlockedFile ---------------

func TestIsBlockedFile(t *testing.T) {
	home := homeDir(t)

	tests := []struct {
		name      string
		path      string
		wantBlock bool
	}{
		{
			name:      "aws credentials blocked",
			path:      filepath.Join(home, ".aws", "credentials"),
			wantBlock: true,
		},
		{
			name:      "netrc blocked",
			path:      filepath.Join(home, ".netrc"),
			wantBlock: true,
		},
		{
			name:      "pgpass blocked",
			path:      filepath.Join(home, ".pgpass"),
			wantBlock: true,
		},
		{
			name:      "pem file blocked",
			path:      filepath.Join(home, "cert.pem"),
			wantBlock: true,
		},
		{
			name:      "p12 file blocked",
			path:      filepath.Join(home, "cert.p12"),
			wantBlock: true,
		},
		{
			name:      "key file blocked",
			path:      filepath.Join(home, "server.key"),
			wantBlock: true,
		},
		{
			name:      "jks file blocked",
			path:      filepath.Join(home, "keystore.jks"),
			wantBlock: true,
		},
		{
			name:      "git-credentials blocked",
			path:      filepath.Join(home, ".git-credentials"),
			wantBlock: true,
		},
		{
			name:      "gpg dir blocked",
			path:      filepath.Join(home, ".gnupg", "pubring.kbx"),
			wantBlock: true,
		},
		{
			name:      "ssh id_rsa blocked",
			path:      filepath.Join(home, ".ssh", "id_rsa"),
			wantBlock: true,
		},
		{
			name:      "ssh id_ed25519 blocked",
			path:      filepath.Join(home, ".ssh", "id_ed25519"),
			wantBlock: true,
		},
		{
			name:      "regular file not blocked",
			path:      filepath.Join(home, "hello.txt"),
			wantBlock: false,
		},
		{
			name:      "go file not blocked",
			path:      filepath.Join(home, "main.go"),
			wantBlock: false,
		},
	}

	if runtime.GOOS == "linux" {
		tests = append(tests, struct {
			name      string
			path      string
			wantBlock bool
		}{"/etc/shadow", "/etc/shadow", true})
		tests = append(tests, struct {
			name      string
			path      string
			wantBlock bool
		}{"/etc/sudoers", "/etc/sudoers", true})
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blocked, reason := isBlockedFile(tc.path)
			if blocked != tc.wantBlock {
				t.Fatalf("isBlockedFile(%q) = %v (reason: %q), want %v", tc.path, blocked, reason, tc.wantBlock)
			}
		})
	}
}

// --------------- extractKeywords ---------------

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantAny  []string
		wantNone []string
	}{
		{
			name:     "simple terms",
			input:    "read directory contents",
			wantAny:  []string{"read", "directory", "contents"},
			wantNone: []string{},
		},
		{
			name:     "stop words filtered",
			input:    "the quick brown fox",
			wantAny:  []string{"quick", "brown", "fox"},
			wantNone: []string{"the"},
		},
		{
			name:     "camelCase split",
			input:    "callReadDir",
			wantAny:  []string{"call", "Read", "Dir"},
			wantNone: []string{},
		},
		{
			name:     "underscore split",
			input:    "call_read_dir",
			wantAny:  []string{"call", "read", "dir"},
			wantNone: []string{},
		},
		{
			name:     "single char words filtered",
			input:    "a b c function",
			wantAny:  []string{"function"},
			wantNone: []string{"a", "b", "c"},
		},
		{
			name:     "max 10 keywords",
			input:    "one two three four five six seven eight nine ten eleven twelve",
			wantAny:  []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"},
			wantNone: []string{"eleven", "twelve"},
		},
		{
			name:     "empty input",
			input:    "",
			wantAny:  []string{},
			wantNone: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractKeywords(tc.input)
			for _, w := range tc.wantAny {
				found := false
				for _, g := range got {
					if strings.EqualFold(g, w) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected keyword %q in result %v", w, got)
				}
			}
			for _, w := range tc.wantNone {
				for _, g := range got {
					if strings.EqualFold(g, w) {
						t.Errorf("did not expect keyword %q in result %v", w, got)
					}
				}
			}
		})
	}
}

// --------------- callReadDir ---------------

func TestCallReadDir(t *testing.T) {
	tmpDir := t.TempDir()
	err := os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("hello"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{
			name:    "missing path",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "valid temp dir",
			args:    map[string]any{"path": tmpDir},
			wantErr: false,
		},
		{
			name:    "with max_entries",
			args:    map[string]any{"path": tmpDir, "max_entries": float64(10)},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origWd, _ := os.Getwd()
			defer os.Chdir(origWd)
			os.Chdir(tmpDir)

			result := callReadDir(tc.args)
			if tc.wantErr && !result.IsError {
				var m map[string]any
				json.Unmarshal([]byte(result.Content[0].Text), &m)
				if _, ok := m["error"]; !ok {
					t.Fatalf("expected error result, got: %s", result.Content[0].Text)
				}
			}
			if !tc.wantErr && result.IsError {
				t.Fatalf("unexpected error: %s", result.Content[0].Text)
			}
		})
	}
}

// --------------- callWriteTextFile ---------------

func TestCallWriteTextFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{
			name:    "missing filename",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "write new file",
			args:    map[string]any{"filename": filepath.Join(tmpDir, "new.txt"), "contents": "hello world"},
			wantErr: false,
		},
		{
			name:    "write to blocked path",
			args:    map[string]any{"filename": filepath.Join(homeDir(t), ".netrc"), "contents": "bad"},
			wantErr: true,
		},
		{
			name:    "empty filename",
			args:    map[string]any{"filename": "", "contents": "data"},
			wantErr: true,
		},
		{
			name:    "empty contents",
			args:    map[string]any{"filename": filepath.Join(tmpDir, "empty.txt"), "contents": ""},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origWd, _ := os.Getwd()
			defer os.Chdir(origWd)
			os.Chdir(tmpDir)

			result := callWriteTextFile(tc.args)
			if tc.wantErr {
				var m map[string]any
				json.Unmarshal([]byte(result.Content[0].Text), &m)
				if _, ok := m["error"]; !ok {
					t.Fatalf("expected error result, got: %s", result.Content[0].Text)
				}
			} else {
				if result.IsError {
					t.Fatalf("unexpected error: %s", result.Content[0].Text)
				}
				filename, _ := tc.args["filename"].(string)
				data, err := os.ReadFile(filename)
				if err != nil {
					t.Fatalf("failed to read written file: %v", err)
				}
				if string(data) != tc.args["contents"] {
					t.Fatalf("file content = %q, want %q", string(data), tc.args["contents"])
				}
			}
		})
	}
}

// --------------- callEditTextFile ---------------

func TestCallEditTextFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "editme.txt")
	err := os.WriteFile(testFile, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		args    map[string]any
		setup   func()
		wantErr bool
		wantContent string
	}{
		{
			name:    "missing filename",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "missing edits",
			args: map[string]any{
				"filename": testFile,
			},
			wantErr: true,
		},
		{
			name: "empty edits array",
			args: map[string]any{
				"filename": testFile,
				"edits":    []any{},
			},
			wantErr: true,
		},
		{
			name: "valid edit",
			args: map[string]any{
				"filename": testFile,
				"edits": []any{
					map[string]any{"old_str": "world", "new_str": "universe"},
				},
			},
			wantContent: "hello universe",
		},
		{
			name: "blocked file",
			args: map[string]any{
				"filename": filepath.Join(homeDir(t), ".netrc"),
				"edits": []any{
					map[string]any{"old_str": "x", "new_str": "y"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origWd, _ := os.Getwd()
			defer os.Chdir(origWd)
			os.Chdir(tmpDir)

			if tc.setup != nil {
				tc.setup()
			}

			result := callEditTextFile(tc.args)
			if tc.wantErr {
				var m map[string]any
				json.Unmarshal([]byte(result.Content[0].Text), &m)
				if _, ok := m["error"]; !ok {
					t.Fatalf("expected error result, got: %s", result.Content[0].Text)
				}
			} else {
				if result.IsError {
					t.Fatalf("unexpected error: %s", result.Content[0].Text)
				}
				if tc.wantContent != "" {
					filename, _ := tc.args["filename"].(string)
					data, err := os.ReadFile(filename)
					if err != nil {
						t.Fatalf("failed to read edited file: %v", err)
					}
					if string(data) != tc.wantContent {
						t.Fatalf("file content = %q, want %q", string(data), tc.wantContent)
					}
				}
			}
		})
	}
}

// --------------- callDeleteTextFile ---------------

func TestCallDeleteTextFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		args    map[string]any
		setup   func()
		wantErr bool
	}{
		{
			name:    "missing filename",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "delete existing file",
			args: map[string]any{
				"filename": filepath.Join(tmpDir, "deleteme.txt"),
			},
			setup: func() {
				os.WriteFile(filepath.Join(tmpDir, "deleteme.txt"), []byte("bye"), 0644)
			},
			wantErr: false,
		},
		{
			name: "delete non-existent file",
			args: map[string]any{
				"filename": filepath.Join(tmpDir, "nope.txt"),
			},
			wantErr: true,
		},
		{
			name: "delete blocked file",
			args: map[string]any{
				"filename": filepath.Join(homeDir(t), ".netrc"),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origWd, _ := os.Getwd()
			defer os.Chdir(origWd)
			os.Chdir(tmpDir)

			if tc.setup != nil {
				tc.setup()
			}

			result := callDeleteTextFile(tc.args)
			if tc.wantErr {
				var m map[string]any
				json.Unmarshal([]byte(result.Content[0].Text), &m)
				if _, ok := m["error"]; !ok {
					t.Fatalf("expected error result, got: %s", result.Content[0].Text)
				}
			} else {
				if result.IsError {
					t.Fatalf("unexpected error: %s", result.Content[0].Text)
				}
				filename, _ := tc.args["filename"].(string)
				if _, err := os.Stat(filename); !os.IsNotExist(err) {
					t.Fatalf("expected file to be deleted, but it exists")
				}
			}
		})
	}
}

// --------------- callCodebaseSearch ---------------

func TestCallCodebaseSearch(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "src", "main.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "src", "util.go"), []byte("package main\n\nfunc helper() string {\n\treturn \"helper\"\n}\n"), 0644)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{
			name:    "missing query",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "search for function",
			args:    map[string]any{"query": "func main"},
			wantErr: false,
		},
		{
			name:    "search for helper",
			args:    map[string]any{"query": "helper function", "path": tmpDir},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origWd, _ := os.Getwd()
			defer os.Chdir(origWd)
			os.Chdir(tmpDir)

			result := callCodebaseSearch(tc.args)
			if tc.wantErr {
				var m map[string]any
				json.Unmarshal([]byte(result.Content[0].Text), &m)
				if _, ok := m["error"]; !ok {
					t.Fatalf("expected error result, got: %s", result.Content[0].Text)
				}
			} else if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content[0].Text)
			}
		})
	}
}
