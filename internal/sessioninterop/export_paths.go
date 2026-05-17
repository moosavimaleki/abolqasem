package sessioninterop

import (
	"crypto/sha256"
	"encoding/hex"
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

func geminiRootDir() string {
	home, _ := os.UserHomeDir()
	base := strings.TrimSpace(os.Getenv("GEMINI_CLI_HOME"))
	if base == "" {
		return filepath.Join(home, ".gemini")
	}
	base = strings.TrimSpace(base)
	if filepath.Base(base) == ".gemini" {
		return base
	}
	return filepath.Join(base, ".gemini")
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

func geminiProjectHash(localPath string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(strings.TrimSpace(localPath))))
	return hex.EncodeToString(sum[:])
}

func geminiContainerName(localPath string) string {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(localPath)))
	base = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return '-'
		default:
			return '-'
		}
	}, base)
	base = strings.Trim(base, "-")
	base = strings.ReplaceAll(base, "_", "-")
	for strings.Contains(base, "--") {
		base = strings.ReplaceAll(base, "--", "-")
	}
	if base == "" {
		base = geminiProjectHash(localPath)
	}
	return base
}
