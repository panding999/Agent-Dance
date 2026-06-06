package config

import (
	"fmt"
	"os"
	"strings"
)

const defaultHTTPAddr = ":8080"

var requiredEnvKeys = []string{
	"DOUBAO_AST_RESOURCE_ID",
	"DOUBAO_AUC_RESOURCE_ID",
	"DATABASE_URL",
	"UPLOAD_DIR",
}

type Config struct {
	DoubaoAPIKey        string
	DoubaoAppID         string
	DoubaoAppKey        string
	DoubaoAccessKey     string
	DoubaoASTResourceID string
	DoubaoASTModelID    string
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

	apiKey := strings.TrimSpace(os.Getenv("DOUBAO_API_KEY"))
	appID := strings.TrimSpace(os.Getenv("DOUBAO_APP_ID"))
	appKey := strings.TrimSpace(os.Getenv("DOUBAO_APP_KEY"))
	accessKey := strings.TrimSpace(os.Getenv("DOUBAO_ACCESS_KEY"))
	if !hasDoubaoAuth(apiKey, appID, appKey, accessKey) {
		missing = append(missing, "DOUBAO_API_KEY or DOUBAO_APP_KEY or DOUBAO_APP_ID+DOUBAO_ACCESS_KEY")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	httpAddr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if httpAddr == "" {
		httpAddr = defaultHTTPAddr
	}

	return Config{
		DoubaoAPIKey:        apiKey,
		DoubaoAppID:         appID,
		DoubaoAppKey:        appKey,
		DoubaoAccessKey:     accessKey,
		DoubaoASTResourceID: strings.TrimSpace(os.Getenv("DOUBAO_AST_RESOURCE_ID")),
		DoubaoASTModelID:    strings.TrimSpace(os.Getenv("DOUBAO_AST_MODEL_ID")),
		DoubaoAUCResourceID: strings.TrimSpace(os.Getenv("DOUBAO_AUC_RESOURCE_ID")),
		DatabaseURL:         strings.TrimSpace(os.Getenv("DATABASE_URL")),
		UploadDir:           strings.TrimSpace(os.Getenv("UPLOAD_DIR")),
		HTTPAddr:            httpAddr,
	}, nil
}

func hasDoubaoAuth(apiKey string, appID string, appKey string, accessKey string) bool {
	return apiKey != "" || appKey != "" || (appID != "" && accessKey != "")
}
