package gollm

import (
	"context"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	_, err := NewClient(context.Background(), "gemini")
	if err == nil || err.Error() != "GEMINI_API_KEY environment variable not set" {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, err = NewClient(context.Background(), "invalid")
	if err == nil || !strings.Contains(err.Error(), "provider \"invalid\" not registered") {
		t.Fatalf("Unexpected error: %v", err)
	}
}
