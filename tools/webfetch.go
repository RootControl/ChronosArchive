package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type WebFetchInput struct {
	URL string `json:"url"`
}

func WebFetch(rawInput json.RawMessage) (string, error) {
	var in WebFetchInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("web_fetch: bad input: %w", err)
	}
	if in.URL == "" {
		return "", fmt.Errorf("web_fetch: url is required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(in.URL)
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}
	defer resp.Body.Close()

	const maxBody = 100 * 1024 // 100 KB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", fmt.Errorf("web_fetch: reading body: %w", err)
	}

	out := fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, string(body))
	if int64(len(body)) == maxBody {
		out += "\n[response truncated at 100KB]"
	}
	return out, nil
}
