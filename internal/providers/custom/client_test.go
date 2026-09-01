package custom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverModelsUsesHeadersAndDeduplicates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" || request.Header.Get("Authorization") != "Bearer key" || request.Header.Get("X-Provider") != "test" {
			t.Fatalf("unexpected request: %s %#v", request.URL, request.Header)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"beta"},{"id":"alpha"},{"id":"alpha"}]}`))
	}))
	defer server.Close()

	models, err := (Client{HTTPClient: server.Client()}).Discover(context.Background(), Config{BaseURL: server.URL + "/v1", Headers: map[string]string{"X-Provider": "test"}}, "key")
	if err != nil || len(models) != 2 || models[0].ID != "alpha" || models[1].UpstreamID != "beta" {
		t.Fatalf("models=%#v err=%v", models, err)
	}
}

func TestResolveModelSupportsOptionalMappingAndPassthrough(t *testing.T) {
	mapped, err := ResolveModel(Config{Models: []Model{{ID: "friendly", UpstreamID: "vendor/internal-model"}}}, "friendly")
	if err != nil || mapped.UpstreamID != "vendor/internal-model" {
		t.Fatalf("mapped=%#v err=%v", mapped, err)
	}
	passthrough, err := ResolveModel(Config{}, "gpt-5.6")
	if err != nil || passthrough.UpstreamID != "gpt-5.6" {
		t.Fatalf("passthrough=%#v err=%v", passthrough, err)
	}
}

func TestValidateRejectsDuplicateAndUnsupportedMappings(t *testing.T) {
	base := Config{ID: "vendor", Name: "Vendor", BaseURL: "https://api.example.test/v1"}
	base.Models = []Model{{ID: "one", UpstreamID: "upstream"}, {ID: "two", UpstreamID: "upstream"}}
	if err := Validate(base); err == nil {
		t.Fatal("expected duplicate upstream mapping rejection")
	}
	base.Models = []Model{{ID: "one", UpstreamID: "upstream", ReasoningEfforts: []string{"ultra"}}}
	if err := Validate(base); err == nil {
		t.Fatal("expected unsupported reasoning effort rejection")
	}
}
