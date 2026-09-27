package aiusechat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aistore"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// Delegation states
const (
	DelegationStateDelegated         = "delegated"
	DelegationStateAgentRunning      = "agent_running"
	DelegationStateAgentOutputObserved = "agent_output_observed"
	DelegationStateAgentCompleted    = "agent_completed"
	DelegationStateDelivering        = "delivering"
	DelegationStateDelivered         = "delivered"
	DelegationStateDeliveryFailed    = "delivery_failed"
	DelegationStatePendingChat       = "pending_chat"
)

const (
	maxDeliveryAttempts    = 10
	delegationStoreScope  = "delegation"
	completionStoreScope  = "delegation-completion"
	completionStoreTTL    = 86400 // 24 hours
)

// DelegationRecord represents a pending delegation from Wave AI to Kilo
type DelegationRecord struct {
	DeliveryId           string    `json:"deliveryId"`
	OriginatingChatId    string    `json:"originatingChatId"`
	OriginatingTabId     string    `json:"originatingTabId"`
	TargetBlockId        string    `json:"targetBlockId"`
	DelegatedAt          time.Time `json:"delegatedAt"`
	BaselineOutputMarker string    `json:"baselineOutputMarker"`
	ObservedNewOutput     string    `json:"observedNewOutput"`
	TargetRunState       string    `json:"targetRunState"`
	DeliveryState        string    `json:"deliveryState"`
	DeliveredAt          *time.Time `json:"deliveredAt,omitempty"`
	LastError            string    `json:"lastError,omitempty"`
	LastPolledAt         time.Time `json:"lastPolledAt,omitempty"`
	CompletionDetectedAt *time.Time `json:"completionDetectedAt,omitempty"`
	AttemptCount         int        `json:"attemptCount,omitempty"`
	NextAttemptAt        int64      `json:"nextAttemptAt,omitempty"`
	MaxAttempts          int        `json:"maxAttempts,omitempty"`
}

// CompletionRecord is the durable completion stored when the originating chat is not active
type CompletionRecord struct {
	DeliveryId        string `json:"deliveryId"`
	OriginatingChatId string `json:"originatingChatId"`
	OriginatingTabId  string `json:"originatingTabId"`
	TargetBlockId     string `json:"targetBlockId"`
	CompletedAt       int64  `json:"completedAt"`
	Result            string `json:"result"`
	DeliveryState     string `json:"deliveryState"`
	AttemptCount      int    `json:"attemptCount"`
	NextAttemptAt     int64  `json:"nextAttemptAt"`
	LastError         string `json:"lastError"`
}

// delegationMemoryStore is an interface enabling test isolation without touching the
// real aistore global singleton (which would write artifact files during go test).
type delegationMemoryStore interface {
	Put(ctx context.Context, opts aistore.MemoryOpts, body string) (string, error)
	GetByKey(ctx context.Context, workspaceId, scope, key string) (*aistore.MemoryRecord, error)
	List(ctx context.Context, opts aistore.MemoryListOpts) ([]*aistore.MemoryRecord, string, error)
	Delete(ctx context.Context, workspaceId, id string) (bool, error)
	DeleteByScope(ctx context.Context, workspaceId, scope string) (int, error)
}

// DelegationStore manages delegation records
type DelegationStore struct {
	records    map[string]*DelegationRecord   // keyed by deliveryId
	byChat     map[string][]*DelegationRecord // keyed by chatId
	memStore   delegationMemoryStore
	mu         sync.RWMutex
}

var globalDelegationStore = &DelegationStore{
	records:  make(map[string]*DelegationRecord),
	byChat:   make(map[string][]*DelegationRecord),
	memStore: aistore.GetMemoryStore(),
}

// GetDelegationStore returns the global delegation store
func GetDelegationStore() *DelegationStore {
	return globalDelegationStore
}

// NewDelegationStore creates a new DelegationStore backed by the provided store
// (useful in tests to avoid writing artifact files via the global aistore singleton)
func NewDelegationStore(memStore delegationMemoryStore) *DelegationStore {
	return &DelegationStore{
		records:  make(map[string]*DelegationRecord),
		byChat:   make(map[string][]*DelegationRecord),
		memStore: memStore,
	}
}

// getMemStore returns the store bound to this DelegationStore (fallback to global)
func (s *DelegationStore) getMemStore() delegationMemoryStore {
	if s.memStore != nil {
		return s.memStore
	}
	return aistore.GetMemoryStore()
}

// Create adds a new delegation record and syncs to durable store
func (s *DelegationStore) Create(record *DelegationRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.DeliveryId] = record
	s.byChat[record.OriginatingChatId] = append(s.byChat[record.OriginatingChatId], record)
	s.syncToStore(record)
}

// Get retrieves a delegation by ID
func (s *DelegationStore) Get(deliveryId string) *DelegationRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.records[deliveryId]
}

// GetPendingByChat returns pending delegations for a chat
func (s *DelegationStore) GetPendingByChat(chatId string) []*DelegationRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*DelegationRecord
	if records, ok := s.byChat[chatId]; ok {
		for _, r := range records {
			if isPendingState(r.DeliveryState) {
				result = append(result, r)
			}
		}
	}
	return result
}

// HasActiveDelegation returns true if the chat has any active delegations
func (s *DelegationStore) HasActiveDelegation(chatId string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if records, ok := s.byChat[chatId]; ok {
		for _, r := range records {
			if isPendingState(r.DeliveryState) {
				return true
			}
		}
	}
	return false
}

// Update modifies an existing delegation record and syncs to durable store
func (s *DelegationStore) Update(record *DelegationRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.DeliveryId] = record
	s.syncToStore(record)
}

// GetAllPending returns all delegations that are not yet terminal.
// Includes retryable delivery_failed and pending_chat records whose NextAttemptAt has passed.
// A NextAttemptAt of -1 means terminal (no more retries).
func (s *DelegationStore) GetAllPending() []*DelegationRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UnixMilli()
	var result []*DelegationRecord
	for _, rec := range s.records {
		if rec.DeliveryState == DelegationStateDelivered {
			continue
		}
		if rec.NextAttemptAt == -1 {
			continue
		}
		if rec.NextAttemptAt > 0 && rec.NextAttemptAt > now {
			continue
		}
		result = append(result, rec)
	}
	return result
}

func isPendingState(state string) bool {
	return state != DelegationStateDelivered && state != "terminal_"+DelegationStateDeliveryFailed
}

func isTerminalState(state string) bool {
	return state == DelegationStateDelivered || state == "terminal_"+DelegationStateDeliveryFailed
}

// syncToStore persists a delegation record to the durable MemoryStore.
// Must be called with s.mu held.
func (s *DelegationStore) syncToStore(record *DelegationRecord) {
	body, err := json.Marshal(record)
	if err != nil {
		log.Printf("delegation: failed to marshal record for sync: %v", err)
		return
	}
	ctx := context.Background()
	_, err = s.getMemStore().Put(ctx, aistore.MemoryOpts{
		Scope:  delegationStoreScope,
		Key:    record.DeliveryId,
		TtlSec: 0,
	}, string(body))
	if err != nil {
		log.Printf("delegation: failed to sync record to store: %v", err)
	}
}

// ResetStore clears all in-memory records and durable store data. Used for test isolation.
func (s *DelegationStore) ResetStore() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make(map[string]*DelegationRecord)
	s.byChat = make(map[string][]*DelegationRecord)
	ctx := context.Background()
	s.getMemStore().DeleteByScope(ctx, "", delegationStoreScope)
}

// Delete removes a delegation record from memory and durable store.
func (s *DelegationStore) Delete(deliveryId string) {
	s.mu.Lock()
	delete(s.records, deliveryId)
	for chatId, records := range s.byChat {
		for i, r := range records {
			if r.DeliveryId == deliveryId {
				s.byChat[chatId] = append(records[:i], records[i+1:]...)
				break
			}
		}
	}
	s.mu.Unlock()
	ctx := context.Background()
	memStore := s.getMemStore()
	memRec, err := memStore.GetByKey(ctx, "", delegationStoreScope, deliveryId)
	if err == nil && memRec != nil {
		memStore.Delete(ctx, "", memRec.Id)
	}
}

// RestoreFromStore loads delegation records from durable storage on startup.
func (s *DelegationStore) RestoreFromStore() {
	ctx := context.Background()
	records, _, err := s.getMemStore().List(ctx, aistore.MemoryListOpts{
		Scope: delegationStoreScope,
		Limit: 1000,
	})
	if err != nil {
		log.Printf("delegation: failed to restore from store: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	restored := 0
	for _, memRec := range records {
		if memRec == nil {
			continue
		}
		var rec DelegationRecord
		if err := json.Unmarshal([]byte(memRec.Body), &rec); err != nil {
			log.Printf("delegation: failed to unmarshal stored record %s: %v", memRec.Key, err)
			continue
		}
		if isTerminalState(rec.DeliveryState) {
			continue
		}
		s.records[rec.DeliveryId] = &rec
		s.byChat[rec.OriginatingChatId] = append(s.byChat[rec.OriginatingChatId], &rec)
		restored++
	}
	if restored > 0 {
		log.Printf("delegation: restored %d pending records from durable store", restored)
	}
}

// StoreCompletion persists a completed delegation result when the originating chat is not active.
func (s *DelegationStore) StoreCompletion(rec *CompletionRecord) {
	body, err := json.Marshal(rec)
	if err != nil {
		log.Printf("delegation: failed to marshal completion: %v", err)
		return
	}
	ctx := context.Background()
	_, err = s.getMemStore().Put(ctx, aistore.MemoryOpts{
		Scope:  completionStoreScope,
		Key:    rec.OriginatingChatId + ":" + rec.DeliveryId,
		TtlSec: completionStoreTTL,
	}, string(body))
	if err != nil {
		log.Printf("delegation: failed to store completion: %v", err)
	}
}

// GetPendingCompletions retrieves stored completions for a given chatId.
func (s *DelegationStore) GetPendingCompletions(chatId string) []*CompletionRecord {
	ctx := context.Background()
	records, _, err := s.getMemStore().List(ctx, aistore.MemoryListOpts{
		Scope: completionStoreScope,
		Limit: 100,
	})
	if err != nil {
		log.Printf("delegation: failed to list completions: %v", err)
		return nil
	}
	var result []*CompletionRecord
	for _, memRec := range records {
		if memRec == nil {
			continue
		}
		if !strings.HasPrefix(memRec.Key, chatId+":") {
			continue
		}
		var cr CompletionRecord
		if err := json.Unmarshal([]byte(memRec.Body), &cr); err != nil {
			continue
		}
		result = append(result, &cr)
	}
	return result
}

// DeleteCompletion removes a single stored completion record for the given chatId+deliveryId.
func (s *DelegationStore) DeleteCompletion(chatId, deliveryId string) {
	ctx := context.Background()
	memStore := s.getMemStore()
	key := chatId + ":" + deliveryId
	memRec, err := memStore.GetByKey(ctx, "", completionStoreScope, key)
	if err != nil || memRec == nil {
		return
	}
	memStore.Delete(ctx, "", memRec.Id)
}

// GenerateDeliveryId creates a unique ID for a delegation
func GenerateDeliveryId() string {
	return fmt.Sprintf("del_%d", time.Now().UnixNano())
}

var agentBinarySet = map[string]bool{
	"kilo":     true,
	"opencode": true,
	"aienv":    true,
	"claude":   true,
	"./kilo":   true,
	".\\kilo":  true,
}

// IsAgentTerminal checks if a terminal block is hosting an agent (like Kilo)
func IsAgentTerminal(ctx context.Context, blockId string) (bool, error) {
	block, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
	if err != nil {
		return false, err
	}

	if block.Meta == nil {
		return false, nil
	}

	if isAgentByModelKey(block.Meta) {
		return true, nil
	}

	if isAgentByCmdAndArgs(block.Meta) {
		return true, nil
	}

	if isAgentByRunOnStart(block.Meta) {
		return true, nil
	}

	return false, nil
}

func isAgentByModelKey(meta map[string]any) bool {
	_, hasModel := meta["agent:model"]
	return hasModel
}

func isAgentByCmdAndArgs(meta map[string]any) bool {
	cmd, ok := meta[waveobj.MetaKey_Cmd].(string)
	if ok && isAgentCommand(cmd) {
		return true
	}

	cmdArgs, ok := meta[waveobj.MetaKey_CmdArgs]
	if !ok {
		return false
	}

	switch v := cmdArgs.(type) {
	case string:
		if isAgentCommand(v) || containsLauncherScript(v) {
			return true
		}
	case []any:
		for _, arg := range v {
			if s, ok := arg.(string); ok {
				if isAgentCommand(s) || containsLauncherScript(s) {
					return true
				}
			}
		}
	case []string:
		for _, s := range v {
			if isAgentCommand(s) || containsLauncherScript(s) {
				return true
			}
		}
	}

	return false
}

func isAgentByRunOnStart(meta map[string]any) bool {
	runOnStart, ok := meta[waveobj.MetaKey_CmdRunOnStart]
	if !ok {
		return false
	}
	switch v := runOnStart.(type) {
	case string:
		return isAgentCommand(v) || containsLauncherScript(v)
	case bool:
		return false
	}
	return false
}

func containsAgentBinary(s string) bool {
	lower := strings.ToLower(s)
	for bin := range agentBinarySet {
		if strings.Contains(lower, bin) {
			return true
		}
	}
	return false
}

func isAgentCommand(s string) bool {
	return containsAgentBinary(s)
}

func containsLauncherScript(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "start-kilo") || strings.Contains(lower, "start-opencode")
}

func computeBackoffMs(attempt int) int64 {
	if attempt <= 0 {
		return 0
	}
	delay := math.Pow(2, float64(attempt-1))
	if delay > 60 {
		delay = 60
	}
	return int64(delay * 1000)
}

func computeNextAttemptAt(attempt int) int64 {
	if attempt <= 0 {
		return 0
	}
	delayMs := computeBackoffMs(attempt)
	return time.Now().UnixMilli() + delayMs
}

func hasReachedMaxAttempts(record *DelegationRecord) bool {
	max := record.MaxAttempts
	if max <= 0 {
		max = maxDeliveryAttempts
	}
	return record.AttemptCount >= max
}