package sessioninterop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func claudeRootDir() string {
	if value := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("CLAUDE_HOME")); value != "" {
		return value
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func codexRootDir() string {
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		return value
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func claudeProjectSlug(localPath string) string {
	localPath = filepath.Clean(strings.TrimSpace(localPath))
	if localPath == "" {
		return "-unknown-project"
	}
	slug := strings.ReplaceAll(localPath, string(filepath.Separator), "-")
	if filepath.Separator != '/' {
		slug = strings.ReplaceAll(slug, "/", "-")
	}
	if !strings.HasPrefix(slug, "-") {
		slug = "-" + slug
	}
	return slug
}

func generateSessionToken() string {
	now := time.Now().UTC()
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", now.UnixNano()&0xffffffff, now.Nanosecond()&0xffff, (now.UnixNano()>>16)&0xffff, (now.UnixNano()>>32)&0xffff, now.UnixNano()&0xffffffffffff)
}

func codexThreadID() string {
	return generateSessionToken()
}
