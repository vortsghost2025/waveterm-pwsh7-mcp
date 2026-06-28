// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aistore

import (
	"context"
	"sync"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *MemoryStore {
	t.Helper()
	tmp := t.TempDir()
	dataDirOverride = func() string { return tmp }
	t.Cleanup(func() {
		dataDirOverride = nil
	})

	return &MemoryStore{
		records: map[string]map[string]*MemoryRecord{},
		lock:    &sync.Mutex{},
	}
}

func TestMemoryStorePutAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Scope: "notes"}, "hello world")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if id == "" {
		t.Fatalf("expected non-empty id")
	}

	rec, err := store.Get(ctx, "ws_a", id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if rec == nil {
		t.Fatalf("expected record")
	}
	if rec.Body != "hello world" {
		t.Fatalf("body mismatch: got %q", rec.Body)
	}
}

func TestMemoryStoreGetByKeyUpsert(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Scope: "prefs", Key: "theme"}, "dark")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	rec, err := store.GetByKey(ctx, "ws_a", "prefs", "theme")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if rec == nil || rec.Body != "dark" {
		t.Fatalf("expected body=dark, got %+v", rec)
	}

	_, err = store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Scope: "prefs", Key: "theme"}, "light")
	if err != nil {
		t.Fatalf("Put upsert failed: %v", err)
	}

	rec2, err := store.GetByKey(ctx, "ws_a", "prefs", "theme")
	if err != nil {
		t.Fatalf("GetByKey after upsert failed: %v", err)
	}
	if rec2.Body != "light" {
		t.Fatalf("expected body=light after upsert, got %q", rec2.Body)
	}
	if rec2.Id != rec.Id {
		t.Fatalf("expected same id after upsert, got %s vs %s", rec2.Id, rec.Id)
	}
}

func TestMemoryStoreListWithScopeAndTagGlob(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Scope: "p1", Tags: []string{"important"}}, "alpha"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Scope: "p2", Tags: []string{"low"}}, "beta"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Scope: "p1", Tags: []string{"important"}}, "gamma"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	records, _, err := store.List(ctx, MemoryListOpts{WorkspaceId: "ws_a", Scope: "p1"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records in scope p1, got %d", len(records))
	}

	records, _, err = store.List(ctx, MemoryListOpts{WorkspaceId: "ws_a", TagGlob: "important"})
	if err != nil {
		t.Fatalf("List tagglob failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 important-tagged records, got %d", len(records))
	}
}

func TestMemoryStoreDeleteReturnsFalseWhenMissing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	deleted, err := store.Delete(ctx, "ws_a", "no-such-id")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if deleted {
		t.Fatalf("expected deleted=false for missing record")
	}
}

func TestMemoryStoreDeleteSucceeds(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a"}, "body")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	deleted, err := store.Delete(ctx, "ws_a", id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !deleted {
		t.Fatalf("expected deleted=true")
	}
	_, err = store.Get(ctx, "ws_a", id)
	if err == nil {
		t.Fatalf("expected error reading deleted record")
	}
}

func TestMemoryStoreSearchMatchesByBodyAndKey(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Key: "alpha"}, "this is a banana"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Key: "beta"}, "this is a mango"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Key: "gamma"}, "kiwi is great"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	matches, err := store.Search(ctx, "ws_a", "", "banana", 50)
	if err != nil {
		t.Fatalf("Search banana failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 banana match, got %d", len(matches))
	}

	matches, err = store.Search(ctx, "ws_a", "", "kiwi", 50)
	if err != nil {
		t.Fatalf("Search kiwi failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 kiwi match, got %d", len(matches))
	}
}

func TestMemorySearchByTag(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Tags: []string{"urgent", "ops"}}, "incident report"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Tags: []string{"misc"}}, "random note"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	matches, err := store.Search(ctx, "ws_a", "", "urgent", 50)
	if err != nil {
		t.Fatalf("Search tag failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 urgent-tag match, got %d", len(matches))
	}
}

func TestMemoryScopeFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Scope: "alpha"}, "a-body"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_a", Scope: "beta"}, "b-body"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	matches, err := store.Search(ctx, "ws_a", "alpha", "body", 50)
	if err != nil {
		t.Fatalf("Search scoped failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 scoped match, got %d", len(matches))
	}
	if matches[0].Scope != "alpha" {
		t.Fatalf("expected scope alpha, got %q", matches[0].Scope)
	}
}

func TestMemoryTTLExpirationOnRead(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.Put(ctx, MemoryOpts{
		WorkspaceId: "ws_a",
		Scope:       "default",
	}, "expiring body")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	store.lock.Lock()
	backdate := time.Now().Add(-10 * time.Second).UnixMilli()
	store.records["ws_a"][id].CreatedAt = backdate
	store.records["ws_a"][id].TtlSec = 1
	store.lock.Unlock()

	_, err = store.Get(ctx, "ws_a", id)
	if err == nil {
		t.Fatalf("expected expiration error from Get")
	}
}

func TestMemoryStoreWorkspacesAreIsolated(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws1", Key: "pref"}, "w1-val"); err != nil {
		t.Fatalf("Put ws1 failed: %v", err)
	}
	if _, err := store.Put(ctx, MemoryOpts{WorkspaceId: "ws2", Key: "pref"}, "w2-val"); err != nil {
		t.Fatalf("Put ws2 failed: %v", err)
	}

	rec1, err := store.GetByKey(ctx, "ws1", "", "pref")
	if err != nil {
		t.Fatalf("GetByKey ws1 failed: %v", err)
	}
	rec2, err := store.GetByKey(ctx, "ws2", "", "pref")
	if err != nil {
		t.Fatalf("GetByKey ws2 failed: %v", err)
	}
	if rec1.Body == rec2.Body {
		t.Fatalf("workspaces should be isolated, both have body=%q", rec1.Body)
	}
}

func TestMemoryStoreDeleteManyCountsAndSkips(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id1, _ := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_d", Scope: "x"}, "a")
	id2, _ := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_d", Scope: "x"}, "b")
	id3, _ := store.Put(ctx, MemoryOpts{WorkspaceId: "ws_d", Scope: "y"}, "c")

	deleted, err := store.DeleteMany(ctx, "ws_d", []string{id1, id2, "missing-id", id3})
	if err != nil {
		t.Fatalf("DeleteMany failed: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted, got %d", deleted)
	}
	if _, err := store.Get(ctx, "ws_d", id1); err == nil {
		t.Fatalf("id1 should be gone")
	}
	if _, err := store.Get(ctx, "ws_d", id3); err == nil {
		t.Fatalf("id3 should be gone")
	}
}

func TestMemoryStoreDeleteManyEmptyInput(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	deleted, err := store.DeleteMany(ctx, "ws_d2", []string{})
	if err != nil {
		t.Fatalf("DeleteMany empty should not err: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

func TestMemoryStoreDeleteByScope(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Put(ctx, MemoryOpts{WorkspaceId: "ws_s", Scope: "temp"}, "t1")
	store.Put(ctx, MemoryOpts{WorkspaceId: "ws_s", Scope: "temp"}, "t2")
	store.Put(ctx, MemoryOpts{WorkspaceId: "ws_s", Scope: "perm"}, "p1")

	deleted, err := store.DeleteByScope(ctx, "ws_s", "temp")
	if err != nil {
		t.Fatalf("DeleteByScope failed: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 temp notes deleted, got %d", deleted)
	}
	records, _, err := store.List(ctx, MemoryListOpts{WorkspaceId: "ws_s"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(records))
	}
	if records[0].Scope != "perm" {
		t.Fatalf("expected remaining to be scope=perm, got %s", records[0].Scope)
	}
}

func TestMemoryStoreDeleteByScopeEmpty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	deleted, err := store.DeleteByScope(ctx, "ws_empty", "nope")
	if err != nil {
		t.Fatalf("DeleteByScope empty should not err: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}
