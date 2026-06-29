// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseWebFetchInput_ValidURL(t *testing.T) {
	input := map[string]any{
		"url": "https://example.com",
	}
	result, err := parseWebFetchInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Url != "https://example.com" {
		t.Errorf("url got=%q want=%q", result.Url, "https://example.com")
	}
	if result.Format != "markdown" {
		t.Errorf("format got=%q want=%q", result.Format, "markdown")
	}
	if *result.Timeout != WebFetchDefaultTimeout {
		t.Errorf("timeout got=%d want=%d", *result.Timeout, WebFetchDefaultTimeout)
	}
}

func TestParseWebFetchInput_NilInput(t *testing.T) {
	_, err := parseWebFetchInput(nil)
	if err == nil {
		t.Errorf("expected error for nil input")
	}
}

func TestParseWebFetchInput_EmptyURL(t *testing.T) {
	input := map[string]any{
		"url": "   ",
	}
	_, err := parseWebFetchInput(input)
	if err == nil {
		t.Errorf("expected error for empty URL")
	}
}

func TestParseWebFetchInput_InvalidScheme(t *testing.T) {
	input := map[string]any{
		"url": "ftp://example.com",
	}
	_, err := parseWebFetchInput(input)
	if err == nil {
		t.Errorf("expected error for invalid scheme")
	}
}

func TestParseWebFetchInput_MissingHost(t *testing.T) {
	input := map[string]any{
		"url": "https://",
	}
	_, err := parseWebFetchInput(input)
	if err == nil {
		t.Errorf("expected error for missing host")
	}
}

func TestParseWebFetchInput_ValidFormats(t *testing.T) {
	for _, format := range []string{"text", "markdown", "html"} {
		input := map[string]any{
			"url":    "https://example.com",
			"format": format,
		}
		result, err := parseWebFetchInput(input)
		if err != nil {
			t.Fatalf("unexpected error for format %q: %v", format, err)
		}
		if result.Format != format {
			t.Errorf("format got=%q want=%q", result.Format, format)
		}
	}
}

func TestParseWebFetchInput_InvalidFormat(t *testing.T) {
	input := map[string]any{
		"url":    "https://example.com",
		"format": "xml",
	}
	_, err := parseWebFetchInput(input)
	if err == nil {
		t.Errorf("expected error for invalid format")
	}
}

func TestParseWebFetchInput_ExplicitTimeout(t *testing.T) {
	timeout := 15
	input := map[string]any{
		"url":     "https://example.com",
		"timeout": timeout,
	}
	result, err := parseWebFetchInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *result.Timeout != timeout {
		t.Errorf("timeout got=%d want=%d", *result.Timeout, timeout)
	}
}

func TestParseWebFetchInput_TimeoutTooLarge(t *testing.T) {
	input := map[string]any{
		"url":     "https://example.com",
		"timeout": WebFetchMaxTimeout + 1,
	}
	_, err := parseWebFetchInput(input)
	if err == nil {
		t.Errorf("expected error for timeout exceeding max")
	}
}

func TestParseWebFetchInput_FormatCaseInsensitive(t *testing.T) {
	input := map[string]any{
		"url":    "https://example.com",
		"format": "MarkDown",
	}
	result, err := parseWebFetchInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Format != "markdown" {
		t.Errorf("format got=%q want=%q", result.Format, "markdown")
	}
}

func TestGetWebFetchToolDefinition(t *testing.T) {
	def := GetWebFetchToolDefinition()
	if def.Name != "webfetch" {
		t.Errorf("name got=%q want=%q", def.Name, "webfetch")
	}
	if !def.Strict {
		t.Errorf("expected strict=true")
	}
	schema := def.InputSchema
	if schema == nil {
		t.Fatal("input schema is nil")
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not a string slice")
	}
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r] = true
	}
	for _, prop := range []string{"url", "format", "timeout"} {
		if !requiredSet[prop] {
			t.Errorf("property %q not in required array", prop)
		}
	}
	if schema["additionalProperties"] != false {
		t.Errorf("expected additionalProperties=false")
	}
}

func TestFetchWebContent_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><h1>Hello World</h1></body></html>"))
	}))
	defer mockServer.Close()

	timeout := 10
	params := &WebFetchToolInput{
		Url:     mockServer.URL,
		Format:  "markdown",
		Timeout: &timeout,
	}

	output, err := fetchWebContent(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output.Content, "Hello World") {
		t.Errorf("content missing expected text: %q", output.Content)
	}
	if output.Url != mockServer.URL {
		t.Errorf("url got=%q want=%q", output.Url, mockServer.URL)
	}
	if output.Format != "markdown" {
		t.Errorf("format got=%q want=%q", output.Format, "markdown")
	}
	if output.Truncated {
		t.Errorf("expected truncated=false")
	}
}

func TestFetchWebContent_HTMLFormat(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><p>Raw HTML</p></body></html>"))
	}))
	defer mockServer.Close()

	timeout := 10
	params := &WebFetchToolInput{
		Url:     mockServer.URL,
		Format:  "html",
		Timeout: &timeout,
	}

	output, err := fetchWebContent(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output.Content, "<p>Raw HTML</p>") {
		t.Errorf("expected raw HTML in output: %q", output.Content)
	}
}

func TestFetchWebContent_TextFormat(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("plain text content"))
	}))
	defer mockServer.Close()

	timeout := 10
	params := &WebFetchToolInput{
		Url:     mockServer.URL,
		Format:  "text",
		Timeout: &timeout,
	}

	output, err := fetchWebContent(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output.Content, "plain text content") {
		t.Errorf("content got=%q", output.Content)
	}
}

func TestFetchWebContent_ServerError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	timeout := 10
	params := &WebFetchToolInput{
		Url:     mockServer.URL,
		Format:  "markdown",
		Timeout: &timeout,
	}

	_, err := fetchWebContent(params)
	if err == nil {
		t.Errorf("expected error for server error")
	}
}

func TestFetchWebContent_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	timeout := 10
	params := &WebFetchToolInput{
		Url:     mockServer.URL,
		Format:  "text",
		Timeout: &timeout,
	}

	_, err := fetchWebContent(params)
	if err == nil {
		t.Errorf("expected error for 404")
	}
}

func TestFetchWebContent_Truncation(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(strings.Repeat("x", WebFetchMaxResponseSize+100)))
	}))
	defer mockServer.Close()

	timeout := 10
	params := &WebFetchToolInput{
		Url:     mockServer.URL,
		Format:  "text",
		Timeout: &timeout,
	}

	output, err := fetchWebContent(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Truncated {
		t.Errorf("expected truncated=true")
	}
	if len(output.Content) > WebFetchMaxResponseSize+100 {
		t.Errorf("content too large: %d", len(output.Content))
	}
}

func TestFetchWebContent_HTMLExtraction(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body>
			<h1>Title</h1>
			<p>Paragraph text.</p>
			<a href="https://example.com/link">click here</a>
			<script>alert("xss")</script>
			<style>.hidden{}</style>
		</body></html>`))
	}))
	defer mockServer.Close()

	timeout := 10
	params := &WebFetchToolInput{
		Url:     mockServer.URL,
		Format:  "markdown",
		Timeout: &timeout,
	}

	output, err := fetchWebContent(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output.Content, "Title") {
		t.Errorf("missing Title in: %q", output.Content)
	}
	if !strings.Contains(output.Content, "Paragraph text") {
		t.Errorf("missing paragraph text in: %q", output.Content)
	}
	if strings.Contains(output.Content, "alert(\"xss\")") {
		t.Errorf("script content should be stripped")
	}
	if strings.Contains(output.Content, ".hidden{}") {
		t.Errorf("style content should be stripped")
	}
	if !strings.Contains(output.Content, "https://example.com/link") {
		t.Errorf("link URL should appear in output: %q", output.Content)
	}
}

func TestFetchWebContent_RedirectLimit(t *testing.T) {
	redirectCount := 0
	var mockServer *httptest.Server
	mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount < 15 {
			http.Redirect(w, r, mockServer.URL, http.StatusFound)
			return
		}
		w.Write([]byte("finally here"))
	}))
	defer mockServer.Close()

	timeout := 10
	params := &WebFetchToolInput{
		Url:     mockServer.URL,
		Format:  "text",
		Timeout: &timeout,
	}

	_, err := fetchWebContent(params)
	if err == nil {
		t.Errorf("expected error for too many redirects")
	}
}
