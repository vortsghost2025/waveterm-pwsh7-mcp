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
	xmlToolCallRE = regexp.MustCompile( ```)
	xmlArgKeyRE   = regexp.MustCompile(<arg_key>(.*?)</arg_key>)
	xmlArgValRE   = regexp.MustCompile(<arg_value>(.*?)</arg_value>)
)

func ParseXMLToolCalls(content string) *ParsedXMLToolCalls {
	result := &ParsedXMLToolCalls{ToolCalls: make([]XMLToolCall, 0)}
	callStarts := xmlToolCallRE.FindAllStringIndex(content, -1)
	if len(callStarts) == 0 {
		result.Text = content
		return result
	}
	result.Text = strings.TrimSpace(content[:callStarts[0][0]])	searchOffset := callStarts[0][0]
	for {
		openIdx := xmlToolCallRE.FindStringIndex(content[searchOffset:])
		if openIdx == nil {
			break
		}
		absOpen := searchOffset + openIdx[0]
		tagClose := content[absOpen:].FindStringIndex(">")
		if tagClose == nil {
			break
		}
		toolNameStart := absOpen + tagClose[1]
		toolName := strings.TrimSpace(content[toolNameStart:])
		nameEnd := strings.Index(toolName, "<arg_key>")
		if nameEnd == -1 {
			searchOffset = toolNameStart + 1
			continue
		}
		toolName = strings.TrimSpace(toolName[:nameEnd])
		if toolName == "" {
			searchOffset = toolNameStart + 1
			continue
		}
		tc := XMLToolCall{ToolName: toolName, Args: make(map[string]any)}
		keyRegion := content[toolNameStart+nameEnd:]
		keyMatches := xmlArgKeyRE.FindAllStringSubmatchIndex(keyRegion, -1)
		for i, km := range keyMatches {
			key := keyRegion[km[2]:km[3]]
			valRegionStart := km[3]
			var valStr string
			if i+1 < len(keyMatches) {
				valRegion := keyRegion[valRegionStart:keyMatches[i+1][0]]
				valMatch := xmlArgValRE.FindStringSubmatch(valRegion)
				if len(valMatch) > 1 {
					valStr = strings.TrimSpace(valMatch[1])
				}
			} else {
				valRegion := keyRegion[valRegionStart:]
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
		closeIdx := xmlArgValRE.FindStringIndex(content[toolNameStart:])
		if closeIdx == nil {
			break
		}
		searchOffset = toolNameStart + closeIdx[1]
	}
	return result
}

func HasXMLToolCall(content string) bool {
	return xmlToolCallRE.MatchString(content)
}

func ToolCallID(index int) string {
	return fmt.Sprintf("xml_call_%d", index)
}