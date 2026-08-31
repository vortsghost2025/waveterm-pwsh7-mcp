// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"errors"
	"testing"
)

// withTermWshMode runs fn with UsingTermWshMode set, restoring the previous
// value afterwards so tests don't leak global state.
func withTermWshMode(enabled bool, fn func()) {
	prev := UsingTermWshMode
	UsingTermWshMode = enabled
	defer func() { UsingTermWshMode = prev }()
	fn()
}

type errWriter struct{ err error }

func (w errWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestWrappedWriterPassthroughWhenNotTermWshMode(t *testing.T) {
	input := []byte("line1\nline2\n")
	var dest bytes.Buffer
	w := &WrappedWriter{dest: &dest}

	withTermWshMode(false, func() {
		n, err := w.Write(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != len(input) {
			t.Errorf("expected n=%d, got %d", len(input), n)
		}
	})
	if !bytes.Equal(dest.Bytes(), input) {
		t.Errorf("passthrough mismatch: got %q, want %q", dest.Bytes(), input)
	}
}

func TestWrappedWriterNoNewlineFastPath(t *testing.T) {
	input := []byte("plain text without newlines")
	var dest bytes.Buffer
	w := &WrappedWriter{dest: &dest}

	withTermWshMode(true, func() {
		n, err := w.Write(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != len(input) {
			t.Errorf("expected n=%d, got %d", len(input), n)
		}
	})
	if !bytes.Equal(dest.Bytes(), input) {
		t.Errorf("no-newline write mismatch: got %q, want %q", dest.Bytes(), input)
	}
}

func TestWrappedWriterCRLFRewrite(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single newline", "a\nb", "a\r\nb"},
		{"multiple newlines", "a\nb\nc\n", "a\r\nb\r\nc\r\n"},
		{"consecutive newlines", "\n\n", "\r\n\r\n"},
		{"existing crlf preserved", "a\r\nb", "a\r\r\nb"},
		{"only newline", "\n", "\r\n"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dest bytes.Buffer
			w := &WrappedWriter{dest: &dest}

			var n int
			var err error
			withTermWshMode(true, func() {
				n, err = w.Write([]byte(tt.input))
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := dest.String(); got != tt.want {
				t.Errorf("rewrite mismatch: got %q, want %q", got, tt.want)
			}
			if n != dest.Len() {
				t.Errorf("expected n to equal bytes written (%d), got %d", dest.Len(), n)
			}
		})
	}
}

func TestWrappedWriterPropagatesDestError(t *testing.T) {
	wantErr := errors.New("dest write failed")
	inputs := [][]byte{
		[]byte("no newlines here"), // fast path
		[]byte("has\nnewline"),     // rewritten path
	}
	for _, input := range inputs {
		w := &WrappedWriter{dest: errWriter{err: wantErr}}
		var err error
		withTermWshMode(true, func() {
			_, err = w.Write(input)
		})
		if !errors.Is(err, wantErr) {
			t.Errorf("input %q: expected dest error to propagate, got %v", input, err)
		}
	}
}
