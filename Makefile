.PHONY: backend-test lint web-build build

backend-test:
	cd server && go test ./...

lint:
	cd server && golangci-lint run

web-build:
	@if [ -f web/package.json ]; then \
		cd web && npm run build; \
	else \
		echo "web/ not scaffolded yet; nothing to build"; \
	fi

build: web-build
	cd server && CGO_ENABLED=0 go build -o dbmanager ./cmd/dbmanager
