package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

const (
	termScrollbackDefaultCount = 200
	termScrollbackMaxCount      = 1000
	termScrollbackTimeoutMs     = 10000
)

func callTermGetScrollback(args map[string]any) ToolCallResult {
	widgetId, _ := args["widget_id"].(string)
	if widgetId == "" {
		return errJSONResult("missing or empty 'widget_id' argument")
	}

	count := termScrollbackDefaultCount
	if c, ok := args["count"].(float64); ok {
		count = int(c)
		if count < 1 {
			count = 1
		}
		if count > termScrollbackMaxCount {
			count = termScrollbackMaxCount
		}
	}

	result, err := rpcGetScrollback(widgetId, 0, count)
	if err != nil {
		return errJSONResult(fmt.Sprintf("rpc scrollback failed: %s", err.Error()))
	}

	cleaned := strings.Join(result.Lines, "\n")
	if len(result.Lines) > 0 {
		cleaned += "\n"
	}

	type scrollbackResult struct {
		TotalLines    int      `json:"totallines"`
		LineStart     int      `json:"linestart"`
		Lines         []string `json:"lines"`
		LastUpdated   int64    `json:"lastupdated"`
		Content       string   `json:"content"`
		ReturnedLines int      `json:"returnedlines"`
	}

	out := scrollbackResult{
		TotalLines:    result.TotalLines,
		LineStart:     result.LineStart,
		Lines:         result.Lines,
		LastUpdated:   result.LastUpdated,
		Content:       cleaned,
		ReturnedLines: len(result.Lines),
	}

	b, _ := json.Marshal(out)
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(b)}}}
}

func callTermSendInput(args map[string]any) ToolCallResult {
	widgetId, _ := args["widget_id"].(string)
	if widgetId == "" {
		return errResult("missing or empty 'widget_id' argument")
	}
	text, _ := args["text"].(string)
	if text == "" {
		return errResult("missing or empty 'text' argument")
	}
	enterInput := false
	if e, ok := args["enter"].(bool); ok {
		enterInput = e
	}

	err := rpcSendInput(widgetId, text, enterInput)
	if err != nil {
		return errResult(fmt.Sprintf("rpc send input failed: %s", err.Error()))
	}

	sendLen := len(text)
	if enterInput {
		sendLen++
	}
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: fmt.Sprintf(`{"success":true,"widget_id":"%s","bytes_sent":%d}`, widgetId, sendLen)}},
	}
}

func callTermListWidgets(args map[string]any) ToolCallResult {
	widgets, err := rpcListWidgets()
	if err != nil {
		return errResult(fmt.Sprintf("rpc list widgets failed: %s", err.Error()))
	}

	b, _ := json.Marshal(widgets)
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: string(b)}},
	}
}

func callTermRunCommand(args map[string]any) ToolCallResult {
	command, _ := args["command"].(string)
	if command == "" {
		return errResult("missing or empty 'command' argument")
	}

	timeoutMs := 30000
	if t, ok := args["timeout_ms"].(float64); ok {
		timeoutMs = int(t)
		if timeoutMs < 1000 {
			timeoutMs = 1000
		}
		if timeoutMs > 120000 {
			timeoutMs = 120000
		}
	}

	data := wshrpc.AgentRunCommandData{
		Command: command,
		Timeout: timeoutMs / 1000,
	}
	opts := &wshrpc.RpcOpts{Timeout: int64(timeoutMs) + 2000}

	client, err := ensureAgentClient()
	if err != nil {
		return errResult(fmt.Sprintf("rpc client error: %s", err.Error()))
	}

	rtn, err := wshclient.AgentRunCommandCommand(client, data, opts)
	if err != nil {
		return errResult(fmt.Sprintf("rpc run command failed: %s", err.Error()))
	}

	type runCmdResult struct {
		ExitCode int    `json:"exit_code"`
		Output   string `json:"output"`
	}

	out := runCmdResult{
		ExitCode: rtn.ExitCode,
		Output:   rtn.Output,
	}

	b, _ := json.Marshal(out)
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(b)}}}
}

func callTermListTerminals(args map[string]any) ToolCallResult {
	rtn, err := rpcTermInfo("")
	if err != nil {
		return errResult(fmt.Sprintf("rpc list terminals failed: %s", err.Error()))
	}

	b, _ := json.Marshal(rtn.Terminals)
	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: string(b)}},
	}
}

func callTermSearchScrollback(args map[string]any) ToolCallResult {
	widgetId, _ := args["widget_id"].(string)
	if widgetId == "" {
		return errJSONResult("missing or empty 'widget_id' argument")
	}
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return errJSONResult("missing or empty 'pattern' argument")
	}
	isRegex := false
	if r, ok := args["isregex"].(bool); ok {
		isRegex = r
	}
	maxMatches := 50
	if m, ok := args["maxmatches"].(float64); ok {
		maxMatches = int(m)
		if maxMatches < 1 {
			maxMatches = 1
		}
		if maxMatches > 200 {
			maxMatches = 200
		}
	}

	result, err := rpcTermSearchScrollback(widgetId, pattern, isRegex, maxMatches)
	if err != nil {
		return errJSONResult(fmt.Sprintf("rpc search scrollback failed: %s", err.Error()))
	}

	matches := make([]map[string]any, 0, len(result.Matches))
	for _, m := range result.Matches {
		matches = append(matches, map[string]any{
			"line":    m.Line,
			"snippet": m.Snippet,
		})
	}

	out := map[string]any{
		"totalmatches": result.TotalMatches,
		"matches":      matches,
	}

	b, _ := json.Marshal(out)
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(b)}}}
}

func callWidgetClearScrollback(args map[string]any) ToolCallResult {
	widgetId, _ := args["widget_id"].(string)
	if widgetId == "" {
		return errJSONResult("missing or empty 'widget_id' argument")
	}

	err := rpcWidgetClearScrollback(widgetId)
	if err != nil {
		return errJSONResult(fmt.Sprintf("rpc clear scrollback failed: %s", err.Error()))
	}

	b, _ := json.Marshal(map[string]any{"cleared": true})
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(b)}}}
}

func resolveMcpSendKey(key string) (sequence string, isSignal bool, sigName string, err error) {
	switch key {
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
		return "", false, "", fmt.Errorf("unsupported key %q", key)
	}
}

func callTermSendKey(args map[string]any) ToolCallResult {
	widgetId, _ := args["widget_id"].(string)
	if widgetId == "" {
		return errResult("missing or empty 'widget_id' argument")
	}
	key, _ := args["key"].(string)
	if key == "" {
		return errResult("missing or empty 'key' argument")
	}
	sequence, isSignal, sigName, err := resolveMcpSendKey(key)
	if err != nil {
		return errResult(err.Error())
	}
	err = rpcSendKey(widgetId, sequence, sigName)
	if err != nil {
		return errResult(fmt.Sprintf("rpc send key failed: %s", err.Error()))
	}
	out := map[string]any{
		"success":   true,
		"widget_id": widgetId,
		"key":       key,
		"sequence":  sequence,
		"signal":    isSignal,
		"signame":   sigName,
	}
	b, _ := json.Marshal(out)
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(b)}}}
}
