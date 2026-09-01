package history

import "time"

type Sample struct {
	Account string             `json:"account"`
	At      time.Time          `json:"at"`
	Plan    string             `json:"plan,omitempty"`
	Windows map[string]float64 `json:"windows"`
}

type Point struct {
	At    time.Time `json:"at"`
	Value float64   `json:"value"`
}

type Series struct {
	Account  string  `json:"account"`
	Window   string  `json:"window"`
	Timezone string  `json:"timezone"`
	Points   []Point `json:"points"`
}
