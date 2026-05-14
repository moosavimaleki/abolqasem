package adapters

import "ai-session-viewer/internal/state"

type InstallScope string

const (
	ScopeUser    InstallScope = "user"
	ScopeProject InstallScope = "project"
)

type AgentAdapter interface {
	Name() string
	InstallHook(scope InstallScope) error
	UninstallHook(scope InstallScope) error
	IsHookInstalled(scope InstallScope) (bool, error)
	NormalizeHookInput(input []byte) (state.HookEvent, error)
}
