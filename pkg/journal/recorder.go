package journal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"sigs.k8s.io/yaml"
)

// Recorder is an interface for recording a structured log of the agent's actions and observations.
type Recorder interface {
	io.Closer

	// Write will add an event to the recorder.
	Write(ctx context.Context, event *Event) error
}

// FileRecorder writes a structured log of the agent's actions and observations to a file.
type FileRecorder struct {
	f *os.File
}

// NewFileRecorder creates a new FileRecorder that writes to the given file.
func NewFileRecorder(path string) (*FileRecorder, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	return &FileRecorder{
		f: file,
	}, nil
}

// Close closes the file.
func (r *FileRecorder) Close() error {
	return r.f.Close()
}

func (r *FileRecorder) Write(ctx context.Context, event *Event) error {
	yamlBytes, err := yaml.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshalling event: %w", err)
	}
	var b bytes.Buffer
	b.Write(yamlBytes)
	b.Write([]byte("\n\n---\n\n"))
	_, err = r.f.Write(b.Bytes())
	return err
}

type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Payload   any       `json:"result,omitempty"`
}

// WriteToFile appends the given content to a file.
// If the file does not exist, it is created.
// Returns an error if any operation fails.
func WriteToFile(fileName string, content string) error {
	// Open the file in append mode, create if it doesn't exist
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close() // Ensure the file is closed after the function exits

	// Append the content to the file
	_, err = file.WriteString(content)
	if err != nil {
		return err
	}

	return nil
}
