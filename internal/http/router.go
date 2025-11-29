package http

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/ondrasimku/media-service-go/internal/auth"
	"github.com/ondrasimku/media-service-go/internal/config"
	"github.com/ondrasimku/media-service-go/internal/http/handler"
	"github.com/ondrasimku/media-service-go/internal/storage"
)

func NewRouter(storage storage.Storage, maxFileSize int64, cfg *config.Config, logger *slog.Logger) *gin.Engine {
	router := gin.Default()

	healthHandler := handler.NewHealthHandler()
	uploadHandler := handler.NewUploadHandler(storage, maxFileSize, logger)

	router.GET("/healthz", healthHandler.Health)

	// authorize later
	router.GET("/files/:fileId", uploadHandler.GetFile)

	jwksClient := auth.NewJWKSClient(cfg.Auth.JWKSUrl, cfg.Auth.JWKSCacheTTL)
	authMiddleware := auth.AuthMiddleware(jwksClient, auth.Config{
		JWKSUrl:      cfg.Auth.JWKSUrl,
		Issuer:       cfg.Auth.Issuer,
		Audience:     cfg.Auth.Audience,
		JWKSCacheTTL: cfg.Auth.JWKSCacheTTL,
	})

	fileRoutes := router.Group("/files")
	fileRoutes.Use(authMiddleware)
	{
		fileRoutes.POST("", auth.RequirePermissions([]string{"files:upload"}), uploadHandler.Upload)
		//fileRoutes.GET("/:fileId", auth.RequirePermissions([]string{}), uploadHandler.GetFile)
	}

	return router
}
