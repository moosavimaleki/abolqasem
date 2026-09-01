package sidecar

import "time"

// Config is the child-process contract used by the supervisor. Secrets are
// deliberately represented as environment variable names, not values.
type Config struct {
	ListenAddress string
	ManagerHome   string
	ModelsCache   string
	UpstreamBase  string
	Proxy         string
	APIKeyEnv     string
}

type Health struct {
	OK                 bool   `json:"ok"`
	Version            string `json:"version,omitempty"`
	ConfiguredAccounts int    `json:"configuredAccounts,omitempty"`
	Loopback           bool   `json:"loopback"`
}

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateReady    State = "ready"
	StateDegraded State = "degraded"
	StateCrashed  State = "crashed"
)

type Status struct {
	State       State     `json:"state"`
	PID         int       `json:"pid,omitempty"`
	CrashCount  int       `json:"crashCount,omitempty"`
	Listen      string    `json:"listen,omitempty"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	LastHealthy time.Time `json:"lastHealthy,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
}
