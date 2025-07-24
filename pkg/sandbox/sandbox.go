// Package sandbox provides Kubernetes-based sandboxed command execution with an exec.Command-like interface
// A sandbox represents an isolated execution environment. Currently implemented using Kubernetes pods,
// but can be extended to support other backends like Docker containers in the future.
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// Sandbox represents a Kubernetes-based sandboxed execution environment
type Sandbox struct {
	name       string
	namespace  string
	image      string
	kubeconfig string
	clientset  *kubernetes.Clientset
	config     *rest.Config
}

// Cmd represents a command to be executed in a sandbox
// It follows the same interface pattern as exec.Cmd
type Cmd struct {
	sandbox *Sandbox
	command []string
	ctx     context.Context

	// Standard streams (similar to exec.Cmd)
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Option represents a configuration option for Sandbox
type Option func(*Sandbox) error

// New creates a new Sandbox instance with the given name and options
func New(name string, opts ...Option) (*Sandbox, error) {
	s := &Sandbox{
		name:      name,
		namespace: "computer", // default namespace
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	// Use default kubeconfig if not specified
	if s.kubeconfig == "" {
		defaultKubeconfig, err := getDefaultKubeconfig()
		if err != nil {
			return nil, fmt.Errorf("no kubeconfig specified and failed to find default: %v", err)
		}
		s.kubeconfig = defaultKubeconfig
	}

	// Initialize Kubernetes client
	config, err := clientcmd.BuildConfigFromFlags("", s.kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("error building kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating Kubernetes client: %v", err)
	}

	s.config = config
	s.clientset = clientset

	return s, nil
}

// getDefaultKubeconfig tries to find the default kubeconfig file
func getDefaultKubeconfig() (string, error) {
	// Try the standard kubeconfig path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %v", err)
	}

	defaultPath := filepath.Join(homeDir, ".kube", "config")
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath, nil
	}

	// Could also check KUBECONFIG environment variable here
	if kubeconfigEnv := os.Getenv("KUBECONFIG"); kubeconfigEnv != "" {
		return kubeconfigEnv, nil
	}

	return "", fmt.Errorf("no kubeconfig found at %s and KUBECONFIG not set", defaultPath)
}

// WithKubeconfig sets the kubeconfig file path
func WithKubeconfig(kubeconfig string) Option {
	return func(s *Sandbox) error {
		s.kubeconfig = kubeconfig
		return nil
	}
}

// WithName sets the sandbox name (deprecated - use constructor parameter instead)
func WithName(name string) Option {
	return func(s *Sandbox) error {
		s.name = name
		return nil
	}
}

// WithNamespace sets the namespace
func WithNamespace(namespace string) Option {
	return func(s *Sandbox) error {
		s.namespace = namespace
		return nil
	}
}

// WithImage sets the container image
func WithImage(image string) Option {
	return func(s *Sandbox) error {
		s.image = image
		return nil
	}
}

// Command creates a new Cmd to execute the given command in the sandbox
// This follows the same interface as exec.Command
func (s *Sandbox) Command(name string, arg ...string) *Cmd {
	cmd := &Cmd{
		sandbox: s,
		command: append([]string{name}, arg...),
		ctx:     context.Background(),
	}
	return cmd
}

// CommandContext creates a new Cmd with a context
func (s *Sandbox) CommandContext(ctx context.Context, name string, arg ...string) *Cmd {
	cmd := &Cmd{
		sandbox: s,
		command: append([]string{name}, arg...),
		ctx:     ctx,
	}
	return cmd
}

// Delete removes the sandbox pod and its associated resources, waiting for them to be fully terminated.
// It does not return an error if the resources are already deleted.
func (s *Sandbox) Delete(ctx context.Context) error {
	var errs []string

	// 1. Initiate deletion of the Pod with a zero grace period for faster removal.
	deleteOptions := metav1.DeleteOptions{
		GracePeriodSeconds: new(int64), // 0 seconds
	}
	err := s.clientset.CoreV1().Pods(s.namespace).Delete(ctx, s.name, deleteOptions)
	if err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Sprintf("failed to initiate pod deletion: %v", err))
	}

	// 2. Initiate deletion of the ConfigMap.
	configMapName := s.name + "-kubeconfig"
	if err := s.deleteKubeconfigMap(ctx, configMapName); err != nil {
		errs = append(errs, fmt.Sprintf("failed to initiate configmap deletion: %v", err))
	}

	// 3. Wait for the Pod to be fully terminated.
	pollErr := wait.PollUntilContextTimeout(ctx, 2*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
		_, getErr := s.clientset.CoreV1().Pods(s.namespace).Get(ctx, s.name, metav1.GetOptions{})
		if errors.IsNotFound(getErr) {
			return true, nil // Pod is gone.
		}
		if getErr != nil {
			return false, getErr // Polling failed with an unexpected error.
		}
		return false, nil // Pod still exists, continue polling.
	})
	if pollErr != nil {
		errs = append(errs, fmt.Sprintf("error waiting for pod deletion: %v", pollErr))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during sandbox deletion: %s", strings.Join(errs, "; "))
	}

	return nil
}

// Run executes the command and waits for it to complete
func (c *Cmd) Run() error {
	return c.execute(nil, nil)
}

// Output runs the command and returns its standard output
func (c *Cmd) Output() ([]byte, error) {
	var stdout bytes.Buffer
	err := c.execute(&stdout, nil)
	return stdout.Bytes(), err
}

// CombinedOutput runs the command and returns its combined standard output and standard error
func (c *Cmd) CombinedOutput() ([]byte, error) {
	var output bytes.Buffer
	err := c.execute(&output, &output)
	return output.Bytes(), err
}

// execute is the internal method that handles the actual pod execution
func (c *Cmd) execute(stdout, stderr io.Writer) error {
	sandbox := c.sandbox

	// Validate required fields
	if sandbox.name == "" || sandbox.image == "" {
		return fmt.Errorf("sandbox name and image must be specified")
	}

	// Check if pod exists and validate its image if it does.
	existingPod, err := c.getPod()
	if err != nil {
		return fmt.Errorf("error checking for existing sandbox: %w", err)
	}

	if existingPod != nil {
		// Sandbox exists. Verify the container image matches.
		var existingImage string
		for _, container := range existingPod.Spec.Containers {
			if container.Name == "main" {
				existingImage = container.Image
				break
			}
		}

		if existingImage != "" && existingImage != sandbox.image {
			return fmt.Errorf(
				"existing sandbox '%s' uses image '%s', but new execution requested image '%s'. Please delete the sandbox first",
				sandbox.name,
				existingImage,
				sandbox.image,
			)
		}
	} else {
		// Pod doesn't exist, create it.
		if err := c.createPod(); err != nil {
			return fmt.Errorf("error creating pod: %v", err)
		}
	}

	// Wait for pod to be ready
	if err := c.waitForPodReady(); err != nil {
		return fmt.Errorf("error waiting for pod to be ready: %v", err)
	}

	// Execute command in pod
	return c.executeInPod(stdout, stderr)
}

// getPod fetches the sandbox pod if it exists. Returns (nil, nil) if not found.
func (c *Cmd) getPod() (*corev1.Pod, error) {
	sandbox := c.sandbox
	pod, err := sandbox.clientset.CoreV1().Pods(sandbox.namespace).Get(c.ctx, sandbox.name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Not an error, just means we need to create it.
		}
		return nil, err
	}
	return pod, nil
}

// createPod creates a new pod for the sandbox, including its kubeconfig configmap
func (c *Cmd) createPod() error {
	sandbox := c.sandbox
	configMapName := sandbox.name + "-kubeconfig"

	// Create a dedicated kubeconfig for the pod to use.
	// This ensures kubectl defaults to the "default" namespace.
	if err := c.createKubeconfigMap(configMapName); err != nil {
		// If the configmap already exists, we can proceed.
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create in-pod kubeconfig: %w", err)
		}
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sandbox.name,
			Namespace: sandbox.namespace,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "normal-user",
			Containers: []corev1.Container{
				{
					Name:    "main",
					Image:   sandbox.image,
					Command: []string{"sleep"},
					Args:    []string{"3600"}, // Sleep for 1 hour to keep container running
					Env: []corev1.EnvVar{
						{
							Name:  "KUBECONFIG",
							Value: "/etc/kube/config",
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "kubeconfig-volume",
							MountPath: "/etc/kube",
							ReadOnly:  true,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "kubeconfig-volume",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: configMapName,
							},
							Items: []corev1.KeyToPath{
								{
									Key:  "config",
									Path: "config",
								},
							},
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	_, podCreateErr := sandbox.clientset.CoreV1().Pods(sandbox.namespace).Create(c.ctx, pod, metav1.CreateOptions{})
	if podCreateErr != nil {
		// If pod creation fails, attempt to clean up the configmap we just created.
		if cleanupErr := sandbox.deleteKubeconfigMap(c.ctx, configMapName); cleanupErr != nil {
			return fmt.Errorf("pod creation failed: %v; ALSO, configmap cleanup failed: %v", podCreateErr, cleanupErr)
		}
		return fmt.Errorf("pod creation failed: %w", podCreateErr)
	}

	return nil
}

// createKubeconfigMap generates a kubeconfig file that uses the pod's service account token
// and sets the default namespace to "default". This is stored in a ConfigMap.
func (c *Cmd) createKubeconfigMap(name string) error {
	sandbox := c.sandbox

	// Use a static string template for the kubeconfig to ensure correctness.
	kubeconfigYAML := `apiVersion: v1
clusters:
- cluster:
    server: https://kubernetes.default.svc
    certificate-authority: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
  name: default
contexts:
- context:
    cluster: default
    namespace: default
    user: default
  name: default
current-context: default
users:
- name: default
  user:
    tokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token`

	// Create the ConfigMap object.
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: sandbox.namespace,
		},
		Data: map[string]string{
			"config": kubeconfigYAML,
		},
	}

	_, err := sandbox.clientset.CoreV1().ConfigMaps(sandbox.namespace).Create(c.ctx, configMap, metav1.CreateOptions{})
	return err
}

// deleteKubeconfigMap cleans up the ConfigMap created for the pod.
func (s *Sandbox) deleteKubeconfigMap(ctx context.Context, name string) error {
	err := s.clientset.CoreV1().ConfigMaps(s.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete kubeconfig configmap: %w", err)
	}
	return nil
}

// waitForPodReady waits for the pod to be ready
func (c *Cmd) waitForPodReady() error {
	sandbox := c.sandbox
	return wait.PollUntilContextTimeout(c.ctx, 2*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		pod, err := sandbox.clientset.CoreV1().Pods(sandbox.namespace).Get(ctx, sandbox.name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// Check if pod is ready
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}

		// Check if pod failed
		if pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("pod %s failed", sandbox.name)
		}

		return false, nil
	})
}

// executeInPod executes the command in the pod
func (c *Cmd) executeInPod(stdout, stderr io.Writer) error {
	sandbox := c.sandbox

	// Use provided writers or default to the Cmd's streams
	if stdout == nil {
		stdout = c.Stdout
		if stdout == nil {
			stdout = os.Stdout
		}
	}
	if stderr == nil {
		stderr = c.Stderr
		if stderr == nil {
			stderr = os.Stderr
		}
	}

	req := sandbox.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(sandbox.name).
		Namespace(sandbox.namespace).
		SubResource("exec")

	commandStr := strings.Join(c.command, " ")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: "main",
		Command:   []string{"/bin/sh", "-c", commandStr},
		Stdin:     c.Stdin != nil,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(sandbox.config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("error creating executor: %v", err)
	}

	err = exec.StreamWithContext(c.ctx, remotecommand.StreamOptions{
		Stdin:  c.Stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return fmt.Errorf("error executing command: %v", err)
	}

	return nil
}
