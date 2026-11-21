# db-manager

A self-hosted database administration interface for MySQL, MariaDB and PostgreSQL. The Vue frontend is embedded in one Go executable. No application database or persistent password store is required.

The integration matrix targets MySQL 8.4, MariaDB 11.4 and PostgreSQL 17. Features are shown according to the connected server's capabilities and account permissions.

## Run a binary

Extract the archive for your OS and architecture, verify it against `checksums.txt`, then run:

```sh
LISTEN=127.0.0.1:8080 ./dbmanager
```

On Windows, run `dbmanager.exe` and set environment variables through PowerShell or system settings. Release archives cover Linux, macOS and Windows on amd64 and arm64. They contain the executable, this README and the changelog. No public release has been published from this checkout.

Open the server URL, log in with your database credentials, and choose a database from the server overview. Database connections originate from the db-manager server, so `localhost` names the server or container running db-manager.

Session cookies are always `Secure`, `HttpOnly` and `SameSite=Strict`. Deploy behind an HTTPS reverse proxy for ordinary remote access. Browsers can treat HTTP localhost as a secure cookie exception for local development. The process serves HTTP; TLS termination belongs at the proxy. Current database connectors use unencrypted database connections, so place db-manager and its database servers on a trusted private network.

## Run with Docker

Build the image locally:

```sh
docker build -t db-manager:local .
docker run --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  -p 127.0.0.1:8080:8080 \
  -e TRUSTED_SERVERS=db.example.internal:5432,mysql.example.internal:3306 \
  db-manager:local
```

The final image uses `gcr.io/distroless/static-debian13:nonroot`, runs as UID/GID 65532, and has no shell. The frontend and Go compiler exist only in build stages. No writable application volume is needed. Connect the container to the database network, and expose its HTTP endpoint through your HTTPS proxy.

For multi-architecture images, use a configured Buildx builder:

```sh
docker buildx build --platform linux/amd64,linux/arm64 -t db-manager:local --output type=oci,dest=db-manager.oci.tar .
```

This writes a local OCI archive. Registry upload is a separate operator action.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `LISTEN` | `:8080` | HTTP listen address. Use `127.0.0.1:8080` for a local binary. |
| `SESSION_TTL` | `30m` | Idle session lifetime, using Go duration syntax. Sessions expire or disappear when the process restarts. |
| `LOGIN_RATE_LIMIT` | `10/min` | Failed login limit per client IP; supports `N/min` or `N/s`. |
| `TRUSTED_SERVERS` | empty | Comma-separated allowed database `host:port` targets. Empty permits arbitrary targets reachable by the server. |

The database account determines SQL permissions. db-manager does not introduce a read-only mode or a second authorization system. Restrict network access and allowed connection targets for the users of your deployment.

## Features and operational limits

- Browse tables and views, filter and sort rows, edit data, and change table columns, indexes, options and foreign keys. Without a primary key, row changes match all column values; duplicate matching rows can all be updated or deleted. The UI warns before these operations.
- Table filters, ordering and pagination persist in the URL. Table actions preserve database/schema context. Long values have compact previews and a full-value viewer; selected rows with a primary key can be updated with confirmation and explicit partial results.
- Row forms distinguish values, NULL and server defaults, preserve exact large numbers, and support JSON, dates, binary files and foreign-key label/ID search. Composite foreign keys use manual entry. Bulk edits require a primary key, save rows sequentially and stop on failure; earlier changes remain.
- SQL drafts and named queries stay in the browser, separated by connection/user/database/schema. Ctrl/Cmd+Enter runs selected SQL or the entire editor. Storage is bounded to 30 workspaces, 50 saved queries per workspace and 1 MB per query. Nothing runs automatically when opening a table SQL action.
- Database text search covers up to 20 selected tables with two concurrent requests and 20 results per text column. Literal %, underscore and backslash are supported; case sensitivity follows database collation. Numeric, binary, JSON and array columns are excluded. Results link to persistent table filters; scans may still be expensive.
- SQL exports offer structure/data/both modes. PostgreSQL data-only exports include local referenced parents before children and reject FK cycles before output; use structure-and-data mode for cycles. MySQL exports use the selected database even when login omitted a default database.
- For PostgreSQL, you can leave the database field empty at login and open a database from the server overview or sidebar. Each tab's catalog, queries, imports and exports use the selected database and schema with your login credentials. To rename or drop the database used for the initial connection, first log in to a different database.
- MySQL database rename is a maintenance operation implemented as copy then drop with source write locks. Views, routines, events, triggers and foreign-key references that cannot be preserved are refused. Copy/rename may require substantial temporary disk space and locks; failed operations can leave a new target or earlier successful steps while retaining the source. Renaming or dropping the login database requires reconnecting without a default database.
- PostgreSQL structure/FK changes use transactions. MySQL DDL commits each step; FK replacement attempts to restore its previous definition on error, and reports if restoration fails. Earlier successful steps can remain applied.
- Object editors accept full SQL CREATE source. SQL console, imports and source editors execute with the connected database account's privileges. Source-header validation checks the selected kind/name; it does not sandbox stored SQL bodies. MySQL routine/trigger/event updates recreate the object and attempt restoration on failure; connection loss can prevent restoration, and object-specific grants may need to be reapplied.
- Procedure calls accept a JSON array of scalar parameters; include `null` for OUT parameters. OUT/INOUT values are shown with the result sets. PostgreSQL overloads are selected by their catalog signatures.
- UI CSV/TSV exports use `\N` for NULL, preserve empty strings, and escape literal leading backslashes. Choose **Database export** encoding when importing these files; choose **External CSV** to retain external fields literally. CSV preview supports column mapping and ignored fields; omitted targets use server defaults. Headers are exported. Legacy API callers retain their previous defaults, including MySQL empty-field-to-NULL behavior when mapping is omitted.
- SQL imports execute statements incrementally. On error, already applied statements remain; CSV imports commit batches. Plain/decompressed SQL import input is limited to 512 MiB. Cancellation stops future work but does not undo committed changes.
- SQL table dumps are not a complete server backup: inspect database-specific objects, users, grants, extensions and external dependencies separately. Keep a database-native backup for recovery.
- MySQL/MariaDB account management and privilege editing require administrative privileges. PostgreSQL role management, SSO, charts and a read-only application mode are outside v0.1.

## Build and develop

Use Go 1.26.8 or newer, Node.js 24 and npm. From the repository root:

```sh
npm --prefix web ci
make build
```

`make build` builds the Vue SPA into `server/web` before compiling `server/cmd/dbmanager`. A bare `go build` can embed stale frontend assets and is not the supported release build path.

Run backend and frontend development servers in separate terminals:

```sh
make dev-backend
make dev-web
```

Vite proxies `/api` to port 8080. The backend supports graceful shutdown on SIGINT/SIGTERM.

## Checks

```sh
make backend-test
make web-test
make lint
make backend-integration
```

Integration tests use testcontainers and require Docker, with the MySQL/MariaDB/PostgreSQL images above. The integration target allows 30 minutes per Go package because each scenario starts fresh database containers. `make lint` also requires `golangci-lint` v2. Frontend tests use two workers to keep resource usage predictable.

## Release preparation

Install [GoReleaser v2.19.0](https://goreleaser.com/install/). From a checkout with its GitHub remote configured, run:

```sh
./scripts/check-release.sh
./scripts/check-release.sh --docker
```

Each run builds a snapshot locally without publishing. GoReleaser configuration validation requires a repository remote even for a snapshot; a checkout without one can be tested in a disposable local clone with a test-only remote. The second command also builds and starts the nonroot read-only Docker image. The script verifies all six target executables, archive checksums, embedded SPA/assets, driver discovery and authenticated endpoint protection. It needs Bash, Node/npm, Go and GoReleaser; the Docker check additionally needs Docker and curl.

`.goreleaser.yaml` always installs the locked frontend dependencies and builds the SPA before cross-compiling with `CGO_ENABLED=0`. Linux/macOS archives use tar.gz, Windows uses zip, and every archive has a SHA-256 checksum.

The release workflow runs the test matrix. Manual workflow dispatch produces snapshot artifacts. An operator-pushed semantic version tag such as `v0.1.0` creates a draft GitHub release and publishes a nonroot multi-architecture image to `ghcr.io/OWNER/REPOSITORY` using the repository token. An operator reviews and publishes the draft release. Manual workflow dispatch creates snapshot artifacts and performs a local Docker smoke check without publishing. This checkout has no configured remote; no release or image has been published here. Configure a GitHub remote and package permissions before pushing a release tag.

A project license has not been selected in this checkout; no distribution license is asserted here.

## Browser end-to-end checks

Playwright tests drive Chromium against the built SPA, Go server and a real DB.
They cover login/logout, table structure and AUTO_INCREMENT/primary keys, row
CRUD/copy/upsert, filters/sorting/pagination, SQL errors and recovery, SQL/CSV
roundtrips, database lifecycle, views and monitoring. See the scenario matrix in
`docs/superpowers/specs/2026-10-29-e2e-workflows-design.md`.

```sh
npm --prefix web ci
(cd web && npx playwright install --with-deps chromium)
./scripts/check-e2e.sh mysql
./scripts/check-e2e.sh mariadb
./scripts/check-e2e.sh postgres
```

Each run builds the application and starts it as a local binary on port 18089;
it refuses to reuse an already running application. Set `E2E_APP_PORT` to use
another free port. Run engines sequentially in one checkout; parallel runs need
separate checkouts because build files and reports are shared. Only the disposable
database uses Docker, on a random loopback
port. The runner removes that container and its volumes on exit, including test
failures. It never uses existing containers. CI runs all three engines separately.
Playwright uses one worker and no retries; failures keep a screenshot and trace in
`web/test-results/` and an HTML report in `web/playwright-report/`.

To test without Docker, provision a **dedicated disposable test database** first.
The user needs table/object privileges and CREATE/DROP DATABASE for lifecycle tests.
Pass explicit connection parameters (for example through your shell environment):

```sh
export E2E_DB_HOST=127.0.0.1 E2E_DB_PORT=3306
export E2E_DB_USER=e2e_user E2E_DB_NAME=dbmanager_e2e
read -rs -p 'Test database password: ' E2E_DB_PASSWORD
export E2E_DB_PASSWORD
./scripts/check-e2e.sh mysql --external
```

External mode creates and removes only uniquely named `e2e_...` test objects, but
an interrupted process can leave these objects behind. Do not point it at a
production database. Narrow a run by passing Playwright arguments after the engine
(and `--external`, when used), e.g. `./scripts/check-e2e.sh mysql structure.spec.ts`.
This is an initial critical-workflow suite; it does not yet cover every engine
feature, all administrative privileges or browsers other than Chromium.
