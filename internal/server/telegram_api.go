package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type telegramBridgeConfig struct {
	BotToken       string            `json:"botToken"`
	AllowedUserIDs []string          `json:"allowedUserIds"`
	ChatIDs        []string          `json:"chatIds,omitempty"`
	Mappings       map[string]string `json:"mappings,omitempty"`
}

func telegramBridgeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "telegram-bridge.json"
	}
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	return filepath.Join(codexHome, "telegram-bridge.json")
}

func loadTelegramBridgeConfig() (telegramBridgeConfig, error) {
	data, err := os.ReadFile(telegramBridgeConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return telegramBridgeConfig{}, nil
	}
	if err != nil {
		return telegramBridgeConfig{}, err
	}
	var config telegramBridgeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return telegramBridgeConfig{}, err
	}
	config.BotToken = strings.TrimSpace(config.BotToken)
	config.AllowedUserIDs = normalizeTelegramIDs(config.AllowedUserIDs)
	config.ChatIDs = normalizeTelegramIDs(config.ChatIDs)
	config.Mappings = normalizeTelegramMappings(config.Mappings)
	return config, nil
}

func saveTelegramBridgeConfig(config telegramBridgeConfig) error {
	config.BotToken = strings.TrimSpace(config.BotToken)
	config.AllowedUserIDs = normalizeTelegramIDs(config.AllowedUserIDs)
	config.ChatIDs = normalizeTelegramIDs(config.ChatIDs)
	config.Mappings = normalizeTelegramMappings(config.Mappings)
	if config.BotToken == "" {
		return errors.New("botToken is required")
	}
	if len(config.AllowedUserIDs) == 0 {
		return errors.New("at least one allowedUserId or * is required")
	}
	path := telegramBridgeConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func normalizeTelegramMappings(values map[string]string) map[string]string {
	result := map[string]string{}
	for chatID, workspaceChatID := range values {
		chatID = strings.TrimSpace(chatID)
		workspaceChatID = strings.TrimSpace(workspaceChatID)
		if !isTelegramNumericID(chatID) || workspaceChatID == "" {
			continue
		}
		result[chatID] = workspaceChatID
	}
	return result
}

func normalizeTelegramIDs(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(value, "telegram:"), "tg:"))
		if value == "" || (value != "*" && !isTelegramNumericID(value)) || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func isTelegramNumericID(value string) bool {
	for index, ch := range value {
		if ch == '-' && index == 0 {
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return value != "" && value != "-"
}

func handleAPITelegramConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	config, err := loadTelegramBridgeConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"botToken": config.BotToken, "allowedUserIds": config.AllowedUserIDs})
}

func handleAPITelegramStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	config, err := loadTelegramBridgeConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, workspaceTelegramBridge.Status(config))
}

func handleAPITelegramConfigure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var config telegramBridgeConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&config); err != nil {
		http.Error(w, "invalid telegram configuration", http.StatusBadRequest)
		return
	}
	existing, _ := loadTelegramBridgeConfig()
	config.ChatIDs = existing.ChatIDs
	config.Mappings = existing.Mappings
	if err := saveTelegramBridgeConfig(config); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	workspaceTelegramBridge.Reload()
	writeJSON(w, map[string]any{"ok": true})
}
