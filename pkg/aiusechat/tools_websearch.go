// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/secretstore"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
)

var (
	WebSearchDefaultNumResults   = 8
	WebSearchMaxNumResults       = 20
	WebSearchDefaultTimeout      = 30
	WebSearchMaxTimeout          = 120
	WebSearchExaBaseURL          = "https://api.exa.ai"
	WebSearchExaSearchEndpoint   = "/search"
	WebSearchEnvAPIKey           = "EXA_API_KEY"
)

type WebSearchToolInput struct {
	Query        string `json:"query"`
	NumResults   *int   `json:"numResults,omitempty"`
	Livecrawl    string `json:"livecrawl,omitempty"`
	Type         string `json:"type,omitempty"`
	ContextMaxChars *int `json:"contextMaxCharacters,omitempty"`
}

type WebSearchToolOutput struct {
	Results    []WebSearchResult `json:"results"`
	ResultCount int              `json:"resultCount"`
	Query      string            `json:"query"`
}

type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Score   float64 `json:"score,omitempty"`
}

type exaSearchRequest struct {
	Query              string `json:"query"`
	NumResults         int    `json:"numResults"`
	Contents           map[string]any `json:"contents,omitempty"`
	UseAutoprompt      *bool  `json:"useAutoprompt,omitempty"`
	Livecrawl          string `json:"livecrawl,omitempty"`
	Type               string `json:"type,omitempty"`
}

type exaSearchResponse struct {
	Results []exaResult `json:"results"`
	AutopromptString string `json:"autopromptString,omitempty"`
}

type exaResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"text"`
	Score   float64 `json:"score,omitempty"`
}

func parseWebSearchInput(input any) (*WebSearchToolInput, error) {
	result := &WebSearchToolInput{}

	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}

	result.Query = strings.TrimSpace(result.Query)
	if result.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if result.NumResults == nil || *result.NumResults <= 0 {
		defaultResults := WebSearchDefaultNumResults
		result.NumResults = &defaultResults
	}
	if *result.NumResults > WebSearchMaxNumResults {
		return nil, fmt.Errorf("numResults must not exceed %d", WebSearchMaxNumResults)
	}

	if result.Livecrawl != "" {
		result.Livecrawl = strings.ToLower(result.Livecrawl)
		switch result.Livecrawl {
		case "fallback", "preferred":
		default:
			return nil, fmt.Errorf("invalid livecrawl '%s': must be 'fallback' or 'preferred'", result.Livecrawl)
		}
	}

	if result.Type != "" {
		result.Type = strings.ToLower(result.Type)
		switch result.Type {
		case "auto", "fast", "deep":
		default:
			return nil, fmt.Errorf("invalid type '%s': must be 'auto', 'fast', or 'deep'", result.Type)
		}
	}

	if result.ContextMaxChars != nil && *result.ContextMaxChars <= 0 {
		result.ContextMaxChars = nil
	}

	return result, nil
}

func GetWebSearchToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "websearch",
		DisplayName: "Search the Web",
		Description: "Search the web using Exa AI - performs real-time web searches. Provides up-to-date information for current events, recent data, and general research. Supports configurable result counts, live crawling, and search types. Use this for accessing information beyond knowledge cutoff or when you need current data.",
		ToolLogName: "gen:websearch",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query. Use the current year when searching for recent information.",
				},
				"numResults": map[string]any{
					"type":        "integer",
					"description": "Number of search results to return. Default: 8, max: 20.",
				},
				"livecrawl": map[string]any{
					"type":        "string",
					"enum":        []string{"fallback", "preferred"},
					"description": "Live crawl mode. 'fallback': use live crawling as backup if cached content unavailable, 'preferred': prioritize live crawling. Default: 'fallback'.",
				},
				"type": map[string]any{
					"type":        "string",
					"enum":        []string{"auto", "fast", "deep"},
					"description": "Search type. 'auto': balanced search, 'fast': quick results, 'deep': comprehensive search. Default: 'auto'.",
				},
				"contextMaxCharacters": map[string]any{
					"type":        "integer",
					"description": "Maximum characters for context string optimized for LLMs. Default: 10000.",
				},
			},
			"required":             []string{"query", "numResults", "livecrawl", "type", "contextMaxCharacters"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseWebSearchInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			return fmt.Sprintf("searching for %q", parsed.Query)
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseWebSearchInput(input)
			if err != nil {
				return nil, err
			}
			return executeWebSearch(parsed)
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseWebSearchInput(input)
			return err
		},
	}
}

func executeWebSearch(params *WebSearchToolInput) (*WebSearchToolOutput, error) {
	timeout := WebSearchDefaultTimeout
	if params.ContextMaxChars != nil && *params.ContextMaxChars > 0 {
		timeout = WebSearchDefaultTimeout
	}

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	contents := map[string]any{
		"text": true,
	}
	if params.ContextMaxChars != nil && *params.ContextMaxChars > 0 {
		contents["text_max_characters"] = *params.ContextMaxChars
	}

	reqBody := exaSearchRequest{
		Query:      params.Query,
		NumResults: *params.NumResults,
		Contents:   contents,
	}

	if params.Livecrawl != "" {
		reqBody.Livecrawl = params.Livecrawl
	}
	if params.Type != "" {
		reqBody.Type = params.Type
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", WebSearchExaBaseURL+WebSearchExaSearchEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	apiKey := os.Getenv(WebSearchEnvAPIKey)
	if apiKey == "" {
		secret, exists, err := secretstore.GetSecret("EXA_KEY")
		if err == nil && exists && secret != "" {
			apiKey = secret
		}
	}
	if apiKey == "" {
		return nil, fmt.Errorf("EXA_API_KEY environment variable not set (also checked secret store EXA_KEY)")
	}
	req.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("search API error (status %d): %s", resp.StatusCode, string(body))
	}

	var exaResp exaSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&exaResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	var results []WebSearchResult
	for _, r := range exaResp.Results {
		results = append(results, WebSearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Content,
			Score:   r.Score,
		})
	}

	return &WebSearchToolOutput{
		Results:     results,
		ResultCount: len(results),
		Query:       params.Query,
	}, nil
}
