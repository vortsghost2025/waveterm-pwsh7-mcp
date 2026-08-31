// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func TestCommandRunStreamEcho(t *testing.T) {
	ws := &WshServer{}
	req := wshrpc.CommandRunStreamRequest{
		BlockID:     "test",
		Command:     "echo hi",
		Interactive: false,
	}
	ch := ws.CommandRunStreamCommand(context.Background(), req)
	var gotStdout string
	var gotExitCode *int
	for resp := range ch {
		if resp.Error != nil {
			t.Fatalf("unexpected error: %v", resp.Error)
		}
		event := resp.Response
		switch event.EventType {
		case wshrpc.CommandRunStreamEvent_Stdout:
			gotStdout += event.Data
		case wshrpc.CommandRunStreamEvent_Exit:
			gotExitCode = event.ExitCode
		case wshrpc.CommandRunStreamEvent_Error:
			t.Fatalf("unexpected error event: %s", event.Error)
		}
	}
	if !strings.Contains(gotStdout, "hi") {
		t.Errorf("expected stdout to contain 'hi', got %q", gotStdout)
	}
	if gotExitCode == nil {
		t.Fatal("expected exit code event")
	}
	if *gotExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", *gotExitCode)
	}
}
