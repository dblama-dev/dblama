#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
engine="${1:-mysql}"
if [[ $# -gt 0 ]]; then shift; fi
external=false
if [[ "${1:-}" == '--external' ]]; then external=true; shift; fi
case "$engine" in mysql|mariadb|postgres) ;; *) echo 'Usage: check-e2e.sh mysql|mariadb|postgres [--external] [Playwright arguments]' >&2; exit 2;; esac
export E2E_ENGINE="$engine"
container=''
cleanup() { if [[ -n "$container" ]]; then docker rm -f -v "$container" >/dev/null; fi; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
if $external; then
  : "${E2E_DB_HOST:?Set E2E_DB_HOST for a disposable test database}"
  : "${E2E_DB_PORT:?Set E2E_DB_PORT}"
  : "${E2E_DB_USER:?Set E2E_DB_USER}"
  : "${E2E_DB_PASSWORD:?Set E2E_DB_PASSWORD}"
  : "${E2E_DB_NAME:?Set E2E_DB_NAME for a disposable test database}"
else
  export E2E_DB_HOST=127.0.0.1 E2E_DB_NAME=dbmanager_e2e E2E_DB_PASSWORD=e2e-password
  case "$engine" in
    mysql) image=mysql:8.4; port=3306; export E2E_DB_USER=root; args=(-e MYSQL_ROOT_PASSWORD="$E2E_DB_PASSWORD" -e MYSQL_ROOT_HOST=% -e MYSQL_DATABASE="$E2E_DB_NAME"); client=(mysql --protocol=tcp -h127.0.0.1 -uroot -p"$E2E_DB_PASSWORD" -e 'SELECT 1');;
    mariadb) image=mariadb:11.4; port=3306; export E2E_DB_USER=root; args=(-e MARIADB_ROOT_PASSWORD="$E2E_DB_PASSWORD" -e MARIADB_ROOT_HOST=% -e MARIADB_DATABASE="$E2E_DB_NAME"); client=(mariadb --protocol=tcp -h127.0.0.1 -uroot -p"$E2E_DB_PASSWORD" -e 'SELECT 1');;
    postgres) image=postgres:17; port=5432; export E2E_DB_USER=postgres; args=(-e POSTGRES_PASSWORD="$E2E_DB_PASSWORD" -e POSTGRES_DB="$E2E_DB_NAME"); client=(psql -h127.0.0.1 -U postgres -d "$E2E_DB_NAME" -c 'SELECT 1');;
  esac
  container=$(docker run -d --label db-manager.e2e=true -p "127.0.0.1::$port" "${args[@]}" "$image")
  export E2E_DB_PORT
  E2E_DB_PORT=$(docker port "$container" "$port/tcp" | sed 's/.*://')
  ready=false
  for ((i=0; i<120; i++)); do
    if docker exec -e PGPASSWORD="$E2E_DB_PASSWORD" "$container" "${client[@]}" >/dev/null 2>&1; then ready=true; break; fi
    sleep 1
  done
  if ! $ready; then docker logs "$container"; echo 'Test database did not become ready' >&2; exit 1; fi
fi
# Always build from the checkout; never silently test an old or user-run server.
make build
npm --prefix web run test:e2e -- "$@"
