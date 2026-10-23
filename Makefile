.PHONY: backend-test backend-integration lint web-build build dev-backend dev-web

backend-test:
	cd server && go test ./...

backend-integration:
	cd server && go test -tags integration ./...

lint:
	cd server && golangci-lint run
	cd web && npm run lint
	cd web && npm run type-check

web-build:
	cd web && npm run build

build: web-build
	cd server && CGO_ENABLED=0 go build -o dbmanager ./cmd/dbmanager

dev-backend:
	cd server && go run ./cmd/dbmanager

dev-web:
	cd web && npm run dev
