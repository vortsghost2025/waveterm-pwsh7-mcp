// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aistore

type MemoryOpts struct {
	WorkspaceId string   `json:"workspaceid"`
	Scope       string   `json:"scope"`
	Key         string   `json:"key"`
	Tags        []string `json:"tags,omitempty"`
	TtlSec      int      `json:"ttlsec,omitempty"`
}

type MemoryRecord struct {
	Id          string   `json:"id"`
	WorkspaceId string   `json:"workspaceid"`
	Scope       string   `json:"scope"`
	Key         string   `json:"key"`
	Tags        []string `json:"tags,omitempty"`
	Body        string   `json:"body"`
	CreatedAt   int64    `json:"createdat"`
	UpdatedAt   int64    `json:"updatedat"`
	TtlSec      int      `json:"ttlsec,omitempty"`
}

type MemoryListOpts struct {
	WorkspaceId string `json:"workspaceid"`
	Scope       string `json:"scope,omitempty"`
	TagGlob     string `json:"tagglob,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
}

type MemorySearchMatch struct {
	Id      string `json:"id"`
	Scope   string `json:"scope"`
	Key     string `json:"key"`
	Snippet string `json:"snippet"`
}

type ToolCallLogEntry struct {
	Timestamp    int64           `json:"timestamp"`
	Id           string          `json:"id"`
	WorkspaceId  string          `json:"workspaceid,omitempty"`
	SessionId    string          `json:"sessionid,omitempty"`
	AgentId      string          `json:"agentid,omitempty"`
	ToolName     string          `json:"toolname"`
	Input        any             `json:"input,omitempty"`
	Output       any             `json:"output,omitempty"`
	Err          string          `json:"err,omitempty"`
	Status       string          `json:"status"` // ok|error|approval-required|approval-denied|started
	DurationMs   int64           `json:"durationms,omitempty"`
	Approved     bool            `json:"approved,omitempty"`
	BlocksUsed   []string        `json:"blocksused,omitempty"`
	Preview      string          `json:"preview,omitempty"`
}

type ToolCallLogQuery struct {
	WorkspaceId string `json:"workspaceid,omitempty"`
	ToolName    string `json:"toolname,omitempty"`
	SessionId   string `json:"sessionid,omitempty"`
	AgentId     string `json:"agentid,omitempty"`
	SinceMs     int64  `json:"sincems,omitempty"`
	UntilMs     int64  `json:"untilms,omitempty"`
	Status      string `json:"status,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}
