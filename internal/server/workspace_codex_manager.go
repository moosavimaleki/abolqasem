package server

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"abolqasem/internal/providers/codex/configoverlay"
	"abolqasem/internal/providers/custom"
	"abolqasem/internal/secrets"
	"abolqasem/internal/state"
	"abolqasem/internal/workspace/agent"
)

const codexManagerGatewaySecretName = "codex-manager-gateway-key"

var workspaceGetCodexManagerSecret = secrets.Get
var workspaceCodexManagerRuntimeDir = func() string {
	return filepath.Join(state.GetStateDir(), "codex-manager", "app-server")
}

var workspaceGetCustomProviderSecret = secrets.Get
var workspaceCustomProviderSecretConfigured = secrets.Configured

type workspaceCodexRuntime struct {
	Env           []string
	ModelProvider string
	Fingerprint   string
}

func workspacePrepareCodexRuntime(baseEnv []string) (workspaceCodexRuntime, error) {
	settings, err := workspaceLoadProviderSettings()
	if err != nil {
		return workspaceCodexRuntime{Env: baseEnv}, err
	}
	if settings.CodexBackend.Mode != state.CodexBackendManager || !settings.CodexBackend.Enabled {
		return workspaceCodexRuntime{Env: baseEnv}, nil
	}

	key, err := workspaceGetCodexManagerSecret(codexManagerGatewaySecretName)
	if err != nil {
		return workspaceCodexRuntime{}, fmt.Errorf("Codex Manager is enabled but its gateway key is unavailable: %w", err)
	}
	overlay, err := configoverlay.Build(configoverlay.Options{
		SourceHome:  workspaceCodexRootDir(),
		RuntimeHome: workspaceCodexManagerRuntimeDir(),
		BaseURL:     settings.CodexBackend.ManagerBaseURL,
	})
	if err != nil {
		return workspaceCodexRuntime{}, err
	}
	env, err := overlay.Environment(key)
	if err != nil {
		return workspaceCodexRuntime{}, err
	}
	return workspaceCodexRuntime{
		Env:           state.MergeEnvOverrides(baseEnv, env),
		ModelProvider: configoverlay.ProviderID,
		Fingerprint:   workspaceCodexProviderFingerprint(configoverlay.ProviderID),
	}, nil
}

// workspacePrepareCodexTurn materializes the provider contract before a
// session is selected. This is important because a custom alias must become
// its upstream model ID for both thread/start and turn/start, while the chat
// itself continues to retain the user's original selection.
func workspacePrepareCodexTurn(request agent.TurnRequest) (agent.TurnRequest, workspaceCodexRuntime, error) {
	settings, err := workspaceLoadProviderSettings()
	if err != nil {
		return request, workspaceCodexRuntime{Env: request.Env}, err
	}
	if settings.CodexBackend.Mode != state.CodexBackendCustom || !settings.CodexBackend.Enabled {
		runtime, err := workspacePrepareCodexRuntime(request.Env)
		if err != nil {
			return request, runtime, err
		}
		request.Env = runtime.Env
		request.CodexModelProvider = runtime.ModelProvider
		return request, runtime, nil
	}

	providerID := strings.TrimSpace(settings.CodexBackend.CustomProviderID)
	provider, ok := settings.CodexBackend.CustomProviders[providerID]
	if !ok || providerID == "" {
		return request, workspaceCodexRuntime{}, fmt.Errorf("custom provider is enabled but no configured provider is selected")
	}
	model, err := custom.ResolveModel(custom.Config{Models: customModels(provider.Models)}, request.Model)
	if err != nil {
		return request, workspaceCodexRuntime{}, fmt.Errorf("resolve custom provider model: %w", err)
	}
	providerRuntimeID := customProviderRuntimeID(providerID)
	overlay, err := configoverlay.BuildCustom(configoverlay.CustomOptions{
		SourceHome: workspaceCodexRootDir(), RuntimeHome: filepath.Join(workspaceCodexManagerRuntimeDir(), "custom", providerRuntimeID),
		ProviderID: providerRuntimeID, ProviderName: provider.Name, BaseURL: provider.BaseURL, APIKeyEnv: provider.EnvKey,
	})
	if err != nil {
		return request, workspaceCodexRuntime{}, err
	}
	apiKey := ""
	if strings.TrimSpace(provider.EnvKey) != "" && workspaceCustomProviderSecretConfigured(customProviderSecretName(providerID)) {
		apiKey, err = workspaceGetCustomProviderSecret(customProviderSecretName(providerID))
		if err != nil {
			return request, workspaceCodexRuntime{}, fmt.Errorf("read custom provider credential: %w", err)
		}
	}
	runtime := workspaceCodexRuntime{
		Env: state.MergeEnvOverrides(request.Env, overlay.OptionalEnvironment(apiKey)), ModelProvider: overlay.ProviderID,
		Fingerprint: workspaceCodexProviderFingerprint(overlay.ProviderID),
	}
	request.Env = runtime.Env
	request.CodexModelProvider = runtime.ModelProvider
	request.Model = model.UpstreamID
	return request, runtime, nil
}

func customModels(models []state.CustomProviderModelSettings) []custom.Model {
	result := make([]custom.Model, 0, len(models))
	for _, model := range models {
		result = append(result, custom.Model{ID: model.ID, UpstreamID: model.UpstreamID, DisplayName: model.DisplayName, ReasoningEfforts: model.ReasoningEfforts, InputModalities: model.InputModalities})
	}
	return result
}

var nonProviderIdentifier = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func customProviderRuntimeID(providerID string) string {
	providerID = nonProviderIdentifier.ReplaceAllString(strings.TrimSpace(providerID), "_")
	return "abolqasem_custom_" + strings.Trim(providerID, "_")
}

func customProviderSecretName(providerID string) string { return "custom-provider-" + providerID }

func customProviderAPIKeyEnv(providerID string) string {
	providerID = strings.ToUpper(nonProviderIdentifier.ReplaceAllString(strings.TrimSpace(providerID), "_"))
	return "ABOLQASEM_CUSTOM_PROVIDER_" + strings.Trim(providerID, "_") + "_API_KEY"
}

func workspaceCodexProviderFingerprint(provider string) string {
	return strings.TrimSpace(strings.ToLower(provider))
}
