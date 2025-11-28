package http

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/ondrasimku/media-service-go/internal/http/handler"
	"github.com/ondrasimku/media-service-go/internal/storage"
)

func NewRouter(storage storage.Storage, maxFileSize int64, logger *slog.Logger) *gin.Engine {
	router := gin.Default()

	healthHandler := handler.NewHealthHandler()
	uploadHandler := handler.NewUploadHandler(storage, maxFileSize, logger)

	router.GET("/healthz", healthHandler.Health)
	router.POST("/files", uploadHandler.Upload)
	router.GET("/files/:fileId", uploadHandler.GetFile)

	return router
}
