package config

import (
	"fmt"
	"os"
	"strings"
)

const defaultHTTPAddr = ":8080"

var requiredEnvKeys = []string{
	"DOUBAO_APP_KEY",
	"DOUBAO_ACCESS_KEY",
	"DOUBAO_AST_RESOURCE_ID",
	"DOUBAO_AUC_RESOURCE_ID",
	"DATABASE_URL",
	"UPLOAD_DIR",
}

type Config struct {
	DoubaoAppKey        string
	DoubaoAccessKey     string
	DoubaoASTResourceID string
	DoubaoAUCResourceID string
	DatabaseURL         string
	UploadDir           string
	HTTPAddr            string
}

func LoadFromEnv() (Config, error) {
	missing := make([]string, 0, len(requiredEnvKeys))
	for _, key := range requiredEnvKeys {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	httpAddr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if httpAddr == "" {
		httpAddr = defaultHTTPAddr
	}

	return Config{
		DoubaoAppKey:        strings.TrimSpace(os.Getenv("DOUBAO_APP_KEY")),
		DoubaoAccessKey:     strings.TrimSpace(os.Getenv("DOUBAO_ACCESS_KEY")),
		DoubaoASTResourceID: strings.TrimSpace(os.Getenv("DOUBAO_AST_RESOURCE_ID")),
		DoubaoAUCResourceID: strings.TrimSpace(os.Getenv("DOUBAO_AUC_RESOURCE_ID")),
		DatabaseURL:         strings.TrimSpace(os.Getenv("DATABASE_URL")),
		UploadDir:           strings.TrimSpace(os.Getenv("UPLOAD_DIR")),
		HTTPAddr:            httpAddr,
	}, nil
}
