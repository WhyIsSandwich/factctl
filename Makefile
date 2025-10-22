.PHONY: build build-all clean test lint

# Default build for current platform
build:
	go build -o factctl cmd/factctl/main.go

# Build for all platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o factctl-linux-amd64 cmd/factctl/main.go
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o factctl-linux-arm64 cmd/factctl/main.go
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o factctl-windows-amd64.exe cmd/factctl/main.go
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o factctl-darwin-amd64 cmd/factctl/main.go

# Build optimized production binary
build-prod:
	go build -ldflags="-s -w" -o factctl cmd/factctl/main.go

# Clean build artifacts
clean:
	rm -f factctl factctl-*

# Run tests
test:
	go test ./...

# Run linter
lint:
	golangci-lint run

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run the application
run: build
	./factctl --help

