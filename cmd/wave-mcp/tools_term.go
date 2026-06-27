package main

import (
	"encoding/json"
	"fmt"
	"strings"
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

	sendText := text
	if enterInput {
		sendText += "\r"
	}

	err := rpcSendInput(widgetId, sendText)
	if err != nil {
		return errResult(fmt.Sprintf("rpc send input failed: %s", err.Error()))
	}

	return ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: fmt.Sprintf(`{"success":true,"widget_id":"%s","bytes_sent":%d}`, widgetId, len(sendText))}},
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
