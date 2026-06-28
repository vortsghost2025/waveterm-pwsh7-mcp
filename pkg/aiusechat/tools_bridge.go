// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
)

const (
	BridgeInboxDefaultPath  = `S:\sean-machine-janitor\bridge\wave-inbox.jsonl`
	BridgeOutboxDefaultPath = `S:\sean-machine-janitor\bridge\wave-outbox.jsonl`
	BridgeMaxMessageBytes   = 20 * 1024
	BridgeMaxReadLines      = 200
)

type BridgeReplyToolInput struct {
	Message  string `json:"message"`
	WidgetId string `json:"widget_id,omitempty"`
	BlockId  string `json:"block_id,omitempty"`
	ThreadId string `json:"thread_id,omitempty"`
}

type BridgeReplyToolOutput struct {
	Success  bool   `json:"success"`
	Filename string `json:"filename"`
	Bytes    int    `json:"bytes"`
}

type BridgeMessage struct {
	Timestamp    string `json:"timestamp"`
	Type         string `json:"type"`
	Direction    string `json:"direction"`
	Source       string `json:"source"`
	Target       string `json:"target,omitempty"`
	Message      string `json:"message"`
	Content      string `json:"content"`
	WidgetId     string `json:"widget_id,omitempty"`
	BlockId      string `json:"block_id,omitempty"`
	ThreadId     string `json:"thread_id,omitempty"`
	TargetWidget string `json:"target_widget,omitempty"`
}

func bridgeOutboxPath() string {
	return BridgeOutboxDefaultPath
}

func bridgeInboxPath() string {
	return BridgeInboxDefaultPath
}

func validateBridgePath(path string, defaultPath string) error {
	if filepath.Clean(path) != filepath.Clean(defaultPath) {
		return fmt.Errorf("bridge path must be %s", defaultPath)
	}
	if runtime.GOOS == "windows" && !filepath.IsAbs(filepath.Clean(path)) {
		return fmt.Errorf("bridge path must be absolute, got %s", path)
	}
	return nil
}

func parseBridgeReplyInput(input any) (*BridgeReplyToolInput, error) {
	result := &BridgeReplyToolInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		return nil, fmt.Errorf("message is required")
	}
	if len([]byte(result.Message)) > BridgeMaxMessageBytes {
		return nil, fmt.Errorf("message is too large (max %d bytes)", BridgeMaxMessageBytes)
	}
	if utilfn.HasBinaryData([]byte(result.Message)) {
		return nil, fmt.Errorf("message appears to contain binary data")
	}
	return result, nil
}

func verifyBridgeReplyInput(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
	params, err := parseBridgeReplyInput(input)
	if err != nil {
		return err
	}
	path := bridgeOutboxPath()
	if err := validateBridgePath(path, BridgeOutboxDefaultPath); err != nil {
		return err
	}
	toolUseData.InputFileName = path
	_ = params
	return nil
}

func bridgeReplyCallback(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
	params, err := parseBridgeReplyInput(input)
	if err != nil {
		return nil, err
	}
	path := bridgeOutboxPath()
	if err := validateBridgePath(path, BridgeOutboxDefaultPath); err != nil {
		return nil, err
	}

	dirPath := filepath.Dir(path)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bridge directory: %w", err)
	}

	msg := BridgeMessage{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Type:         "reply",
		Direction:    "assistant_reply",
		Source:       "wave-ai-assistant",
		Target:       "janitor-wave-ai",
		Message:      params.Message,
		Content:      params.Message,
		WidgetId:     params.WidgetId,
		BlockId:      params.BlockId,
		ThreadId:     params.ThreadId,
		TargetWidget: params.WidgetId,
	}
	line, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode bridge message: %w", err)
	}
	line = append(line, '\n')

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open bridge outbox: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(line); err != nil {
		return nil, fmt.Errorf("failed to write bridge outbox: %w", err)
	}

	toolUseData.InputFileName = path
	return &BridgeReplyToolOutput{
		Success:  true,
		Filename: path,
		Bytes:    len(line),
	}, nil
}

func bridgeSelfIntroWrapperCallback(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
	params := input.(*BridgeReplyToolInput)
	message := "Friendly greeting message from assistant to Wave AI via terminal bridge channel"
	params.Message = message
	params.WidgetId = "test_widget"
	params.BlockId = "test_block"
	hex, _ := utilfn.RandomHexString(8)
	params.ThreadId = "hello_wave_ai_" + hex
	dir := filepath.Dir(bridgeOutboxPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	msg := BridgeMessage{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Type:         "reply",
		Direction:    "assistant_reply",
		Source:       "wave-ai-assistant",
		Target:       "janitor-wave-ai",
		Message:      params.Message,
		Content:      params.Message,
		WidgetId:     params.WidgetId,
		BlockId:      params.BlockId,
		ThreadId:     params.ThreadId,
		TargetWidget: params.WidgetId,
	}
	line, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	line = append(line, '\n')
	if err := os.WriteFile(bridgeOutboxPath(), line, 0644); err != nil {
		return nil, err
	}
	toolUseData.InputFileName = bridgeOutboxPath()
	return &BridgeReplyToolOutput{
		Success:  true,
		Filename: bridgeOutboxPath(),
		Bytes:    len(line),
	}, nil
}

func GetAISelfIntroToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:         "ai_self_intro",
		DisplayName:  "Send Self-Introduction to Wave AI",
		Description:  "Send a friendly greeting from the assistant to Wave AI via the terminal bridge channel (S:\\sean-machine-janitor\\bridge\\wave-outbox.jsonl). Requires write access to the local bridge directory.",
		ToolLogName:  "bridge:selfintro",
		InputSchema: map[string]any{
			"type": "object",
			"properties":  map[string]any{},
			"required":    []string{},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			return "sending self-introduction to Wave AI via bridge channel"
		},
		ToolAnyCallback: bridgeSelfIntroWrapperCallback,
		ToolApproval: func(input any) string { return uctypes.ApprovalAutoApproved },
	}
}

func GetBridgeWriteReplyToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "bridge_write_reply",
		DisplayName: "Write Terminal Bridge Reply",
		Description: "Append a Wave AI assistant reply for a terminal agent to the local mailbox at S:\\sean-machine-janitor\\bridge\\wave-outbox.jsonl. This fixed-path reply channel is auto-approved and only writes validated text to the bridge outbox. Use this as the safe assistant-to-terminal reply channel fallback when term_send_input is unavailable. Never use run_command with echo as the assistant reply channel.",
		ToolLogName: "bridge:writereply",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Reply text to append for the terminal agent.",
				},
				"widget_id": map[string]any{
					"type":        "string",
					"description": "Optional 8-character terminal widget ID this reply targets.",
				},
				"block_id": map[string]any{
					"type":        "string",
					"description": "Optional full terminal block ID this reply targets.",
				},
				"thread_id": map[string]any{
					"type":        "string",
					"description": "Optional conversation or task ID to correlate the reply.",
				},
			},
			"required":             []string{"message", "widget_id", "block_id", "thread_id"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseBridgeReplyInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			preview := utilfn.TruncateString(parsed.Message, 160)
			return fmt.Sprintf("writing terminal bridge reply to %s: %s", bridgeOutboxPath(), preview)
		},
		ToolAnyCallback: bridgeReplyCallback,
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: verifyBridgeReplyInput,
	}
}

type BridgeReadInboxToolInput struct {
	Count int `json:"count,omitempty"`
}

type BridgeReadInboxToolOutput struct {
	Filename string   `json:"filename"`
	Count    int      `json:"count"`
	Lines    []string `json:"lines"`
}

func parseBridgeReadInboxInput(input any) (*BridgeReadInboxToolInput, error) {
	result := &BridgeReadInboxToolInput{Count: 50}
	if input == nil {
		return result, nil
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.Count < 0 {
		return nil, fmt.Errorf("count must be positive")
	}
	if result.Count > BridgeMaxReadLines {
		result.Count = BridgeMaxReadLines
	}
	return result, nil
}

func bridgeReadInboxCallback(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
	params, err := parseBridgeReadInboxInput(input)
	if err != nil {
		return nil, err
	}
	path := bridgeInboxPath()
	if err := validateBridgePath(path, BridgeInboxDefaultPath); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &BridgeReadInboxToolOutput{Filename: path, Count: 0, Lines: []string{}}, nil
		}
		return nil, fmt.Errorf("failed to read bridge inbox: %w", err)
	}
	if utilfn.HasBinaryData(data) {
		return nil, fmt.Errorf("bridge inbox appears to contain binary data")
	}

	text := string(data)
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start := len(lines) - params.Count
	if start < 0 {
		start = 0
	}
	selected := append([]string(nil), lines[start:]...)
	toolUseData.InputFileName = path
	return &BridgeReadInboxToolOutput{
		Filename: path,
		Count:    len(selected),
		Lines:    selected,
	}, nil
}

func GetBridgeReadInboxToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "bridge_read_inbox",
		DisplayName: "Read Terminal Bridge Inbox",
		Description: "Read recent terminal-agent messages from the local mailbox at S:\\sean-machine-janitor\\bridge\\wave-inbox.jsonl. Use this as the safe terminal-to-Wave mailbox fallback when wsh ai -s -m is unavailable.",
		ToolLogName: "bridge:readinbox",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Number of most recent JSONL lines to return (default: 50, max: 200).",
				},
			},
			"required":             []string{"count"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			return fmt.Sprintf("reading terminal bridge inbox from %s", bridgeInboxPath())
		},
		ToolAnyCallback: bridgeReadInboxCallback,
	}
}
