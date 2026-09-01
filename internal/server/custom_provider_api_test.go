package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCustomProviderPreviewMapsModelAndRedactsHeaders(t *testing.T) {
	body := `{
  "provider":{"id":"vendor","name":"Vendor","baseUrl":"https://vendor.example/v1","wireApi":"responses","models":[{"id":"fast","upstreamId":"vendor-fast","reasoningEfforts":["high"]}]},
  "headers":{"X-Provider-Key":"header-secret"},
  "apiKey":"api-secret",
  "modelId":"fast"
}`
	request := httptest.NewRequest(http.MethodPost, "/api/custom-providers/preview", bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	handleAPICustomProviderPreview(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("preview failed: %d %s", response.Code, response.Body.String())
	}
	encoded := response.Body.String()
	if !strings.Contains(encoded, `"upstreamModelId":"vendor-fast"`) || strings.Contains(encoded, "header-secret") || strings.Contains(encoded, "api-secret") {
		t.Fatalf("preview leaked secret or lost mapping: %s", encoded)
	}
}

func TestCustomProviderTestDiscoversModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer api-key" {
			t.Fatalf("unexpected upstream request: %s %#v", r.URL, r.Header)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"vendor-model"}]}`))
	}))
	defer upstream.Close()
	body := `{"provider":{"id":"vendor","name":"Vendor","baseUrl":"` + upstream.URL + `/v1","wireApi":"responses"},"apiKey":"api-key","discover":true}`
	response := httptest.NewRecorder()
	handleAPICustomProviderTest(response, httptest.NewRequest(http.MethodPost, "/api/custom-providers/test", bytes.NewBufferString(body)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "vendor-model") || strings.Contains(response.Body.String(), "api-key") {
		t.Fatalf("unexpected test response: %d %s", response.Code, response.Body.String())
	}
}
