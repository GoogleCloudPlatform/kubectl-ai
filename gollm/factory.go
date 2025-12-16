// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gollm

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"

	"k8s.io/klog/v2"
)

var globalRegistry registry

type registry struct {
	mutex     sync.Mutex
	providers map[string]FactoryFunc
}

func (r *registry) listProviders() []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	providers := make([]string, 0, len(r.providers))
	for k := range r.providers {
		providers = append(providers, k)
	}
	return providers
}

type ClientOptions struct {
	URL           *url.URL
	SkipVerifySSL bool
	// Extend with more options as needed
}

// Option is a functional option for configuring ClientOptions.
type Option func(*ClientOptions)

// WithSkipVerifySSL enables skipping SSL certificate verification for HTTP clients.
func WithSkipVerifySSL() Option {
	return func(o *ClientOptions) {
		o.SkipVerifySSL = true
	}
}

type FactoryFunc func(ctx context.Context, opts ClientOptions) (Client, error)

func RegisterProvider(id string, factoryFunc FactoryFunc) error {
	return globalRegistry.RegisterProvider(id, factoryFunc)
}

func (r *registry) RegisterProvider(id string, factoryFunc FactoryFunc) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.providers == nil {
		r.providers = make(map[string]FactoryFunc)
	}
	_, exists := r.providers[id]
	if exists {
		return fmt.Errorf("provider %q is already registered", id)
	}
	r.providers[id] = factoryFunc
	return nil
}

func (r *registry) NewClient(ctx context.Context, providerID string, opts ...Option) (Client, error) {
	// providerID can be just an ID, for example "gemini" instead of "gemini://"
	if !strings.Contains(providerID, "/") && !strings.Contains(providerID, ":") {
		providerID = providerID + "://"
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	u, err := url.Parse(providerID)
	if err != nil {
		return nil, fmt.Errorf("parsing provider id %q: %w", providerID, err)
	}

	factoryFunc := r.providers[u.Scheme]
	if factoryFunc == nil {
		return nil, fmt.Errorf("provider %q not registered. Available providers: %v", u.Scheme, r.listProviders())
	}

	// Build ClientOptions
	clientOpts := ClientOptions{
		URL: u,
	}
	// Support environment variable override for SkipVerifySSL
	if v := os.Getenv("LLM_SKIP_VERIFY_SSL"); v == "1" || strings.ToLower(v) == "true" {
		clientOpts.SkipVerifySSL = true
	}
	for _, opt := range opts {
		opt(&clientOpts)
	}

	return factoryFunc(ctx, clientOpts)
}

/*
NewClient builds a Client based on the LLM_CLIENT environment variable or the provided providerID.
If providerID is not empty, it overrides the value from LLM_CLIENT.
Supports Option parameters and the LLM_SKIP_VERIFY_SSL environment variable.
*/
func NewClient(ctx context.Context, providerID string, opts ...Option) (Client, error) {
	if providerID == "" {
		s := os.Getenv("LLM_CLIENT")
		if s == "" {
			return nil, fmt.Errorf("LLM_CLIENT is not set. Available providers: %v", globalRegistry.listProviders())
		}
		providerID = s
	}

	return globalRegistry.NewClient(ctx, providerID, opts...)
}

// APIError represents an error returned by the LLM client.
type APIError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("API Error: Status=%d, Message='%s', OriginalErr=%v", e.StatusCode, e.Message, e.Err)
	}
	return fmt.Sprintf("API Error: Status=%d, Message='%s'", e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// IsRetryableFunc defines the signature for functions that check if an error is retryable.
// TODO (droot): Adjust the signature to allow underlying client to relay the backoff
// delay etc. for example, Gemini's error codes contain retryDelay information.
type IsRetryableFunc func(error) bool

// DefaultIsRetryableError provides a default implementation based on common HTTP codes and network errors.
func DefaultIsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusConflict, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Add other error checks specific to LLM clients if needed
	// e.g., if errors.Is(err, specificLLMRateLimitError) { return true }

	return false
}

// createCustomHTTPClient returns an *http.Client that optionally skips SSL certificate verification.
// This is shared by all providers that need custom HTTP transport.
func createCustomHTTPClient(skipVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	if skipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	}

	return &http.Client{
		Transport: transport,
		Timeout:   180 * time.Second,
	}
}

// RetryConfig holds the configuration for the retry mechanism (same as before)
type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
	Jitter         bool
}

// DefaultRetryConfig provides sensible defaults (same as before)
var DefaultRetryConfig = RetryConfig{
	MaxAttempts:    5,
	InitialBackoff: 200 * time.Millisecond, // Slightly increased default
	MaxBackoff:     10 * time.Second,
	BackoffFactor:  2.0,
	Jitter:         true,
}

// Retry executes the provided operation with retries, returning the result and error.
// It's now generic to handle any return type T.
func Retry[T any](
	ctx context.Context,
	config RetryConfig,
	isRetryable IsRetryableFunc,
	operation func(ctx context.Context) (T, error),
) (T, error) {
	var lastErr error
	var zero T // Zero value of the return type T

	log := klog.FromContext(ctx)

	backoff := config.InitialBackoff

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		log.V(2).Info("Retry attempt started", "attempt", attempt, "maxAttempts", config.MaxAttempts, "backoff", backoff)
		result, err := operation(ctx)

		if err == nil {
			log.V(2).Info("Retry attempt succeeded", "attempt", attempt)
			return result, nil
		}
		lastErr = err // Store the last error encountered

		// Check if context was cancelled *after* the operation
		select {
		case <-ctx.Done():
			log.Info("Context cancelled after attempt %d failed.", "attempt", attempt)
			return zero, ctx.Err() // Return context error preferentially
		default:
			// Context not cancelled, proceed with error checking
		}

		if !isRetryable(lastErr) {
			log.Info("Attempt failed with non-retryable error", "attempt", attempt, "error", lastErr)
			return zero, lastErr // Return the non-retryable error immediately
		}

		log.Info("Attempt failed with retryable error", "attempt", attempt, "error", lastErr)

		if attempt == config.MaxAttempts {
			// Max attempts reached
			break
		}

		// Calculate wait time
		waitTime := backoff
		if config.Jitter {
			waitTime += time.Duration(rand.Float64() * float64(backoff) / 2)
		}

		log.V(2).Info("Waiting before next retry attempt", "waitTime", waitTime, "nextAttempt", attempt+1, "maxAttempts", config.MaxAttempts)

		// Wait or react to context cancellation
		select {
		case <-time.After(waitTime):
			// Wait finished
		case <-ctx.Done():
			log.Info("Context cancelled while waiting for retry after attempt %d.", "attempt", attempt)
			return zero, ctx.Err()
		}

		// Increase backoff
		backoff = time.Duration(float64(backoff) * config.BackoffFactor)
		if backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	// If the loop finished, it means all attempts failed
	errFinal := fmt.Errorf("operation failed after %d attempts: %w", config.MaxAttempts, lastErr)
	return zero, errFinal
}

// retryChat is a generic decorator that adds retry logic to any Chat implementation.
type retryChat[C Chat] struct {
	underlying  Chat // The actual client implementation being wrapped
	config      RetryConfig
	isRetryable IsRetryableFunc
}

// NewRetryChat creates a new Chat that wraps the given underlying client
// with retry logic using the provided configuration.
// It returns the Chat interface type, hiding the generic implementation detail.
func NewRetryChat[C Chat](
	underlying C,
	config RetryConfig,
) Chat {
	return &retryChat[C]{
		underlying: underlying,
		config:     config,
	}
}

// Embed implements the Client interface for the retryClient decorator.
func (rc *retryChat[C]) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	// Define the operation
	operation := func(ctx context.Context) (ChatResponse, error) {
		return rc.underlying.Send(ctx, contents...)
	}

	// Execute with retry
	return Retry[ChatResponse](ctx, rc.config, rc.underlying.IsRetryableError, operation)
}

// Embed implements the Client interface for the retryClient decorator.
func (rc *retryChat[C]) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	return rc.underlying.SendStreaming(ctx, contents...)
}

func (rc *retryChat[C]) SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error {
	return rc.underlying.SetFunctionDefinitions(functionDefinitions)
}

func (rc *retryChat[C]) IsRetryableError(err error) bool {
	return rc.underlying.IsRetryableError(err)
}

func (rc *retryChat[C]) Initialize(messages []*api.Message) error {
	return rc.underlying.Initialize(messages)
}

func (rc *retryChat[C]) SaveMessages(path string) error {
	return rc.underlying.SaveMessages(path)
}
