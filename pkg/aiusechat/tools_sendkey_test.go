// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import "testing"

func TestResolvedSendKeySequenceSafeKeys(t *testing.T) {
	cases := []struct {
		key      string
		wantSeq  string
		wantIsSig bool
		wantSig  string
	}{
		{"escape", "\x1b", false, ""},
		{"esc", "\x1b", false, ""},
		{"backspace", "\x7f", false, ""},
		{"delete", "\x1b[3~", false, ""},
		{"home", "\x1b[H", false, ""},
		{"end", "\x1b[F", false, ""},
		{"arrowup", "\x1b[A", false, ""},
		{"arrowdown", "\x1b[B", false, ""},
		{"arrowleft", "\x1b[D", false, ""},
		{"arrowright", "\x1b[C", false, ""},
		{"up", "\x1b[A", false, ""},
		{"down", "\x1b[B", false, ""},
		{"left", "\x1b[D", false, ""},
		{"right", "\x1b[C", false, ""},
		{"tab", "\t", false, ""},
	}
	for _, tc := range cases {
		gotSeq, gotIsSig, gotSigName, err := ResolvedSendKeySequence(tc.key)
		if err != nil {
			t.Errorf("key %q: unexpected error: %v", tc.key, err)
			continue
		}
		if gotSeq != tc.wantSeq {
			t.Errorf("key %q: sequence got=%q want=%q", tc.key, gotSeq, tc.wantSeq)
		}
		if gotSigName != tc.wantSig {
			t.Errorf("key %q: sigName got=%q want=%q", tc.key, gotSigName, tc.wantSig)
		}
		if gotIsSig != tc.wantIsSig {
			t.Errorf("key %q: isSignal got=%v want=%v", tc.key, gotIsSig, tc.wantIsSig)
		}
		if IsDangerousKey(tc.key) {
			t.Errorf("key %q: unexpectedly classified as dangerous", tc.key)
		}
	}
}

func TestResolvedSendKeySequenceSignalKeys(t *testing.T) {
	cases := []struct {
		key       string
		wantSig   string
		wantIsSig bool
		wantSeq   string
	}{
		{"ctrlc", "SIGINT", true, ""},
		{"ctrlz", "SIGTSTP", true, ""},
		{"ctrld", "", false, "\x04"},
		{"ctrlbackslash", "SIGQUIT", true, ""},
		{"sigterm", "SIGTERM", true, ""},
		{"sigkill", "SIGKILL", true, ""},
	}
	for _, tc := range cases {
		gotSeq, gotIsSig, gotSigName, err := ResolvedSendKeySequence(tc.key)
		if err != nil {
			t.Errorf("key %q: unexpected error: %v", tc.key, err)
			continue
		}
		if gotSeq != tc.wantSeq {
			t.Errorf("key %q: sequence got=%q want=%q", tc.key, gotSeq, tc.wantSeq)
		}
		if gotSigName != tc.wantSig {
			t.Errorf("key %q: sigName got=%q want=%q", tc.key, gotSigName, tc.wantSig)
		}
		if gotIsSig != tc.wantIsSig {
			t.Errorf("key %q: isSignal got=%v want=%v", tc.key, gotIsSig, tc.wantIsSig)
		}
		if !IsDangerousKey(tc.key) {
			t.Errorf("key %q: expected dangerous, got safe", tc.key)
		}
	}
}

func TestResolvedSendKeySequenceDangerousNonSignal(t *testing.T) {
	if IsDangerousKey("enter") {
		t.Errorf("enter: expected safe, got dangerous")
	}
	seq, isSig, sigName, err := ResolvedSendKeySequence("enter")
	if err != nil {
		t.Errorf("enter: unexpected error: %v", err)
	}
	if seq != "\r" {
		t.Errorf("enter: seq got=%q want=%q", seq, "\r")
	}
	if isSig {
		t.Errorf("enter: should not be a signal")
	}
	if sigName != "" {
		t.Errorf("enter: sigName got=%q want=%q", sigName, "")
	}
	if !IsDangerousKey("ctrlbreak") {
		t.Errorf("ctrlbreak: expected dangerous, got safe")
	}
}

func TestResolvedSendKeySequenceUnknown(t *testing.T) {
	_, _, _, err := ResolvedSendKeySequence("bogus_key_xyz")
	if err == nil {
		t.Errorf("expected error for unknown key")
	}
}

func TestResolvedSendKeySequenceCaseInsensitive(t *testing.T) {
	gotSeq, _, _, err := ResolvedSendKeySequence("ENTER")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if gotSeq != "\r" {
		t.Errorf("got=%q want=%q", gotSeq, "\r")
	}
}
