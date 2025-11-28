package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ondrasimku/media-service-go/internal/storage"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

type UploadHandler struct {
	storage     storage.Storage
	maxSize     int64
	allowedMIME map[string]bool
	logger      *slog.Logger
}

func NewUploadHandler(storage storage.Storage, maxSize int64, logger *slog.Logger) *UploadHandler {
	allowedMIME := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	}

	return &UploadHandler{
		storage:     storage,
		maxSize:     maxSize,
		allowedMIME: allowedMIME,
		logger:      logger,
	}
}

type UploadResponse struct {
	FileID      string `json:"fileId"`
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

func (h *UploadHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		h.logger.Warn("Failed to get file from form", "error", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "No file provided",
		})
		return
	}

	if file.Size > h.maxSize {
		h.logger.Warn("File too large", "size", file.Size, "max", h.maxSize)
		c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
			Error: "File too large",
		})
		return
	}

	src, err := file.Open()
	if err != nil {
		h.logger.Error("Failed to open uploaded file", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to process file",
		})
		return
	}
	defer src.Close()

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		ext := strings.ToLower(filepath.Ext(file.Filename))
		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".webp":
			contentType = "image/webp"
		default:
			contentType = "application/octet-stream"
		}
	}

	if !h.allowedMIME[contentType] {
		h.logger.Warn("Unsupported MIME type", "contentType", contentType)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Unsupported file type",
			Details: "Allowed types: image/jpeg, image/png, image/webp",
		})
		return
	}

	limitedReader := io.LimitReader(src, h.maxSize+1)

	ctx := c.Request.Context()
	fileInfo, err := h.storage.Save(ctx, limitedReader, storage.SaveOptions{
		Directory:    "avatars",
		ContentType:  contentType,
		OriginalName: file.Filename,
	})

	if err != nil {
		h.logger.Error("Failed to save file", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to save file",
		})
		return
	}

	response := UploadResponse{
		FileID:      fileInfo.ID,
		URL:         fileInfo.URL,
		ContentType: fileInfo.ContentType,
		Size:        fileInfo.Size,
	}

	h.logger.Info("File uploaded successfully", "fileId", fileInfo.ID, "size", fileInfo.Size)
	c.JSON(http.StatusOK, response)
}

func (h *UploadHandler) GetFile(c *gin.Context) {
	fileID := c.Param("fileId")
	if fileID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "File ID is required",
		})
		return
	}

	ctx := c.Request.Context()
	file, fileInfo, err := h.storage.Open(ctx, fileID)
	if err != nil {
		h.logger.Warn("File not found", "fileId", fileID, "error", err)
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "File not found",
		})
		return
	}
	defer file.Close()

	contentType := fileInfo.ContentType
	if contentType == "" || contentType == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(fileInfo.Path))
		switch ext {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".webp":
			contentType = "image/webp"
		default:
			contentType = "application/octet-stream"
		}
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Length", fmt.Sprintf("%d", fileInfo.Size))
	c.DataFromReader(http.StatusOK, fileInfo.Size, contentType, file, nil)
}
