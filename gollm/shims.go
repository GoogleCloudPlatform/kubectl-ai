package gollm

// // singletonIterator implements the ChatResponseIterator for an LLM that does not support streaming.
// type singletonChatResponseIterator struct {
// 	response ChatResponse
// }

// var _ ChatResponseIterator = &singletonChatResponseIterator{}

// func (i *singletonChatResponseIterator) Next() (ChatResponse, error) {
// 	response := i.response
// 	i.response = nil
// 	return response, nil
// }

func singletonChatResponseIterator(response ChatResponse) ChatResponseIterator {
	return func(yield func(ChatResponse, error) bool) {
		if !yield(response, nil) {
			return
		}
	}
}
