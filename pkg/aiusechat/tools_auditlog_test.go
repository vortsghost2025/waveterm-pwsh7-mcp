// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
)

func TestParseFlexibleTimeDuration(t *testing.T) {
	now := time.Now().UnixMilli()
	cases := []struct {
		input    string
		wantDiff int64 // approximate, ms. negative = in past
		wantErr  bool
	}{
		{"1h", 3600 * 1000, false},
		{"30m", 1800 * 1000, false},
		{"5s", 5000, false},
	}
	for _, tc := range cases {
		gotMs, err := parseFlexibleTime(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("input %q: unexpected error %v", tc.input, err)
			continue
		}
		diff := now - gotMs
		if diff < 0 {
			diff = -diff
		}
		tol := int64(2000)
		if diff < tc.wantDiff-tol || diff > tc.wantDiff+tol {
			t.Errorf("input %q: diff=%dms want ~%dms", tc.input, diff, tc.wantDiff)
		}
	}
}

func TestParseFlexibleTimeInvalid(t *testing.T) {
	_, err := parseFlexibleTime("not-a-time-or-duration")
	if err == nil {
		t.Errorf("expected error for unknown format")
	}
}

func TestParseAuditQueryInputDefaults(t *testing.T) {
	in := &AuditQueryInput{}
	err := utilfn.ReUnmarshal(in, map[string]any{})
	if err != nil {
		// ReUnmarshal is symmetric; not strictly needed
	}
	parsed, err := parseAuditQueryInput(map[string]any{})
	if err != nil {
		t.Fatalf("default parse failed: %v", err)
	}
	if parsed.ToolName != "" {
		t.Errorf("unexpected default toolname: %s", parsed.ToolName)
	}
}

func TestParseAuditTailInputLimits(t *testing.T) {
	parsed, err := parseAuditTailInput(map[string]any{"maxlines": float64(1000)})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed.MaxLines != 200 {
		t.Errorf("expected cap at 200, got %d", parsed.MaxLines)
	}
	parsed, err = parseAuditTailInput(map[string]any{"maxlines": float64(20)})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed.MaxLines != 20 {
		t.Errorf("expected 20, got %d", parsed.MaxLines)
	}
	parsed, err = parseAuditTailInput(map[string]any{})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed.MaxLines != 50 {
		t.Errorf("expected default 50, got %d", parsed.MaxLines)
	}
}
