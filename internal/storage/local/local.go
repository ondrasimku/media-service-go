package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/ondrasimku/media-service-go/internal/storage"
)

type LocalStorage struct {
	baseDir       string
	publicBaseURL string
}

func NewLocalStorage(baseDir, publicBaseURL string) (*LocalStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &LocalStorage{
		baseDir:       baseDir,
		publicBaseURL: publicBaseURL,
	}, nil
}

func (s *LocalStorage) Save(ctx context.Context, r io.Reader, opts storage.SaveOptions) (storage.FileInfo, error) {
	id := uuid.New().String()

	dir := filepath.Join(s.baseDir, opts.Directory)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return storage.FileInfo{}, fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := filepath.Join(dir, id)
	file, err := os.Create(filePath)
	if err != nil {
		return storage.FileInfo{}, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	size, err := io.Copy(file, r)
	if err != nil {
		os.Remove(filePath)
		return storage.FileInfo{}, fmt.Errorf("failed to write file: %w", err)
	}

	url := fmt.Sprintf("%s/files/%s", s.publicBaseURL, id)

	return storage.FileInfo{
		ID:          id,
		Path:        filePath,
		ContentType: opts.ContentType,
		Size:        size,
		URL:         url,
	}, nil
}

func (s *LocalStorage) Open(ctx context.Context, id string) (io.ReadSeekCloser, storage.FileInfo, error) {
	dirs := []string{"avatars", "files"}

	for _, dir := range dirs {
		filePath := filepath.Join(s.baseDir, dir, id)
		file, err := os.Open(filePath)
		if err == nil {
			stat, err := file.Stat()
			if err != nil {
				file.Close()
				continue
			}

			contentType := "application/octet-stream"
			ext := filepath.Ext(filePath)
			switch ext {
			case ".jpg", ".jpeg":
				contentType = "image/jpeg"
			case ".png":
				contentType = "image/png"
			case ".webp":
				contentType = "image/webp"
			}

			info := storage.FileInfo{
				ID:          id,
				Path:        filePath,
				ContentType: contentType,
				Size:        stat.Size(),
				URL:         fmt.Sprintf("%s/files/%s", s.publicBaseURL, id),
			}

			return file, info, nil
		}
	}

	return nil, storage.FileInfo{}, fmt.Errorf("file not found")
}

func (s *LocalStorage) Delete(ctx context.Context, id string) error {
	dirs := []string{"avatars", "files"}

	for _, dir := range dirs {
		filePath := filepath.Join(s.baseDir, dir, id)
		if err := os.Remove(filePath); err == nil {
			return nil
		}
	}

	return fmt.Errorf("file not found")
}
