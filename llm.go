package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseReActResponse parses the LLM response into a ReActResponse struct
func parseReActResponse(input string) (*ReActResponse, error) {
	cleaned := strings.TrimSpace(input)

	first := strings.Index(cleaned, "```json")
	last := strings.LastIndex(cleaned, "```")
	if first == -1 || last == -1 {
		fmt.Printf("\n%s\n", cleaned)
		return nil, fmt.Errorf("no JSON code block found")
	}
	cleaned = cleaned[first+7 : last]

	cleaned = strings.ReplaceAll(cleaned, "\n", "")
	cleaned = strings.TrimSpace(cleaned)

	var reActResp ReActResponse
	if err := json.Unmarshal([]byte(cleaned), &reActResp); err != nil {
		fmt.Printf("\n%s\n", cleaned)
		return nil, err
	}
	return &reActResp, nil
}
