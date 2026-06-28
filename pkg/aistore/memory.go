// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aistore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

const MemoryStoreDir = "aistore"
const DefaultLimit = 50
const MaxLimit = 200
const PruneInterval = 5 * time.Minute

var globalMemStore = &MemoryStore{
	records: map[string]map[string]*MemoryRecord{},
	lock:    &sync.Mutex{},
}

func GetMemoryStore() *MemoryStore {
	return globalMemStore
}

type MemoryStore struct {
	records map[string]map[string]*MemoryRecord
	lock    *sync.Mutex
}

var dataDirOverride func() string

func (s *MemoryStore) getDataDir() string {
	if dataDirOverride != nil {
		return dataDirOverride()
	}
	return wavebase.GetWaveDataDir()
}

func (s *MemoryStore) getWorkspaceDir(workspaceId string) string {
	if workspaceId == "" {
		workspaceId = "global"
	}
	return filepath.Join(s.getDataDir(), MemoryStoreDir, workspaceId)
}

func (s *MemoryStore) recordPath(workspaceId, id string) string {
	return filepath.Join(s.getWorkspaceDir(workspaceId), id+".json")
}

func (s *MemoryStore) loadWorkspace(workspaceId string) error {
	dir := s.getWorkspaceDir(workspaceId)
	wsRecords := map[string]*MemoryRecord{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			s.records[workspaceId] = wsRecords
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var rec MemoryRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		if rec.TtlSec > 0 && time.Now().UnixMilli() > rec.CreatedAt+int64(rec.TtlSec)*1000 {
			os.Remove(filepath.Join(dir, entry.Name()))
			continue
		}
		wsRecords[id] = &rec
	}
	s.records[workspaceId] = wsRecords
	return nil
}

func (s *MemoryStore) ensureWorkspaceLoaded(workspaceId string) error {
	if _, ok := s.records[workspaceId]; !ok {
		if err := s.loadWorkspace(workspaceId); err != nil {
			return err
		}
	}
	return nil
}

func (s *MemoryStore) writeRecord(workspaceId string, rec *MemoryRecord) error {
	dir := s.getWorkspaceDir(workspaceId)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(s.recordPath(workspaceId, rec.Id), data, 0600)
}

func (s *MemoryStore) deleteRecordFile(workspaceId, id string) error {
	return os.Remove(s.recordPath(workspaceId, id))
}

func (s *MemoryStore) Put(ctx context.Context, opts MemoryOpts, body string) (string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if opts.Scope == "" {
		opts.Scope = "notes"
	}
	now := time.Now().UnixMilli()

	if err := s.ensureWorkspaceLoaded(opts.WorkspaceId); err != nil {
		return "", err
	}

	wsRecords := s.records[opts.WorkspaceId]

	var rec *MemoryRecord
	if opts.Key != "" {
		for _, r := range wsRecords {
			if r.Scope == opts.Scope && r.Key == opts.Key {
				rec = r
				break
			}
		}
	}

	if rec == nil {
		id := uuid.New().String()
		rec = &MemoryRecord{
			Id:          id,
			WorkspaceId: opts.WorkspaceId,
			Scope:       opts.Scope,
			Key:         opts.Key,
			Tags:        opts.Tags,
			Body:        body,
			CreatedAt:   now,
			UpdatedAt:   now,
			TtlSec:      opts.TtlSec,
		}
		wsRecords[id] = rec
	} else {
		rec.Body = body
		if opts.Tags != nil {
			rec.Tags = opts.Tags
		}
		if opts.TtlSec > 0 {
			rec.TtlSec = opts.TtlSec
		}
		rec.UpdatedAt = now
	}

	if err := s.writeRecord(opts.WorkspaceId, rec); err != nil {
		return "", err
	}
	return rec.Id, nil
}

func (s *MemoryStore) Get(ctx context.Context, workspaceId, id string) (*MemoryRecord, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.ensureWorkspaceLoaded(workspaceId); err != nil {
		return nil, err
	}

	wsRecords := s.records[workspaceId]
	rec, ok := wsRecords[id]
	if !ok {
		return nil, fmt.Errorf("memory record %q not found", id)
	}

	if rec.TtlSec > 0 && time.Now().UnixMilli() > rec.CreatedAt+int64(rec.TtlSec)*1000 {
		delete(wsRecords, id)
		s.deleteRecordFile(workspaceId, id)
		return nil, fmt.Errorf("memory record %q not found", id)
	}

	return rec, nil
}

func (s *MemoryStore) GetByKey(ctx context.Context, workspaceId, scope, key string) (*MemoryRecord, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if scope == "" {
		scope = "notes"
	}

	if err := s.ensureWorkspaceLoaded(workspaceId); err != nil {
		return nil, err
	}

	wsRecords := s.records[workspaceId]
	for _, rec := range wsRecords {
		if rec.Scope == scope && rec.Key == key {
			if rec.TtlSec > 0 && time.Now().UnixMilli() > rec.CreatedAt+int64(rec.TtlSec)*1000 {
				delete(wsRecords, rec.Id)
				s.deleteRecordFile(workspaceId, rec.Id)
				continue
			}
			return rec, nil
		}
	}
	return nil, fmt.Errorf("memory record with scope %q and key %q not found", scope, key)
}

func tagsMatch(tags []string, tagGlob string) bool {
	if tagGlob == "" {
		return true
	}
	for _, tag := range tags {
		if ok, _ := filepath.Match(tagGlob, tag); ok {
			return true
		}
	}
	return false
}

func (s *MemoryStore) List(ctx context.Context, opts MemoryListOpts) ([]*MemoryRecord, string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if opts.Limit <= 0 || opts.Limit > MaxLimit {
		opts.Limit = DefaultLimit
	}

	if err := s.ensureWorkspaceLoaded(opts.WorkspaceId); err != nil {
		return nil, "", err
	}

	wsRecords := s.records[opts.WorkspaceId]
	now := time.Now().UnixMilli()

	var filtered []*MemoryRecord
	for _, rec := range wsRecords {
		if rec.TtlSec > 0 && now > rec.CreatedAt+int64(rec.TtlSec)*1000 {
			delete(wsRecords, rec.Id)
			s.deleteRecordFile(opts.WorkspaceId, rec.Id)
			continue
		}
		if opts.Scope != "" && rec.Scope != opts.Scope {
			continue
		}
		if opts.TagGlob != "" && !tagsMatch(rec.Tags, opts.TagGlob) {
			continue
		}
		filtered = append(filtered, rec)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt > filtered[j].UpdatedAt
	})

	startIdx := 0
	if opts.Cursor != "" {
		for i, rec := range filtered {
			if rec.Id == opts.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	if startIdx >= len(filtered) {
		return nil, "", nil
	}

	endIdx := startIdx + opts.Limit
	if endIdx > len(filtered) {
		endIdx = len(filtered)
	}

	result := filtered[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(filtered) {
		nextCursor = filtered[endIdx-1].Id
	}

	return result, nextCursor, nil
}

func (s *MemoryStore) Delete(ctx context.Context, workspaceId, id string) (bool, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.ensureWorkspaceLoaded(workspaceId); err != nil {
		return false, err
	}

	wsRecords := s.records[workspaceId]
	if _, ok := wsRecords[id]; !ok {
		return false, nil
	}

	delete(wsRecords, id)
	if err := s.deleteRecordFile(workspaceId, id); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

func (s *MemoryStore) DeleteMany(ctx context.Context, workspaceId string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.ensureWorkspaceLoaded(workspaceId); err != nil {
		return 0, err
	}
	wsRecords := s.records[workspaceId]
	deleted := 0
	for _, id := range ids {
		if _, ok := wsRecords[id]; !ok {
			continue
		}
		delete(wsRecords, id)
		if err := s.deleteRecordFile(workspaceId, id); err != nil && !os.IsNotExist(err) {
			continue
		}
		deleted++
	}
	return deleted, nil
}

func (s *MemoryStore) DeleteByScope(ctx context.Context, workspaceId, scope string) (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.ensureWorkspaceLoaded(workspaceId); err != nil {
		return 0, err
	}
	wsRecords := s.records[workspaceId]
	deleted := 0
	for id, rec := range wsRecords {
		if scope != "" && rec.Scope != scope {
			continue
		}
		delete(wsRecords, id)
		if err := s.deleteRecordFile(workspaceId, id); err != nil && !os.IsNotExist(err) {
			continue
		}
		deleted++
	}
	return deleted, nil
}

func (s *MemoryStore) Search(ctx context.Context, workspaceId, scope, query string, limit int) ([]MemorySearchMatch, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if limit <= 0 || limit > MaxLimit {
		limit = DefaultLimit
	}

	if err := s.ensureWorkspaceLoaded(workspaceId); err != nil {
		return nil, err
	}

	wsRecords := s.records[workspaceId]
	now := time.Now().UnixMilli()
	queryLower := strings.ToLower(query)

	var matches []MemorySearchMatch
	for _, rec := range wsRecords {
		if rec.TtlSec > 0 && now > rec.CreatedAt+int64(rec.TtlSec)*1000 {
			delete(wsRecords, rec.Id)
			s.deleteRecordFile(workspaceId, rec.Id)
			continue
		}
		if scope != "" && rec.Scope != scope {
			continue
		}

		if !strings.Contains(strings.ToLower(rec.Body), queryLower) &&
			!strings.Contains(strings.ToLower(rec.Key), queryLower) {
			tagMatch := false
			for _, tag := range rec.Tags {
				if strings.Contains(strings.ToLower(tag), queryLower) {
					tagMatch = true
					break
				}
			}
			if !tagMatch {
				continue
			}
		}

		snippet := rec.Body
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}

		matches = append(matches, MemorySearchMatch{
			Id:      rec.Id,
			Scope:   rec.Scope,
			Key:     rec.Key,
			Snippet: snippet,
		})

		if len(matches) >= limit {
			break
		}
	}

	return matches, nil
}

func (s *MemoryStore) PruneExpired(ctx context.Context, workspaceId string) (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.ensureWorkspaceLoaded(workspaceId); err != nil {
		return 0, err
	}

	wsRecords := s.records[workspaceId]
	now := time.Now().UnixMilli()
	deleted := 0

	for id, rec := range wsRecords {
		if rec.TtlSec > 0 && now > rec.CreatedAt+int64(rec.TtlSec)*1000 {
			delete(wsRecords, id)
			s.deleteRecordFile(workspaceId, id)
			deleted++
		}
	}

	return deleted, nil
}
