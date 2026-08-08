package cmd

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	log "github.com/sirupsen/logrus"
)

// DoTraeLogin exchanges a Trae CLI personal access token and saves the credential.
func DoTraeLogin(cfg *config.Config, pat string, options *LoginOptions) {
	if options == nil {
		options = &LoginOptions{}
	}
	promptFn := options.Prompt
	if promptFn == nil {
		promptFn = defaultProjectPrompt()
	}
	manager := newAuthManager()
	record, savedPath, err := manager.Login(context.Background(), "trae", cfg, &sdkAuth.LoginOptions{
		Prompt: promptFn,
		Metadata: map[string]string{
			"pat": pat,
		},
	})
	if err != nil {
		log.Errorf("Trae CLI authentication failed: %v", err)
		return
	}
	if savedPath != "" {
		fmt.Printf("Authentication saved to %s\n", savedPath)
	}
	if record != nil {
		fmt.Println("Trae CLI authentication successful")
	}
}
