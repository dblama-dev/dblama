# Developing dblama

The Go server starts in `cmd/dblama/`. The API and database drivers live in `internal/`, and the Vue application lives in `frontend/`. Run the commands below from the repository root.

## Local development

Install Go 1.26.8 or newer, Node.js 24, npm and Make. Install the frontend dependencies once per checkout with `npm --prefix frontend ci`.

Run `make dev-backend` in one terminal and `make dev-web` in another. Open the URL printed by Vite. It reloads the frontend as you edit and forwards `/api` requests to the Go server on port 8080. Restart the backend after editing Go code.

## Build a standalone binary

```sh
make build
LISTEN=127.0.0.1:8080 ./dblama
```

`make build` compiles the Vue application into `frontend/dist/` before Go embeds it in the executable. Open [localhost:8080](http://localhost:8080) to use the built application.

## Test a change

| Command | What it checks |
| --- | --- |
| `make backend-test` | Go unit tests |
| `make web-test` | Frontend unit and component tests |
| `make lint` | Go lint, ESLint and TypeScript types. Requires golangci-lint v2. |
| `make backend-integration` | Database integration tests. Requires Docker. |

For a browser test, install Chromium and run Playwright against one database engine at a time. The runner builds dblama and starts a disposable database in Docker.

```sh
(cd frontend && npx playwright install --with-deps chromium)
./scripts/check-e2e.sh postgres
```

Use `mysql`, `mariadb` or `postgres` as the engine. To run one scenario, append its filename, for example `./scripts/check-e2e.sh mysql session.spec.ts`.

## Check release archives

With GoReleaser v2.19.0 installed and a GitHub remote configured, run `./scripts/check-release.sh`. It builds local snapshot archives and verifies the executables, checksums and embedded frontend. Add `--docker` to build and start the container image as well.
