// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"context"
	"fmt"
	"strings"

	"github.com/wavetermdev/waveterm/pkg/aistore"
	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
)

type NotePutInput struct {
	WorkspaceId string              `json:"workspaceid,omitempty"`
	Scope       string              `json:"scope,omitempty"`
	Key         string              `json:"key,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	TtlSec      int                 `json:"ttlsec,omitempty"`
	Body        string              `json:"body"`
	Operations  []NotePutOperation  `json:"operations,omitempty"`
}

type NotePutOperation struct {
	Scope  string   `json:"scope,omitempty"`
	Key    string   `json:"key,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	TtlSec int      `json:"ttlsec,omitempty"`
	Body   string   `json:"body"`
}

type NotePutOutput struct {
	Id string `json:"id"`
}

type NotePutBatchOutput struct {
	Results []NotePutBatchResult `json:"results"`
}

type NotePutBatchResult struct {
	Id  string `json:"id,omitempty"`
	Err string `json:"err,omitempty"`
}

type NoteGetInput struct {
	WorkspaceId string   `json:"workspaceid,omitempty"`
	Id          string   `json:"id,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Key         string   `json:"key,omitempty"`
	Operations  []NoteGetOperation `json:"operations,omitempty"`
}

type NoteGetOperation struct {
	Id    string `json:"id,omitempty"`
	Scope string `json:"scope,omitempty"`
	Key   string `json:"key,omitempty"`
}

type NoteGetOutput struct {
	Record *aistore.MemoryRecord `json:"record,omitempty"`
}

type NoteGetBatchOutput struct {
	Results []NoteGetBatchResult `json:"results"`
}

type NoteGetBatchResult struct {
	Record *aistore.MemoryRecord `json:"record,omitempty"`
	Err    string                `json:"err,omitempty"`
}

type NoteListInput struct {
	WorkspaceId string `json:"workspaceid,omitempty"`
	Scope       string `json:"scope,omitempty"`
	TagGlob     string `json:"tagglob,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type NoteListOutput struct {
	Records    []*aistore.MemoryRecord `json:"records"`
	NextCursor string                  `json:"nextcursor,omitempty"`
}

type NoteDeleteInput struct {
	WorkspaceId string `json:"workspaceid,omitempty"`
	Id          string `json:"id"`
}

type NoteDeleteOutput struct {
	Deleted bool `json:"deleted"`
}

type NoteDeleteManyInput struct {
	WorkspaceId string   `json:"workspaceid,omitempty"`
	Ids         []string `json:"ids"`
}

type NoteDeleteManyOutput struct {
	Deleted int `json:"deleted"`
}

type NoteDeleteByScopeInput struct {
	WorkspaceId string `json:"workspaceid,omitempty"`
	Scope       string `json:"scope"`
}

type NoteDeleteByScopeOutput struct {
	Deleted int `json:"deleted"`
}

type NoteSearchInput struct {
	WorkspaceId string `json:"workspaceid,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Query       string `json:"query"`
	Limit       int    `json:"limit,omitempty"`
}

type NoteSearchOutput struct {
	Matches []aistore.MemorySearchMatch `json:"matches"`
}

func GetNotePutToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "note_put",
		DisplayName: "Save a Note",
		Description: "Save a piece of information (note) to the AI agent's persistent memory store. Notes can be tagged, scoped, and given an optional TTL (time-to-live). The note is retrievable by ID or key. Use this to remember information across conversations, user preferences, and project context.",
		ToolLogName: "gen:note_put",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"body": map[string]any{
					"type":        "string",
					"description": "The main content of the note to save",
				},
				"scope": map[string]any{
					"type":        "string",
					"description": "Optional scope for organization (e.g. 'project', 'user', 'preference')",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Optional unique key for this note, for easy retrieval without knowing the ID",
				},
				"tags": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional tags for search and categorization",
				},
				"ttlsec": map[string]any{
					"type":        "integer",
					"description": "Optional time-to-live in seconds (note auto-deletes after this duration)",
				},
				"operations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"scope": map[string]any{"type": "string", "description": "Scope for organization"},
							"key":   map[string]any{"type": "string", "description": "Unique key for retrieval"},
							"tags":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags for categorization"},
							"ttlsec": map[string]any{"type": "integer", "description": "TTL in seconds"},
							"body": map[string]any{"type": "string", "description": "Note content"},
						},
						"required":             []string{"body"},
						"additionalProperties": false,
					},
					"description": "Optional batch mode: array of note operations. When provided, single-item fields (body, scope, key, tags, ttlsec) are ignored. Each operation returns a result in the same order.",
				},
			},
			"required":             []string{"body", "scope", "key", "tags", "ttlsec", "operations"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseNotePutInput(input)
			if err != nil {
				return nil, err
			}
			if len(parsed.Operations) > 0 {
				return handleNotePutBatch(parsed)
			}
			store := aistore.GetMemoryStore()
			opts := aistore.MemoryOpts{
				WorkspaceId: parsed.WorkspaceId,
				Scope:       parsed.Scope,
				Key:         parsed.Key,
				Tags:        parsed.Tags,
				TtlSec:      parsed.TtlSec,
			}
			id, err := store.Put(context.Background(), opts, parsed.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to save note: %w", err)
			}
			return &NotePutOutput{Id: id}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseNotePutInput(input)
			return err
		},
	}
}

func GetNoteGetToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "note_get",
		DisplayName: "Get a Note",
		Description: "Retrieve a saved note by its ID or key. Returns the full note including body, tags, and creation time.",
		ToolLogName: "gen:note_get",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The ID of the note to retrieve (either id or key is required)",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "The unique key of the note to retrieve (either id or key is required)",
				},
				"scope": map[string]any{
					"type":        "string",
					"description": "Optional scope, required if using key instead of id",
				},
				"operations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":    map[string]any{"type": "string", "description": "ID of the note to retrieve"},
							"scope": map[string]any{"type": "string", "description": "Scope for lookup"},
							"key":   map[string]any{"type": "string", "description": "Key of the note to retrieve"},
						},
						"required":             []string{},
						"additionalProperties": false,
					},
					"description": "Optional batch mode: array of get operations. When provided, single-item fields (id, key, scope) are ignored. Each operation returns a result in the same order.",
				},
			},
			"required":             []string{"id", "key", "scope", "operations"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseNoteGetInput(input)
			if err != nil {
				return nil, err
			}
			if len(parsed.Operations) > 0 {
				return handleNoteGetBatch(parsed)
			}
			store := aistore.GetMemoryStore()
			var rec *aistore.MemoryRecord
			if parsed.Key != "" {
				rec, err = store.GetByKey(context.Background(), parsed.WorkspaceId, parsed.Scope, parsed.Key)
			} else {
				rec, err = store.Get(context.Background(), parsed.WorkspaceId, parsed.Id)
			}
			if err != nil {
				return nil, err
			}
			return &NoteGetOutput{Record: rec}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseNoteGetInput(input)
			return err
		},
	}
}

func GetNoteListToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "note_list",
		DisplayName: "List Notes",
		Description: "List saved notes with optional filtering by scope and tag glob pattern. Returns notes ordered by most recently updated first. Use the cursor from the response to paginate.",
		ToolLogName: "gen:note_list",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scope": map[string]any{
					"type":        "string",
					"description": "Optional scope filter",
				},
				"tagglob": map[string]any{
					"type":        "string",
					"description": "Optional glob pattern to filter by tags (e.g. '*important*')",
				},
			"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of notes to return (default 50, max 200)",
				},
			},
			"required":             []string{"scope", "tagglob", "limit"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseNoteListInput(input)
			if err != nil {
				return nil, err
			}
			store := aistore.GetMemoryStore()
			opts := aistore.MemoryListOpts{
				WorkspaceId: parsed.WorkspaceId,
				Scope:       parsed.Scope,
				TagGlob:     parsed.TagGlob,
				Limit:       parsed.Limit,
			}
			records, cursor, err := store.List(context.Background(), opts)
			if err != nil {
				return nil, err
			}
			return &NoteListOutput{Records: records, NextCursor: cursor}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseNoteListInput(input)
			return err
		},
	}
}

func GetNoteDeleteToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "note_delete",
		DisplayName: "Delete a Note",
		Description: "Delete a saved note by its ID. Once deleted, the note cannot be recovered.",
		ToolLogName: "gen:note_delete",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The ID of the note to delete",
				},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseNoteDeleteInput(input)
			if err != nil {
				return nil, err
			}
			store := aistore.GetMemoryStore()
			deleted, err := store.Delete(context.Background(), parsed.WorkspaceId, parsed.Id)
			if err != nil {
				return nil, err
			}
			return &NoteDeleteOutput{Deleted: deleted}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseNoteDeleteInput(input)
			return err
		},
	}
}

func GetNoteSearchToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "note_search",
		DisplayName: "Search Notes",
		Description: "Search saved notes by text content (substring match against body, key, and tags). Returns matching notes with snippets.",
		ToolLogName: "gen:note_search",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Text to search for in note bodies, keys, and tags",
				},
				"scope": map[string]any{
					"type":        "string",
					"description": "Optional scope to limit search within",
				},
			"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of matches to return (default 20, max 100)",
				},
			},
			"required":             []string{"query", "scope", "limit"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseNoteSearchInput(input)
			if err != nil {
				return nil, err
			}
			store := aistore.GetMemoryStore()
			matches, err := store.Search(context.Background(), parsed.WorkspaceId, parsed.Scope, parsed.Query, parsed.Limit)
			if err != nil {
				return nil, err
			}
			return &NoteSearchOutput{Matches: matches}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseNoteSearchInput(input)
			return err
		},
	}
}

func handleNotePutBatch(parsed *NotePutInput) (*NotePutBatchOutput, error) {
	store := aistore.GetMemoryStore()
	results := make([]NotePutBatchResult, 0, len(parsed.Operations))
	for _, op := range parsed.Operations {
		body := strings.TrimSpace(op.Body)
		if body == "" {
			results = append(results, NotePutBatchResult{Err: "body is required"})
			continue
		}
		opts := aistore.MemoryOpts{
			WorkspaceId: parsed.WorkspaceId,
			Scope:       op.Scope,
			Key:         op.Key,
			Tags:        op.Tags,
			TtlSec:      op.TtlSec,
		}
		id, err := store.Put(context.Background(), opts, body)
		if err != nil {
			results = append(results, NotePutBatchResult{Err: fmt.Sprintf("failed to save note: %v", err)})
		} else {
			results = append(results, NotePutBatchResult{Id: id})
		}
	}
	return &NotePutBatchOutput{Results: results}, nil
}

func parseNotePutInput(input any) (*NotePutInput, error) {
	result := &NotePutInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if len(result.Operations) == 0 {
		result.Body = strings.TrimSpace(result.Body)
		if result.Body == "" {
			return nil, fmt.Errorf("body is required (or provide operations array for batch mode)")
		}
	}
	return result, nil
}

func parseNoteGetInput(input any) (*NoteGetInput, error) {
	result := &NoteGetInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if len(result.Operations) == 0 {
		if result.Id == "" && result.Key == "" {
			return nil, fmt.Errorf("either id or key is required (or provide operations array for batch mode)")
		}
	}
	return result, nil
}

func handleNoteGetBatch(parsed *NoteGetInput) (*NoteGetBatchOutput, error) {
	store := aistore.GetMemoryStore()
	results := make([]NoteGetBatchResult, 0, len(parsed.Operations))
	for _, op := range parsed.Operations {
		if op.Id == "" && op.Key == "" {
			results = append(results, NoteGetBatchResult{Err: "either id or key is required"})
			continue
		}
		var rec *aistore.MemoryRecord
		var err error
		if op.Key != "" {
			rec, err = store.GetByKey(context.Background(), parsed.WorkspaceId, op.Scope, op.Key)
		} else {
			rec, err = store.Get(context.Background(), parsed.WorkspaceId, op.Id)
		}
		if err != nil {
			results = append(results, NoteGetBatchResult{Err: fmt.Sprintf("failed to get note: %v", err)})
		} else {
			results = append(results, NoteGetBatchResult{Record: rec})
		}
	}
	return &NoteGetBatchOutput{Results: results}, nil
}

func parseNoteListInput(input any) (*NoteListInput, error) {
	result := &NoteListInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.Limit <= 0 || result.Limit > 200 {
		result.Limit = 50
	}
	return result, nil
}

func parseNoteDeleteInput(input any) (*NoteDeleteInput, error) {
	result := &NoteDeleteInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.Id == "" {
		return nil, fmt.Errorf("id is required")
	}
	return result, nil
}

func parseNoteSearchInput(input any) (*NoteSearchInput, error) {
	result := &NoteSearchInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	result.Query = strings.TrimSpace(result.Query)
	if result.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if result.Limit <= 0 || result.Limit > 100 {
		result.Limit = 20
	}
	return result, nil
}

func GetNoteDeleteManyToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "note_delete_many",
		DisplayName: "Delete Multiple Notes",
		Description: "Delete multiple notes by id in one call. Pass an array of ids. Skips any ids that don't exist. Returns the actual count deleted. Destructive - requires user approval.",
		ToolLogName: "gen:note_delete_many",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ids": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Array of note IDs to delete (max 500)",
				},
				"workspaceid": map[string]any{
					"type":        "string",
					"description": "Optional workspace ID",
				},
			},
			"required":             []string{"ids", "workspaceid"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseNoteDeleteManyInput(input)
			if err != nil {
				return nil, err
			}
			store := aistore.GetMemoryStore()
			deleted, err := store.DeleteMany(context.Background(), parsed.WorkspaceId, parsed.Ids)
			if err != nil {
				return nil, err
			}
			return &NoteDeleteManyOutput{Deleted: deleted}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalNeedsApproval
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseNoteDeleteManyInput(input)
			return err
		},
	}
}

func GetNoteDeleteByScopeToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "note_delete_by_scope",
		DisplayName: "Delete All Notes in Scope",
		Description: "Delete every note within a scope in one call. Useful for cleanup at the start/end of a session. Destructive - the entire scope is wiped. Requires user approval.",
		ToolLogName: "gen:note_delete_by_scope",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scope": map[string]any{
					"type":        "string",
					"description": "Scope to wipe (e.g. 'temp', 'session-2026-06-28')",
				},
				"workspaceid": map[string]any{
					"type":        "string",
					"description": "Optional workspace ID",
				},
			},
			"required":             []string{"scope", "workspaceid"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseNoteDeleteByScopeInput(input)
			if err != nil {
				return nil, err
			}
			store := aistore.GetMemoryStore()
			deleted, err := store.DeleteByScope(context.Background(), parsed.WorkspaceId, parsed.Scope)
			if err != nil {
				return nil, err
			}
			return &NoteDeleteByScopeOutput{Deleted: deleted}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalNeedsApproval
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseNoteDeleteByScopeInput(input)
			return err
		},
	}
}

func parseNoteDeleteManyInput(input any) (*NoteDeleteManyInput, error) {
	result := &NoteDeleteManyInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if len(result.Ids) == 0 {
		return nil, fmt.Errorf("ids array is required and must not be empty")
	}
	if len(result.Ids) > 500 {
		return nil, fmt.Errorf("too many ids (max 500)")
	}
	return result, nil
}

func parseNoteDeleteByScopeInput(input any) (*NoteDeleteByScopeInput, error) {
	result := &NoteDeleteByScopeInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	result.Scope = strings.TrimSpace(result.Scope)
	if result.Scope == "" {
		return nil, fmt.Errorf("scope is required")
	}
	return result, nil
}
