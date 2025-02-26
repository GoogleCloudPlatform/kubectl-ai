package gollm

// We define some standard structs to allow for persistence of the LLM requests and responses.
// This lets us store the history of the conversation for later analysis.

type RecordCompletionResponse struct {
	Text string `json:"text"`
	Raw  any    `json:"raw"`
}
