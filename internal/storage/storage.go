package storage

import (
	"context"
	"io"
)

type SaveOptions struct {
	Directory    string
	ContentType  string
	OriginalName string
}

type FileInfo struct {
	ID          string
	Path        string
	ContentType string
	Size        int64
	URL         string
}

type Storage interface {
	Save(ctx context.Context, r io.Reader, opts SaveOptions) (FileInfo, error)
	Open(ctx context.Context, id string) (io.ReadSeekCloser, FileInfo, error)
	Delete(ctx context.Context, id string) error
}
