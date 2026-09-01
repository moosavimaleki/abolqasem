package recommendation

import (
	"time"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/limits"
)

type Label string

const (
	Best  Label = "best"
	Risk  Label = "risk"
	Save  Label = "save"
	Stale Label = "stale"
	Check Label = "check"
	Login Label = "login"
	OK    Label = "ok"
)

type Result struct {
	Account       string  `json:"account"`
	Label         Label   `json:"label"`
	Score         float64 `json:"score"`
	Reason        string  `json:"reason"`
	Recommendable bool    `json:"recommendable"`
	Best          bool    `json:"best"`
	Remaining     float64 `json:"remaining,omitempty"`
	Target        float64 `json:"target,omitempty"`
	Health        float64 `json:"health,omitempty"`
	PacingLabel   string  `json:"pacingLabel,omitempty"`
}

type Candidate struct {
	Name   string
	Plan   account.Plan
	State  account.State
	Limits limits.Snapshot
}

type Selection struct {
	Results map[string]Result
	Best    *Result
	At      time.Time
}
