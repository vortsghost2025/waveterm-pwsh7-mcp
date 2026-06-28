// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"fmt"
	"strings"
)

type SendKeyInput struct {
	WidgetId string `json:"widget_id"`
	Key      string `json:"key"`
}

type SendKeyOutput struct {
	Sent     bool   `json:"sent"`
	Sequence string `json:"sequence,omitempty"`
}

func IsDangerousKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "enter", "ctrlc", "ctrlz", "ctrld", "ctrlbackslash", "sigterm", "sigkill", "ctrlbreak":
		return true
	}
	return false
}

func ResolvedSendKeySequence(key string) (sequence string, isSignal bool, sigName string, err error) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "enter":
		return "\r", false, "", nil
	case "tab":
		return "\t", false, "", nil
	case "space":
		return " ", false, "", nil
	case "escape", "esc":
		return "\x1b", false, "", nil
	case "backspace":
		return "\x7f", false, "", nil
	case "delete":
		return "\x1b[3~", false, "", nil
	case "home":
		return "\x1b[H", false, "", nil
	case "end":
		return "\x1b[F", false, "", nil
	case "pageup":
		return "\x1b[5~", false, "", nil
	case "pagedown":
		return "\x1b[6~", false, "", nil
	case "arrowup", "up":
		return "\x1b[A", false, "", nil
	case "arrowdown", "down":
		return "\x1b[B", false, "", nil
	case "arrowright", "right":
		return "\x1b[C", false, "", nil
	case "arrowleft", "left":
		return "\x1b[D", false, "", nil
	case "ctrlc", "ctrl-c":
		return "", true, "SIGINT", nil
	case "ctrlz", "ctrl-z":
		return "", true, "SIGTSTP", nil
	case "ctrld", "ctrl-d":
		return "\x04", false, "", nil
	case "ctrlbackslash", "ctrl-\\", "ctrlbreak":
		return "", true, "SIGQUIT", nil
	case "sigterm":
		return "", true, "SIGTERM", nil
	case "sigkill":
		return "", true, "SIGKILL", nil
	default:
		return "", false, "", fmt.Errorf("unsupported key %q (allowed: enter,tab,escape,space,backspace,delete,home,end,pageup,pagedown,arrowup/dn/lf/rt,ctrlc,ctrlz,ctrld,ctrlbackslash,sigterm,sigkill)", key)
	}
}

func resolveSendKeyInput(widgetId, key string, resolve func(shortId string) (fullId string, err error)) (blockId, sequence string, isSignal bool, sigName string, err error) {
	if widgetId == "" {
		return "", "", false, "", fmt.Errorf("widget_id is required")
	}
	if key == "" {
		return "", "", false, "", fmt.Errorf("key is required")
	}
	if resolve == nil {
		resolve = func(s string) (string, error) { return s, nil }
	}
	fullId, err := resolve(widgetId)
	if err != nil {
		return "", "", false, "", err
	}
	sequence, isSignal, sigName, err = ResolvedSendKeySequence(key)
	if err != nil {
		return fullId, "", false, "", err
	}
	return fullId, sequence, isSignal, sigName, nil
}
