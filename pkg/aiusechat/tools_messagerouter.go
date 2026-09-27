// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// MessageRoutingToolInput is the input for determining message routing
type MessageRoutingToolInput struct {
	// RecipientType: "user" or "terminal_agent"
	RecipientType string `json:"recipient_type"`
	// Message: the message content
	Message string `json:"message"`
	// TargetBlockId: optional, the terminal block ID if recipient is terminal_agent
	TargetBlockId string `json:"target_block_id,omitempty"`
	// TargetWidgetId: optional, the widget ID if recipient is terminal_agent
	TargetWidgetId string `json:"target_widget_id,omitempty"`
}

// MessageRoutingToolOutput is the output from the routing tool
type MessageRoutingToolOutput struct {
	RoutingDecision    string `json:"routing_decision"`
	ChannelUsed        string `json:"channel_used"`
	DualOutputPrevented bool   `json:"dual_output_prevented"`
	Success            bool   `json:"success"`
	Error              string `json:"error,omitempty"`
}

// parseMessageRoutingInput parses the input for the message routing tool
func parseMessageRoutingInput(input any) (*MessageRoutingToolInput, error) {
	result := &MessageRoutingToolInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	result.RecipientType = strings.TrimSpace(result.RecipientType)
	if result.RecipientType == "" {
		return nil, fmt.Errorf("recipient_type is required")
	}
	if result.RecipientType != "user" && result.RecipientType != "terminal_agent" {
		return nil, fmt.Errorf("recipient_type must be 'user' or 'terminal_agent'")
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		return nil, fmt.Errorf("message is required")
	}
	if result.RecipientType == "terminal_agent" && result.TargetBlockId == "" {
		return nil, fmt.Errorf("target_block_id is required when recipient is terminal_agent")
	}
	return result, nil
}

// verifyMessageRoutingInput verifies the input for the message routing tool
func verifyMessageRoutingInput(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
	_, err := parseMessageRoutingInput(input)
	if err != nil {
		return err
	}
	return nil
}

// messageRoutingCallback is the callback for the message routing tool
func messageRoutingCallback(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
	params, err := parseMessageRoutingInput(input)
	if err != nil {
		return nil, err
	}

	output := &MessageRoutingToolOutput{
		Success: true,
	}

	ctx := context.Background()

	if params.RecipientType == "user" {
		// Route to user chat panel - NO bridge output
		output.RoutingDecision = "route_to_user_chat"
		output.ChannelUsed = "chat_panel_only"
		output.DualOutputPrevented = true
		// No action needed - the normal chat response will be used
		return output, nil
	}

	// Recipient is terminal_agent - route to bridge ONLY
	if params.RecipientType == "terminal_agent" {
		// Verify the target block exists and is a terminal
		block, err := wstore.DBGet[*waveobj.Block](ctx, params.TargetBlockId)
		if err != nil {
			output.Success = false
			output.Error = fmt.Sprintf("target block not found: %v", err)
			return output, nil
		}

		viewType, ok := block.Meta["view"].(string)
		if !ok || viewType != "term" {
			output.Success = false
			output.Error = "target block is not a terminal"
			return output, nil
		}

		// Route to bridge outbox ONLY - NO chat panel output
		output.RoutingDecision = "route_to_terminal_agent_via_bridge"
		output.ChannelUsed = "bridge_outbox_only"
		output.DualOutputPrevented = true

		// Write to bridge outbox
		msg := BridgeMessage{
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Type:         "message",
			Direction:    "assistant_to_agent",
			Source:       "wave-ai-assistant",
			Target:       "terminal-agent",
			Message:      params.Message,
			Content:      params.Message,
			WidgetId:     params.TargetWidgetId,
			BlockId:      params.TargetBlockId,
			TargetWidget: params.TargetWidgetId,
		}

		line, err := json.Marshal(msg)
		if err != nil {
			output.Success = false
			output.Error = fmt.Sprintf("failed to marshal bridge message: %v", err)
			return output, nil
		}
		line = append(line, '\n')

		// Write to inbox (agents read from inbox)
		inboxPath := bridgeInboxPath()
		err = appendToFile(inboxPath, line)
		if err != nil {
			output.Success = false
			output.Error = fmt.Sprintf("failed to write to bridge inbox: %v", err)
			return output, nil
		}

		output.Success = true
		return output, nil
	}

	output.Success = false
	output.Error = "invalid recipient type"
	return output, nil
}

// appendToFile appends bytes to a file
func appendToFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}

// GetMessageRoutingToolDefinition returns the tool definition for message routing
func GetMessageRoutingToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "message_router",
		DisplayName: "Deterministic Message Router",
		Description: "DETERMINISTIC message router. Use this to route messages to either the user (chat panel) or a terminal agent (bridge). NEVER dual-output. This tool enforces single-channel routing.",
		ToolLogName: "message_router",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recipient_type": map[string]any{
					"type":        "string",
					"description": "Who is the recipient? 'user' for chat panel, 'terminal_agent' for Kilo/OpenCode CLI",
					"enum":        []string{"user", "terminal_agent"},
				},
				"message": map[string]any{
					"type":        "string",
					"description": "The message content to deliver",
				},
				"target_block_id": map[string]any{
					"type":        "string",
					"description": "Terminal block ID (required when recipient_type is 'terminal_agent')",
				},
				"target_widget_id": map[string]any{
					"type":        "string",
					"description": "Widget ID (optional, for terminal agent targeting)",
				},
			},
			"required":             []string{"recipient_type", "message"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			params, _ := input.(*MessageRoutingToolInput)
			if params != nil {
				return fmt.Sprintf("routing message to %s via %s", params.RecipientType, params.TargetBlockId)
			}
			return "routing message"
		},
		ToolAnyCallback: messageRoutingCallback,
		ToolApproval:    func(input any) string { return uctypes.ApprovalAutoApproved },
	}
}

// GetDeterministicRoutingToolDefinition returns a simpler tool that just validates routing
func GetDeterministicRoutingToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "validate_message_routing",
		DisplayName: "Validate Message Routing",
		Description: "Validate that you are routing a message correctly. Use BEFORE sending any message to ensure you're not dual-outputting. Returns error if routing is ambiguous.",
		ToolLogName: "validate_routing",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"intent": map[string]any{
					"type": "string",
					"description": "Your intent: 'reply_to_user', 'reply_to_agent', 'delegate_to_agent', 'route_agent_completion'",
					"enum": []string{"reply_to_user", "reply_to_agent", "delegate_to_agent", "route_agent_completion"},
				},
				"planned_channel": map[string]any{
					"type": "string",
					"description": "Which channel you plan to use: 'chat_panel', 'bridge_outbox', 'bridge_inbox', 'waveai_add_context'",
					"enum": []string{"chat_panel", "bridge_outbox", "bridge_inbox", "waveai_add_context"},
				},
			},
			"required":             []string{"intent", "planned_channel"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			return "validating message routing"
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			intent, _ := input.(map[string]any)["intent"].(string)
			channel, _ := input.(map[string]any)["planned_channel"].(string)

			valid := false
			switch intent {
			case "reply_to_user":
				valid = channel == "chat_panel"
			case "reply_to_agent":
				valid = channel == "bridge_outbox" || channel == "term_send_input"
			case "delegate_to_agent":
				valid = channel == "bridge_inbox" || channel == "term_spawn_agent"
			case "route_agent_completion":
				valid = channel == "waveai_add_context"
			}

			if !valid {
				return map[string]any{
					"valid": false,
					"error": fmt.Sprintf("invalid routing: intent=%q requires channel=%q but you planned %q", intent, getRequiredChannel(intent), channel),
					"fix":   getRequiredChannel(intent),
				}, nil
			}

			return map[string]any{
				"valid":   true,
				"message": "routing is correct - proceed with the planned channel",
			}, nil
		},
		ToolApproval: func(input any) string { return uctypes.ApprovalAutoApproved },
	}
}

func getRequiredChannel(intent string) string {
	switch intent {
	case "reply_to_user":
		return "chat_panel"
	case "reply_to_agent":
		return "bridge_outbox or term_send_input"
	case "delegate_to_agent":
		return "bridge_inbox or term_spawn_agent"
	case "route_agent_completion":
		return "waveai_add_context"
	default:
		return "unknown"
	}
}