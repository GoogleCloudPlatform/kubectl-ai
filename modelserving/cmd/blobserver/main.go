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
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"k8s.io/klog/v2"
)

// Note: hashes can be obtained from e.g. curl https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF/raw/main/Llama-3.3-70B-Instruct-Q6_K/Llama-3.3-70B-Instruct-Q6_K-00001-of-00002.gguf
//  It is also currently in the x-linked-etag header when we do a HEAD request

var knownBlobs = []knownBlob{
	{
		Hash: "77ebb031649ac7a16b89b4078feb197d56a61941b703a980233069a2670c811b",
		URL:  "https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF/resolve/main/Llama-3.3-70B-Instruct-Q6_K/Llama-3.3-70B-Instruct-Q6_K-00001-of-00002.gguf",
	},
	{
		Hash: "f50428a8c9912e949f5273174e66c9febdc7cd21617595de2dc0e5c9df536434",
		URL:  "https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF/resolve/main/Llama-3.3-70B-Instruct-Q6_K/Llama-3.3-70B-Instruct-Q6_K-00002-of-00002.gguf",
	},

	{
		Hash: "ecb6908345e7a10be94511eae715b6b6eadbc518b7c1dd0fd5ba8816b62b4dc9",
		URL:  "https://huggingface.co/unsloth/gemma-3-12b-it-GGUF/resolve/main/gemma-3-12b-it-Q4_K_M.gguf",
	},
}

var preloadBlobs = []string{
	//  "ecb6908345e7a10be94511eae715b6b6eadbc518b7c1dd0fd5ba8816b62b4dc9", // Preloading some blobs is particularly helpful during development
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	log := klog.FromContext(ctx)

	listen := ":8080"
	cacheDir := os.Getenv("CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "~/.cache/blobserver/blobs"
	}
	flag.StringVar(&listen, "listen", listen, "listen address")
	flag.StringVar(&cacheDir, "cache-dir", cacheDir, "cache directory")
	flag.Parse()

	if strings.HasPrefix(cacheDir, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		cacheDir = filepath.Join(homeDir, strings.TrimPrefix(cacheDir, "~/"))
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("creating cache directory %q: %w", cacheDir, err)
	}

	cacheBucket := os.Getenv("CACHE_BUCKET")
	if cacheBucket == "" {
		return fmt.Errorf("Must specify CACHE_BUCKET env var")
	}
	if strings.HasPrefix(cacheBucket, "gs://") {
		cacheBucket = strings.TrimPrefix(cacheBucket, "gs://")
		log.Info("using GCS cache", "bucket", cacheBucket)
	} else {
		return fmt.Errorf("CACHE_BUCKET must be a GCS bucket URL (gs://<bucketName>)")
	}

	blobCache := &blobCache{
		BaseDir: cacheDir,
		gcsStore: &gcsStore{
			Bucket: cacheBucket,
		},
		jobs: make(map[string]*downloadJob),
	}

	s := &httpServer{
		blobCache: blobCache,
	}

	// Preload some blobs

	for _, hash := range preloadBlobs {
		f, err := s.blobCache.GetBlob(ctx, hash)
		if err != nil {
			return fmt.Errorf("error getting blob: %w", err)
		}
		log.Info("got blob", "hash", hash, "path", f.Name())
		f.Close()
	}

	klog.Infof("serving on %q", listen)
	if err := http.ListenAndServe(listen, s); err != nil {
		return fmt.Errorf("serving on %q: %w", listen, err)
	}

	return nil
}

type httpServer struct {
	blobCache *blobCache
}

func (s *httpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tokens := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(tokens) == 1 {
		if r.Method == "GET" {
			hash := tokens[0]
			s.serveGETBlob(w, r, hash)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}

func (s *httpServer) serveGETBlob(w http.ResponseWriter, r *http.Request, hash string) {
	ctx := r.Context()

	log := klog.FromContext(ctx)

	// TODO: Validate hash is hex, right length etc

	f, err := s.blobCache.GetBlob(ctx, hash)
	if err != nil {
		log.Error(err, "error getting blob")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	p := f.Name()

	klog.Infof("serving blob %q", p)
	http.ServeFile(w, r, p)
}

type blobCache struct {
	BaseDir  string
	gcsStore *gcsStore

	mutex sync.Mutex
	jobs  map[string]*downloadJob
}

type knownBlob struct {
	Hash string
	URL  string
}

func (c *blobCache) SourceForBlob(ctx context.Context, hash string) (string, error) {
	// log := klog.FromContext(ctx)

	source := ""
	for _, knownBlob := range knownBlobs {
		if knownBlob.Hash == hash {
			source = knownBlob.URL
		}
	}

	if source == "" {
		return "", fmt.Errorf("blob %q not known", hash)
	}

	return source, nil
}

func (c *blobCache) GetBlob(ctx context.Context, hash string) (*os.File, error) {
	// log := klog.FromContext(ctx)

	localPath := filepath.Join(c.BaseDir, hash)
	f, err := os.Open(localPath)
	if err == nil {
		return f, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("opening blob %q: %w", hash, err)
	}

	if err := c.downloadBlob(ctx, hash, localPath); err != nil {
		return nil, fmt.Errorf("downloading blob %q: %w", hash, err)
	}

	return os.Open(localPath)
}

func (c *blobCache) downloadBlob(ctx context.Context, hash string, destinationPath string) error {
	// log := klog.FromContext(ctx)

	c.mutex.Lock()
	job := c.jobs[hash]
	if job != nil {
		c.mutex.Unlock()
		return fmt.Errorf("download of %q already in progress", hash)
	}

	job = &downloadJob{
		Hash:            hash,
		DestinationPath: destinationPath,
	}

	c.jobs[hash] = job

	c.mutex.Unlock()

	jobCtx := context.Background()
	err := job.run(jobCtx, c)
	c.mutex.Lock()
	delete(c.jobs, hash)
	c.mutex.Unlock()
	return err
}

type downloadJob struct {
	Hash            string
	DestinationPath string
}

func (j *downloadJob) run(ctx context.Context, blobCache *blobCache) error {
	log := klog.FromContext(ctx)

	gcsStore := blobCache.gcsStore
	hash := j.Hash
	destinationPath := j.DestinationPath

	if err := gcsStore.Download(ctx, hash, destinationPath); err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			// Fallthrough to download from upstream source
		} else {
			return fmt.Errorf("downloading from GCS: %w", err)
		}
	} else {
		return nil
	}

	log.Info("blob not found in GCS cache; will download from upstream", "hash", hash)

	source, err := blobCache.SourceForBlob(ctx, hash)
	if err != nil {
		return fmt.Errorf("getting source for blob %q: %w", hash, err)
	}

	if err := j.downloadFromSource(ctx, source); err != nil {
		return fmt.Errorf("downloading blob %q: %w", hash, err)
	}
	if err := gcsStore.Upload(ctx, destinationPath, hash); err != nil {
		return fmt.Errorf("uploading to GCS: %w", err)
	}

	return nil
}

func (j *downloadJob) downloadFromSource(ctx context.Context, sourceURL string) error {
	log := klog.FromContext(ctx)

	log.Info("downloading blob", "source", sourceURL, "destination", j.DestinationPath)

	req, err := http.NewRequestWithContext(ctx, "GET", sourceURL, nil)
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

	n, err := writeToFile(ctx, resp.Body, j.DestinationPath)
	if err != nil {
		return err
	}

	log.Info("downloaded blob from source", "source", sourceURL, "bytes", n, "duration", time.Since(startedAt))

	return nil
}

func writeToFile(ctx context.Context, src io.Reader, destinationPath string) (int64, error) {
	log := klog.FromContext(ctx)

	dir := filepath.Dir(destinationPath)
	tempFile, err := os.CreateTemp(dir, "download")
	if err != nil {
		return 0, fmt.Errorf("creating temp file: %w", err)
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

	n, err := io.Copy(tempFile, src)
	if err != nil {
		return n, fmt.Errorf("downloading from upstream source: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return n, fmt.Errorf("closing temp file: %w", err)
	}
	shouldCloseTempFile = false

	if err := os.Rename(tempFile.Name(), destinationPath); err != nil {
		return n, fmt.Errorf("renaming temp file: %w", err)
	}
	shouldDeleteTempFile = false

	return n, nil
}

type gcsStore struct {
	Bucket string
}

func (j *gcsStore) Upload(ctx context.Context, sourcePath string, hash string) error {
	log := klog.FromContext(ctx)

	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer src.Close()

	objectKey := hash
	gcsURL := "gs://" + j.Bucket + "/" + objectKey

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("creating GCS storage client: %w", err)
	}
	defer client.Close()

	obj := client.Bucket(j.Bucket).Object(objectKey)
	objAttrs, err := obj.Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			objAttrs = nil
			log.Info("object not found in GCS", "url", gcsURL)
			// Fallthrough to upload object
		} else {
			return fmt.Errorf("getting object attributes for %q: %w", gcsURL, err)
		}
	}
	if objAttrs != nil {
		log.Info("object already exists in GCS", "url", gcsURL)
		return nil
	}

	log.Info("uploading blob to GCS", "source", sourcePath, "destination", gcsURL)

	startedAt := time.Now()
	w := obj.NewWriter(ctx)
	n, err := io.Copy(w, src)
	if err != nil {
		return fmt.Errorf("uploading to GCS: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing GCS writer: %w", err)
	}

	log.Info("uploaded blob to GCS", "url", gcsURL, "bytes", n, "duration", time.Since(startedAt))

	return nil
}

func (j *gcsStore) Download(ctx context.Context, hash string, destinationPath string) error {
	log := klog.FromContext(ctx)

	objectKey := hash
	gcsURL := "gs://" + j.Bucket + "/" + objectKey

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("creating GCS storage client: %w", err)
	}
	defer client.Close()

	// TODO: Do a head so we can output clearer log messages?

	log.Info("downloading blob from GCS", "source", gcsURL, "destination", destinationPath)

	startedAt := time.Now()
	r, err := client.Bucket(j.Bucket).Object(objectKey).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("opening object from GCS %q: %w", gcsURL, err)
	}
	defer r.Close()

	n, err := writeToFile(ctx, r, destinationPath)
	if err != nil {
		return fmt.Errorf("downloading from GCS: %w", err)
	}

	log.Info("downloaded blob from GCS", "source", gcsURL, "destination", destinationPath, "bytes", n, "duration", time.Since(startedAt))

	return nil
}
