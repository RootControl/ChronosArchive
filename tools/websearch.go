package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type webSearchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// ddgResult is one item from the DuckDuckGo Instant Answer API.
type ddgResult struct {
	Text     string `json:"Text"`
	FirstURL string `json:"FirstURL"`
}

// ddgResponse is the top-level DuckDuckGo Instant Answer API response.
type ddgResponse struct {
	AbstractText   string      `json:"AbstractText"`
	AbstractURL    string      `json:"AbstractURL"`
	AbstractSource string      `json:"AbstractSource"`
	RelatedTopics  []ddgResult `json:"RelatedTopics"`
	Answer         string      `json:"Answer"`
}

var webSearchClient = &http.Client{Timeout: 15 * time.Second}

// WebSearch queries the DuckDuckGo Instant Answer API (no API key required)
// and returns a formatted list of results.
func WebSearch(rawInput json.RawMessage) (string, error) {
	var in webSearchInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("web_search: bad input: %w", err)
	}
	if in.Query == "" {
		return "", fmt.Errorf("web_search: query is required")
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 5
	}

	apiURL := "https://api.duckduckgo.com/?q=" + url.QueryEscape(in.Query) +
		"&format=json&no_html=1&no_redirect=1&skip_disambig=1"

	resp, err := webSearchClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("web_search: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", fmt.Errorf("web_search: reading response: %w", err)
	}

	var ddg ddgResponse
	if err := json.Unmarshal(body, &ddg); err != nil {
		return "", fmt.Errorf("web_search: parsing response: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Search results for: %q\n\n", in.Query)

	if ddg.Answer != "" {
		fmt.Fprintf(&sb, "Answer: %s\n\n", ddg.Answer)
	}

	if ddg.AbstractText != "" {
		fmt.Fprintf(&sb, "Summary (%s): %s\nURL: %s\n\n", ddg.AbstractSource, ddg.AbstractText, ddg.AbstractURL)
	}

	count := 0
	for _, r := range ddg.RelatedTopics {
		if r.Text == "" || r.FirstURL == "" {
			continue
		}
		fmt.Fprintf(&sb, "%d. %s\n   %s\n", count+1, r.Text, r.FirstURL)
		count++
		if count >= in.MaxResults {
			break
		}
	}

	if count == 0 && ddg.AbstractText == "" && ddg.Answer == "" {
		sb.WriteString("No results found. Try web_fetch with a specific URL or a different query.")
	}

	return sb.String(), nil
}
