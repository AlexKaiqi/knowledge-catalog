package clientconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Endpoint struct {
	Server string `json:"server"`
}

func SavedServer() string {
	for _, path := range []string{filepath.Join(ConfigDir(), "client.json"), filepath.Join(ConfigDir(), "session-taihu.json"), filepath.Join(ConfigDir(), "session-local.json")} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var config Endpoint
		if json.Unmarshal(raw, &config) == nil && NormalizeServer(config.Server) != "" {
			return NormalizeServer(config.Server)
		}
	}
	return ""
}

func ConfigDir() string {
	if dir := strings.TrimSpace(os.Getenv("KC_CONFIG_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".kc")
	}
	return filepath.Join(home, ".config", "kc")
}

func NormalizeServer(server string) string {
	return strings.TrimRight(strings.TrimSpace(server), "/")
}

func SessionPath(server, filename string) string {
	digest := sha256.Sum256([]byte(NormalizeServer(server)))
	return filepath.Join(ConfigDir(), "sessions", hex.EncodeToString(digest[:]), filename)
}

// ServerURL selects an explicit service, environment selection, then the saved endpoint.
func ServerURL(explicit string) string {
	if value := strings.TrimSpace(explicit); value != "" {
		return value
	}
	if server := strings.TrimSpace(os.Getenv("KC_SERVER_URL")); server != "" {
		return server
	}
	return SavedServer()
}
