package custom

type Model struct {
	ID               string   `json:"id"`
	UpstreamID       string   `json:"upstreamId"`
	DisplayName      string   `json:"displayName,omitempty"`
	ReasoningEfforts []string `json:"reasoningEfforts,omitempty"`
	InputModalities  []string `json:"inputModalities,omitempty"`
}
type Provider struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	BaseURL string  `json:"baseUrl"`
	WireAPI string  `json:"wireApi"`
	Models  []Model `json:"models,omitempty"`
}
