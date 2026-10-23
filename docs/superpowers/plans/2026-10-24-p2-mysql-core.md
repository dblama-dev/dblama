# db-manager v0.1 — План 2/6: MySQL-ядро (данные + структура)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Полная работа с таблицами MySQL/MariaDB: список/создание/правка структуры/удаление, браузинг и правка данных (пагинация, сортировка, фильтры), плюс UI: дерево объектов, грид данных, редакторы строки и структуры.

**Architecture:** Расширение `driver.Driver` (без ломки существующих реализаций — новый функционал отдельными методами интерфейса + новые типы в `driver/types.go`). Весь генерируемый SQL — через новый пакет `sqlgen` (экранирование идентификаторов бэктиками, значения — параметрами). Модель SaveTable — «полное желаемое состояние» Adminer (OldName у колонок, пустое определение = DROP). Фронтенд: TanStack Table (сервер-сайд пагинация/сортировка/фильтры), дерево объектов в сайдбаре, структура таблицы одним PUT.

**Tech Stack:** Go 1.23+ (chi, go-sql-driver/mysql, testcontainers-go — уже подключены), Vue 3 + TS (Naive UI, Pinia — уже подключены), новые зависимости: `@tanstack/vue-table` (фронт). На бэке новых зависимостей нет.

**Spec:** `docs/superpowers/specs/2026-10-23-db-manager-design.md` (§4.2 Driver, §4.5 типы, §4.6 транзакции, §6 API таблиц/данных, §8 UI данных/структуры). Дорожная карта: этот план — П2; консоль/дампы — П3, PostgreSQL — П4, capability-объекты — П5.

## Global Constraints

- Все идентификаторы в генерируемом SQL экранируются ТОЛЬКО через `sqlgen.QuoteIdent` (бэктики, `` ` `` → ` `` `); значения — только параметрами `?`. Ноль конкатенации пользовательских строк в SQL (спека §9.4).
- Операторы фильтров — только из белого списка `allowedOperators` (спека: `=, !=, <, >, <=, >=, LIKE, NOT LIKE, REGEXP, NOT REGEXP, IN, NOT IN, IS NULL, IS NOT NULL`); неизвестный оператор → 400 bad_request.
- NULL и пустая строка различимы везде: JSON `null` = SQL NULL, `""` = пустая строка (спека §8).
- Пагинация: limit по умолчанию 50, hard cap 1000 (`?limit` больше → 400); totalRows — точный COUNT на каждый запрос (спека §4.2: truncated=false всегда).
- UPDATE/DELETE строки при отсутствии PK — по совокупности значений ВСЕХ колонок (как Adminer, спека §13.3); при дубликатах затрагивается >1 строка — допустимо, документируется комментарием.
- SaveTable — полное желаемое состояние: у каждой колонки OldName (пустой OldName = ADD, пустое определение при заполненном Name/OldName = DROP, иначе MODIFY/CHANGE); `table==""` → CREATE TABLE; rename через `TableDef.NewName` (спека §4.2).
- MySQL: DDL авто-коммитится — SaveTable выполняет шаги поштучно, при ошибке возвращает ошибку с номером шага и текстом (спека §4.6).
- Интеграционные тесты: tag `integration`, skip на `testing.Short`, mysql:8.4 + mariadb:11.4 (root/dbmanager-test) — как в П1.
- Коммиты: conventional commits; каждый таск заканчивается коммитом.
- Линт: golangci-lint (server/), eslint+vue-tsc (web/). Тесты Vitest для новых утилит/сторов фронта.
- Пароли/DSN не логируются (П1 уже соблюдён; новый код не добавляет логирования тел).

## Review Focus

Пять классов входных данных, которые спека подразумевает, но happy-path не трогает:

1. **Идентификаторы со спецсимволами** (имя таблицы/колонки с бэктиком, кавычкой, юникодом, зарезервированное слово `order`) → экранирование бэктиками спасает. Тест: golden `TestQuoteIdent` (Task 1) + интеграционный `TestCRUD_ReservedAndSpecialNames` (Task 10).
2. **SQL-инъекция через фильтр** (`value = "x'; DROP TABLE t--"`, оператор-подделка `"=1 OR 1=1"`) → значение параметром, оператор из белого списка. Тесты: `TestBuildSelect_InjectionSafe` (Task 1), `TestSelectRows_UnknownOperator` API-тест (Task 8), интеграционный `TestSelectRows_InjectionValue` (Task 7).
3. **NULL vs пустая строка** в фильтре, вставке, правке (`value=""` ≠ NULL) → JSON null ⇄ SQL NULL. Тесты: `TestBuildSelect_NullFilter` (Task 1), интеграционный `TestRowLifecycle_NullVsEmpty` (Task 7).
4. **Строка без PK** (таблица без первичного ключа) → UPDATE/DELETE по всем колонкам; фронт работает без row-key. Тесты: интеграционный `TestUpdateRow_NoPK` (Task 7), фронтовый тест грида с составным rowKey (Task 12).
5. **Правка структуры с потерями** (DROP колонки с данными, rename с OldName несуществующей, AUTO_INCREMENT без ключа) → MySQL-ошибка возвращается с номером шага; состояние таблицы не молча меняется. Тест: интеграционный `TestSaveTable_ErrorStepReporting` (Task 5).

---

### Task 1: Пакет sqlgen — экранирование и сборка SELECT/INSERT/UPDATE/DELETE

**Files:**
- Create: `server/internal/sqlgen/sqlgen.go` (QuoteIdent, BuildSelect), `server/internal/sqlgen/writes.go` (BuildInsert, BuildUpdate, BuildDelete)
- Test: `server/internal/sqlgen/sqlgen_test.go` (golden), `server/internal/sqlgen/writes_test.go`

**Interfaces:**
- Produces (используют Tasks 5, 6, 7 и все последующие планы):
```go
func QuoteIdent(name string) string // `a` ; `a``b` ; пустая строка → паника недопустима, возвращать "` `"-пусто? Нет: валидация вызывающим; QuoteIdent("" ) → "` `"
// ВАЖНО: QuoteIdent не валидирует пустоту — вызывающий (драйвер/sqlgen-билдеры) обязан отклонить пустые имена ошибкой ErrEmptyIdentifier
var ErrEmptyIdentifier = errors.New("sqlgen: empty identifier")

type Filter struct { Column, Operator, Value string } // Value для IN — через запятую; для IS NULL/IS NOT NULL — игнорируется
type Sort struct { Column string; Desc bool }
type SelectQuery struct {
    Columns []string // пусто = *
    Filters []Filter
    Sorts   []Sort
    Limit   int // <=0 → 50
    Offset  int
}
func AllowedOperators() []string // =, !=, <, >, <=, >=, LIKE, NOT LIKE, REGEXP, NOT REGEXP, IN, NOT IN, IS NULL, IS NOT NULL
func BuildSelect(table string, q SelectQuery) (query string, args []any, err error)
    // SELECT `c1`,`c2` FROM `t` WHERE `c` LIKE ? ORDER BY `c` DESC LIMIT ? OFFSET ?
    // IS NULL/IS NOT NULL — без плейсхолдера; IN/NOT IN — список ?,?,?; ORDER BY только по QuoteIdent(Column)
func BuildInsert(table string, cols []string) (query string, argsPlaceholder string, err error)
    // INSERT INTO `t` (`a`,`b`) VALUES (?,?) — argsPlaceholder не нужен: вернуть строку с нужным числом ?, отказ от multi-row в v1-сигнатуре не требуется; вернуть query и nil
func BuildUpdate(table string, cols []string, where map[string]any) (query string, args []any, err error)
    // UPDATE `t` SET `a`=?,`b`=? WHERE `pk`=? (where map → порядок ключей сортированный, детерминизм для тестов)
func BuildDelete(table string, where map[string]any) (query string, args []any, err error)
```
- Значения фильтров/where — всегда `?`; NULL-фильтр: Filter.Value == "" и Operator IS NULL/IS NOT NULL (Value игнорируется); остальные операторы с пустым Value → ошибка.

- [ ] **Step 1: Падающие тесты** — golden-тесты: `TestQuoteIdent` (`a`→`` `a` ``, `a``b`→`` `a``b` ``, `order`→`` `order` ``); `TestBuildSelect` (все части: колонки, фильтры разных операторов, сортировка, limit/offset; детерминизм при повторном вызове); `TestBuildSelect_NullFilter` (IS NULL без `?`); `TestBuildSelect_InjectionSafe` (Value `"x'; DROP TABLE t--"` попадает только в args); `TestBuildSelect_UnknownOperator` → ошибка; `TestBuildSelect_LimitCap` (Limit=5000 → clamped 1000? НЕТ — cap на уровне API-хендлера; sqlgen принимает как есть); `TestBuildInsert`, `TestBuildUpdate_WhereSorted`, `TestBuildDelete`, `TestQuoteIdent_Empty` → ErrEmptyIdentifier из Build*.
- [ ] **Step 2: FAIL-прогон** `go test ./internal/sqlgen/ -v`.
- [ ] **Step 3: Реализовать** sqlgen.go + writes.go.
- [ ] **Step 4: PASS + lint.**
- [ ] **Step 5: Commit** `feat(sqlgen): identifier quoting and select/insert/update/delete builders`.

### Task 2: Драйвер — ListTables

**Files:**
- Create: `server/internal/driver/mysql/tables.go`
- Modify: `server/internal/driver/types.go` (+ TableInfo), `server/internal/driver/driver.go` если интерфейс в отдельном файле — фактически расширение интерфейса Driver в types.go
- Test: `server/internal/driver/mysql/tables_integration_test.go` (tag `integration`)

**Interfaces:**
- Consumes: `driver.Driver` (П1).
- Produces:
```go
// types.go
type TableInfo struct {
    Name      string `json:"name"`
    Rows      int64  `json:"rows"`
    SizeBytes int64  `json:"sizeBytes"`
    Engine    string `json:"engine"`
    Comment   string `json:"comment"`
    IsView    bool   `json:"isView"`
}
// Driver interface += ListTables(ctx context.Context, db string) ([]TableInfo, error)
```
- mysql/tables.go: `information_schema.TABLES WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME`; TABLE_ROWS (approx), DATA_LENGTH+INDEX_LENGTH, ENGINE, TABLE_COMMENT, TABLE_TYPE='VIEW' → IsView. Пустой db → ошибка без запроса.

- [ ] **Step 1: Падающий интеграционный тест** — `TestListTables` (создать таблицу `t_p2` с комментарием; список содержит её, IsView=false, Engine in {InnoDB, MyISAM, ...}, SizeBytes ≥ 0; views: `CREATE VIEW v_p2 AS SELECT 1` → IsView=true).
- [ ] **Step 2: FAIL-прогон** `go test -tags integration ./internal/driver/mysql/ -run TestListTables -v` (компиляция падает — метода нет).
- [ ] **Step 3: Реализовать** TableInfo + ListTables.
- [ ] **Step 4: PASS + юнит- suite зелёный + lint.**
- [ ] **Step 5: Commit** `feat(mysql): list tables and views`.

### Task 3: Драйвер — TableDef (чтение структуры)

**Files:**
- Create: `server/internal/driver/mysql/def.go`
- Modify: `server/internal/driver/types.go` (+ ColumnDef, DefaultValue, Index, IndexColumn, ForeignKey, TableOptions, TableDef)
- Test: `server/internal/driver/mysql/def_integration_test.go` (tag `integration`)

**Interfaces:**
- Produces (используют Tasks 4, 8, 13):
```go
type DefaultValue struct {
    Value           string `json:"value"`
    IsSQLExpression bool   `json:"isSqlExpression,omitempty"` // CURRENT_TIMESTAMP, NULL-литерал в MySQL-смысле и т.п.
}
type ColumnDef struct {
    Name          string        `json:"name"`
    OldName       string        `json:"oldName,omitempty"` // чтение = Name
    Type          string        `json:"type"`              // базовый: "int", "varchar", ...
    Length        string        `json:"length,omitempty"`  // из COLUMN_TYPE: "(11)" без скобок → "11"; enum/set — ""
    Unsigned      bool          `json:"unsigned,omitempty"`
    Nullable      bool          `json:"nullable"`
    Default       *DefaultValue `json:"default,omitempty"` // nil = нет DEFAULT
    AutoIncrement bool          `json:"autoIncrement,omitempty"`
    OnUpdate      string        `json:"onUpdate,omitempty"`
    Generated     string        `json:"generated,omitempty"` // "" | "VIRTUAL" | "STORED"
    Collation     string        `json:"collation,omitempty"`
    Comment       string        `json:"comment,omitempty"`
    EnumValues    []string      `json:"enumValues,omitempty"` // парсинг COLUMN_TYPE enum('a','b') с экранированием ''
    Position      string        `json:"position,omitempty"`   // чтение = "" (позиция в ORDINAL_ORDER)
}
type IndexColumn struct { Name string `json:"name"`; SubPart int `json:"subPart,omitempty"` }
type Index struct { Name string `json:"name"`; Type string `json:"type"`; Columns []IndexColumn `json:"columns"` } // Type: PRIMARY|UNIQUE|INDEX|FULLTEXT|SPATIAL
type ForeignKey struct {
    Name       string   `json:"name"`
    Columns    []string `json:"columns"`
    RefTable   string   `json:"refTable"`
    RefColumns []string `json:"refColumns"`
    OnDelete   string   `json:"onDelete,omitempty"` // CASCADE|SET NULL|NO ACTION|RESTRICT
    OnUpdate   string   `json:"onUpdate,omitempty"`
}
type TableOptions struct { Engine string `json:"engine,omitempty"`; Collation string `json:"collation,omitempty"`; Comment string `json:"comment,omitempty"`; AutoIncrement int64 `json:"autoIncrement,omitempty"` }
type TableDef struct {
    NewName     string        `json:"newName,omitempty"` // чтение = ""? НЕТ: чтение = имя таблицы; SaveTable сравнивает
    Columns     []ColumnDef   `json:"columns"`
    Indexes     []Index       `json:"indexes"`
    ForeignKeys []ForeignKey  `json:"foreignKeys"`
    Options     TableOptions  `json:"options"`
}
// Driver interface += TableDef(ctx context.Context, db, table string) (TableDef, error)
```
- def.go: COLUMNS (ORDER BY ORDINAL_POSITION; парсинг COLUMN_TYPE на тип/length/unsigned/enum-значения; COLUMN_DEFAULT: NULL → nil, строка → DefaultValue, CURRENT_TIMESTAMP* → IsSQLExpression=true; EXTRA: auto_increment/VIRTUAL GENERATED/STORED GENERATED/on update ...), STATISTICS (индексы, GROUP BY INDEX_NAME, NON_UNIQUE+INDEX_NAME=PRIMARY → PRIMARY, SUB_PART), KEY_COLUMN_USAGE+REFERENTIAL_CONSTRAINTS (FK в этой таблице; ref-таблица может быть в другой схеме — квалифицировать при необходимости). TABLE_NAME mismatch → ошибка not-found-стиля (fmt "table %q not found").

- [ ] **Step 1: Падающий интеграционный тест** — `TestTableDef_RoundTrip` (создать SQL-ом таблицу: int unsigned auto_increment PK, varchar(64) NOT NULL DEFAULT 'x', enum('a','b'), decimal(10,2) NULL, generated STORED, CURRENT_TIMESTAMP on update, comment, FULLTEXT-индекс, FK на вторую таблицу с ON DELETE CASCADE, collation utf8mb4_0900_ai_ci → TableDef: колонки совпали по полям, EnumValues=[a b], индексы PRIMARY+FULLTEXT, FK с RefTable/OnDelete, Options.Engine/Collation). `TestTableDef_NotFound` → ошибка содержит `not found`.
- [ ] **Step 2: FAIL.**
- [ ] **Step 3: Реализовать** типы + def.go.
- [ ] **Step 4: PASS + lint.**
- [ ] **Step 5: Commit** `feat(mysql): read table definition from information_schema`.

### Task 4: Драйвер — SaveTable (CREATE/ALTER, модель желаемого состояния)

**Files:**
- Create: `server/internal/driver/mysql/save.go`
- Modify: `server/internal/driver/mysql/mysql.go` (реализация метода)
- Test: `server/internal/driver/mysql/save_integration_test.go` (tag `integration`), `server/internal/driver/mysql/save_build_test.go` (юнит на генерацию DDL без сервера)

**Interfaces:**
- Consumes: sqlgen.QuoteIdent, Task 3 типы.
- Produces: `Driver interface += SaveTable(ctx context.Context, db, table string, desired TableDef) error`
  - `table==""` → CREATE TABLE `desired.NewName` (колонки по порядку, PRIMARY/индексы/FK inline, опции).
  - иначе ALTER: пошагово (спека §4.6): шаг 1 rename (если NewName != "" && != table), далее колонки: OldName=="" → ADD [FIRST|AFTER pos]; определение пустое (Type=="" при заполненном Name) → DROP `Name`; OldName != Name → CHANGE; равные → MODIFY; шаг индексы/FK → делегировать SaveIndexes (Task 5 внутри этого таска НЕ вызывается — FK inline в ALTER, индексы через SaveIndexes отдельным вызовом из хендлера; НО PRIMARY при создании колонки с auto_increment нужен сразу — inline ADD PRIMARY KEY допустим в CREATE, в ALTER — через отдельный шаг ADD PRIMARY KEY в этом же SaveTable).
  - Ошибка шага: `fmt.Errorf("step %d: %w", i, err)`.
  - Юнит-часть: внутренние функции `buildCreateTable(db string, d TableDef) (string, []any, error)` и `buildAlterTable(db, table string, d TableDef) ([]string, []any, error)` — тестируются golden без БД (сигнатуры точные, локальные для пакета).
- Колонка → SQL-фрагмент: тип с length, unsigned, NOT NULL, DEFAULT ('literal' / выражение как есть — IsSQLExpression), AUTO_INCREMENT, ON UPDATE, GENERATED (VIRTUAL/STORED + выражение в Default.Value AS (`expr`)), COLLATE, COMMENT.

- [ ] **Step 1: Падающие golden юнит-тесты** — `TestBuildCreateTable` (полная таблица из Task 3 теста → точный DDL-текст), `TestBuildAlterTable` (ADD + CHANGE + DROP + rename колонки, rename таблицы), `TestBuildAlterTable_ErrorStep` — не применим (это runtime), только генерация.
- [ ] **Step 2: FAIL.**
- [ ] **Step 3: Реализовать** save.go: build-функции + SaveTable-метод (exec поштучно, ctx-таймаут не добавляем — общий пул).
- [ ] **Step 4: PASS + lint.**
- [ ] **Step 5: Падающие интеграционные тесты** — `TestSaveTable_CreateAndReadBack` (SaveTable("" → def) → TableDef round-trip равен желаемому по ключевым полям); `TestSaveTable_AlterColumns` (create 2 колонки → desired с CHANGE/DROP/ADD → read-back совпадает); `TestSaveTable_RenameTable` (read-back по новому имени); `TestSaveTable_ErrorStepReporting` (desired с FK на несуществующую таблицу → ошибка содержит `step`, таблица не осталась в промежуточном частичном состоянии ИЛИ состояние задокументировано — проверяем только что ошибка пришла с шагом).
- [ ] **Step 6: FAIL → PASS + lint.**
- [ ] **Step 7: Commit** `feat(mysql): save table with desired-state alter model`.

### Task 5: Драйвер — SaveIndexes, TruncateTables, DropTables, DropViews

**Files:**
- Modify: `server/internal/driver/mysql/save.go` (+ buildAlterAddIndex), `server/internal/driver/mysql/tables.go` (+ методы)
- Test: `server/internal/driver/mysql/save_integration_test.go` (доп. кейсы)

**Interfaces:**
- Produces:
```go
// Driver interface +=
SaveIndexes(ctx context.Context, db, table string, desired []Index) error
    // полный желаемый список: существующие не из desired → DROP INDEX; отсутствующие → ADD (PRIMARY KEY / UNIQUE / FULLTEXT / SPATIAL / INDEX name)
TruncateTables(ctx context.Context, db string, tables []string) error // TRUNCATE по каждому; первая ошибка прерывает
DropTables(ctx context.Context, db string, tables []string) error     // DROP TABLE по каждому
DropViews(ctx context.Context, db string, views []string) error       // DROP VIEW
```
- [ ] **Step 1: Падающие интеграционные тесты** — `TestSaveIndexes_RoundTrip` (создать таблицу, SaveIndexes [PRIMARY(id), UNIQUE(name)] → TableDef.Indexes совпадают; затем desired [PRIMARY(id)] → UNIQUE исчез); `TestTruncateTables` (вставить строки → truncate → COUNT=0); `TestDropTables` / `TestDropViews` (объект исчезает из ListTables).
- [ ] **Step 2: FAIL.**
- [ ] **Step 3: Реализовать.**
- [ ] **Step 4: PASS + lint.**
- [ ] **Step 5: Commit** `feat(mysql): indexes, truncate, drop tables and views`.

### Task 6: Драйвер — SelectRows (браузинг данных)

**Files:**
- Create: `server/internal/driver/mysql/data.go`
- Modify: `server/internal/driver/types.go` (+ SelectQuery переезд? НЕТ — SelectQuery уже в sqlgen; driver-тип переиспользует sqlgen.SelectQuery через alias? РЕШЕНИЕ: `driver.SelectQuery = sqlgen.SelectQuery` (type alias) + Page/ColumnMeta в types.go)
- Test: `server/internal/driver/mysql/data_integration_test.go` (tag `integration`)

**Interfaces:**
- Consumes: sqlgen.BuildSelect.
- Produces:
```go
// types.go
type ColumnMeta struct { Name string `json:"name"`; Type string `json:"type"`; Nullable bool `json:"nullable"` }
type Page struct {
    Columns   []ColumnMeta `json:"columns"`
    Rows      [][]any      `json:"rows"` // JSON-совместимые значения; NULL → nil; []byte → hex-строка "0x..." (спека §4.5)
    TotalRows int64        `json:"totalRows"`
}
// Driver interface += SelectRows(ctx context.Context, db, table string, q sqlgen.SelectQuery) (Page, error)
// Driver interface += PrimaryKey(ctx context.Context, db, table string) ([]string, error) // имена PK-колонок; пусто если нет
```
- data.go: COUNT(*) по тем же фильтрам (без сортировки/лимита); SELECT по BuildSelect; сканирование: []byte (BLOB/BINARY) → hex-строка с префиксом "0x"; time.Time → RFC3339-строка; остальное как есть.

- [ ] **Step 1: Падающие интеграционные тесты** — `TestSelectRows_Pagination` (seed 7 строк → limit 3 offset 2 → 3 строки в порядке сортировки, TotalRows=7); `TestSelectRows_Filters` (LIKE, =, IS NULL, IN); `TestSelectRows_NullVsEmpty` (строка с NULL и "" в разных колонках → nil и "" в rows различимы); `TestSelectRows_InjectionValue` (фильтр Value с `'; DROP TABLE` → данные не повреждены, результат пуст); `TestPrimaryKey` (PK-таблица → [id]; таблица без PK → []).
- [ ] **Step 2: FAIL.**
- [ ] **Step 3: Реализовать.**
- [ ] **Step 4: PASS + lint.**
- [ ] **Step 5: Commit** `feat(mysql): server-side row browsing with filters and pagination`.

### Task 7: Драйвер — Insert/Update/Delete/Upsert/LastInsertID

**Files:**
- Modify: `server/internal/driver/mysql/data.go`
- Test: `server/internal/driver/mysql/data_integration_test.go` (доп. кейсы)

**Interfaces:**
- Produces:
```go
type RowValues map[string]any // JSON null → nil → SQL NULL; "" → пустая строка
type PK map[string]any        // column → value (WHERE по PK; при отсутствии PK хендлер передаёт все колонки — решение АПИ-слоя, драйверу всё равно)
// Driver interface +=
InsertRow(ctx context.Context, db, table string, vals RowValues) error
UpdateRow(ctx context.Context, db, table string, where PK, vals RowValues) error
DeleteRows(ctx context.Context, db, table string, where []PK) error // пакетно: один DELETE с OR (a=? AND b=?) OR (...) — либо цикл; РЕШЕНИЕ: цикл отдельных DELETE в транзакции
UpsertRows(ctx context.Context, db, table string, rows []RowValues) error // INSERT ... ON DUPLICATE KEY UPDATE (Caps.Upsert=true для mysql/maria)
LastInsertID(ctx context.Context) (int64, error)
```
- [ ] **Step 1: Падающие интеграционные тесты** — `TestInsertRow_NullVsEmpty` (nil и "" лягут как NULL и ''); `TestUpdateRow_ByPK`; `TestUpdateRow_NoPK` (таблица без PK: where = все колонки; дубликат → затронуты обе — проверяем затронутые через affected? UpdateRow возвращает error; для проверки создадим 2 одинаковые строки, обновим по where=их общим значениям → обе изменились); `TestDeleteRows_Multiple`; `TestUpsertRows` (вставка с существующим PK → значение обновлено); `TestInsertRow_LastInsertID` (auto_increment → LastInsertID после InsertRow).
- [ ] **Step 2: FAIL.**
- [ ] **Step 3: Реализовать.** LastInsertID — хранить результат последнего INSERT в драйвере (mutex-protected int64) ИЛИ читать `LAST_INSERT_ID()` на том же conn — РЕШЕНИЕ: сохранять в поле драйвера из InsertRow/UpsertRows (sql.Result.LastInsertId), потокобезопасно.
- [ ] **Step 4: PASS + lint.**
- [ ] **Step 5: Commit** `feat(mysql): row write operations with upsert and last insert id`.

### Task 8: API — эндпоинты таблиц и данных

**Files:**
- Create: `server/internal/api/handlers_tables.go`, `server/internal/api/handlers_rows.go`
- Modify: `server/internal/api/router.go` (маршруты), `server/internal/api/handlers_session.go` (GET /api/session — без изменений)
- Test: `server/internal/api/router_test.go` (доп. fake-кейсы)

**Interfaces:**
- Consumes: Tasks 2–7 методы драйвера, П1 конверт/middleware.
- Produces (маршруты, все auth, все НЕ-GET требуют X-Requested-With):
```
GET    /api/databases/{db}/tables                    → {tables: [TableInfo]}
GET    /api/databases/{db}/tables/{t}                → TableDef
PUT    /api/databases/{db}/tables/{t}                → 204 (t="_new" → CREATE; body TableDef)
DELETE /api/databases/{db}/tables/{t}                → 204 (query ?isView=true|false)
POST   /api/databases/{db}/tables/{t}/truncate       → 204
POST   /api/databases/{db}/tables/{t}/indexes        → 204 (body [Index])
GET    /api/databases/{db}/tables/{t}/rows           → Page; ?limit&offset&order=col:asc|col:desc&filter=col:op:value (multi)
POST   /api/databases/{db}/tables/{t}/rows           → {lastInsertId: N} (body RowValues)
PATCH  /api/databases/{db}/tables/{t}/rows           → 204 (body {where: PK, values: RowValues})
DELETE /api/databases/{db}/tables/{t}/rows           → 204 (body {where: [PK]})
POST   /api/databases/{db}/tables                    → 204 (body {action: "truncate"|"drop", tables: [string]})
GET    /api/databases/{db}/tables/{t}/pk             → {columns: [string]}
```
- Разбор query-параметров: `order=name:desc`; `filter=col:op:value` где value может содержать `:` — split по ПЕРВЫМ двум `:` (n=3); limit cap 1000 → больше = 400 bad_request; неизвестный оператор → 400; пустые db/t → 400.
- Ошибки драйвера → 500 `driver_error` с сообщением; если ошибка валидации внутри (ErrEmptyIdentifier) → 400.

- [ ] **Step 1: Падающие API-тесты (fake-драйвер)** — расширить fake Driver П1: реализовать новые методы с записью вызовов. `TestTablesListHandler`, `TestTableDefHandler`, `TestSaveTableHandler_New` (t=_new), `TestRowsHandler_QueryParsing` (order+multi-filter+limit cap), `TestRowsHandler_UnknownOperator` → 400, `TestRowsHandler_InsertReturnsLastInsertId`, `TestBulkTablesHandler`, `TestRowsHandler_MutationRequiresCSRF` уже покрыто middleware — не дублируем.
- [ ] **Step 2: FAIL.**
- [ ] **Step 3: Реализовать** handlers + маршруты.
- [ ] **Step 4: PASS + lint.**
- [ ] **Step 5: Commit** `feat(api): tables and rows endpoints`.

### Task 9: Интеграционный сквозной тест API (реальный MySQL через роутер)

**Files:**
- Create: `server/internal/api/router_integration_tables_test.go` (tag `integration`)
- Test: тот же файл

**Interfaces:**
- Consumes: Tasks 2–8 (всё), testcontainers-хелпер П1.

- [ ] **Step 1: Падающий сквозной тест** — `TestAPI_TableLifecycle` (mysql:8.4): логин → POST create table (_new, def с PK+varchar+enum) → GET def совпадает → GET tables содержит → POST rows insert {lastInsertId} → GET rows с фильтром LIKE → PATCH rows → GET rows проверка → DELETE rows → POST truncate → PUT indexes → DELETE table → GET tables не содержит. `TestAPI_ReservedIdentifiers` (таблица `order`, колонки `select`, `where` со спецсимволом `a`b` — весь CRUD проходит). Повторить lifecycle для mariadb:11.4 (table-driven).
- [ ] **Step 2: FAIL → фиксы проводки → PASS.**
- [ ] **Step 3: lint.**
- [ ] **Step 4: Commit** `test(api): end-to-end table lifecycle against live servers`.

### Task 10: Фронт — API-типы и каталог-стор

**Files:**
- Modify: `web/src/api/types.ts` (+ TableInfo, ColumnDef, DefaultValue, Index, IndexColumn, ForeignKey, TableOptions, TableDef, ColumnMeta, Page, SelectQuery-типы зеркала)
- Create: `web/src/stores/catalog.ts`
- Test: `web/src/stores/catalog.test.ts`

**Interfaces:**
- Produces:
```ts
// types.ts: зеркала Go-типов Task 3/6 (camelCase, как json-теги)
export interface TableInfo { name: string; rows: number; sizeBytes: number; engine: string; comment: string; isView: boolean }
export interface DefaultValue { value: string; isSqlExpression?: boolean }
export interface ColumnDef { name: string; oldName?: string; type: string; length?: string; unsigned?: boolean; nullable: boolean; default?: DefaultValue | null; autoIncrement?: boolean; onUpdate?: string; generated?: string; collation?: string; comment?: string; enumValues?: string[]; position?: string }
export interface IndexColumn { name: string; subPart?: number }
export interface Index { name: string; type: string; columns: IndexColumn[] }
export interface ForeignKey { name: string; columns: string[]; refTable: string; refColumns: string[]; onDelete?: string; onUpdate?: string }
export interface TableOptions { engine?: string; collation?: string; comment?: string; autoIncrement?: number }
export interface TableDef { newName?: string; columns: ColumnDef[]; indexes: Index[]; foreignKeys: ForeignKey[]; options: TableOptions }
export interface ColumnMeta { name: string; type: string; nullable: boolean }
export interface Page { columns: ColumnMeta[]; rows: (unknown)[][]; totalRows: number }
// stores/catalog.ts
export const useCatalogStore = defineStore('catalog', {
  state: () => ({ tables: [] as TableInfo[], loading: false, error: null as string | null }),
  actions: { fetchTables(db: string), invalidate(db: string) } // invalidate: refetch если кэш этой БД
})
```
- Инвалидация после DDL: компоненты вызывают `invalidate(db)` после PUT/DELETE/truncate (стор не отслеживает сам).

- [ ] **Step 1: Падающие тесты** — `catalog.test.ts` (fetchTables заполняет tables; ошибка → error; invalidate перезапрашивает).
- [ ] **Step 2: FAIL.** — [ ] **Step 3: Реализовать.** — [ ] **Step 4: PASS + type-check + lint.**
- [ ] **Step 5: Commit** `feat(web): table api types and catalog store`.

### Task 11: Фронт — дерево объектов в сайдбаре

**Files:**
- Create: `web/src/components/ObjectTree.vue`
- Modify: `web/src/components/AppSidebar.vue` (рендер ObjectTree), `web/src/router/index.ts` (+ маршрут `/db/:db/table/:t` с query `?view=data|structure`, дефолт data), `web/src/i18n/locales/ru.ts`, `web/src/i18n/locales/en.ts` (+ключи tree.*), `web/src/views/ServerView.vue` (клик по БД → переход `/db/:db`)
- Test: `web/src/components/ObjectTree.test.ts`

**Interfaces:**
- Consumes: catalog store, session store (caps — П2 показывает только группу «Таблицы»; другие группы по caps появятся в П5).
- Produces: дерево `БД (актуальная из сессии) → Таблицы (список TableInfo; isView в отдельную группу Views)`, поиск по имени (n-input filter), клик по таблице → `router.push(/db/{db}/table/{name})`. Выбор БД: n-select из списка БД (запрос /api/databases) в шапке сайдбара.

- [ ] **Step 1: Падающий тест** — `ObjectTree.test.ts`: рендерит список таблиц; фильтр отсеивает; клик по таблице вызывает router.push на `/db/mydb/table/t1` (mock router).
- [ ] **Step 2: FAIL.** — [ ] **Step 3: Реализовать.** — [ ] **Step 4: PASS + type-check + lint.**
- [ ] **Step 5: Commit** `feat(web): object tree sidebar with table navigation`.

### Task 12: Фронт — грид данных таблицы (TanStack Table, сервер-сайд)

**Files:**
- Create: `web/src/components/DataGrid.vue`, `web/src/lib/tableQuery.ts` (сборка query-строки order/filter из состояния)
- Modify: `web/src/views/TableView.vue` (новый view: хлебные крошки + переключение data/structure + DataGrid), `web/package.json` (+ `@tanstack/vue-table`)
- Test: `web/src/lib/tableQuery.test.ts`, `web/src/components/DataGrid.test.ts`

**Interfaces:**
- Consumes: GET rows (Task 8), formatBytes (П1).
- Produces:
```ts
// lib/tableQuery.ts
export function buildRowsQuery(state: { limit: number; offset: number; sort: { column: string; desc: boolean } | null; filters: { column: string; op: string; value: string }[] }): string
// "limit=50&offset=0&order=name:asc&filter=age:gte:30" (URLSearchParams, encode)
```
- DataGrid: n-data-table поверх TanStack (или n-data-table напрямую с сервер-сайд remote-режимом — Naive умеет remote: РЕШЕНИЕ — n-data-table с `remote` + pagination/sorter колбэки, TanStack не тянем в П2, т.к. Naive покрывает; зависимость @tanstack/vue-table в П2 НЕ добавляется — спека предлагает стек, но выбор внутри стека за планом; запись в отчёт). Колонки из Page.columns; NULL-ячейка — курсив «NULL»; multi-select строк (checkbox) → кнопка Delete выделенных; кнопка «Add row» (открывает RowModal Task 13); клик по строке → редактирование; строка фильтров над гридом (оператор — n-select из фиксированного списка AllowedOperators, значение — input; кнопка Применить); пагинация: page-size select (25/50/100/200), счётчик TotalRows; сортировка по клику на колонку (remote).
- RowKey: из GET pk (сторе catalog или локально в TableView): если PK есть — значение PK-колонок (join " | "), иначе индекс строки (`__idx_${offset+i}` — правка строки без PK по индексу НЕ поддерживается в UI, только delete-all-matching? РЕШЕНИЕ: без PK кнопки edit/delete у строки скрыты, показывается предупреждение «таблица без первичного ключа»).

- [ ] **Step 1: Падающие тесты** — `tableQuery.test.ts` (сборка строки query, encode спецсимволов); `DataGrid.test.ts`: рендер колонок из Page; NULL показывается как «NULL» (курсив-класс); пагинация вызывает перезагрузку с новым offset; select строк + delete вызывает client.delete с where по PK.
- [ ] **Step 2: FAIL.** — [ ] **Step 3: Реализовать.** — [ ] **Step 4: PASS + type-check + lint.**
- [ ] **Step 5: Commit** `feat(web): server-side data grid with filters, pagination, row selection`.

### Task 13: Фронт — модалка вставки/правки строки

**Files:**
- Create: `web/src/components/RowModal.vue`
- Modify: `web/src/components/DataGrid.vue` (интеграция), `web/src/i18n/locales/*.ts` (+row.*)
- Test: `web/src/components/RowModal.test.ts`

**Interfaces:**
- Consumes: POST/PATCH rows, ColumnDef (нужен TableDef? РЕШЕНИЕ: типизированные поля строятся из Page.columns (имя+тип+nullable) — достаточно для браузинга; enum-значения требуют ColumnDef —TableView загружает TableDef параллельно и передаёт в модалку).
- Produces: модалка: поле на колонку; тип ввода по типу: число → n-input-number, enum → n-select (enumValues), всё остальное → n-input (textarea для длинных); NULL-тумблер на каждом поле (включён = отправлять null, поле disabled); кнопка Save → insert (новая) или patch (where = PK строки, передан пропсом); ошибки → notification (message драйвера).

- [ ] **Step 1: Падающие тесты** — `RowModal.test.ts`: рендерит поле на колонку; NULL-тумблер отправляет null и блокирует ввод; enum рендерит n-select с опциями; save вызывает client.patch с where=PK и values без null-выключенных полей.
- [ ] **Step 2: FAIL.** — [ ] **Step 3: Реализовать.** — [ ] **Step 4: PASS + type-check + lint.**
- [ ] **Step 5: Commit** `feat(web): row insert/edit modal with null toggles and typed inputs`.

### Task 14: Фронт — редактор структуры таблицы

**Files:**
- Create: `web/src/views/structure/StructureEditor.vue` (список колонок), `web/src/views/structure/IndexEditor.vue`, `web/src/views/structure/OptionsEditor.vue`
- Modify: `web/src/views/TableView.vue` (view=structure), `web/src/i18n/locales/*.ts` (+structure.*)
- Test: `web/src/views/structure/StructureEditor.test.ts`

**Interfaces:**
- Consumes: GET/PUT TableDef (Task 8), POST indexes.
- Produces: редактор колонок — таблица строк: имя, тип (input с datalist распространённых типов: int, varchar, text, decimal, datetime, enum...), length, unsigned (checkbox, показывается для числовых), nullable (checkbox), default (input + checkbox «SQL-выражение» + NULL-check), auto_increment (radio — одна колонка), on update, generated (select: нет/VIRTUAL/STORED + expression input), collation, comment, кнопки up/down (позиция), delete (отметка к удалению: строка с существующим Name удаляется через пустое определение — UI посылает строку с name и type:"" ). Новые строки без oldName. Save → один PUT TableDef (existing rows → oldName=name если name не менялся, иначе oldName=старое имя; renaming UI: редактируемое имя, oldName фиксируется при загрузке) + invalidate. Индексы: список (имя, тип, колонки) + add/remove → POST indexes. Опции: engine/collation/comment/auto_increment → PUT. FK-редактор — П5 (чтение показываем списком read-only в П2? РЕШЕНИЕ: FK показываются read-only списком, правка FK — П5, зафиксировать TODO-комментарием).

- [ ] **Step 1: Падающие тесты** — `StructureEditor.test.ts`: рендер колонок из def; переименование колонки → PUT получает oldName=старое, name=новое; отметка удаления → PUT содержит строку name с type:""; add row → колонка без oldName; up/down меняет порядок; Save отправляет один PUT.
- [ ] **Step 2: FAIL.** — [ ] **Step 3: Реализовать.** — [ ] **Step 4: PASS + type-check + lint + build.**
- [ ] **Step 5: Commit** `feat(web): table structure editor with column/index/options editing`.

### Task 15: Сквозная ручная верификация + make-цели

**Files:**
- Modify: `Makefile` (без изменений — цели П1 уже покрывают; проверить только)

**Steps:**
- [ ] **Step 1: Полные проверки** — `make lint && make backend-test && make web-build && make build`, `go test -tags integration ./...` (docker) — всё зелёное.
- [ ] **Step 2: Ручной сценарий** — docker mysql:8.4 → make build → браузером: логин → дерево → выбрать таблицу → браузинг с фильтром и сортировкой → добавить строку (NULL-тумблер) → отредактировать → удалить → создать таблицу через структуру → drop. Остановить всё. (Контроллер выполнит через browser-use.)
- [ ] **Step 3: Commit** (если есть правки) `chore: p2 verification fixes`.

---

## Самопроверка плана (выполнена при написании)

- **Покрытие спеки для П2**: §4.2 методы данных/структуры — Tasks 2–7; §4.5 типы — Task 3/6; §4.6 (MySQL поштучный DDL) — Task 4; §6 API таблиц/данных — Task 8–9; §8 дерево/данные/структура — Tasks 11–14; §9.4 инъекции — Task 1/8 тесты. FK-редактирование, BLOB-превью, copy строки, экспорт выборки — П3/П5 (осознанно).
- **Согласованность**: sqlgen.SelectQuery переиспользуется драйвером и API; RowValues/PK — общие; json-теги зеркалятся в web/src/api/types.ts.
- **Review Focus**: 5 пунктов закреплены тестами (Tasks 1, 7, 8, 9, 12).
- **Пропорция**: тела не транскрибируются; сигнатуры точные.
- **Отклонение от стека спеки**: TanStack Table заменён на remote-режим n-data-table (Naive UI) для П2 — грид сервер-сайд; TanStack подключается в П3+ только если появится клиентская виртуализация. Спека — vision document; выбор внутри заявленного стека (Naive UI тоже в стеке).
