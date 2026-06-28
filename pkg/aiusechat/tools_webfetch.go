// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
	"golang.org/x/net/html"
)

const (
	WebFetchMaxResponseSize = 5 * 1024 * 1024
	WebFetchDefaultTimeout  = 30
	WebFetchMaxTimeout      = 120
)

type WebFetchToolInput struct {
	Url     string  `json:"url"`
	Format  string  `json:"format,omitempty"`
	Timeout *int    `json:"timeout,omitempty"`
}

type WebFetchToolOutput struct {
	Content string `json:"content"`
	Url     string `json:"url"`
	Format  string `json:"format"`
	Truncated bool `json:"truncated,omitempty"`
}

func parseWebFetchInput(input any) (*WebFetchToolInput, error) {
	result := &WebFetchToolInput{}

	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}

	result.Url = strings.TrimSpace(result.Url)
	if result.Url == "" {
		return nil, fmt.Errorf("url is required")
	}

	parsedUrl, err := url.Parse(result.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsedUrl.Scheme != "http" && parsedUrl.Scheme != "https" {
		return nil, fmt.Errorf("URL must start with http:// or https://")
	}
	if parsedUrl.Host == "" {
		return nil, fmt.Errorf("URL must have a valid host")
	}

	if result.Format == "" {
		result.Format = "markdown"
	}
	result.Format = strings.ToLower(result.Format)
	switch result.Format {
	case "text", "markdown", "html":
	default:
		return nil, fmt.Errorf("invalid format '%s': must be 'text', 'markdown', or 'html'", result.Format)
	}

	if result.Timeout == nil || *result.Timeout <= 0 {
		defaultTimeout := WebFetchDefaultTimeout
		result.Timeout = &defaultTimeout
	}
	if *result.Timeout > WebFetchMaxTimeout {
		return nil, fmt.Errorf("timeout must not exceed %d seconds", WebFetchMaxTimeout)
	}

	return result, nil
}

func GetWebFetchToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "webfetch",
		DisplayName: "Fetch Web Content",
		Description: "Fetch content from a specified URL. Returns the content as text, markdown, or raw HTML. Use this to retrieve web pages, documentation, APIs, or any online resource.",
		ToolLogName: "gen:webfetch",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch content from (must be http:// or https://)",
				},
				"format": map[string]any{
					"type":        "string",
					"enum":        []string{"text", "markdown", "html"},
					"default":     "markdown",
					"description": "The format to return the content in: 'text' (plain text), 'markdown' (converted from HTML), or 'html' (raw HTML)",
				},
				"timeout": map[string]any{
					"type":         "integer",
					"default":      30,
					"description":  "Timeout in seconds (max 120)",
				},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseWebFetchInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			return fmt.Sprintf("fetching %s (format: %s)", parsed.Url, parsed.Format)
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseWebFetchInput(input)
			if err != nil {
				return nil, err
			}
			return fetchWebContent(parsed)
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseWebFetchInput(input)
			return err
		},
	}
}

func fetchWebContent(params *WebFetchToolInput) (*WebFetchToolOutput, error) {
	timeout := time.Duration(*params.Timeout) * time.Second
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", params.Url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	switch params.Format {
	case "markdown":
		req.Header.Set("Accept", "text/markdown;q=1.0, text/x-markdown;q=0.9, text/plain;q=0.8, text/html;q=0.7, */*;q=0.1")
	case "text":
		req.Header.Set("Accept", "text/plain;q=1.0, text/markdown;q=0.9, text/html;q=0.8, */*;q=0.1")
	case "html":
		req.Header.Set("Accept", "text/html;q=1.0, application/xhtml+xml;q=0.9, text/plain;q=0.8, */*;q=0.1")
	default:
		req.Header.Set("Accept", "*/*")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, WebFetchMaxResponseSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	truncated := len(body) > WebFetchMaxResponseSize
	if truncated {
		body = body[:WebFetchMaxResponseSize]
	}

	contentType := resp.Header.Get("Content-Type")
	isHTML := strings.Contains(contentType, "text/html")

	var content string
	switch params.Format {
	case "markdown":
		if isHTML {
			content = htmlToText(string(body))
		} else {
			content = string(body)
		}
	case "text":
		if isHTML {
			content = htmlToText(string(body))
		} else {
			content = string(body)
		}
	case "html":
		content = string(body)
	default:
		content = string(body)
	}

	if truncated {
		content += "\n\n[Response truncated: exceeded maximum size]"
	}

	return &WebFetchToolOutput{
		Content:   content,
		Url:       params.Url,
		Format:    params.Format,
		Truncated: truncated,
	}, nil
}

func htmlToText(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}
	return extractText(doc, true)
}

func extractText(n *html.Node, topLevel bool) string {
	var buf strings.Builder

	var skipTag bool
	if n.Type == html.ElementNode {
		tag := strings.ToLower(n.Data)
		switch tag {
		case "script", "style", "noscript", "iframe", "object", "embed", "svg", "canvas":
			skipTag = true
		}
	}

	if !skipTag {
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				if buf.Len() > 0 {
					buf.WriteByte(' ')
				}
				buf.WriteString(text)
			}
		}

		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			switch tag {
			case "br":
				buf.WriteString("\n")
			case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "li", "blockquote", "section", "article", "nav", "header", "footer", "details", "summary":
				if buf.Len() > 0 {
					buf.WriteString("\n\n")
				}
			case "td", "th":
				buf.WriteString(" | ")
			case "a":
				// anchor text handled by children
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			childText := extractText(c, false)
			buf.WriteString(childText)
		}

		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "a":
				// check for href attribute
				for _, attr := range n.Attr {
					if attr.Key == "href" && attr.Val != "" && !strings.HasPrefix(attr.Val, "#") {
						// Only append URL if there was visible text
						innerText := strings.TrimSpace(collectInnerText(n))
						if innerText != "" {
							buf.WriteString(fmt.Sprintf(" (%s)", attr.Val))
						}
						break
					}
				}
			}
		}
	}

	result := buf.String()
	result = collapseWhitespace(result)
	return strings.TrimSpace(result)
}

func collectInnerText(n *html.Node) string {
	var buf strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			buf.WriteString(c.Data)
		}
		if c.Type == html.ElementNode {
			buf.WriteString(collectInnerText(c))
		}
	}
	return strings.TrimSpace(buf.String())
}

func collapseWhitespace(s string) string {
	var buf strings.Builder
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				buf.WriteByte(' ')
				inSpace = true
			}
		} else {
			buf.WriteRune(r)
			inSpace = false
		}
	}
	return buf.String()
}
