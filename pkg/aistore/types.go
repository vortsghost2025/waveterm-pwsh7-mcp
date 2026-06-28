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
