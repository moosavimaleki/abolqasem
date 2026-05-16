package server

import "testing"

func TestIsLocalFileRoute(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/home/user/project/main.go:12", want: true},
		{path: "/home/user/project/main.go", want: true},
		{path: "/C:/Users/user/project/main.go:12", want: true},
		{path: "C:/Users/user/project/main.go:12", want: true},
		{path: "/styles.css", want: false},
		{path: "/api/state", want: false},
		{path: "/", want: false},
	}

	for _, test := range tests {
		if got := isLocalFileRoute(test.path); got != test.want {
			t.Fatalf("isLocalFileRoute(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}
