// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"strings"
	"testing"
)

func TestSystemPromptAdvertisesAvailableCommandExecution(t *testing.T) {
	required := []string{
		"When term_run_command is exposed, you can execute shell commands",
		"Never claim shell execution is restricted, disabled, unavailable",
		"Only provide manual copy-paste commands when term_run_command is absent",
		"Use it directly for safe requested command execution",
	}

	for _, phrase := range required {
		if !strings.Contains(SystemPromptText_OpenAI, phrase) {
			t.Fatalf("SystemPromptText_OpenAI missing command-capability directive %q", phrase)
		}
	}
}
