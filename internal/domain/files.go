package domain

import "time"

type FileMetadata struct {
	ID           string
	OriginalName string
	ContentType  string
	Size         int64
	Path         string
	CreatedAt    time.Time
}
