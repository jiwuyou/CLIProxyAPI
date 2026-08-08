package management

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
)

// ImportTraeIDECredential imports an uploaded Trae IDE storage.json file.
func (h *Handler) ImportTraeIDECredential(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config unavailable"})
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	source, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to read file: %v", err)})
		return
	}
	defer func() { _ = source.Close() }()

	temporary, err := os.CreateTemp("", "cliproxy-trae-storage-*.json")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stage credential"})
		return
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err = io.Copy(temporary, source); err != nil {
		_ = temporary.Close()
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to read file: %v", err)})
		return
	}
	if err = temporary.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stage credential"})
		return
	}

	ctx := context.Background()
	if requestContext := c.Request.Context(); requestContext != nil {
		ctx = requestContext
	}
	authenticator := sdkAuth.NewTraeAuthenticator()
	record, err := authenticator.Login(ctx, h.cfg, &sdkAuth.LoginOptions{Metadata: map[string]string{
		"path":    temporaryPath,
		"edition": strings.TrimSpace(c.PostForm("edition")),
	}})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Trae IDE credential", "message": err.Error()})
		return
	}
	savedPath, err := h.saveTokenRecord(ctx, record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save_failed", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"auth_file": filepath.Base(savedPath),
		"auth_kind": "ide",
		"label":     record.Label,
	})
}
