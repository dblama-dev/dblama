.PHONY: backend-test backend-integration lint web-build web-test build dev-backend dev-web

backend-test:
	go test ./...

backend-integration:
	go test -tags integration -timeout 30m ./...

lint:
	golangci-lint run
	cd frontend && npm run lint
	cd frontend && npm run type-check

web-build:
	cd frontend && npm run build

web-test:
	cd frontend && npm run test

# Build the SPA before Go embeds frontend/dist in the executable.
build: web-build
	CGO_ENABLED=0 go build -o dblama ./cmd/dblama

dev-backend:
	go run ./cmd/dblama

dev-web:
	cd frontend && npm run dev
