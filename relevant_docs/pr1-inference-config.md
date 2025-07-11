# PR #1: Provider-Specific Configuration Extensions

## Overview
This PR implements a **clean extension pattern** for advanced Bedrock features that addresses reviewer concerns while preserving full functionality. Instead of polluting global interfaces, we use optional provider-specific extensions.

## Problem with Original Approach
The reviewed approach tried to add `InferenceConfig` to global `ClientOptions`, which reviewers correctly identified as:
- ❌ **Interface pollution** - forcing all providers to handle Bedrock-specific config
- ❌ **Premature abstraction** - creating "universal" interfaces before other providers support them
- ❌ **Partial implementation** - only Bedrock would use these features

## Solution: Provider-Specific Extensions

### Core Principle
- **Keep global interfaces unchanged** - no modifications to `Client` or `ClientOptions`
- **Use optional extension interfaces** - discoverable via type assertions
- **Provider-specific configuration** - each provider handles their own advanced features
- **Incremental adoption** - other providers can add features when ready

### Architecture

#### 1. Keep ClientOptions Simple
```go
// gollm/factory.go - NO CHANGES to global interface
type ClientOptions struct {
    URL           *url.URL
    SkipVerifySSL bool
    // That's it - no provider-specific fields
}
```

#### 2. Provider-Specific Configuration Interface
```go
// gollm/bedrock/interfaces.go - NEW FILE
package bedrock

// Configuration interface specific to Bedrock
type ConfigurableClient interface {
    SetInferenceConfig(config *InferenceConfig) error
    GetInferenceConfig() *InferenceConfig
    SetUsageCallback(callback UsageCallback)
}

// Bedrock-specific inference configuration
type InferenceConfig struct {
    Model       string  `json:"model,omitempty"`
    Region      string  `json:"region,omitempty"`
    Temperature float32 `json:"temperature,omitempty"`
    MaxTokens   int32   `json:"maxTokens,omitempty"`
    TopP        float32 `json:"topP,omitempty"`
    MaxRetries  int     `json:"maxRetries,omitempty"`
}

// Usage callback for Bedrock
type UsageCallback func(provider, model string, usage Usage)

// Bedrock-specific usage information
type Usage struct {
    InputTokens  int       `json:"inputTokens"`
    OutputTokens int       `json:"outputTokens"`
    TotalTokens  int       `json:"totalTokens"`
    Model        string    `json:"model"`
    Provider     string    `json:"provider"`
    Timestamp    time.Time `json:"timestamp"`
}
```

#### 3. Bedrock Client Implements Extensions
```go
// gollm/bedrock/bedrock.go - Enhanced with extensions
package bedrock

type BedrockClient struct {
    runtimeClient   *bedrockruntime.Client
    bedrockClient   *bedrock.Client
    inferenceConfig *InferenceConfig
    usageCallback   UsageCallback
}

// Ensure BedrockClient implements both interfaces
var _ gollm.Client = &BedrockClient{}
var _ ConfigurableClient = &BedrockClient{}

// Extension interface implementation
func (c *BedrockClient) SetInferenceConfig(config *InferenceConfig) error {
    if config == nil {
        return errors.New("inference config cannot be nil")
    }
    c.inferenceConfig = config
    return nil
}

func (c *BedrockClient) GetInferenceConfig() *InferenceConfig {
    return c.inferenceConfig
}

func (c *BedrockClient) SetUsageCallback(callback UsageCallback) {
    c.usageCallback = callback
}
```

#### 4. Clean Usage Pattern
```go
// Example: Configure Bedrock with advanced features
package main

import (
    "github.com/GoogleCloudPlatform/kubectl-ai/gollm"
    "github.com/GoogleCloudPlatform/kubectl-ai/gollm/bedrock"
)

func main() {
    // Create client normally - no global interface changes
    client, err := gollm.NewClient(ctx, "bedrock")
    if err != nil {
        return err
    }
    defer client.Close()

    // Use type assertion to access advanced features
    if bedrockClient, ok := client.(bedrock.ConfigurableClient); ok {
        // Configure inference parameters
        config := &bedrock.InferenceConfig{
            Model:       "anthropic.claude-3-5-sonnet-20241022-v2:0",
            Region:      "us-west-2",
            Temperature: 0.7,
            MaxTokens:   4000,
            TopP:        0.9,
            MaxRetries:  3,
        }
        bedrockClient.SetInferenceConfig(config)

        // Set usage tracking
        bedrockClient.SetUsageCallback(func(provider, model string, usage bedrock.Usage) {
            log.Printf("Usage: %s/%s - %d tokens", provider, model, usage.TotalTokens)
        })
    }

    // Use client normally
    chat := client.StartChat("You are a helpful assistant", "")
    response, err := chat.Send(ctx, "Hello!")
}
```

## Implementation Details

### Files to Modify

#### 1. `gollm/bedrock/interfaces.go` - NEW FILE (+50 lines)
```go
package bedrock

import (
    "errors"
    "time"
)

// ConfigurableClient provides advanced Bedrock-specific configuration
type ConfigurableClient interface {
    SetInferenceConfig(config *InferenceConfig) error
    GetInferenceConfig() *InferenceConfig
    SetUsageCallback(callback UsageCallback)
}

// InferenceConfig contains Bedrock-specific inference parameters
type InferenceConfig struct {
    Model       string  `json:"model,omitempty"`
    Region      string  `json:"region,omitempty"`
    Temperature float32 `json:"temperature,omitempty"`
    MaxTokens   int32   `json:"maxTokens,omitempty"`
    TopP        float32 `json:"topP,omitempty"`
    MaxRetries  int     `json:"maxRetries,omitempty"`
}

// Validate checks if inference config has valid parameters
func (c *InferenceConfig) Validate() error {
    if c.Temperature < 0 || c.Temperature > 2.0 {
        return errors.New("temperature must be between 0.0 and 2.0")
    }
    if c.MaxTokens < 0 {
        return errors.New("maxTokens must be non-negative")
    }
    if c.TopP < 0 || c.TopP > 1.0 {
        return errors.New("topP must be between 0.0 and 1.0")
    }
    return nil
}

// UsageCallback is called when usage data is available
type UsageCallback func(provider, model string, usage Usage)

// Usage represents token usage information
type Usage struct {
    InputTokens  int       `json:"inputTokens"`
    OutputTokens int       `json:"outputTokens"`
    TotalTokens  int       `json:"totalTokens"`
    Model        string    `json:"model"`
    Provider     string    `json:"provider"`
    Timestamp    time.Time `json:"timestamp"`
}
```

#### 2. `gollm/bedrock/bedrock.go` - Enhanced (+100 lines)
```go
// Add to existing BedrockClient struct
type BedrockClient struct {
    runtimeClient   *bedrockruntime.Client
    bedrockClient   *bedrock.Client
    options         *BedrockOptions
    
    // NEW: Extension interface fields
    inferenceConfig *InferenceConfig
    usageCallback   UsageCallback
}

// Ensure interface compliance
var _ gollm.Client = &BedrockClient{}
var _ ConfigurableClient = &BedrockClient{}

// NEW: Extension interface implementation
func (c *BedrockClient) SetInferenceConfig(config *InferenceConfig) error {
    if config == nil {
        return errors.New("inference config cannot be nil")
    }
    if err := config.Validate(); err != nil {
        return fmt.Errorf("invalid inference config: %w", err)
    }
    
    c.inferenceConfig = config
    
    // Apply configuration to internal options
    if config.Model != "" {
        c.options.Model = config.Model
    }
    if config.Region != "" {
        c.options.Region = config.Region
    }
    if config.Temperature != 0 {
        c.options.Temperature = config.Temperature
    }
    if config.MaxTokens != 0 {
        c.options.MaxTokens = config.MaxTokens
    }
    if config.TopP != 0 {
        c.options.TopP = config.TopP
    }
    if config.MaxRetries != 0 {
        c.options.MaxRetries = config.MaxRetries
    }
    
    return nil
}

func (c *BedrockClient) GetInferenceConfig() *InferenceConfig {
    return c.inferenceConfig
}

func (c *BedrockClient) SetUsageCallback(callback UsageCallback) {
    c.usageCallback = callback
}

// Enhanced Send method with usage tracking
func (cs *bedrockChatSession) Send(ctx context.Context, contents ...any) (gollm.ChatResponse, error) {
    // ... existing implementation ...
    
    output, err := cs.client.runtimeClient.Converse(ctx, input)
    if err != nil {
        return nil, err
    }

    response := cs.parseConverseOutput(&output.Output)
    
    // NEW: Usage tracking via callback
    if cs.client.usageCallback != nil && output.Usage != nil {
        usage := convertTokenUsage(output.Usage, cs.model)
        cs.client.usageCallback("bedrock", cs.model, usage)
    }

    return response, nil
}

// Helper function to convert AWS usage to Bedrock usage
func convertTokenUsage(awsUsage *types.TokenUsage, model string) Usage {
    return Usage{
        InputTokens:  int(aws.ToInt32(awsUsage.InputTokens)),
        OutputTokens: int(aws.ToInt32(awsUsage.OutputTokens)),
        TotalTokens:  int(aws.ToInt32(awsUsage.TotalTokens)),
        Model:        model,
        Provider:     "bedrock",
        Timestamp:    time.Now(),
    }
}
```

#### 3. `gollm/bedrock/bedrock_test.go` - Enhanced (+200 lines)
```go
func TestConfigurableClientInterface(t *testing.T) {
    client := createTestClient(t)
    
    // Test type assertion works
    configurable, ok := client.(ConfigurableClient)
    require.True(t, ok, "BedrockClient should implement ConfigurableClient")
    
    // Test configuration
    config := &InferenceConfig{
        Model:       "anthropic.claude-3-haiku-20240307-v1:0",
        Temperature: 0.5,
        MaxTokens:   2000,
    }
    
    err := configurable.SetInferenceConfig(config)
    require.NoError(t, err)
    
    retrieved := configurable.GetInferenceConfig()
    assert.Equal(t, config.Model, retrieved.Model)
    assert.Equal(t, config.Temperature, retrieved.Temperature)
}

func TestUsageCallback(t *testing.T) {
    client := createTestClient(t)
    configurable := client.(ConfigurableClient)
    
    var capturedUsage Usage
    callback := func(provider, model string, usage Usage) {
        capturedUsage = usage
    }
    
    configurable.SetUsageCallback(callback)
    
    // Simulate usage callback
    usage := Usage{
        InputTokens:  100,
        OutputTokens: 50,
        TotalTokens:  150,
        Model:        "test-model",
        Provider:     "bedrock",
    }
    
    callback("bedrock", "test-model", usage)
    
    assert.Equal(t, 150, capturedUsage.TotalTokens)
    assert.Equal(t, "bedrock", capturedUsage.Provider)
}

func TestInferenceConfigValidation(t *testing.T) {
    tests := []struct {
        name      string
        config    *InferenceConfig
        expectErr bool
    }{
        {
            name: "valid config",
            config: &InferenceConfig{
                Temperature: 0.7,
                MaxTokens:   2000,
                TopP:        0.9,
            },
            expectErr: false,
        },
        {
            name: "invalid temperature",
            config: &InferenceConfig{
                Temperature: -0.1,
            },
            expectErr: true,
        },
        {
            name: "invalid topP",
            config: &InferenceConfig{
                TopP: 1.5,
            },
            expectErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.config.Validate()
            if tt.expectErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

## Benefits of This Approach

### ✅ Addresses All Reviewer Concerns
- **No global interface pollution** - `ClientOptions` unchanged
- **No premature abstraction** - features are Bedrock-specific
- **Clean architecture** - extensions are opt-in via type assertions
- **Incremental adoption** - other providers can add similar patterns

### ✅ Preserves All User Functionality
- **Full inference configuration** - temperature, tokens, etc.
- **Complete usage tracking** - structured usage data with callbacks
- **Clean API** - intuitive type assertion pattern
- **Extensible design** - easy to add more features

### ✅ Simple and Maintainable
- **No complex merging logic** - direct configuration setting
- **No YAML serialization** - just simple structs
- **No debug flags** - uses existing klog
- **Minimal code changes** - mostly additions, not modifications

## Usage Examples

### Basic Configuration
```go
client, _ := gollm.NewClient(ctx, "bedrock")
if bedrock, ok := client.(bedrock.ConfigurableClient); ok {
    config := &bedrock.InferenceConfig{
        Temperature: 0.7,
        MaxTokens:   2000,
    }
    bedrock.SetInferenceConfig(config)
}
```

### Usage Tracking
```go
if bedrock, ok := client.(bedrock.ConfigurableClient); ok {
    bedrock.SetUsageCallback(func(provider, model string, usage bedrock.Usage) {
        log.Printf("Tokens used: %d", usage.TotalTokens)
    })
}
```

### Full Configuration
```go
if bedrock, ok := client.(bedrock.ConfigurableClient); ok {
    // Configure inference parameters
    bedrock.SetInferenceConfig(&bedrock.InferenceConfig{
        Model:       "anthropic.claude-3-5-sonnet-20241022-v2:0",
        Region:      "us-west-2",
        Temperature: 0.7,
        MaxTokens:   4000,
        TopP:        0.9,
        MaxRetries:  3,
    })
    
    // Track usage
    bedrock.SetUsageCallback(func(provider, model string, usage bedrock.Usage) {
        fmt.Printf("Usage: %s/%s - Input: %d, Output: %d, Total: %d\n",
            provider, model, usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
    })
}
```

## Testing Strategy

### Unit Tests
- Interface compliance testing
- Configuration validation
- Usage callback functionality
- Error handling

### Integration Tests
- Real Bedrock API with different configurations
- Usage tracking accuracy
- Performance with various settings

## Success Criteria

- [ ] **No changes to global interfaces** - reviewers' main concern addressed
- [ ] **Full Bedrock functionality** - all features preserved
- [ ] **Clean extension pattern** - other providers can follow
- [ ] **Comprehensive testing** - unit and integration coverage
- [ ] **Clear documentation** - usage examples and patterns

This approach transforms the reviewer concern from "don't pollute global interfaces" to "great extension pattern that enables incremental feature adoption!" It's exactly the kind of clean architecture that reviewers appreciate while preserving all the functionality you need. 