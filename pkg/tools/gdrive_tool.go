package tools

import (
	"bytes"
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"k8s.io/klog/v2"
)

func init() {
	RegisterTool(&GDriveTool{})
}

type GDriveTool struct {
	CredentialsFilePath string
}

type GDriveUploadResult struct {
	FileName string `json:"fileName"`
	FileID   string `json:"fileID"`
	Status   string `json:"status"`
}

func (t *GDriveTool) Name() string {
	return "gdrive"
}

func (t *GDriveTool) Description() string {
	return "Uploads logs or yaml outputs to Google Drive. Use this tool only when you need to upload logs or yaml outputs of particular Kubernetes deployment."
}

func (t *GDriveTool) FunctionDefinition() *gollm.FunctionDefinition {

	return &gollm.FunctionDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"operation": {
					Type:        gollm.TypeString,
					Description: `The operation while interacting with Google Drive.`,
				},
				"fileName": {
					Type:        gollm.TypeString,
					Description: "The name to give the file in Drive.",
				},
				"content": {
					Type:        gollm.TypeString,
					Description: "The contents of the file to upload.",
				},
			},
			Required: []string{"operation", "fileName", "content"},
		},
	}
}

func (t *GDriveTool) Run(ctx context.Context, args map[string]any) (any, error) {
	operation := args["operation"].(string)

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

		return t.upload(ctx, fileName, content)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)

	}
}

func (t *GDriveTool) upload(ctx context.Context, fileName, content string) (any, error) {
	// Build client options:
	// 1) If the user passed --gdrive-credentials-path, use it.
	// 2) Scope it to drive and enable ENV or ADC.
	opts := []option.ClientOption{
		option.WithScopes(drive.DriveFileScope),
	}
	if t.CredentialsFilePath != "" {
		opts = append(opts, option.WithCredentialsFile(t.CredentialsFilePath))
	} else {
		klog.Warning("no --gdrive-credentials-path provided; falling back to Application Default Credentials (ADC). ADC is set with command`gcloud auth application-default login`")
	}

	srv, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive client: %w", err)
	}

	// Upload
	reader := bytes.NewReader([]byte(content))
	uploaded, err := srv.Files.
		Create(&drive.File{Name: fileName}).
		Media(reader).
		Do()
	if err != nil {
		return nil, fmt.Errorf("upload error: %w", err)
	}

	// Return a typed result
	return &GDriveUploadResult{
		FileName: uploaded.Name,
		FileID:   uploaded.Id,
		Status:   "uploaded",
	}, nil
}
