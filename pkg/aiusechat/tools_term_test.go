// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import "testing"

func TestSelectTermRunCommandTargetReady(t *testing.T) {
	got := selectTermRunCommandTarget("req-block", true, []struct{ BlockId, ShellState string }{})
	if got.BlockId != "req-block" {
		t.Errorf("BlockId: got %q, want req-block", got.BlockId)
	}
	if !got.HasFound {
		t.Error("HasFound: got false, want true")
	}
}

func TestSelectTermRunCommandTargetBusyWithAlternate(t *testing.T) {
	others := []struct{ BlockId, ShellState string }{
		{BlockId: "other-a", ShellState: "ready"},
	}
	got := selectTermRunCommandTarget("req-block", false, others)
	if got.BlockId != "other-a" {
		t.Errorf("BlockId: got %q, want other-a", got.BlockId)
	}
	if !got.HasFound {
		t.Error("HasFound: got false, want true")
	}
}

func TestSelectTermRunCommandTargetBusyNoAlternate(t *testing.T) {
	others := []struct{ BlockId, ShellState string }{
		{BlockId: "other-b", ShellState: "running-command"},
		{BlockId: "other-c", ShellState: ""},
	}
	got := selectTermRunCommandTarget("req-block", false, others)
	if got.BlockId != "" {
		t.Errorf("BlockId: got %q, want empty", got.BlockId)
	}
	if got.HasFound {
		t.Error("HasFound: got true, want false")
	}
}
