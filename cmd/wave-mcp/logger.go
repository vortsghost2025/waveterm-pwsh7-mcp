package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type AuditLogger struct {
	file *os.File
	logger *log.Logger
}

func NewAuditLogger() *AuditLogger {
	logDir := filepath.Join(os.TempDir(), "wave-mcp-logs")
	os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, fmt.Sprintf("wave-mcp-%s.log", time.Now().Format("2006-01-02")))
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("warning: cannot open audit log %s: %v", logPath, err)
		return &AuditLogger{logger: log.Default()}
	}
	return &AuditLogger{
		file:   f,
		logger: log.New(f, "", log.LstdFlags),
	}
}

func (al *AuditLogger) Log(toolName string, args interface{}, result string, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	al.logger.Printf("TOOL=%s ARGS=%v RESULT=%q ERROR=%s", toolName, args, truncate(result, 500), errStr)
}

func (al *AuditLogger) Close() {
	if al.file != nil {
		al.file.Close()
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
