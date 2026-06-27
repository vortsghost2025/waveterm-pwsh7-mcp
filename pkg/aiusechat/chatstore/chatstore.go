// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package chatstore

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

const (
	// MaxNativeMessages is the maximum number of messages sent to the API.
	// When a conversation exceeds this, older messages are replaced with a
	// compact summary to stay within the context window.
	// A typical tool-using conversation produces ~3 native messages per turn
	// (user, assistant+tool_call, tool_result), so 50 ≈ 16 conversation turns.
	MaxNativeMessages = 50

	// compactionSummaryLen is the max characters per message in the compaction summary.
	compactionSummaryLen = 150

	// dbPersistTimeout is the timeout for DB persist operations.
	dbPersistTimeout = 2 * time.Second
)

type ChatStore struct {
	lock sync.Mutex
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
		// Cache miss — try loading from DB
		chat = cs.loadFromDB(chatId)
		if chat == nil {
			return nil
		}
		cs.chats[chatId] = chat
	}

	// Copy the chat to prevent concurrent access issues
	copyChat := &uctypes.AIChat{
		ChatId: chat.ChatId,
		APIType: chat.APIType,
		Model: chat.Model,
		APIVersion: chat.APIVersion,
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

	// Also delete from DB
	cs.deleteFromDB(chatId)
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
		// Try loading from DB first
		chat = cs.loadFromDB(chatId)
		if chat == nil {
			// Create new chat
			chat = &uctypes.AIChat{
				ChatId: chatId,
				APIType: aiOpts.APIType,
				Model: aiOpts.Model,
				APIVersion: aiOpts.APIVersion,
				NativeMessages: make([]uctypes.GenAIMessage, 0),
			}
			cs.chats[chatId] = chat
		} else {
			cs.chats[chatId] = chat
		}
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
			cs.persistToDBAsync(chat)
			return nil
		}
	}

	// Append the new message if no duplicate found
	chat.NativeMessages = append(chat.NativeMessages, message)

	// Persist to DB asynchronously
	cs.persistToDBAsync(chat)

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

	removed := len(chat.NativeMessages) < initialLen
	if removed {
		cs.persistToDBAsync(chat)
	}

	return removed
}

// --- DB persistence methods ---

// persistToDBAsync writes the chat to the SQLite db_aichat table in a goroutine.
// Must be called with cs.lock held. The lock is released before the goroutine runs.
func (cs *ChatStore) persistToDBAsync(chat *uctypes.AIChat) {
	jsonData, err := uctypes.MarshalAIChat(chat)
	if err != nil {
		log.Printf("chatstore: error marshaling chat %s for persist: %v", chat.ChatId, err)
		return
	}
	chatId := chat.ChatId
	dataStr := string(jsonData)
	go func() {
		ctx, cancelFn := context.WithTimeout(context.Background(), dbPersistTimeout)
		defer cancelFn()
		err := wstore.WithTx(ctx, func(tx *wstore.TxWrap) error {
			query := `INSERT INTO db_aichat (chatid, data) VALUES (?, ?)
				ON CONFLICT(chatid) DO UPDATE SET data = excluded.data`
			tx.Exec(query, chatId, dataStr)
			return nil
		})
		if err != nil {
			log.Printf("chatstore: error persisting chat %s to DB: %v", chatId, err)
		}
	}()
}

// loadFromDB reads a chat from the SQLite db_aichat table.
// Must be called with cs.lock held.
func (cs *ChatStore) loadFromDB(chatId string) *uctypes.AIChat {
	ctx, cancelFn := context.WithTimeout(context.Background(), dbPersistTimeout)
	defer cancelFn()

	type chatRow struct {
		ChatId string
		Data   string
	}
	var row chatRow
	err := wstore.WithTx(ctx, func(tx *wstore.TxWrap) error {
		query := `SELECT chatid, data FROM db_aichat WHERE chatid = ?`
		found := tx.Get(&row, query, chatId)
		if !found {
			return wstore.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil
	}

	chat, err := uctypes.UnmarshalAIChat([]byte(row.Data))
	if err != nil {
		log.Printf("chatstore: error unmarshaling chat %s from DB: %v", chatId, err)
		return nil
	}

	return chat
}

// deleteFromDB removes a chat from the SQLite db_aichat table.
// Must be called with cs.lock held.
func (cs *ChatStore) deleteFromDB(chatId string) {
	ctx, cancelFn := context.WithTimeout(context.Background(), dbPersistTimeout)
	defer cancelFn()

	err := wstore.WithTx(ctx, func(tx *wstore.TxWrap) error {
		query := `DELETE FROM db_aichat WHERE chatid = ?`
		tx.Exec(query, chatId)
		return nil
	})
	if err != nil {
		log.Printf("chatstore: error deleting chat %s from DB: %v", chatId, err)
	}
}

// --- Compaction logic (unchanged) ---

func compactNativeMessages(msgs []uctypes.GenAIMessage) []uctypes.GenAIMessage {
	if len(msgs) <= MaxNativeMessages {
		return msgs
	}

	keepTail := MaxNativeMessages - 1
	dropEnd := len(msgs) - keepTail
	if dropEnd <= 1 {
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
		summaryText = fmt.Sprintf("[Earlier conversation compacted — %d messages omitted]\n%s",
			len(msgs[1:dropEnd]), strings.Join(summaryLines, "\n"))
	} else {
		summaryText = fmt.Sprintf("[Earlier conversation compacted — %d messages omitted]", len(msgs[1:dropEnd]))
	}

	log.Printf("compacting chat: total=%d max=%d dropping=%d summaryLen=%d",
		len(msgs), MaxNativeMessages, dropEnd-1, len(summaryText))

	result := make([]uctypes.GenAIMessage, 0, MaxNativeMessages)
	result = append(result, msgs[0]) // keep system/first message
	result = append(result, &uctypes.CompactionSummaryMessage{Text: summaryText})
	result = append(result, msgs[dropEnd:]...)

	return result
}
