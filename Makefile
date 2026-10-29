.PHONY: backend-test backend-integration lint web-build web-test build dev-backend dev-web

backend-test:
	cd server && go test ./...

backend-integration:
	cd server && go test -tags integration -timeout 30m ./...

lint:
	cd server && golangci-lint run
	cd web && npm run lint
	cd web && npm run type-check

web-build:
	cd web && npm run build

web-test:
	cd web && npm run test

# NOTE: server/web/index.html (and the other server/web assets) are Vite
# build artifacts committed for go:embed; they are NOT hand-maintained.
# `make build` is the supported build path — it runs the Vite build first
# so the embedded SPA matches web/src. A bare `go build` embeds stale
# assets.
build: web-build
	cd server && CGO_ENABLED=0 go build -o dbmanager ./cmd/dbmanager

dev-backend:
	cd server && go run ./cmd/dbmanager

dev-web:
	cd web && npm run dev
