// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package chatstore

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
)

const (
	// MaxNativeMessages is the maximum number of messages sent to the API.
	// When a conversation exceeds this, older messages are replaced with a
	// compact summary to stay within the model's context window.
	// A typical tool-using conversation produces ~3 native messages per turn
	// (user, assistant+tool_call, tool_result), so 50 ≈ 16 conversation turns.
	MaxNativeMessages = 50

	// compactionSummaryLen is the max characters per message in the compaction summary.
	compactionSummaryLen = 150
)

type ChatStore struct {
	lock  sync.Mutex
	chats map[string]*uctypes.AIChat
}

var DefaultChatStore = &ChatStore{
	chats: make(map[string]*uctypes.AIChat),
}

func (cs *ChatStore) Get(chatId string) *uctypes.AIChat {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	chat := cs.chats[chatId]
	if chat == nil {
		return nil
	}

	// Copy the chat to prevent concurrent access issues
	copyChat := &uctypes.AIChat{
		ChatId:         chat.ChatId,
		APIType:        chat.APIType,
		Model:          chat.Model,
		APIVersion:     chat.APIVersion,
		NativeMessages: make([]uctypes.GenAIMessage, len(chat.NativeMessages)),
	}
	// Apply compaction to the returned copy so we never send an oversized
	// conversation to the API. The original chat.NativeMessages is preserved
	// in full for UI display; compaction only affects the API-bound copy.
	copyChat.NativeMessages = compactNativeMessages(chat.NativeMessages)

	return copyChat
}

func (cs *ChatStore) Delete(chatId string) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	delete(cs.chats, chatId)
}

func (cs *ChatStore) CountUserMessages(chatId string) int {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	chat := cs.chats[chatId]
	if chat == nil {
		return 0
	}

	count := 0
	for _, msg := range chat.NativeMessages {
		if msg.GetRole() == "user" {
			count++
		}
	}
	return count
}

func (cs *ChatStore) PostMessage(chatId string, aiOpts *uctypes.AIOptsType, message uctypes.GenAIMessage) error {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	chat := cs.chats[chatId]
	if chat == nil {
		// Create new chat
		chat = &uctypes.AIChat{
			ChatId:         chatId,
			APIType:        aiOpts.APIType,
			Model:          aiOpts.Model,
			APIVersion:     aiOpts.APIVersion,
			NativeMessages: make([]uctypes.GenAIMessage, 0),
		}
		cs.chats[chatId] = chat
	} else {
		// Verify that the AI options match
		if chat.APIType != aiOpts.APIType {
			return fmt.Errorf("API type mismatch: expected %s, got %s (must start a new chat)", chat.APIType, aiOpts.APIType)
		}
		if !uctypes.AreModelsCompatible(chat.APIType, chat.Model, aiOpts.Model) {
			return fmt.Errorf("model mismatch: expected %s, got %s (must start a new chat)", chat.Model, aiOpts.Model)
		}
		if chat.APIVersion != aiOpts.APIVersion {
			return fmt.Errorf("API version mismatch: expected %s, got %s (must start a new chat)", chat.APIVersion, aiOpts.APIVersion)
		}
	}

	// Check for existing message with same ID (idempotency)
	messageId := message.GetMessageId()
	for i, existingMessage := range chat.NativeMessages {
		if existingMessage.GetMessageId() == messageId {
			// Replace existing message with same ID
			chat.NativeMessages[i] = message
			return nil
		}
	}

	// Append the new message if no duplicate found
	chat.NativeMessages = append(chat.NativeMessages, message)

	return nil
}

func (cs *ChatStore) RemoveMessage(chatId string, messageId string) bool {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	chat := cs.chats[chatId]
	if chat == nil {
		return false
	}

	initialLen := len(chat.NativeMessages)
	chat.NativeMessages = slices.DeleteFunc(chat.NativeMessages, func(msg uctypes.GenAIMessage) bool {
		return msg.GetMessageId() == messageId
	})

	return len(chat.NativeMessages) < initialLen
}

// compactNativeMessages applies context window compaction to a copy of the
// conversation's native messages. It preserves the full chat history in the
// store (for UI rendering) but returns a trimmed version for API calls so
// the request never exceeds the model's context window.
//
// Strategy:
//   - If len(msgs) <= MaxNativeMessages, return as-is (no compaction).
//   - Otherwise, keep the first message (usually system context) and the
//     most recent (MaxNativeMessages-2) messages intact.
//   - Replace the dropped middle section with a single synthetic user
//     message containing a summary of the compacted conversation.
//
// This prevents the "Unterminated string" / 400 error that occurs when the
// JSON body grows so large it exceeds the model's context window or gets
// truncated at the transport layer.
func compactNativeMessages(msgs []uctypes.GenAIMessage) []uctypes.GenAIMessage {
	if len(msgs) <= MaxNativeMessages {
		return msgs
	}

	// Build a summary of the messages being dropped.
	// We drop from index 1 (keeping msgs[0]) up to
	// len(msgs)-keepTail, where keepTail = MaxNativeMessages-2
	// (2 slots: one for the summary message, one for msgs[0]).
	keepTail := MaxNativeMessages - 2
	if keepTail < 1 {
		keepTail = 1
	}
	dropEnd := len(msgs) - keepTail
	if dropEnd <= 1 {
		// Nothing meaningful to compact
		return msgs
	}

	// Build summary of dropped messages (msgs[1:dropEnd])
	var summaryLines []string
	for _, m := range msgs[1:dropEnd] {
		line := m.GetContentSummary()
		if len(line) > compactionSummaryLen {
			line = line[:compactionSummaryLen] + "..."
		}
		if line != "" {
			summaryLines = append(summaryLines, line)
		}
	}

	var summaryText string
	if len(summaryLines) > 0 {
		summaryText = fmt.Sprintf("[Earlier conversation compacted — %d messages summarized:]\n%s",
			len(msgs[1:dropEnd]), strings.Join(summaryLines, "\n"))
	} else {
		summaryText = fmt.Sprintf("[Earlier conversation compacted — %d messages omitted]", len(msgs[1:dropEnd]))
	}

	log.Printf("chatstore: compacting conversation from %d to %d messages (dropped %d, summary %d bytes)\n",
		len(msgs), MaxNativeMessages, dropEnd-1, len(summaryText))

	// Assemble compacted slice:
	// [0] = original first message (system/user)
	// [1] = synthetic summary message
	// [2..] = recent tail messages
	result := make([]uctypes.GenAIMessage, 0, MaxNativeMessages)
	result = append(result, msgs[0])
	result = append(result, &uctypes.CompactionSummaryMessage{Text: summaryText})
	result = append(result, msgs[dropEnd:]...)

	return result
}
