package tools

import (
	"context"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

type Tool interface {
	Execute(ctx context.Context) (any, error)
	BuildFunctionDefinition() *gollm.FunctionDefinition
}
