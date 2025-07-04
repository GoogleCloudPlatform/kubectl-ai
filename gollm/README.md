# gollm

A Go library for calling into multiple Large Language Model (LLM) providers with a unified interface.

This library is intended for use by kubectl-ai, but may prove useful for other similar go tools in future.

Note that the library is still evolving and will likely make incompatible changes often.  We are focusing on kubectl-ai's use-case,
but will consider changes to support additional use-cases.

## Overview

gollm provides a consistent API for interacting with various LLM providers, making it easy to switch between different models and services without changing your application code. The library supports both chat-based conversations and single completions, with features like function calling, streaming responses, and retry logic.

## Features

- **Multi-provider support**: OpenAI, Azure OpenAI, Google Gemini, Ollama, LlamaCPP, Grok, and more
- **Unified interface**: Consistent API across all providers
- **Chat conversations**: Multi-turn conversations with conversation history
- **Function calling**: Define and use custom functions with LLMs
- **Streaming support**: Real-time streaming responses
- **Retry logic**: Built-in retry mechanisms with configurable backoff
- **Response schemas**: Constrain LLM responses to specific JSON schemas
- **SSL configuration**: Optional SSL certificate verification skipping
- **Environment-based configuration**: Easy setup via environment variables

## Providers

| Provider | ID | Description |
|----------|----|-------------|
| OpenAI | `openai://` | OpenAI's GPT models |
| Azure OpenAI | `azopenai://` | Microsoft Azure's OpenAI service |
| Google Gemini | `gemini://` | Google's Gemini models |
| Vertex AI | `vertexai://` | Google Cloud Vertex AI (via Gemini) |
| Ollama | `ollama://` | Local Ollama models |
| LlamaCPP | `llamacpp://` | Local LlamaCPP models |
| Grok | `grok://` | xAI's Grok models |

## Quick Start

### Installation

```bash
go get github.com/GoogleCloudPlatform/kubectl-ai/gollm
```

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

func main() {
    ctx := context.Background()
    
    // Create a client using environment variable
    client, err := gollm.NewClient(ctx, "")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()
    
    // Start a chat conversation
    chat := client.StartChat("You are a helpful assistant.", "gpt-3.5-turbo")
    
    // Send a message
    response, err := chat.Send(ctx, "Hello, how are you?")
    if err != nil {
        log.Fatal(err)
    }
    
    // Print the response
    for _, candidate := range response.Candidates() {
        fmt.Println(candidate.String())
    }
}
```

### Environment Configuration

Set the `LLM_CLIENT` environment variable to specify your preferred provider:

```bash
# OpenAI
export LLM_CLIENT="openai://api.openai.com"
export OPENAI_API_KEY="your-api-key"

# Azure OpenAI
export LLM_CLIENT="azopenai://your-resource.openai.azure.com"
export AZURE_OPENAI_API_KEY="your-api-key"

# Google Gemini
export LLM_CLIENT="gemini://generativelanguage.googleapis.com"
export GOOGLE_API_KEY="your-api-key"

# Ollama (local)
export LLM_CLIENT="ollama://localhost:11434"
```


## Examples

### Single Completion

```go
ctx := context.Background()
client, err := gollm.NewClient(ctx, "openai://api.openai.com")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

req := &gollm.CompletionRequest{
    Model:  "gpt-3.5-turbo",
    Prompt: "Write a short poem about programming",
}

response, err := client.GenerateCompletion(ctx, req)
if err != nil {
    log.Fatal(err)
}

fmt.Println(response.Response())
```

### Streaming Chat

```go
ctx := context.Background()
client, err := gollm.NewClient(ctx, "openai://api.openai.com")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

chat := client.StartChat("You are a helpful assistant.", "gpt-3.5-turbo")

// Send a streaming message
iterator, err := chat.SendStreaming(ctx, "Tell me a story about a robot")
if err != nil {
    log.Fatal(err)
}

// Process streaming response
for response := range iterator {
    if response.V1 != nil {
        for _, candidate := range response.V1.Candidates() {
            for _, part := range candidate.Parts() {
                if text, ok := part.AsText(); ok {
                    fmt.Print(text)
                }
            }
        }
    }
    if response.V2 != nil {
        // Handle error
        log.Printf("Error: %v", response.V2)
        break
    }
}
```

### Function Calling

```go
// Define a function that the LLM can call
functionDef := &gollm.FunctionDefinition{
    Name:        "get_weather",
    Description: "Get the current weather for a location",
    Parameters: &gollm.Schema{
        Type: gollm.TypeObject,
        Properties: map[string]*gollm.Schema{
            "location": {
                Type:        gollm.TypeString,
                Description: "The city and state, e.g. San Francisco, CA",
            },
            "unit": {
                Type:        gollm.TypeString,
                Description: "The temperature unit to use. Infer this from the user's location.",
                Required:    []string{"location"},
            },
        },
    },
}

chat := client.StartChat("You are a helpful assistant.", "gpt-3.5-turbo")
chat.SetFunctionDefinitions([]*gollm.FunctionDefinition{functionDef})

response, err := chat.Send(ctx, "What's the weather like in San Francisco?")
if err != nil {
    log.Fatal(err)
}

// Check for function calls in the response
for _, candidate := range response.Candidates() {
    for _, part := range candidate.Parts() {
        if functionCalls, ok := part.AsFunctionCalls(); ok {
            for _, call := range functionCalls {
                fmt.Printf("Function call: %s with args %v\n", call.Name, call.Arguments)
                
                // Execute the function and send the result back
                result := executeWeatherFunction(call.Arguments)
                chat.Send(ctx, gollm.FunctionCallResult{
                    ID:     call.ID,
                    Name:   call.Name,
                    Result: result,
                })
            }
        }
    }
}
```

### Response Schema Constraints

```go
// Define a schema for structured responses
schema := &gollm.Schema{
    Type: gollm.TypeObject,
    Properties: map[string]*gollm.Schema{
        "name": {
            Type:        gollm.TypeString,
            Description: "The person's name",
        },
        "age": {
            Type:        gollm.TypeInteger,
            Description: "The person's age",
        },
        "interests": {
            Type: gollm.TypeArray,
            Items: &gollm.Schema{
                Type: gollm.TypeString,
            },
            Description: "List of interests",
        },
    },
    Required: []string{"name", "age"},
}

client.SetResponseSchema(schema)

// Now all responses will be constrained to match this schema
response, err := chat.Send(ctx, "Tell me about a person named Alice who is 30 years old")
```

### Retry Logic

```go
// Configure retry behavior
retryConfig := gollm.RetryConfig{
    MaxAttempts:    3,
    InitialBackoff: time.Second,
    MaxBackoff:     30 * time.Second,
    BackoffFactor:  2.0,
    Jitter:         true,
}

// Create a chat with retry logic
chat := client.StartChat("You are a helpful assistant.", "gpt-3.5-turbo")
retryChat := gollm.NewRetryChat(chat, retryConfig)

// Use the retry chat - it will automatically retry on retryable errors
response, err := retryChat.Send(ctx, "Hello!")
```

### Building Schemas from Go Types

```go
type Person struct {
    Name     string   `json:"name"`
    Age      int      `json:"age"`
    Interests []string `json:"interests,omitempty"`
}

// Automatically build a schema from a Go struct
schema := gollm.BuildSchemaFor(reflect.TypeOf(Person{}))

// Use the schema to constrain responses
client.SetResponseSchema(schema)
```

## Configuration Options

### Client Options

```go
// Create a client with custom options
client, err := gollm.NewClient(ctx, "openai://api.openai.com",
    gollm.WithSkipVerifySSL(), // Skip SSL verification (for development)
)
```

### Environment Variables

- `LLM_CLIENT`: The provider URL to use (e.g., "openai://api.openai.com")
- `LLM_SKIP_VERIFY_SSL`: Set to "1" or "true" to skip SSL certificate verification
- Provider-specific API keys (e.g., `OPENAI_API_KEY`, `GOOGLE_API_KEY`)

## Error Handling

The library provides structured error handling with retryable error detection:

```go
var apiErr *gollm.APIError
if errors.As(err, &apiErr) {
    fmt.Printf("API Error: Status=%d, Message=%s\n", apiErr.StatusCode, apiErr.Message)
}

// Check if an error is retryable
if chat.IsRetryableError(err) {
    // Implement retry logic
}
```

## Adding a provider

To add a new provider:

1. Create a new file (e.g., `myprovider.go`)
2. Implement the `Client` interface
3. Register the provider in an `init()` function:

```go
func init() {
    if err := gollm.RegisterProvider("myprovider", myProviderFactory); err != nil {
        panic(err)
    }
}
```

## License

This project is licensed under the Apache License, Version 2.0. See the LICENSE file for details.
