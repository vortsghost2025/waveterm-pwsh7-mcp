// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestParseWebSearchInput_ValidQuery(t *testing.T) {
	input := map[string]any{
		"query": "test query",
	}
	result, err := parseWebSearchInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Query != "test query" {
		t.Errorf("query got=%q want=%q", result.Query, "test query")
	}
	if *result.NumResults != WebSearchDefaultNumResults {
		t.Errorf("numResults got=%d want=%d", *result.NumResults, WebSearchDefaultNumResults)
	}
}

func TestParseWebSearchInput_NilInput(t *testing.T) {
	_, err := parseWebSearchInput(nil)
	if err == nil {
		t.Errorf("expected error for nil input")
	}
}

func TestParseWebSearchInput_EmptyQuery(t *testing.T) {
	input := map[string]any{
		"query": "   ",
	}
	_, err := parseWebSearchInput(input)
	if err == nil {
		t.Errorf("expected error for empty query")
	}
}

func TestParseWebSearchInput_MaxResults(t *testing.T) {
	input := map[string]any{
		"query":      "test",
		"numResults": WebSearchMaxNumResults,
	}
	result, err := parseWebSearchInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *result.NumResults != WebSearchMaxNumResults {
		t.Errorf("numResults got=%d want=%d", *result.NumResults, WebSearchMaxNumResults)
	}
}

func TestParseWebSearchInput_TooManyResults(t *testing.T) {
	input := map[string]any{
		"query":      "test",
		"numResults": WebSearchMaxNumResults + 1,
	}
	_, err := parseWebSearchInput(input)
	if err == nil {
		t.Errorf("expected error for too many results")
	}
}

func TestParseWebSearchInput_InvalidLivecrawl(t *testing.T) {
	input := map[string]any{
		"query":     "test",
		"livecrawl": "invalid",
	}
	_, err := parseWebSearchInput(input)
	if err == nil {
		t.Errorf("expected error for invalid livecrawl")
	}
}

func TestParseWebSearchInput_ValidLivecrawl(t *testing.T) {
	for _, lc := range []string{"fallback", "preferred"} {
		input := map[string]any{
			"query":     "test",
			"livecrawl": lc,
		}
		result, err := parseWebSearchInput(input)
		if err != nil {
			t.Fatalf("unexpected error for livecrawl=%q: %v", lc, err)
		}
		if result.Livecrawl != lc {
			t.Errorf("livecrawl got=%q want=%q", result.Livecrawl, lc)
		}
	}
}

func TestParseWebSearchInput_InvalidType(t *testing.T) {
	input := map[string]any{
		"query": "test",
		"type":  "invalid",
	}
	_, err := parseWebSearchInput(input)
	if err == nil {
		t.Errorf("expected error for invalid type")
	}
}

func TestParseWebSearchInput_ValidTypes(t *testing.T) {
	for _, ty := range []string{"auto", "fast", "deep"} {
		input := map[string]any{
			"query": "test",
			"type":  ty,
		}
		result, err := parseWebSearchInput(input)
		if err != nil {
			t.Fatalf("unexpected error for type=%q: %v", ty, err)
		}
		if result.Type != ty {
			t.Errorf("type got=%q want=%q", result.Type, ty)
		}
	}
}

func TestParseWebSearchInput_NegativeContextMaxChars(t *testing.T) {
	input := map[string]any{
		"query":                "test",
		"contextMaxCharacters": -1,
	}
	result, err := parseWebSearchInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContextMaxChars != nil {
		t.Errorf("contextMaxChars should be nil for negative value, got=%v", *result.ContextMaxChars)
	}
}

func TestGetWebSearchToolDefinition(t *testing.T) {
	tool := GetWebSearchToolDefinition()
	if tool.Name != "websearch" {
		t.Errorf("name got=%q want=%q", tool.Name, "websearch")
	}
	if tool.DisplayName != "Search the Web" {
		t.Errorf("displayName got=%q want=%q", tool.DisplayName, "Search the Web")
	}
	if tool.ToolLogName != "gen:websearch" {
		t.Errorf("toolLogName got=%q want=%q", tool.ToolLogName, "gen:websearch")
	}
	if !tool.Strict {
		t.Errorf("expected strict=true")
	}
	if tool.ToolAnyCallback == nil {
		t.Errorf("expected ToolAnyCallback to be set")
	}
	if tool.ToolCallDesc == nil {
		t.Errorf("expected ToolCallDesc to be set")
	}
	if tool.ToolApproval == nil {
		t.Errorf("expected ToolApproval to be set")
	}
	if tool.ToolVerifyInput == nil {
		t.Errorf("expected ToolVerifyInput to be set")
	}
	if tool.InputSchema == nil {
		t.Errorf("expected InputSchema to be set")
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}
	if _, ok := props["query"]; !ok {
		t.Errorf("expected 'query' property in schema")
	}
	if _, ok := props["numResults"]; !ok {
		t.Errorf("expected 'numResults' property in schema")
	}
	if _, ok := props["livecrawl"]; !ok {
		t.Errorf("expected 'livecrawl' property in schema")
	}
	if _, ok := props["type"]; !ok {
		t.Errorf("expected 'type' property in schema")
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok {
		t.Fatalf("expected required to be []string")
	}
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("required got=%v want=[query]", required)
	}
}

func TestExecuteWebSearch_Success(t *testing.T) {
	var receivedQuery string
	var receivedNumResults int
	var receivedLivecrawl string
	var receivedType string
	var receivedAPIKey string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != WebSearchExaSearchEndpoint {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		receivedAPIKey = r.Header.Get("x-api-key")
		if receivedAPIKey != "test-api-key" {
			t.Errorf("expected api key 'test-api-key', got %q", receivedAPIKey)
		}

		var reqBody exaSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		receivedQuery = reqBody.Query
		receivedNumResults = reqBody.NumResults
		receivedLivecrawl = reqBody.Livecrawl
		receivedType = reqBody.Type

		resp := exaSearchResponse{
			Results: []exaResult{
				{
					Title:   "Test Result",
					URL:     "https://example.com",
					Content: "Test content",
					Score:   0.95,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	origBaseURL := WebSearchExaBaseURL
	WebSearchExaBaseURL = mockServer.URL
	defer func() { WebSearchExaBaseURL = origBaseURL }()

	origEnvKey := WebSearchEnvAPIKey
	WebSearchEnvAPIKey = "TEST_API_KEY"
	defer func() { WebSearchEnvAPIKey = origEnvKey }()

	os.Setenv("TEST_API_KEY", "test-api-key")
	defer os.Unsetenv("TEST_API_KEY")

	params := &WebSearchToolInput{
		Query:      "test query",
		NumResults: intPtr(5),
		Livecrawl:  "preferred",
		Type:       "deep",
	}

	output, err := executeWebSearch(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatalf("expected non-nil output")
	}
	if output.Query != "test query" {
		t.Errorf("query got=%q want=%q", output.Query, "test query")
	}
	if output.ResultCount != 1 {
		t.Errorf("resultCount got=%d want=%d", output.ResultCount, 1)
	}
	if len(output.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Results))
	}
	if output.Results[0].Title != "Test Result" {
		t.Errorf("title got=%q want=%q", output.Results[0].Title, "Test Result")
	}
	if output.Results[0].URL != "https://example.com" {
		t.Errorf("url got=%q want=%q", output.Results[0].URL, "https://example.com")
	}
	if output.Results[0].Content != "Test content" {
		t.Errorf("content got=%q want=%q", output.Results[0].Content, "Test content")
	}
	if output.Results[0].Score != 0.95 {
		t.Errorf("score got=%v want=%v", output.Results[0].Score, 0.95)
	}

	if receivedQuery != "test query" {
		t.Errorf("received query got=%q want=%q", receivedQuery, "test query")
	}
	if receivedNumResults != 5 {
		t.Errorf("received numResults got=%d want=%d", receivedNumResults, 5)
	}
	if receivedLivecrawl != "preferred" {
		t.Errorf("received livecrawl got=%q want=%q", receivedLivecrawl, "preferred")
	}
	if receivedType != "deep" {
		t.Errorf("received type got=%q want=%q", receivedType, "deep")
	}
}

func TestExecuteWebSearch_MissingAPIKey(t *testing.T) {
	params := &WebSearchToolInput{
		Query:      "test query",
		NumResults: intPtr(5),
	}

	_, err := executeWebSearch(params)
	if err == nil {
		t.Errorf("expected error for missing API key")
	}
}

func TestExecuteWebSearch_ServerError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer mockServer.Close()

	origBaseURL := WebSearchExaBaseURL
	WebSearchExaBaseURL = mockServer.URL
	defer func() { WebSearchExaBaseURL = origBaseURL }()

	os.Setenv("TEST_API_KEY", "test-api-key")
	defer os.Unsetenv("TEST_API_KEY")

	params := &WebSearchToolInput{
		Query:      "test query",
		NumResults: intPtr(5),
	}

	_, err := executeWebSearch(params)
	if err == nil {
		t.Errorf("expected error for server error")
	}
}

func TestExecuteWebSearch_InvalidJSONResponse(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer mockServer.Close()

	origBaseURL := WebSearchExaBaseURL
	WebSearchExaBaseURL = mockServer.URL
	defer func() { WebSearchExaBaseURL = origBaseURL }()

	os.Setenv("TEST_API_KEY", "test-api-key")
	defer os.Unsetenv("TEST_API_KEY")

	params := &WebSearchToolInput{
		Query:      "test query",
		NumResults: intPtr(5),
	}

	_, err := executeWebSearch(params)
	if err == nil {
		t.Errorf("expected error for invalid JSON response")
	}
}

