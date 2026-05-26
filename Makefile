APP=abolqasem
PKG=./cmd/ai-agent-manager
DIST=dist

.PHONY: clean build test build-all web-build

web-build:
	sh scripts/prepare-web-assets.sh

clean:
	rm -rf $(DIST)

test:
	gofmt -l . | (! grep .)
	go vet ./...
	go test ./...

build: web-build
	mkdir -p $(DIST)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X ai-agent-manager/internal/buildinfo.Version=dev" -o $(DIST)/$(APP) $(PKG)

build-all: clean web-build
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X ai-agent-manager/internal/buildinfo.Version=dev" -o $(DIST)/$(APP)-linux-amd64 $(PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X ai-agent-manager/internal/buildinfo.Version=dev" -o $(DIST)/$(APP)-linux-arm64 $(PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w -X ai-agent-manager/internal/buildinfo.Version=dev" -o $(DIST)/$(APP)-darwin-amd64 $(PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w -X ai-agent-manager/internal/buildinfo.Version=dev" -o $(DIST)/$(APP)-darwin-arm64 $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -X ai-agent-manager/internal/buildinfo.Version=dev" -o $(DIST)/$(APP)-windows-amd64.exe $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags="-s -w -X ai-agent-manager/internal/buildinfo.Version=dev" -o $(DIST)/$(APP)-windows-arm64.exe $(PKG)
