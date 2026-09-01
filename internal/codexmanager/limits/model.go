package limits

import "time"

type Window struct {
	Label             string     `json:"label"`
	UsedPercent       float64    `json:"usedPercent"`
	RemainingPercent  float64    `json:"remainingPercent"`
	WindowMinutes     *int       `json:"windowMinutes,omitempty"`
	ResetAfterSeconds *int       `json:"resetAfterSeconds,omitempty"`
	ResetAt           *time.Time `json:"resetAt,omitempty"`
	Reached           bool       `json:"reached"`
}

type Credits struct {
	HasCredits bool   `json:"hasCredits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance,omitempty"`
}

type Limit struct {
	ID           string   `json:"id"`
	Name         string   `json:"name,omitempty"`
	Allowed      bool     `json:"allowed"`
	LimitReached bool     `json:"limitReached"`
	Plan         string   `json:"plan,omitempty"`
	Windows      []Window `json:"windows"`
	Credits      *Credits `json:"credits,omitempty"`
}

type Snapshot struct {
	Account     string    `json:"account"`
	Limits      []Limit   `json:"limits"`
	Plan        string    `json:"plan,omitempty"`
	ReachedType string    `json:"reachedType,omitempty"`
	FetchedAt   time.Time `json:"fetchedAt"`
	Error       string    `json:"error,omitempty"`
}
