// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package xmltoolparse

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type XMLToolCall struct {
	ToolName string
	Args     map[string]any
}

type ParsedXMLToolCalls struct {
	ToolCalls []XMLToolCall
	Text      string
}

var (
	xmlToolCallRE   = regexp.MustCompile("```tool")
	xmlArgKeyRE     = regexp.MustCompile(`<arg_key>(.*?)</arg_key>`)
	xmlArgValRE     = regexp.MustCompile(`<arg_value>(.*?)</arg_value>`)
	xmlCloseFenceRE = regexp.MustCompile("\n```")
)

func ParseXMLToolCalls(content string) *ParsedXMLToolCalls {
	result := &ParsedXMLToolCalls{ToolCalls: make([]XMLToolCall, 0)}
	callStarts := xmlToolCallRE.FindAllStringIndex(content, -1)
	if len(callStarts) == 0 {
		result.Text = content
		return result
	}
	result.Text = strings.TrimSpace(content[:callStarts[0][0]])
	searchOffset := callStarts[0][0]
	for {
		openIdx := xmlToolCallRE.FindStringIndex(content[searchOffset:])
		if openIdx == nil {
			break
		}
		absOpen := searchOffset + openIdx[0]
		afterFence := absOpen + len("```tool")
		newlineIdx := strings.Index(content[afterFence:], "\n")
		if newlineIdx == -1 {
			break
		}
		toolName := strings.TrimSpace(content[afterFence : afterFence+newlineIdx])
		if strings.HasPrefix(toolName, "tool") {
			toolName = strings.TrimSpace(toolName[len("tool"):])
		}
		if toolName == "" {
			searchOffset = afterFence + newlineIdx + 1
			continue
		}
		firstArgIdx := strings.Index(content[afterFence:], "<arg_key>")
		if firstArgIdx == -1 {
			break
		}
		bodyStart := afterFence + firstArgIdx
		body := content[bodyStart:]
		endIdx := len(body)
		if m := xmlCloseFenceRE.FindStringIndex(body); m != nil {
			endIdx = m[0]
		} else if m := xmlToolCallRE.FindStringIndex(body); m != nil {
			endIdx = m[0]
		}
		body = body[:endIdx]
		tc := XMLToolCall{ToolName: toolName, Args: make(map[string]any)}
		keyMatches := xmlArgKeyRE.FindAllStringSubmatchIndex(body, -1)
		for i, km := range keyMatches {
			key := body[km[2]:km[3]]
			valRegionStart := km[3]
			var valStr string
			if i+1 < len(keyMatches) {
				valRegion := body[valRegionStart:keyMatches[i+1][0]]
				valMatch := xmlArgValRE.FindStringSubmatch(valRegion)
				if len(valMatch) > 1 {
					valStr = strings.TrimSpace(valMatch[1])
				}
			} else {
				valRegion := body[valRegionStart:]
				valMatch := xmlArgValRE.FindStringSubmatch(valRegion)
				if len(valMatch) > 1 {
					valStr = strings.TrimSpace(valMatch[1])
				}
			}
			parsedVal := any(valStr)
			if valStr != "" {
				var jsonVal any
				if err := json.Unmarshal([]byte(valStr), &jsonVal); err == nil {
					parsedVal = jsonVal
				}
			}
			tc.Args[key] = parsedVal
		}
		result.ToolCalls = append(result.ToolCalls, tc)
		searchOffset = bodyStart + endIdx + 1
		if searchOffset > len(content) {
			break
		}
	}
	return result
}

func HasXMLToolCall(content string) bool {
	return xmlToolCallRE.MatchString(content)
}

func ToolCallID(index int) string {
	return fmt.Sprintf("xml_call_%d", index)
}
