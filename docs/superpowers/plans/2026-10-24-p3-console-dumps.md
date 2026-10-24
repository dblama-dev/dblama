# db-manager v0.1 — План 3/6: SQL-консоль + экспорт/импорт

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** SQL-консоль (мульти-стейтменты, EXPLAIN, отмена, таймаут, история) и экспорт/импорт (SQL/CSV/TSV + gzip, потоковая запись, прогресс импорта).

**Architecture:** Драйвер получает ExecSQL/Explain/CancelQuery (отдельное соединение на запрос + реестр queryID→conn для KILL QUERY) и потоковые Export/ImportSQL/ImportCSV (курсор + батчи INSERT по 100, gzip-обёртка, DELIMITER-парсер импорта). API: query/cancel/explain + streaming export (chunked) + multipart import с JSON-lines прогрессом. Фронт: CodeMirror 6 консоль с мульти-гридами, диалоги экспорта/импорта.

**Tech Stack:** Go (существующий стек; новых зависимостей нет — gzip/multipart из stdlib), web: + CodeMirror 6 (`@codemirror/state`, `@codemirror/view`, `@codemirror/commands`, `@codemirror/lang-sql`, `codemirror` meta-package).

**Spec:** `docs/superpowers/specs/2026-10-23-db-manager-design.md` (§4.2 ExecSQL/Explain/CancelQuery/Export/ImportSQL/ImportCSV, §6 query-эндпоинты, §7 экспорт/импорт, §8 консоль+диалоги, §9.7 таймауты/лимит 100k). Дорожная карта: П3 = консоль+дампы; П4 = PostgreSQL; П5 = capability-объекты; П6 = релиз.

## Global Constraints

- Сырой SQL от клиента проходит ТОЛЬКО в ExecSQL/Explain и импорт SQL-файла (назначение консоли, спека §4.2/§9.4) — остальной SQL по-прежнему только через sqlgen.
- Консоль: мульти-стейтменты; Result{Columns,Rows,AffectedRows,LastInsertID,Truncated,Seconds}; лимит строк на стейтмент default 1000 (maxRows из запроса, hard cap 100000 — спека §9.7); флаг truncated.
- Таймаут консольного запроса из opts (default 60s, спека §6); отмена — CancelQuery(queryID) (KILL QUERY для MySQL); queryID выдаёт клиент (UUID).
- Экспорт — поточная запись в io.Writer (никогда не буферизуем весь дамп в памяти); SQL-дамп: структура из ShowCreateTable-генерации, данные — батчированные INSERT по 100 строк; LOCK TABLES не используем (спека §7).
- Экспорт форматы: SQL (опции IF NOT EXISTS/DROP IF EXISTS/gzip), CSV, CSV(';'), TSV; объекты: вся БД / выбранные таблицы+views (спека §7).
- Импорт: SQL-файл — построчное исполнение с учётом DELIMITER-блоков (триггеры/рутины), gzip-файлы; CSV → INSERT (разделитель, enclosure, перевод строки, пропуск заголовка). Прогресс — JSON-стрим `{"done":N,"total":M,"error":...}` по chunked-ответу (спека §7).
- Пароли/DSN не логируются. Тесты: tag `integration`, skip на Short, mysql:8.4+mariadb:11.4.
- Коммиты conventional; lint golangci-lint + eslint/vue-tsc; Vitest для новых утилит/сторов.
- Параметр `db` во всех новых методах квалифицируется/используется консистентно с П2 (writes через `db`.`table`; консоль исполняет в контексте БД: `USE`-эквивалент — CONNECTION-уровневый `db` в DSN коннекта консоли).

## Review Focus

Пять классов входных данных, которые спека подразумевает, но happy-path не трогает:

1. **Мульти-стейтмент со смешанными результатами** (`SELECT 1; UPDATE t SET x=1; SELECT 2`) → массив Result'ов с разными типами (rows vs affected); ошибки середины прерывают цепочку, предыдущие результаты возвращаются с ошибкой. Тест: интеграционный `TestExecSQL_MixedStatements` (Task 2).
2. **DELIMITER-блоки в импорте** (процедура/триггер с `;;` внутри тела, `DELIMITER ;;` ... `DELIMITER ;`) → сплиттер не режет тело рутины. Тест: `TestSplitStatements_DelimiterBlocks` (Task 4) + интеграционный импорт триггера.
3. **Отмена долгого запроса** (SELECT SLEEP(30) + cancel через 1с) → запрос завершается < 5с с query_cancelled, соединение возвращается в пул чистым. Тест: интеграционный `TestExecSQL_Cancel` (Task 3).
4. **BLOB и NULL в CSV-экспорте** (NULL → пустое поле без кавычек, значение с разделителем/переводом строки → enclosure; hex для binary) → round-trip CSV→импорт восстанавливает. Тест: `TestExportCSV_SpecialValues` (Task 5) + round-trip тест.
5. **Большой SQL-файл импорта** (50MB+) → построчное чтение без загрузки всего в память; прогресс total корректен (bytes-based). Тест: интеграционный с генерируемым дампом (Task 7).

---

### Task 0: Подготовка — зависимость CodeMirror и правки parked из П2

**Files:**
- Modify: `web/package.json` (+CodeMirror-пакеты — ставится здесь, чтобы П3-таски не дergали lockfile), `server/internal/driver/mysql/data.go` (defer tx.Rollback pattern — P2 Minor), `server/internal/driver/mysql/save.go` (реордер существующих колонок — P2 final-review Important, если не попал в фикс-волну П2)

**Interfaces:**
- Consumes: всё из П2.
- Produces: чистая база для П3.

- [ ] **Step 1:** `cd web && npm install @codemirror/state @codemirror/view @codemirror/commands @codemirror/lang-sql codemirror` (lockfile обновится).
- [ ] **Step 2:** parked-фиксы П2 (список из ledger П2): DeleteRows defer-rollback; qualifyWrite precondition check; колонки-реордер (backend placement для MODIFY/CHANGE + frontend position + тест эффекта).
- [ ] **Step 3:** все тесты зелёные (go unit+integration, vitest, lint).
- [ ] **Step 4:** Commit `chore: p3 prep — codemirror deps and p2 follow-ups`.

### Task 1: Драйвер — ExecSQL с мульти-стейтментами (dedicated conn)

**Files:**
- Create: `server/internal/driver/mysql/console.go`
- Modify: `server/internal/driver/types.go` (+Result уже есть? НЕТ — Result из спеки §4.5 ещё не заведён; добавить ExecOpts, Result), driver interface
- Test: `server/internal/driver/mysql/console_integration_test.go` (tag integration)

**Interfaces:**
```go
// types.go
type ExecOpts struct { MaxRows int; TimeoutSec int } // 0 → defaults (1000 / 60)
type Result struct {
    Columns      []ColumnMeta `json:"columns"`
    Rows         [][]any      `json:"rows"`
    AffectedRows int64        `json:"affectedRows,omitempty"`
    LastInsertID int64        `json:"lastInsertId,omitempty"`
    Truncated    bool         `json:"truncated,omitempty"`
    Seconds      float64      `json:"seconds"`
}
// Driver interface +=
ExecSQL(ctx context.Context, db, query string, queryID string, opts ExecOpts) ([]Result, error)
CancelQuery(ctx context.Context, queryID string) error // неизвестный queryID → nil (уже завершён) или ошибка? РЕШЕНИЕ: nil
```
- console.go: dedicated `db.Conn(ctx)` на запрос (DSN с db — conn в контексте нужной БД через full DSN с DBName? Драйвер-пул создан на DSN БД логина; для другой db — `USE db` на этом conn перед исполнением); multi-statement через go-sql-driver (DSN param multiStatements=true — добавить в dsn.go Config; для выделенного conn можно отдельный DSN); контекст с таймаутом TimeoutSec; реестр queryID→conn (map + mutex) для CancelQuery → `KILL QUERY <id>` где id = connection id (SELECT CONNECTION_ID()); rows→Result с лимитом MaxRows+truncated; значения через convertValue (П2). Записывать Seconds. Дефер: снятие из реестра + conn.Close().

- [ ] **Step 1: Падающие интеграционные тесты** — TestExecSQL_MultiSelect (2 SELECT → 2 Result с rows); TestExecSQL_MixedStatements (SELECT; UPDATE affected; SELECT); TestExecSQL_MidError (SELECT 1; BAD SQL; SELECT 2 → ошибка содержит текст, НО первый результат доступен? РЕШЕНИЕ: возвращаем ([]Result{первый}, err)); TestExecSQL_RowLimit (MaxRows=2 на 5 строках → 2 rows, truncated=true); TestExecSQL_Timeout (SELECT SLEEP(5) с TimeoutSec=1 → ошибка за <3с, context deadline). Capture FAIL.
- [ ] **Step 2: FAIL → реализовать → PASS (обе картинки).**
- [ ] **Step 3: lint; Commit** `feat(mysql): exec sql with multi statements and row limits`.

### Task 2: Драйвер — CancelQuery (KILL QUERY)

**Files:**
- Modify: `server/internal/driver/mysql/console.go`
- Test: `console_integration_test.go` (доп.)

**Interfaces:**
- Consumes: Task 1 реестр queryID→(conn, connID).
- Produces: рабочая отмена.

- [ ] **Step 1: Падающий интеграционный тест** — TestExecSQL_Cancel: горутина ExecSQL(SELECT SLEEP(30), queryID="q1"); sleep 1s; CancelQuery("q1"); ExecSQL вернулся за <5s с ошибкой (текст содержит killed/interrupted — снапшот обоих серверов); после — пул живой (следующий ExecSQL работает). Capture FAIL.
- [ ] **Step 2: реализовать** (connID фиксируется при регистрации: SELECT CONNECTION_ID() на выделенном conn; CancelQuery: KILL QUERY <connID> через ОБЫЧНЫЙ пул).
- [ ] **Step 3: PASS + lint; Commit** `feat(mysql): cancel long running console queries`.

### Task 3: sqlgen — сплиттер стейтментов с DELIMITER

**Files:**
- Create: `server/internal/sqlgen/split.go`
- Test: `server/internal/sqlgen/split_test.go`

**Interfaces:**
```go
func SplitStatements(sqlText string) []string
    // ';' вне строк/комментариев/DELIMITER-блоков; DELIMITER ;; ... ; переключает терминатор (как mysql-клиент);
    // строковые литералы ('', \'', ''), backtick-идентификаторы, -- и # комментарии, /* */ блоки — всё учитывается
```
- [ ] **Step 1: Падающие тесты** — простые; строка с ';' внутри; комментарий с ';'; DELIMITER-блок процедуры с ';;' и 'SELECT 1;' внутри тела; пустой вход → []; хвост без терминатора.
- [ ] **Step 2: FAIL → реализовать → PASS + lint; Commit** `feat(sqlgen): statement splitter with delimiter blocks`.

### Task 4: Драйвер — ShowCreateTable

**Files:**
- Modify: `server/internal/driver/mysql/save.go` или новый `dump.go`
- Test: интеграционный

**Interfaces:**
```go
// Driver interface +=
ShowCreateTable(ctx context.Context, db, table string) (string, error)
```
- [ ] **Step 1: интеграционный тест** — SHOW CREATE TABLE round-trip: создаём таблицу SQL-ом → ShowCreateTable содержит `CREATE TABLE` и имя; несуществующая → ошибка. FAIL → реализовать (один запрос) → PASS. Commit `feat(mysql): show create table`.

### Task 5: Пакет dump — экспорт SQL/CSV/TSV (поточный)

**Files:**
- Create: `server/internal/dump/export.go` (+writers)
- Modify: driver interface += Export(ctx, req ExportRequest, w io.Writer) error (реализация в mysql/dump.go)
- Test: `server/internal/dump/export_test.go` (юнит на CSV/TSV-писателей), `server/internal/driver/mysql/dump_integration_test.go`

**Interfaces:**
```go
// types.go (или dump-пакет; РЕШЕНИЕ: types.go, чтобы Driver не зависел от dump)
type ExportRequest struct {
    Format     string   `json:"format"`     // "sql"|"csv"|"csv_semicolon"|"tsv"
    Tables     []string `json:"tables"`     // пусто = вся БД
    DropIfExists bool   `json:"dropIfExists,omitempty"`
    IfNotExists  bool   `json:"ifNotExists,omitempty"`
    Gzip        bool   `json:"gzip,omitempty"`
    // для csv/tsv: одна таблица (первая из Tables), выборка целиком
}
```
- export.go (mysql/dump.go): SQL: для каждой таблицы — header (`-- Table: ...`), DROP IF EXISTS опция, SHOW CREATE TABLE (+IF NOT EXISTS преобразование — осторожно: показать точную семантику), данные через SelectRows батчами по 100 строк (SELECT по PK-порядку если есть), INSERT с sqlQuoteString-экранированием значений (многозначный INSERT `INSERT INTO t VALUES (...),(...)`); NULL → NULL; binary → hex-литерал 0x...; SET NAMES в шапке. CSV/TSV: заголовок опционально? РЕШЕНИЕ: заголовок всегда (первая строка); NULL → пустое поле; enclosure только при необходимости (разделитель/кавычка/перевод строки в значении); значения из []byte-binary → hex строка.
- Gzip: gzip.Writer поверх w при req.Gzip.
- [ ] **Step 1: юнит-тесты CSV/TSV-писателя** (кавычки, разделители в значениях, NULL, переводы строк). FAIL → PASS.
- [ ] **Step 2: интеграционные тесты** — TestExportSQL_RoundTrip (создать БД с таблицей+данными → Export SQL → импорт в новую БД (Task 7 importer или сырой ExecSQL) → структура и данные совпадают); TestExportCSV_SpecialValues (NULL, значение с запятой/кавычкой/переводом строки, binary); TestExport_Gzip (magic bytes 1f 8b). Commit `feat(dump): streaming sql and csv/tsv export`.

### Task 6: Драйвер — ImportSQL (DELIMITER, gzip, прогресс)

**Files:**
- Create: `server/internal/driver/mysql/import.go`
- Modify: driver interface += ImportSQL(ctx, db string, r io.Reader, progress func(done, total int64)) error
- Test: `import_integration_test.go`

**Interfaces:**
- Consumes: sqlgen.SplitStatements, ExecSQL-механика (dedicated conn).
- Produces: импорт; total = bytes прочитано (io.Reader обёрнут счётчиком), done = исполнено байт.

- [ ] **Step 1: интеграционные тесты** — TestImportSQL_Basic (дамп из Task 5 → импорт → данные совпадают); TestImportSQL_DelimiterBlock (дамп с триггером через DELIMITER → триггер создан, тело цело); TestImportSQL_Gzip (gzip-вход); TestImportSQL_Progress (progress вызывался, done==total в конце); TestImportSQL_MidError (файл с ошибкой в середине → ошибка, прогресс отражает позицию). FAIL → реализовать → PASS. Commit `feat(mysql): streaming sql import with delimiter support`.

### Task 7: Драйвер — ImportCSV

**Files:**
- Modify: `server/internal/driver/mysql/import.go`
- Test: интеграционный

**Interfaces:**
```go
type CSVImportOpts struct { Separator string; Enclosure string; SkipHeader bool } // defaults , " none false
// Driver interface += ImportCSV(ctx, db, table string, r io.Reader, opts CSVImportOpts, progress func(done, total int64)) error
```
- [ ] **Step 1: интеграционные тесты** — TestImportCSV_Basic (вставка в существующую таблицу, типы: число/строка/NULL-пустое); TestImportCSV_Quoted (значение с разделителем и кавычками); TestImportCSV_HeaderSkip. FAIL → реализовать (encoding/csv stdlib + батчевая вставка по 100 через BuildInsert-механику multi-row) → PASS. Commit `feat(mysql): csv import with configurable separator`.

### Task 8: API — query/cancel/explain + экспорт/импорт эндпоинты

**Files:**
- Create: `server/internal/api/handlers_query.go`, `server/internal/api/handlers_transfer.go`
- Modify: `server/internal/api/router.go`
- Test: `router_test.go` (fake-расширения), `router_integration_test.go`-доп? (нет — интеграция Task 9)

**Interfaces:**
```
POST /api/databases/{db}/query        {sql, maxRows?, timeoutSec?, queryID} → {results: [Result]}  (mutation: CSRF)
POST /api/query/{queryID}/cancel      → 204
POST /api/query                       {db, sql} → {result: Result}  // EXPLAIN: MySQL `EXPLAIN <sql>`
POST /api/databases/{db}/export       body ExportRequest → потоковый ответ (Content-Type по формату, Content-Disposition attachment; gzip → .gz)
POST /api/databases/{db}/import       multipart: file + fields (type=sql|csv, table?, separator?, enclosure?, skipHeader?) → JSON-lines прогресс: {"done":N,"total":M} ... {"done":..,"error":".."} | финал {"ok":true}
```
- EXPLAIN: префикс `EXPLAIN ` + сырой SQL (назначение консоли); таймаут 60s.
- Export: валидация формата → 400; поток через http.ResponseWriter (flusher для чанков); ошибки ДО записи тела → 400/500; после начала стрима — ошибки в прогресс-строке нельзя (SQL-экспорт пишет в буфер? РЕШЕНИЕ: сначала заголовки, ошибки драйвера середины — обрываем соединение; тесты это не проверяют — документируем).
- Import: multipart парсинг (1GB cap? РЕШЕНИЕ: 512MB), файл → reader (gzip-detect по magic bytes), прогресс-стрим w/ http.Flusher.

- [ ] **Step 1: падающие fake-тесты** — TestQueryHandler (multi results, queryID передан), TestQueryHandler_Validation (пустой sql → 400, queryID пуст → 400), TestCancelHandler, TestExplainHandler, TestExportHandler_Validation (неизвестный формат → 400), TestExportHandler_Stream (fake driver пишет данные → ответ содержит), TestImportHandler_Multipart (fake → прогресс JSON-lines формат). FAIL → реализовать → PASS + lint. Commit `feat(api): console, export and import endpoints`.

### Task 9: Интеграционный e2e экспорт→импорт

**Files:**
- Create: `server/internal/api/router_integration_transfer_test.go` (tag integration)

**Interfaces:**
- Consumes: Tasks 5–8.

- [ ] **Step 1: сквозной тест** — создать БД с 2 таблицами+данными через API → POST export (sql) → прочитать ответ → POST import в новую БД (multipart) → дождаться финала прогресс-стрима → GET def/data новой БД совпадают (round-trip из спеки §11). Обе картинки. Commit `test(api): export import round trip against live servers`.

### Task 10: Фронт — консоль (CodeMirror 6)

**Files:**
- Create: `web/src/views/ConsoleView.vue`, `web/src/lib/consoleHistory.ts`, `web/src/components/ResultGrid.vue`
- Modify: `web/src/router/index.ts` (+/db/:db/console), `web/src/components/ObjectTree.vue` (+пункт «Консоль» в дереве БД), i18n, AppSidebar?
- Test: `web/src/lib/consoleHistory.test.ts`, `web/src/components/ResultGrid.test.ts`

**Interfaces:**
```ts
// lib/consoleHistory.ts
export function loadHistory(serverKey: string): string[]          // localStorage `dbm-console-${serverKey}`
export function pushHistory(serverKey: string, sql: string): void // dedupe подряд, cap 100
// ResultGrid: рендер одного Result (rows | affected | error-подсветка),_SECONDS_, truncated-метка
```
- ConsoleView: CodeMirror 6 (lang-sql, тёмная тема от n-config-provider? — CodeMirror сам по себе; минимальная тема), кнопки: Выполнить всё / Выполнить выделение, EXPLAIN (обёртка выбранного/всего), форматтер? (спека упоминает форматтер — РЕШЕНИЕ: без внешнего форматтера в П3; ключ оставить, TODO в П5), история (localStorage per-server, выпадающий список + Alt+Up/Down), таймер выполнения (Seconds из последнего Result), queryID = crypto.randomUUID() на каждый запуск, отмена (кнопка во время исполнения), мульти-ResultGrid'ы вертикально.
- [ ] **Step 1: падающие тесты** — history (persist/dedupe/cap); ResultGrid (rows рендер, affected, truncated badge). FAIL → реализовать → PASS + type-check + lint. Commit `feat(web): sql console with codemirror and history`.

### Task 11: Фронт — экспорт/импорт диалоги

**Files:**
- Create: `web/src/views/ExportView.vue`, `web/src/views/ImportView.vue`
- Modify: router (+/db/:db/export, /db/:db/import), ObjectTree (+пункты), i18n
- Test: `ExportView.test.ts`, `ImportView.test.ts`

**Interfaces:**
- ExportView: выбор формата (sql/csv/csv; /tsv), чекбоксы (DROP IF EXISTS, IF NOT EXISTS, gzip), выбор таблиц (чекбоксы из catalog store; пусто = вся БД) → POST /export через fetch → blob → download (a[download]); имя файла из Content-Disposition.
- ImportView: input file, выбор типа (sql/csv авто?), для CSV — таблица (select), разделитель/enclosure/skipHeader; POST multipart через fetch (XMLHttpRequest для прогресса? fetch + ReadableStream чтения ответа — РЕШЕНИЕ: fetch + response.body.getReader() чтение JSON-lines, прогресс-бар n-progress), ошибки из финальной строки.
- [ ] **Step 1: падающие тесты** — ExportView (опции → тело запроса; выбор таблиц), ImportView (multipart форма строится; прогресс-строки парсятся). FAIL → реализовать → PASS + build. Commit `feat(web): export and import dialogs with progress`.

### Task 12: Верификация

- [ ] **Step 1:** make lint/backend-test/web-build/build + полная integration — зелёные.
- [ ] **Step 2:** браузерный сценарий контроллера: консоль (мульти-стейтмент, EXPLAIN, история), экспорт SQL → импорт в новую БД, CSV round-trip.
- [ ] **Step 3:** Commit при необходимости `chore: p3 verification fixes`.

---

## Самопроверка плана (выполнена)

- **Покрытие спеки для П3**: §4.2 ExecSQL/Explain/CancelQuery/Export/ImportSQL/ImportCSV — Tasks 1–7; §6 query/export/import — Task 8–9; §7 форматы/опции/прогресс — Tasks 5–8; §8 консоль+диалоги — Tasks 10–11; §9.7 лимиты — Task 1 (hard cap 100k в API-валидации Task 8). Форматтер — осознанно отложен (TODO П5), записано.
- **Согласованность**: Result/ExecOpts/ExportRequest/CSVImportOpts в types.go (Driver не зависит от dump-пакета); SplitStatements в sqlgen; qualifiedIdent-паттерн сохранён.
- **Review Focus**: 5 пунктов закреплены (Tasks 1, 3, 4, 5, 6).
- **Пропорция**: сигнатуры точные, тела не транскрибируются.
