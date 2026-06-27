package aiusechat

import (
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
)

func TestBridgeWriteReplyRequiresMessage(t *testing.T) {
	_, err := parseBridgeReplyInput(&BridgeReplyToolInput{Message: "   "})
	if err == nil {
		t.Fatalf("expected error for empty message")
	}
}

func TestBridgeReplyRejectsOversizedMessage(t *testing.T) {
	_, err := parseBridgeReplyInput(&BridgeReplyToolInput{Message: strings.Repeat("a", BridgeMaxMessageBytes+1)})
	if err == nil {
		t.Fatalf("expected error for oversized message")
	}
}

func TestBridgeReadInboxParsesCount(t *testing.T) {
	params, err := parseBridgeReadInboxInput(&BridgeReadInboxToolInput{Count: BridgeMaxReadLines + 1})
	if err != nil {
		t.Fatalf("parseBridgeReadInboxInput returned error: %v", err)
	}
	if params.Count != BridgeMaxReadLines {
		t.Fatalf("expected count to be capped at %d, got %d", BridgeMaxReadLines, params.Count)
	}
}

func TestBridgeReadInboxRejectsNegativeCount(t *testing.T) {
	_, err := parseBridgeReadInboxInput(&BridgeReadInboxToolInput{Count: -1})
	if err == nil {
		t.Fatalf("expected error for negative count")
	}
}

func TestBridgePathValidationRestrictsMailboxLocation(t *testing.T) {
	if err := validateBridgePath(BridgeOutboxDefaultPath, BridgeOutboxDefaultPath); err != nil {
		t.Fatalf("expected default outbox path to be valid: %v", err)
	}
	if err := validateBridgePath(BridgeInboxDefaultPath, BridgeInboxDefaultPath); err != nil {
		t.Fatalf("expected default inbox path to be valid: %v", err)
	}
	if err := validateBridgePath(`C:\temp\wave-outbox.jsonl`, BridgeOutboxDefaultPath); err == nil {
		t.Fatalf("expected non-bridge path to be rejected")
	}
}

func TestBridgePathsAreDefaultMailboxes(t *testing.T) {
	if bridgeOutboxPath() != BridgeOutboxDefaultPath {
		t.Fatalf("unexpected outbox path %q", bridgeOutboxPath())
	}
	if bridgeInboxPath() != BridgeInboxDefaultPath {
		t.Fatalf("unexpected inbox path %q", bridgeInboxPath())
	}
}

func TestBridgeWriteReplyIsAutoApproved(t *testing.T) {
	def := GetBridgeWriteReplyToolDefinition()
	if def.ToolApproval == nil {
		t.Fatalf("bridge_write_reply should define approval behavior")
	}
	approval := def.ToolApproval(&BridgeReplyToolInput{Message: "hello"})
	if approval != uctypes.ApprovalAutoApproved {
		t.Fatalf("expected bridge_write_reply to be auto-approved, got %q", approval)
	}
}

func TestBridgeReplyMessageIncludesVisibleFields(t *testing.T) {
	msg := BridgeMessage{
		Type:      "reply",
		Direction: "assistant_reply",
		Source:    "wave-ai-assistant",
		Target:    "janitor-wave-ai",
		Message:   "hello",
		Content:   "hello",
	}
	if msg.Source == "" || msg.Content != msg.Message || msg.Type != "reply" {
		t.Fatalf("bridge reply should include visible source/content fields: %+v", msg)
	}
}
