package cmd

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	log "github.com/sirupsen/logrus"
)

// DoTraeImport imports a Trae IDE storage.json credential.
func DoTraeImport(cfg *config.Config, storagePath, edition string, options *LoginOptions) {
	if options == nil {
		options = &LoginOptions{}
	}
	manager := newAuthManager()
	record, savedPath, err := manager.Login(context.Background(), "trae", cfg, &sdkAuth.LoginOptions{
		NoBrowser: options.NoBrowser,
		Prompt:    options.Prompt,
		Metadata: map[string]string{
			"path":    storagePath,
			"edition": edition,
		},
	})
	if err != nil {
		log.Errorf("Trae credential import failed: %v", err)
		return
	}
	if savedPath != "" {
		fmt.Printf("Authentication saved to %s\n", savedPath)
	}
	if record != nil && record.Label != "" {
		fmt.Printf("Imported Trae account %s\n", record.Label)
	}
}
