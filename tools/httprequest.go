package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPRequestInput struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func HTTPRequest(rawInput json.RawMessage) (string, error) {
	var in HTTPRequestInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("http_request: bad input: %w", err)
	}
	if in.URL == "" {
		return "", fmt.Errorf("http_request: url is required")
	}
	if in.Method == "" {
		in.Method = "GET"
	}

	var bodyReader io.Reader
	if in.Body != "" {
		bodyReader = bytes.NewBufferString(in.Body)
	}

	req, err := http.NewRequest(in.Method, in.URL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("http_request: creating request: %w", err)
	}
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http_request: %w", err)
	}
	defer resp.Body.Close()

	const maxBody = 100 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", fmt.Errorf("http_request: reading body: %w", err)
	}

	// Include response headers in output.
	var headerLines string
	for k, vals := range resp.Header {
		for _, v := range vals {
			headerLines += fmt.Sprintf("%s: %s\n", k, v)
		}
	}

	out := fmt.Sprintf("HTTP %d\n%s\n%s", resp.StatusCode, headerLines, string(body))
	if int64(len(body)) == maxBody {
		out += "\n[response truncated at 100KB]"
	}
	return out, nil
}
