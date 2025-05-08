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
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	rpcPort := 50052

	// Hard coded because we will probably tweak this (e.g. do autodiscovery)
	workerCount := 4
	hostTemplate := "rpc-server-%d.rpc-server"

	maxDownloadAttempts := 5

	llmModel := os.Getenv("LLM_MODEL")
	flag.StringVar(&llmModel, "llm-model", llmModel, "path or URL to LLM model.")

	llmChunks := os.Getenv("LLM_CHUNKS")
	flag.StringVar(&llmChunks, "llm-chunks", llmChunks, "chunks to combine to build llm model, as a comma separated list of sha256s to download from the blobserver")

	blobserver := os.Getenv("BLOBSERVER")
	if blobserver == "" {
		blobserver = "http://blobserver"
	}
	flag.StringVar(&blobserver, "blobserver", blobserver, "base url to blobserver")

	klog.InitFlags(nil)

	flag.Parse()

	blobserverURL, err := url.Parse(blobserver)
	if err != nil {
		return fmt.Errorf("parsing blobserver url %q: %w", blobserver, err)
	}
	modelLoader := &ModelLoader{
		blobserverURL:       blobserverURL,
		maxDownloadAttempts: maxDownloadAttempts,
	}
	if strings.HasPrefix(llmModel, "http://") || strings.HasPrefix(llmModel, "https://") {
		tmpDir := os.TempDir()
		localPath := filepath.Join(tmpDir, "model.bin")
		if err := modelLoader.downloadToFile(ctx, llmModel, localPath); err != nil {
			return fmt.Errorf("downloading model: %w", err)
		}
		llmModel = localPath
	}

	if llmChunks != "" {
		tmpDir := os.TempDir()
		basePath := filepath.Join(tmpDir, "model")
		chunks := strings.Split(llmChunks, ",")
		chunkPaths, err := modelLoader.downloadSplitModel(ctx, chunks, basePath)
		if err != nil {
			return fmt.Errorf("downloading model chunks: %w", err)
		}
		llmModel = chunkPaths[0]
	}

	// Wait till we can resolve all the RPC servers; they can take a while to launch
	var rpcHosts []string
	for i := 0; i < workerCount; i++ {
		host := fmt.Sprintf(hostTemplate, i)

		attempt := 0
		maxAttempts := 100
		for {
			attempt++
			ips, err := net.LookupIP(host)
			if err != nil {
				if attempt >= maxAttempts {
					return fmt.Errorf("looking up host %q: %w", host, err)
				}
				klog.Warningf("retrying lookup of host %q: %v", host, err)
				time.Sleep(3 * time.Second)
				continue
			}
			if len(ips) == 0 {
				return fmt.Errorf("host %q resolved, but did not return IPs", host)
			}
			klog.Infof("resolved host %q to %v", host, ips)

			// We use the IP addresses so we don't have to rely on DNS from here on
			host = ips[0].String()
			break
		}

		rpcHosts = append(rpcHosts, fmt.Sprintf("%s:%d", host, rpcPort))

	}

	args := []string{}

	args = append(args, "--model", llmModel)
	args = append(args, "--host", "0.0.0.0")
	args = append(args, "--jinja") // Needed for chat, no env-var
	args = append(args, "--verbose")
	args = append(args, "--rpc", strings.Join(rpcHosts, ","))
	args = append(args, flag.Args()...)

	klog.Infof("starting llama-server with args: %v", args)

	cmd := exec.CommandContext(ctx, "/llama-server", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	env := os.Environ()
	env = append(env, fmt.Sprintf("LLAMA_ARG_FLASH_ATTN=yes"))   // Needed for chat (I think?)
	env = append(env, fmt.Sprintf("LLAMA_ARG_N_GPU_LAYERS=999")) // Offload all the layers
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting llama-server: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("llama-server exited with error: %w", err)
	}
	return nil
}

type ModelLoader struct {
	// blobserverURL is the base URL to the blobserver, typically http://blobserver
	blobserverURL *url.URL

	// maxDownloadAttempts is the number of times to attempt a download before failing
	maxDownloadAttempts int
}

func (l *ModelLoader) downloadToFile(ctx context.Context, url string, destPath string) error {
	log := klog.FromContext(ctx)

	attempt := 0
	for {
		attempt++

		err := l.downloadToFileNoRetry(ctx, url, destPath)
		if err == nil {
			return nil
		}

		if attempt >= l.maxDownloadAttempts {
			return err
		}

		log.Error(err, "downloading url, will retry", "url", url, "attempt", attempt)
		time.Sleep(5 * time.Second)
	}
}

func (l *ModelLoader) downloadToFileNoRetry(ctx context.Context, url string, destPath string) error {
	log := klog.FromContext(ctx)

	dir := filepath.Dir(destPath)
	tempFile, err := os.CreateTemp(dir, "model")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	shouldDeleteTempFile := true
	defer func() {
		if shouldDeleteTempFile {
			if err := os.Remove(tempFile.Name()); err != nil {
				log.Error(err, "removing temp file", "path", tempFile.Name)
			}
		}
	}()

	shouldCloseTempFile := true
	defer func() {
		if shouldCloseTempFile {
			if err := tempFile.Close(); err != nil {
				log.Error(err, "closing temp file", "path", tempFile.Name)
			}
		}
	}()

	if err := l.downloadToWriter(ctx, url, tempFile); err != nil {
		return fmt.Errorf("downloading from %q: %w", url, err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	shouldCloseTempFile = false

	if err := os.Rename(tempFile.Name(), destPath); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	shouldDeleteTempFile = false

	return nil
}

func (l *ModelLoader) downloadSplitModel(ctx context.Context, chunks []string, baseName string) ([]string, error) {
	// log := klog.FromContext(ctx)

	var chunkPaths []string

	for i, chunk := range chunks {
		// llama-server expects chunked models to follow a fixed format
		chunkPath := baseName + fmt.Sprintf("-%05d-of-%05d.gguf", i+1, len(chunks))

		url := l.blobserverURL.JoinPath(chunk)
		if err := l.downloadToFile(ctx, url.String(), chunkPath); err != nil {
			return nil, fmt.Errorf("downloading chunk from %q: %w", url, err)
		}
		chunkPaths = append(chunkPaths, chunkPath)
	}

	return chunkPaths, nil
}

func (l *ModelLoader) downloadToWriter(ctx context.Context, url string, w io.Writer) error {
	log := klog.FromContext(ctx)

	log.Info("downloading from url", "url", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	startedAt := time.Now()

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("doing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status downloading from upstream source: %v", resp.Status)
	}

	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return fmt.Errorf("downloading from upstream source: %w", err)
	}

	log.Info("downloaded blob", "url", url, "bytes", n, "duration", time.Since(startedAt))

	return nil
}
