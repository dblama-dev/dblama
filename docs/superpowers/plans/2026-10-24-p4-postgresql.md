# db-manager v0.1 — План 4/6: PostgreSQL-драйвер

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Полноценный PostgreSQL-драйвер (postgres:17 в матрице), реализующий весь накопленный контракт Driver (данные+структура+консоль+дампы), схемы (?schema=), sequences, DDL-транзакции.

**Architecture:** Новый пакет `internal/driver/postgres` на jackc/pgx/v5/stdlib (database/sql поверх pgx — тот же код пулов/сканирования, что у MySQL-драйвера). Схема — параметр методов через таблицу из БЕЗ schema-префикса `table` + отдельный `schema` в контексте запроса: спека §13.5 запрещает `SET search_path` на общем пуле — все запросы квалифицируют `schema.table` явно (иначе default_path). Регистрация в main: driver id `postgres`, порт 5432. Caps: Sequences/Schemas/Upsert/Monitor/Comments true; Partitions/Events/Users/MoveCopy false (v0.1). DDL-правки в одной транзакции (§4.6). Запись UI-пути: ?schema= на эндпоинтах таблиц/данных.

**Tech Stack:** Go: + `github.com/jackc/pgx/v5/stdlib` (единственная новая зависимость плана). Интеграционная матрица: + postgres:17 (testcontainers postgres module). Фронт: схема-селектор (n-select) на экранах таблиц/данных, sequence-строки в дереве — БЕЗ новых зависимостей.

**Spec:** `docs/superpowers/specs/2026-10-23-db-manager-design.md` (§4.1–4.6, §5, §6 `?schema=`, §7 дампы, §9, §13.5). Дорожная карта: П4 = PostgreSQL; П5 = views/triggers/routines/users/monitoring; П6 = релиз.

## Global Constraints

- Все запросы к системным каталогам — через information_schema/pg_catalog с квалификацией схемы (не `SET search_path`; спека §13.5).
- Идентификаторы PG — в двойных кавычках через `sqlgen.QuoteIdentPg` (новая функция: `"` → `""`); значения — параметрами `$N`.
- DDL-правки (SaveTable+SaveIndexes+FK) — в ОДНОЙ транзакции (спека §4.6, pgx поддерживает DDL-транзакции); ошибка → rollback с номером шага.
- EXPLAIN: PG использует `EXPLAIN <sql>` (без ANALYZE в v0.1 — read-only, спека §4.2 «EXPLAIN ANALYZE-обёртка» уточняется: EXPLAIN без ANALYZE безопаснее для произвольного SQL — PIN).
- LastInsertID: PG не имеет LAST_INSERT_ID — `INSERT ... RETURNING` недоступен через интерфейс InsertRow(vals) error; PIN: InsertRow исполняет RETURNING id, если у таблицы один serial-PK (определён по pg_catalog), и запоминает в driver-state (как MySQL); иначе 0.
- Upsert: `INSERT ... ON CONFLICT (pk...) DO UPDATE SET ...` (Caps.Upsert=true).
- Мульти-стейтменты ExecSQL: pgx stdlib database/sql + simple protocol (`default_query_exec_mode=simple_protocol` DSN param) — NextResultSet работает; dedicated conn + `SET search_path TO schema` РАЗРЕШЁН на выделенном conn (он одноразовый — не общий пул; уточнение §13.5).
- CancelQuery: `SELECT pg_cancel_backend(pid)` — dedicated conn хранит свой pid (`SELECT pg_backend_pid()`).
- Экспорт SQL: собственная генерация DDL (pg_dump-стиль) из каталогов (спека §7 «собственная генерация для PG»); данные — батчированные INSERT по 100.
- Интеграционная матрица: mysql:8.4, mariadb:11.4 (существующие) + postgres:17 (root-пользователь postgres, пароль dbmanager-test). Все существующие integration-тесты продолжают зелёность.
- Коммиты conventional; lint; TDD; пароли/DSN не логируются.

## Review Focus

1. **Имена схем/таблиц сmixed-case и спецсимволы** (PG хранит в нижнем регистре без кавычек; `"MyTable"` ≠ `MyTable`) → квалификация через QuoteIdentPg везде; тест интеграционный с таблицей `"MixedCase"` и схемой `"Odd Schema"`.
2. **Типы PG без MySQL-аналогов** (serial/bigserial/uuid/jsonb/numeric/array/timestamptz) → TableDef-чтение и запись; тест round-trip таблицы с этими типами.
3. **search_path-утечка** (SET search_path на общем пуле ломает параллельные запросы — §13.5) → код-ревью: ноль SET search_path вне выделенных conn; тест: два параллельных SelectRows в разных схемах дают правильные данные.
4. **SaveTable-транзакция** (ошибка середины → полный rollback, никакой частичной структуры) → тест: desired с ошибкой на шаге 3 → структура не изменилась вовсе.
5. **Пустые/NULL-значения в CSV/TSV-экспорте и jsonb-ячейки** → jsonb сериализуется как строка JSON; NULL/empty различимы. Round-trip тест.

---

### Task 1: Пакет postgres — подключение, caps, список БД

**Files:**
- Create: `server/internal/driver/postgres/postgres.go`, `server/internal/driver/postgres/identifier.go`
- Modify: `server/internal/sqlgen/` (+`QuoteIdentPg(name string) string` — двойные кавычки, удвоение `"`; golden-тест), `server/cmd/dbmanager/main.go` (+Register postgres)
- Test: `server/internal/sqlgen/sqlgen_test.go` (+), `server/internal/driver/postgres/postgres_integration_test.go` (tag integration, image postgres:17, POSTGRES_PASSWORD=dbmanager-test)

**Interfaces:**
- Consumes: driver contract (П1).
- Produces: `Connect(ctx, cfg) (driver.Driver, error)` — sql.Open("pgx", dsn: `host=... port=... user=... password=... dbname=... sslmode=disable`, MaxOpenConns=4, ping); Info{Flavor:"postgres", Version из SELECT version()}; Caps: Sequences/Schemas/Upsert/Monitor/Comments true, остальные false кроме базовых; ListDatabases (pg_database, datname ≠ template0/1, size pg_database_size); Close.
- identifier.go: `qualify(schema, table string) (string, error)` через QuoteIdentPg.

- [ ] **Step 1: golden-тест QuoteIdentPg** (FAIL → реализовать → PASS).
- [ ] **Step 2: интеграционные тесты** — TestConnect_OK (flavor postgres, version строка), TestConnect_WrongPassword, TestListDatabases (содержит postgres), TestClose. FAIL → реализовать → PASS → lint → Commit `feat(postgres): connect, caps, list databases`.

### Task 2: Таблицы и структура — ListTables, TableDef, PrimaryKey

**Files:**
- Create: `server/internal/driver/postgres/tables.go`, `server/internal/driver/postgres/def.go`
- Test: `tables_integration_test.go`, `def_integration_test.go`

**Interfaces:**
- ListTables(ctx, db, schema?) — НЕТ: driver-интерфейс не имеет schema-параметра. РЕШЕНИЕ (binding): PG-драйвер хранит «текущую схему» — ListTables использует search_path-эквивалент: схема по умолчанию = первая из current_schemas()? Спека §6: эндпоинты принимают `?schema=`. Драйверу нужна схема. ВАРИАНТ: интерфейс-расширение `SchemaAware` (type-assertion, §4.1): `SetSchema(ctx, schema)` меняет поле драйвера (mutex) — но общий пул! БЕЗОПАСНО: schema хранится в driver-instance (mutex), все методы квалифицируют её в SQL-текст (никаких SET) — параллельные запросы с разными схемами невозможны в рамках одной сессии API (один драйвер = одна сессия = одна схема за раз; API-хендлер выставляет схему из ?schema= перед вызовом). PIN: `SetSchema(schema string) error` в SchemaManager-extension (§4.3), вызывается хендлером; дефолт = public; пустая схема → public.
- ListTables: information_schema.tables WHERE table_schema=$1 AND table_type='BASE TABLE'; size: pg_total_relation_size(quote_ident); rows: pg_class.reltuples; comment: obj_description.
- TableDef: information_schema.columns (data_type, character_maximum_length, numeric_precision/scale, is_nullable, column_default, collation_name, udt_name для jsonb/uuid/array-детекта, is_generated/generated expression pg_get_expr(adbin)), pg_indexes/pg_index (индексы: PRIMARY/UNIQUE/INDEX), information_schema.table_constraints+constraint_column_usage+referential_constraints (FK: confdeltype/confupdtype маппинг CASCADE/SET NULL/RESTRICT/NO ACTION), TableOptions (PG: без engine; comment, автincrement нет).
- PrimaryKey: pg_index indisprimary → att names по порядку.
- Тест round-trip: таблица с serial PK, varchar NOT NULL DEFAULT, numeric(10,2), jsonb, uuid DEFAULT gen_random_uuid(), timestamptz DEFAULT now(), text[] — все поля совпадают; MixedCase-таблица; "Odd Schema"-схема. Commit `feat(postgres): list tables and read table definitions`.

### Task 3: SaveTable + SaveIndexes (DDL-транзакция)

**Files:**
- Create: `server/internal/driver/postgres/save.go`
- Test: `save_integration_test.go`, `save_build_test.go` (golden без БД)

**Interfaces:**
- SaveTable: table=="" → CREATE TABLE (NewName может быть schema-квалифицированным? НЕТ — schema из SetSchema; таблица создаётся в текущей схеме); ALTER: пошагово ВНУТРИ tx (BEGIN; stmt1; stmt2; ...; COMMIT) — ошибка шага i → rollback, error `step %d: %w`; rename: `ALTER TABLE ... RENAME TO`; колонки: ADD/DROP/ALTER COLUMN (PG: ALTER COLUMN TYPE/SET NOT NULL/DROP NOT NULL/SET DEFAULT/DROP DEFAULT/SET STATISTICS...; rename: RENAME COLUMN); Placement FIRST/AFTER не поддерживается PG — Position игнорируется (PIN, UI не показывает стрелки для PG — фронт по flavor); options: только comment (COMMENT ON TABLE).
- SaveIndexes: diff как MySQL: CREATE [UNIQUE] INDEX name ON table (cols) / DROP INDEX schema.index; PRIMARY: добавление через ALTER TABLE ADD PRIMARY KEY; удаление PK: ALTER TABLE ... DROP CONSTRAINT.
- Golden: buildCreateTable/buildAlterTable тексты; интеграционные: create+readback, alter (add/drop/alter column), транзакционный rollback-тест (Review Focus 4), rename. Commit `feat(postgres): save table with transactional ddl`.

### Task 4: Данные — SelectRows, Insert/Update/Delete/Upsert, LastInsertID

**Files:**
- Create: `server/internal/driver/postgres/data.go`
- Test: `data_integration_test.go`

**Interfaces:**
- SelectRows: BUILD на PG-диалекте — НЕТ: sqlgen строит MySQL-бэктики. РЕШЕНИЕ (binding): sqlgen рефакторится на диалект: `sqlgen.NewBuilder(dialect)` с QuoteIdent-функцией на входе; BuildSelect/BuildUpdate/BuildDelete принимают quoter; MySQL-вызовы не меняются (обёртки). Плейсхолдеры: PG `$1,$2...` — билдер получает dialect и рендерит `?` (MySQL) или `$N` (PG). Golden-тесты дополнены PG-вариантами.
- convertValue: pgx возвращает нативные типы; time.Time → RFC3339; []byte → hex; jsonb/uuid → string; arrays → JSON-строка (PIN).
- InsertRow: RETURNING — если таблица имеет single serial-колонку (кэш по (schema,table)), добавить `RETURNING <col>` и запомнить (mutex); иначе без RETURNING, LastInsertID() → 0.
- UpdateRow/DeleteRows: аналогично MySQL (params $N).
- UpsertRows: ON CONFLICT (pk) DO UPDATE SET (Caps.Upsert).
- Интеграционные: пагинация/фильтры/NULL-vs-empty/инъекция-value/no-PK update/параллельные схемы (Review Focus 3)/LastInsertID после вставки. Commit `feat(postgres): row browsing and write operations`.

### Task 5: Консоль и дампы — ExecSQL/Cancel/Export/Import

**Files:**
- Create: `server/internal/driver/postgres/console.go`, `server/internal/driver/postgres/dump.go`, `server/internal/driver/postgres/import.go`
- Test: `console_integration_test.go`, `dump_integration_test.go`, `import_integration_test.go`

**Interfaces:**
- ExecSQL: simple_protocol DSN (multi-statement), dedicated conn, SET search_path на выделенном conn (допустимо — одноразовый), лимиты/Seconds/Truncated как MySQL; CancelQuery: pg_backend_pid() реестр + pg_cancel_backend.
- ShowCreateTable: собственной генерации `CREATE TABLE ...` из каталогов (колонки+типы+defaults+constraints+indexes) — PIN формат: pg_dump-подобный, достаточно для импорта своим же importer'ом.
- Export: SQL (генерация + данные keyset по PK) / CSV / TSV / gzip — writers переиспользуются из internal/dump; ImportSQL (SplitStatements — в PG-дампах DELIMITER нет, но сплиттер совместим), ImportCSV.
- Интеграционные: round-trip export→import (как П3), мульти-стейтмент, отмена (pg_sleep), Review Focus 5 (jsonb/NULL в CSV). Commit `feat(postgres): console, streaming export and import`.

### Task 6: API — ?schema= + схемы/sequences (SchemaManager, SequenceManager)

**Files:**
- Create: `server/internal/api/handlers_schemas.go`
- Modify: `server/internal/api/handlers_tables.go`/`handlers_rows.go` (schema query-param → SetSchema перед вызовом; только для flavor postgres — type-assertion SchemaAware; MySQL → игнор параметра), `router.go`
- Test: `router_test.go` (+fake SchemaAware), интеграционный postgres-кейс

**Interfaces:**
- SchemaManager (§4.3): `ListSchemas(ctx) ([]string, error)`, `CurrentSchema(ctx) (string, error)`, `SetSchema(schema string) error`.
- SequenceManager (§4.3): `ListSequences(ctx, schema) ([]SequenceInfo, error)`, `CreateSequence(ctx, schema, name, opts)`, `DropSequence`, `AlterSequence(restart/increment)` — тип SequenceInfo{name, lastValue, increment}.
- Эндпоинты: GET /api/schemas → {schemas:[...]}; GET /api/sequences?schema= → {sequences:[...]}; POST /api/sequences; DELETE /api/sequences/{name}?schema=; PATCH /api/sequences/{name}. Capability-гейтинг: 404 driver_not_supported при отсутствии интерфейса (спека §4.4).
- ?schema= на tables/rows/table-def/pk эндпоинтах: handler делает `if sa, ok := sess.Driver.(driver.SchemaAware); ok { sa.SetSchema(q.Get("schema")) }`.
- Тесты: fake + интеграционный (postgres: схемы список, ?schema= переключает, sequences CRUD). Commit `feat(api): schema and sequence endpoints with schema parameter`.

### Task 7: Фронт — схема-селектор и sequences в дереве

**Files:**
- Modify: `web/src/stores/catalog.ts` (fetchTables(db, schema?)), `web/src/api/types.ts` (+SequenceInfo), `web/src/components/ObjectTree.vue` (Схемы группа + Sequences группа по caps; схема-селектор для postgres), `web/src/views/TableView.vue`/DataGrid (schema в запросах), `web/src/i18n/locales/*.ts`
- Test: `ObjectTree.test.ts` (+), `catalog.test.ts` (+)

**Interfaces:**
- caps.schemas → в дереве группа «Схемы» (клик → смена схемы); caps.sequences → группа «Sequences» (список, создание/удаление через простую модалку — минимально).
- Схема хранится в catalog store (db + schema); все запросы таблиц/данных добавляют &schema=; при смене схемы invalidate.
- Тесты: дерево с caps postgres (группы видны), schema-параметр в запросах. Commit `feat(web): schema selector and sequences tree for postgres`.

### Task 8: Верификация

- [ ] **Step 1:** make lint/backend-test/web-build/build + ПОЛНАЯ integration матрица (mysql+mariadb+postgres) — зелёные.
- [ ] **Step 2:** браузерный сценарий контроллера: логин postgres → дерево со схемами/sequences → таблица с jsonb → данные → консоль EXPLAIN → экспорт/импорт round-trip.
- [ ] **Step 3:** Commit `chore: p4 verification fixes` при необходимости.

---

## Самопроверка плана (выполнена)

- **Покрытие спеки для П4**: §4.1–4.2 полный контракт PG; §4.3 SchemaManager/SequenceManager (ViewManager/TriggerManager/RoutineManager — П5, both-flavor); §4.4 Caps PG; §4.6 DDL-транзакции; §6 ?schema=; §7 дампы PG (собственная генерация); §9; §13.5 search_path (Review Focus 3). Users/Monitor для PG — П5.
- **Согласованность**: sqlgen-диалект (quoter+placeholder) — MySQL-вызовы не меняются; driver.SchemaAware через type-assertion (§4.1); writers из internal/dump переиспользуются.
- **Review Focus**: 5 пунктов закреплены (Tasks 1–5).
- **Риски**: pgx stdlib + simple_protocol для мульти-стейтментов — валидировать в Task 5 первым шагом (spike-тест), fallback: pgx native multi-statement через pgx.Conn.Exec (без database/sql) — ограничить зоной console.go.
