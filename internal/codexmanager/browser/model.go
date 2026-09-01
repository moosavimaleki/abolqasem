package browser

type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Directory   string `json:"directory"`
	Root        string `json:"root"`
	Path        string `json:"path"`
	CookieDB    string `json:"cookieDb"`
	LocalState  string `json:"localState"`
	Preferences string `json:"preferences"`
}

type Cookie struct {
	Name   string `json:"name"`
	Value  string `json:"-"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
	Secure bool   `json:"secure"`
}

type Session struct {
	ProfileID   string `json:"profileId"`
	Account     string `json:"account,omitempty"`
	Application string `json:"application"`
	Device      string `json:"device,omitempty"`
	Current     bool   `json:"current"`
	Revocable   bool   `json:"revocable"`
}
