package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactCodexLogValueRemovesPromptAndSecrets(t *testing.T) {
	payload := map[string]any{
		"method": "turn/start",
		"params": map[string]any{
			"apiKey":        "sk-testsecret123456789",
			"authorization": "Bearer ghp_testsecret123456789",
			"tokenUsage": map[string]any{
				"totalTokens": 42,
			},
			"input": []any{
				map[string]any{
					"type": "text",
					"text": "prompt with sk-anothersecret123456789",
				},
			},
		},
	}

	data, err := json.Marshal(redactCodexLogValue(payload))
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	logLine := string(data)
	for _, secret := range []string{"sk-testsecret", "ghp_testsecret", "sk-anothersecret", "prompt with"} {
		if strings.Contains(logLine, secret) {
			t.Fatalf("expected %q to be redacted from %s", secret, logLine)
		}
	}
	if !strings.Contains(logLine, `"method":"turn/start"`) {
		t.Fatalf("expected method to remain in redacted log, got %s", logLine)
	}
	if !strings.Contains(logLine, `"totalTokens":42`) {
		t.Fatalf("expected non-secret token usage to remain in redacted log, got %s", logLine)
	}
}

func TestRedactCodexLogTextRemovesCommonSecretFormats(t *testing.T) {
	logLine := "Authorization: Bearer ghp_testsecret123456789 OPENAI_API_KEY=sk-testsecret123456789 token=secret-token-value"

	redacted := redactCodexLogText(logLine)
	for _, secret := range []string{"ghp_testsecret", "sk-testsecret", "secret-token-value"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("expected %q to be redacted from %s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "Authorization: Bearer [redacted]") {
		t.Fatalf("expected authorization prefix to remain, got %s", redacted)
	}
}
