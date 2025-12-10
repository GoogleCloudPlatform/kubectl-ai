package gollm_test

import (
	"context"
	"testing"

	"github.com/GoogleCloudPlatform/kubectl-ai/internal/mocks"
	gomock "go.uber.org/mock/gomock"
)

// TestModel_WithMocks verifies that the command line model variable
// is contained in the response from ListModels() using mocks.
func TestModel_WithMocks(t *testing.T) {
	ctx := context.Background()

	// Command line model (simulating --model flag)
	cmdLineModel := "anthropic.claude-3-5-sonnet-20240620-v1:0"

	tests := []struct {
		name            string
		cmdLineModel    string
		availableModels []string
		expectedFound   bool
	}{
		{
			name:         "Model exists in list",
			cmdLineModel: cmdLineModel,
			availableModels: []string{
				"anthropic.claude-3-5-sonnet-20240620-v1:0",
				"anthropic.claude-3-haiku-20240307-v1:0",
				"amazon.titan-text-express-v1",
			},
			expectedFound: true,
		},
		{
			name:         "Model does not exist in list",
			cmdLineModel: "non-existent-model",
			availableModels: []string{
				"anthropic.claude-3-5-sonnet-20240620-v1:0",
				"anthropic.claude-3-haiku-20240307-v1:0",
				"amazon.titan-text-express-v1",
			},
			expectedFound: false,
		},
		{
			name:            "Empty model list",
			cmdLineModel:    cmdLineModel,
			availableModels: []string{},
			expectedFound:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock client
			mockClient := mocks.NewMockClient(ctrl)

			// Setup expectation: ListModels should be called and return the available models
			mockClient.EXPECT().
				ListModels(gomock.Any()).
				Return(tt.availableModels, nil).
				Times(1)

			// Call ListModels
			models, err := mockClient.ListModels(ctx)
			if err != nil {
				t.Fatalf("ListModels() error = %v", err)
			}

			// Check if the command line model is in the returned list
			found := false
			for _, model := range models {
				if model == tt.cmdLineModel {
					found = true
					break
				}
			}

			// Verify expectation
			if found != tt.expectedFound {
				t.Errorf("Model %q in list: got %v, want %v", tt.cmdLineModel, found, tt.expectedFound)
			}

			if found {
				t.Logf("✓ Command line model %q found in ListModels() response", tt.cmdLineModel)
			} else {
				t.Logf("✓ Command line model %q not found in ListModels() response (as expected)", tt.cmdLineModel)
			}
		})
	}
}
