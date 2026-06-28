// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aistore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestAuditLogger(t *testing.T) *AuditLogger {
	t.Helper()
	tmp := t.TempDir()
	al := newAuditLogger()
	al.getDataDir = func() string { return tmp }
	t.Cleanup(func() { al.CloseAll() })
	return al
}

func countEntries(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		return -1
	}
	defer f.Close()
	r := bufio.NewReader(f)
	count := 0
	for {
		_, err := r.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return -1
		}
		count++
	}
	return count
}

func TestAuditLoggerRecordAndQuery(t *testing.T) {
	al := newTestAuditLogger(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	entries := []ToolCallLogEntry{
		{ToolName: "note_put", Status: "ok", WorkspaceId: "ws_x", SessionId: "s1", AgentId: "agent-1", Timestamp: now},
		{ToolName: "note_get", Status: "ok", WorkspaceId: "ws_x", SessionId: "s1", AgentId: "agent-1", Timestamp: now + 100},
		{ToolName: "term_send_input", Status: "started", WorkspaceId: "ws_x", SessionId: "s2", AgentId: "agent-2", Timestamp: now + 200},
		{ToolName: "note_delete", Status: "approval-required", WorkspaceId: "ws_x", SessionId: "s1", AgentId: "agent-1", Timestamp: now + 300},
		{ToolName: "note_delete", Status: "approval-denied", WorkspaceId: "ws_x", SessionId: "s1", AgentId: "agent-1", Timestamp: now + 400},
	}
	for _, e := range entries {
		if err := al.Record(ctx, e); err != nil {
			t.Fatalf("Record failed: %v", err)
		}
	}

	got, err := al.Query(ctx, ToolCallLogQuery{WorkspaceId: "ws_x"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(got))
	}

	got, err = al.Query(ctx, ToolCallLogQuery{WorkspaceId: "ws_x", ToolName: "note_delete"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 note_delete entries, got %d", len(got))
	}

	got, err = al.Query(ctx, ToolCallLogQuery{WorkspaceId: "ws_x", SessionId: "s2"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session-s2 entry, got %d", len(got))
	}

	got, err = al.Query(ctx, ToolCallLogQuery{WorkspaceId: "ws_x", Status: "approval-required"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 approval-required, got %d", len(got))
	}

	got, err = al.Query(ctx, ToolCallLogQuery{WorkspaceId: "ws_x", SinceMs: now + 250})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries after now+250, got %d", len(got))
	}

	got, err = al.Query(ctx, ToolCallLogQuery{WorkspaceId: "ws_x", AgentId: "agent-2"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry for agent-2, got %d", len(got))
	}
}

func TestAuditLoggerConcurrentWrite(t *testing.T) {
	al := newTestAuditLogger(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_ = al.Record(ctx, ToolCallLogEntry{
					ToolName:    "concurrent_test",
					Status:      "ok",
					WorkspaceId: "ws_c",
					SessionId:   "sess",
					Timestamp:   time.Now().UnixMilli(),
				})
			}
		}(i)
	}
	wg.Wait()
	got, err := al.Query(ctx, ToolCallLogQuery{WorkspaceId: "ws_c"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("expected 100 entries, got %d", len(got))
	}
}

func TestAuditLoggerTail(t *testing.T) {
	al := newTestAuditLogger(t)
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		_ = al.Record(ctx, ToolCallLogEntry{
			ToolName:    "tailtest",
			Status:      "ok",
			WorkspaceId: "ws_t",
			Timestamp:   time.Now().UnixMilli(),
		})
	}
	out, err := al.Tail(ctx, "ws_t", "", 20)
	if err != nil {
		t.Fatalf("Tail failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 20 {
		t.Fatalf("expected 20 tail lines, got %d", len(lines))
	}
	for _, line := range lines {
		var e ToolCallLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("tail line not valid JSON: %v", err)
		}
		if e.ToolName != "tailtest" {
			t.Fatalf("unexpected tool in tail: %s", e.ToolName)
		}
	}
}

func TestAuditLoggerPurgeOld(t *testing.T) {
	al := newTestAuditLogger(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()
	_ = al.Record(ctx, ToolCallLogEntry{ToolName: "fresh", Status: "ok", WorkspaceId: "ws_p", Timestamp: now})

	old := now - 30*86400*1000
	oldTime := time.UnixMilli(old)
	dir := filepath.Join(al.dataDir(), auditLogDir, "ws_p")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	oldDay := oldTime.UTC().Format("2006-01-02")
	oldPath := filepath.Join(dir, oldDay+".log")
	oldLine := `{"toolname":"old","workspaceid":"ws_p","timestamp":` + fmt.Sprintf("%d", old) + `,` + `"status":"ok"}`
	if err := os.WriteFile(oldPath, []byte(oldLine+"\n"), 0600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes failed: %v", err)
	}
	got, err := al.Query(ctx, ToolCallLogQuery{WorkspaceId: "ws_p"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 before purge, got %d", len(got))
	}
	deleted, err := al.PurgeOld(ctx, "ws_p", 7)
	if err != nil {
		t.Fatalf("PurgeOld failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old log should be removed")
	}
}
