.PHONY: ui build test clean dev-ui all

BINARY  = gtp_proxy
VERSION = 0.0.1d

all: ui build

# Build the React UI (required before `make build`)
ui:
	cd web && ([ -f package-lock.json ] && npm ci || npm install) && npm run build

# Build the Go binary (embeds web/dist if present)
build:
	mkdir -p bin
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/proxy

# Run tests
test:
	go test ./...

# Start Vite dev server (proxies API to localhost:8080)
dev-ui:
	cd web && npm run dev

clean:
	rm -rf bin/ web/dist/
