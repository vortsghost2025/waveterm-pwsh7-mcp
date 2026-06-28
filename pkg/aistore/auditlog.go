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
	"time"

	"github.com/google/uuid"
)

const auditLogDir = "auditlog"
const maxLogLineBytes = 64 * 1024

var globalAuditLog = newAuditLogger()
var DefaultDataDir = func() string { return "" }

func SetDefaultDataDir(fn func() string) {
	if fn != nil {
		DefaultDataDir = fn
	}
}

type AuditLogger struct {
	lock       sync.Mutex
	fhByWs     map[string]*os.File
	wrByWs     map[string]*bufio.Writer
	currPathWs map[string]string
	getDataDir func() string
}

func newAuditLogger() *AuditLogger {
	return &AuditLogger{
		fhByWs:     map[string]*os.File{},
		wrByWs:     map[string]*bufio.Writer{},
		currPathWs: map[string]string{},
		getDataDir: nil,
	}
}

func GetAuditLogger() *AuditLogger {
	return globalAuditLog
}

func (l *AuditLogger) dataDir() string {
	if l.getDataDir != nil {
		return l.getDataDir()
	}
	return DefaultDataDir()
}

func (l *AuditLogger) logPath(workspaceId, sessionId string, ts time.Time) string {
	if workspaceId == "" {
		workspaceId = "global"
	}
	day := ts.UTC().Format("2006-01-02")
	dir := filepath.Join(l.dataDir(), auditLogDir, workspaceId)
	if sessionId != "" {
		return filepath.Join(dir, fmt.Sprintf("sessions-%s.log", sessionId))
	}
	return filepath.Join(dir, day+".log")
}

func (l *AuditLogger) writeLocked(entry ToolCallLogEntry) error {
	if entry.Id == "" {
		entry.Id = uuid.New().String()
	}
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().UnixMilli()
	}
	if entry.WorkspaceId == "" {
		entry.WorkspaceId = "global"
	}
	ts := time.UnixMilli(entry.Timestamp)
	path := l.logPath(entry.WorkspaceId, entry.SessionId, ts)
	if path != l.currPathWs[entry.WorkspaceId+":"+entry.SessionId] {
		if prev := l.wrByWs[entry.WorkspaceId+":"+entry.SessionId]; prev != nil {
			prev.Flush()
		}
		if fh := l.fhByWs[entry.WorkspaceId+":"+entry.SessionId]; fh != nil {
			fh.Close()
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		fh, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		l.fhByWs[entry.WorkspaceId+":"+entry.SessionId] = fh
		l.wrByWs[entry.WorkspaceId+":"+entry.SessionId] = bufio.NewWriterSize(fh, 8*1024)
		l.currPathWs[entry.WorkspaceId+":"+entry.SessionId] = path
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if len(data) > maxLogLineBytes {
		data = data[:maxLogLineBytes]
	}
	wr := l.wrByWs[entry.WorkspaceId+":"+entry.SessionId]
	if _, err := wr.Write(data); err != nil {
		return err
	}
	if _, err := wr.WriteString("\n"); err != nil {
		return err
	}
	return wr.Flush()
}

func (l *AuditLogger) Record(ctx context.Context, entry ToolCallLogEntry) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.writeLocked(entry)
}

func (l *AuditLogger) Flush(workspaceId, sessionId string) {
	l.lock.Lock()
	defer l.lock.Unlock()
	key := workspaceId + ":" + sessionId
	if wr, ok := l.wrByWs[key]; ok {
		wr.Flush()
	}
}

func (l *AuditLogger) Close(workspaceId, sessionId string) {
	l.lock.Lock()
	defer l.lock.Unlock()
	key := workspaceId + ":" + sessionId
	if wr, ok := l.wrByWs[key]; ok {
		wr.Flush()
		delete(l.wrByWs, key)
	}
	if fh, ok := l.fhByWs[key]; ok {
		fh.Close()
		delete(l.fhByWs, key)
	}
	delete(l.currPathWs, key)
}

func (l *AuditLogger) CloseAll() {
	l.lock.Lock()
	defer l.lock.Unlock()
	for k, wr := range l.wrByWs {
		wr.Flush()
		_ = k
	}
	for k, fh := range l.fhByWs {
		fh.Close()
		_ = k
	}
	l.wrByWs = map[string]*bufio.Writer{}
	l.fhByWs = map[string]*os.File{}
	l.currPathWs = map[string]string{}
}

func (l *AuditLogger) Query(ctx context.Context, q ToolCallLogQuery) ([]ToolCallLogEntry, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	dir := filepath.Join(l.dataDir(), auditLogDir)
	if q.WorkspaceId != "" {
		dir = filepath.Join(dir, q.WorkspaceId)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	var results []ToolCallLogEntry
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		r := bufio.NewReader(f)
		for {
			line, err := r.ReadBytes('\n')
			if len(line) > 0 {
				var entry ToolCallLogEntry
				if jerr := json.Unmarshal(line, &entry); jerr == nil {
					if q.WorkspaceId != "" && entry.WorkspaceId != q.WorkspaceId {
						continue
					}
					if q.ToolName != "" && entry.ToolName != q.ToolName {
						continue
					}
					if q.SessionId != "" && entry.SessionId != q.SessionId {
						continue
					}
					if q.AgentId != "" && entry.AgentId != q.AgentId {
						continue
					}
					if q.Status != "" && entry.Status != q.Status {
						continue
					}
					if q.SinceMs > 0 && entry.Timestamp < q.SinceMs {
						continue
					}
					if q.UntilMs > 0 && entry.Timestamp > q.UntilMs {
						continue
					}
					results = append(results, entry)
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
		f.Close()
	}
	if len(results) > q.Limit {
		results = results[len(results)-q.Limit:]
	}
	return results, nil
}

func (l *AuditLogger) Tail(ctx context.Context, workspaceId, sessionId string, maxLines int) (string, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	if maxLines <= 0 {
		maxLines = 50
	}
	ts := time.Now()
	path := l.logPath(workspaceId, sessionId, ts)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxLogLineBytes)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n"), nil
}

func (l *AuditLogger) PurgeOld(ctx context.Context, workspaceId string, olderThanDays int) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	if olderThanDays <= 0 {
		olderThanDays = 14
	}
	dir := filepath.Join(l.dataDir(), auditLogDir)
	if workspaceId != "" {
		dir = filepath.Join(dir, workspaceId)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) && strings.HasSuffix(e.Name(), ".log") && !strings.HasPrefix(e.Name(), "sessions-") {
			full := filepath.Join(dir, e.Name())
			if err := os.Remove(full); err == nil {
				count++
			}
		}
	}
	return count, nil
}
