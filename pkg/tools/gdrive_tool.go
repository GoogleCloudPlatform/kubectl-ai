age tools

import (
	"context"
	"fmt"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// GoogleDriveTool is a tool for interacting with Google Drive
type GoogleDriveTool struct {
	credentialsFile string
}

// NewGoogleDriveTool creates a new Google Drive tool
func NewGoogleDriveTool(credentialsFile string) *GoogleDriveTool {
	return &GoogleDriveTool{
		credentialsFile: credentialsFile,
	}
}

// Name returns the name of the tool
func (g *GoogleDriveTool) Name() string {
	return "google-drive"
}

// Description returns the description of the tool
func (g *GoogleDriveTool) Description() string {
	return "A tool for interacting with Google Drive"
}

// Run executes the tool with the given arguments
func (g *GoogleDriveTool) Run(ctx context.Context, args map[string]any) (any, error) {
	operation, ok := args["operation"].(string)
	if !ok {
		return nil, fmt.Errorf("operation not specified")
	}

	switch operation {
	case "upload":
		fileName, ok := args["fileName"].(string)
		if !ok {
			return nil, fmt.Errorf("fileName not specified")
		}
		content, ok := args["content"].(string)
		if !ok {
			return nil, fmt.Errorf("content not specified")
		}
		return g.uploadFile(ctx, fileName, content)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

// uploadFile uploads a file to Google Drive
func (g *GoogleDriveTool) uploadFile(ctx context.Context, fileName, content string) (string, error) {
	driveService, err := drive.NewService(ctx, option.WithCredentialsFile(g.credentialsFile))
	if err != nil {
		return "", fmt.Errorf("failed to create Google Drive client: %w", err)
	}

	// Create a new file on Google Drive
	file := &drive.File{
		Name: fileName,
	}
	fileContent := []byte(content)
	_, err = driveService.Files.Create(file).Media(fileContent).Do()
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	return "File uploaded successfully", nil
}
