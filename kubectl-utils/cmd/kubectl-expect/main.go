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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/kubectl-utils/pkg/kel"
	"github.com/GoogleCloudPlatform/kubectl-ai/kubectl-utils/pkg/kube"
	celtypes "github.com/google/cel-go/common/types"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

func run(ctx context.Context) error {
	// log := klog.FromContext(ctx)

	namespace := ""
	kubeconfig := ""

	pflag.StringVarP(&namespace, "namespace", "n", namespace, "If present, the namespace scope for this CLI request")
	pflag.StringVar(&kubeconfig, "kubeconfig", kubeconfig, "Path to the kubeconfig file to use for CLI requests.")

	klog.InitFlags(nil)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	args := pflag.Args()

	if len(args) < 2 {
		return fmt.Errorf("expected [target] [cel-expression]")
	}

	target := args[0]
	celExpressionText := args[1]

	kubeClient, err := kube.NewClient(kubeconfig)
	if err != nil {
		return err
	}

	tokens := strings.Split(target, "/")
	if len(tokens) != 2 {
		return fmt.Errorf("expected target like Pod/<name>")
	}

	// Find the resource (kind) the user is asking about
	resource, err := kubeClient.FindResource(ctx, tokens[0])
	if err != nil {
		return err
	}

	// Compute namespace, defaulting to kubeconfig or default
	if namespace == "" && resource.Namespaced {
		namespace, err = kubeClient.DefaultNamespace()
		if err != nil {
			return err
		}
	}

	// Compile the CEL expression
	env, err := kel.NewEnv()
	if err != nil {
		return fmt.Errorf("initializing CEL: %w", err)
	}
	celExpression, err := kel.NewExpression(env, celExpressionText)
	if err != nil {
		return err
	}

	// build a pretty-printer for outputting status while polling
	printer, err := celExpression.BuildStatusPrinter(ctx)
	if err != nil {
		return fmt.Errorf("building status printer: %w", err)
	}

	// Get ready to get the object
	id := types.NamespacedName{
		Namespace: namespace,
		Name:      tokens[1],
	}

	gv := schema.GroupVersion{
		Group:   resource.Group,
		Version: resource.Version,
	}
	gvr := gv.WithResource(resource.Name)
	gvk := gv.WithKind(resource.Kind)

	client := kubeClient.ForGVR(gvr, id.Namespace)

	// Poll the object until the CEL expression returns true
	for {
		// We _could_ watch...
		time.Sleep(1 * time.Second)

		u, err := client.Get(ctx, id.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("getting %s %s: %w", gvk.Kind, id.Name, err)
		}

		out, err := celExpression.Eval(ctx, u)
		if err != nil {
			return err
		}

		done := false
		switch out.Type() {
		case celtypes.BoolType:
			v := out.Value().(bool)
			if v {
				done = true
			}
		default:
			return fmt.Errorf("unhandled type for CEL expression: %v", out.Type())
		}
		if done {
			break
		}

		// Pretty print some intermediate values if we can
		if printer != nil {
			s := printer(ctx, u)
			fmt.Printf("waiting for %q (%s)\n", celExpression.CELText, s)
		} else {
			fmt.Printf("waiting for %q\n", celExpression.CELText)
		}
	}

	return nil
}
