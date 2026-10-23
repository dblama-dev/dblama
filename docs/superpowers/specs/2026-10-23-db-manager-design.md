# db-manager — дизайн-спека v0.1

Дата: 2026-10-23
Статус: согласован (brainstorming), ожидает ревью
Рабочее название: **db-manager** (финальное имя продукта — открытым вопросом)

## 1. Цель и границы

Опенсорс-инструмент веб-администрирования БД класса Adminer с современным интерфейсом.
Стек: Go (бэкенд) + Vue 3 (SPA). Не клон phpMyAdmin: без pmadb, designer'а, setup-скрипта,
репликации, тем оформления как продукта, десятков переводов.

**Поддерживаемые СУБД v0.1**: MySQL 8+, MariaDB 10.11+, PostgreSQL 14+.

**Вне рамок v0.1** (потенциально позже): SQLite/MS SQL/Oracle/CockroachDB, управление
пользователями PostgreSQL, чарты, режим read-only, SSO.

## 2. Ключевые решения (зафиксировано в brainstorming)

| Решение | Выбор |
|---|---|
| Модель доступа | Как Adminer: вход под учёткой самой БД; бэкенд без своей учётной системы и хранилища |
| Архитектура СУБД-специфики | Единый интерфейс драйвера + capability-расширения (type-assertion) |
| Объём v0.1 | Полный набор Adminer (см. §10) |
| UI-стек | Vue 3 + Naive UI + TanStack Table (грид) + CodeMirror 6 (SQL) |
| Раскладка UI | Сайдбар-дерево + один текущий экран (БЕЗ вкладок) + хлебные крошки + история браузера |
| Распространение | Один бинарник со встроенным SPA (go:embed) + Docker-образ |
| Языки UI | Русский, английский (vue-i18n) |
| Темы | Тёмная/светлая (Naive UI) |

Модель драйвера сверена с реальным кодом Adminer (репозиторий vrana/adminer, изучен
`adminer/drivers/mysql.inc.php`, `pgsql.inc.php`, `include/driver.inc.php`, `user.inc.php`):
общий контракт ~50 функций на драйвер, capabilities зависят от версии сервера
(`support($feature)`), правка структуры — «полное желаемое состояние» с `old_name` у колонок.

## 3. Процессная модель и раскладка

Один процесс: HTTP-сервер обслуживает `/api/*` и отдаёт статик SPA (go:embed).
Конфигурация — только переменные окружения:

| Переменная | По умолчанию | Смысл |
|---|---|---|
| `LISTEN` | `:8080` | адрес HTTP-сервера |
| `SESSION_TTL` | `30m` | idle-TTL сессии |
| `LOGIN_RATE_LIMIT` | `10/min` | неудачных логинов per-IP |
| `TRUSTED_SERVERS` | пусто (выкл) | опциональный allowlist `host:port` СУБД |

```
db-manager/
  server/
    cmd/dbmanager/main.go
    internal/
      api/            # chi-роутер, хендлеры, middleware (auth, ratelimit, errors)
      session/        # store: map[ID]*Session + janitor (idle-TTL)
      driver/         # контракт: driver.go, types.go, registry.go
        mysql/        # один пакет на MySQL и MariaDB (протокол общий; flavor по VERSION())
        postgres/
      sqlgen/         # экранирование идентификаторов, сборка WHERE/ORDER/LIMIT
      dump/           # экспорт (SQL/CSV/gzip), импорт (SQL/CSV)
      config/
  web/                # Vue 3 SPA (Vite, TS)
  docs/superpowers/specs/
  .github/workflows/
```

Зависимости Go: `chi/v5`, `go-sql-driver/mysql`, `jackc/pgx/v5/stdlib`. MySQL и MariaDB —
один драйвер; flavor определяется после коннекта и показывается в UI.

## 4. Слой драйверов

### 4.1. Жизненный цикл

`driver.Connect(cfg ConnConfig)` открывает `*sql.DB` (MaxOpenConns=4, ping) и возвращает
**экземпляр драйвера**, привязанный к соединению. `Caps()` вычисляется один раз при коннекте
(зависят от типа и версии сервера). Хендлеры API получают драйвер из сессии и проверяют
расширения через type-assertion:

```go
if um, ok := drv.(UserManager); ok { ... } else { ErrDriverNotSupported }
```

### 4.2. Основной интерфейс

```go
type Driver interface {
    Info() DriverInfo   // name, flavor: "mysql"|"maria"|"postgres", версия сервера, юзер, хост
    Caps() Caps         // см. 4.4

    // Метаданные для UI-редакторов
    ColumnTypes() []TypeInfo      // типы колонок для выпадающих списков
    Collations() []Collation
    Operators() []Operator        // операторы фильтра браузинга: =, !=, <, >, LIKE, REGEXP, IN, ...
    TriggerOpts() TriggerOpts
    RoutineOpts() RoutineOpts

    // Базы данных
    ListDatabases(ctx) ([]DatabaseInfo, error)          // имя, размер, коллация
    CreateDatabase(ctx, name string, collation string) error
    DropDatabase(ctx, name string) error
    RenameDatabase(ctx, oldName, newName string) error  // MySQL: create+dump+drop; PG: ALTER DATABASE RENAME

    // Таблицы и структура
    ListTables(ctx, db string) ([]TableInfo, error)     // имя, rows, size, engine, comment, isView
    TableDef(ctx, db, table string) (TableDef, error)   // колонки + индексы + FK + опции (TableOptions)
    SaveTable(ctx, db, table string, desired TableDef) error
        // модель Adminer: полное желаемое состояние; у каждой колонки OldName
        // (пустой OldName = ADD, пустое определение = DROP, иначе MODIFY/CHANGE);
        // table=="" => CREATE TABLE; включает rename таблицы (TableDef.NewName)
        // и опции (engine, collation, comment, auto_increment)
    SaveIndexes(ctx, db, table string, desired []Index) error  // отдельной операцией (как alter_indexes в Adminer)
    TruncateTables(ctx, db string, tables []string) error
    DropTables(ctx, db string, tables []string) error
    DropViews(ctx, db string, views []string) error
    MoveTables(ctx, db string, tables []string, targetDB string) error
    CopyTables(ctx, db string, tables []string, targetDB string) error
    ShowCreateTable(ctx, db, table string) (string, error)     // вкладка "SQL" у таблицы

    // Данные
    SelectRows(ctx, db, table string, q SelectQuery) (Page, error)
        // q: колонки (по умолчанию *), фильтры []Filter{Column, Operator, Value},
        //    сортировка []Sort, limit (по умолчанию 50), offset
        // Page: rows (значения как JSON-совместимые + типизированные), totalRows (COUNT),
        //       truncated=false всегда (лимит контролирует клиент)
    InsertRow(ctx, db, table string, vals RowValues) error
    UpdateRow(ctx, db, table string, pk PK, vals RowValues) error  // WHERE по PK; при отсутствии PK — по всем колонкам
    UpsertRows(ctx, db, table string, rows []RowValues) error      // ON DUPLICATE KEY / ON CONFLICT
    DeleteRows(ctx, db, table string, where []PK) error
    LastInsertID(ctx) (int64, error)
    Warnings(ctx) ([]Warning, error)                               // MySQL; PG — пусто

    // SQL-консоль
    ExecSQL(ctx, db, sql string, opts ExecOpts) ([]Result, error)
        // мульти-стейтменты; Result: колонки+строки | affectedRows | lastInsertID;
        // лимит строк на стейтмент (по умолчанию 1000, флаг truncated); таймаут из opts
    Explain(ctx, db, sql string) (Result, error)   // EXPLAIN (MySQL) / EXPLAIN ANALYZE-обёртка (PG)
    CancelQuery(ctx, queryID string) error         // отмена своей долгой консольной команды

    // Дамп
    Export(ctx, req ExportRequest, w io.Writer) error   // поточная запись; см. §7
    ImportSQL(ctx, db string, r io.Reader, progress func(done, total int64)) error
    ImportCSV(ctx, db, table string, r io.Reader, opts CSVImportOpts) error
}
```

Все идентификаторы в генерируемом SQL экранируются слоем `sqlgen` (бэктики для MySQL,
`"` для PG); значения — параметризованные запросы. Сырой SQL от клиента проходит только
в `ExecSQL`/`Explain` (назначение консоли) и в импорте SQL-файла.

### 4.3. Расширения (capability-интерфейсы)

| Интерфейс | Методы | Реализация в v0.1 |
|---|---|---|
| `UserManager` | ListUsers, CreateUser, DropUser, RenameUser, SetPassword, Limits (MAX_QUERIES_PER_HOUR...), Grants(user) — из SHOW GRANTS, Grant/Revoke(priv, level) — global/db/table, PrivilegeCatalog() — матрица из SHOW PRIVILEGES | MySQL/MariaDB |
| `ViewManager` | ListViews, AlterView (CREATE OR REPLACE ... AS SELECT), DropView | оба |
| `TriggerManager` | ListTriggers, CreateTrigger, DropTrigger (исходник — SQL) | оба |
| `RoutineManager` | ListRoutines (процедуры+функции), CreateRoutine, DropRoutine, CallProcedure | оба |
| `EventManager` | ListEvents, CreateEvent, DropEvent (ON SCHEDULE ...) | MySQL/MariaDB |
| `SequenceManager` | ListSequences, CreateSequence, DropSequence, AlterSequence (restart/increment) | PostgreSQL |
| `SchemaManager` | ListSchemas, CurrentSchema, SetSchema | PostgreSQL |
| `PartitionManager` | ListPartitions, AddPartition, DropPartition, TruncatePartition | MySQL/MariaDB |
| `Monitor` | Processes, StatusVars, ServerVars, KillProcess | оба (набор различается) |

### 4.4. Caps (вычисляются при коннекте)

```go
type Caps struct {
    Views, Triggers, Routines, Procedures bool // PG: Procedures только 11+
    Events, Partitions bool                    // MySQL/MariaDB
    Sequences, Schemas bool                    // PostgreSQL
    Users bool                                 // MySQL/MariaDB в v0.1
    Monitor bool                               // processes/variables/status/kill
    MoveCopy bool                              // перенос/копирование таблиц между БД
    Comments bool                              // комментарии объектов
    Upsert bool
    DescendingIndexes bool                     // MySQL 8+
}
```

Caps отдаются в `GET /api/session`; UI скрывает неподдерживаемые разделы. Хендлеры
capability-эндпоинтов при отсутствии интерфейса возвращают ошибку `driver_not_supported`.

### 4.5. Ключевые типы

```go
type ColumnDef struct {
    Name, OldName         string
    Type                  string   // "int", "varchar", ...
    Length                string
    Unsigned              bool     // MySQL
    Nullable              bool
    Default               *DefaultValue // {Value, IsSQLExpression} — MySQL хранит выражения и литералы по-разному
    AutoIncrement         bool
    OnUpdate              string   // MySQL CURRENT_TIMESTAMP(...)
    Generated             string   // "" | "VIRTUAL" | "STORED" (+ выражение в Default)
    Collation             string
    Comment               string
    EnumValues            []string // enum/set
    Position              string   // "" | "FIRST" | "AFTER <col>"
}

type TableDef struct {
    NewName      string     // при rename
    Columns      []ColumnDef
    Indexes      []Index    // для чтения; правка — SaveIndexes
    ForeignKeys  []ForeignKey
    Options      TableOptions // engine, collation, comment, autoIncrement
}

type Result struct {
    Columns      []ColumnMeta  // имя, тип (метка драйвера), nullable
    Rows         [][]any       // значения JSON-совместимы
    AffectedRows int64
    LastInsertID int64
    Truncated    bool
    Seconds      float64
}
```

Чтение структуры — через information_schema (у Adminer так же); enum/set-значения парсятся
из COLUMN_TYPE. BLOB в JSON ответе — hex-строка с флагом; UI решает, показывать hex/картинку/текст.

### 4.6. Транзакции правок

PG: структурные правки (SaveTable + SaveIndexes + FK) выполняются в одной транзакции
(DDL-транзакции поддерживаются). MySQL/MariaDB: DDL авто-коммитится — выполняем поштучно,
при ошибке возвращаем номер шага и сообщение (как Adminer).

## 5. Сессии и подключения

- `POST /api/session` → драйвер `Connect` (ping) → сессия: ID (256 бит crypto/rand),
  cookie `HttpOnly; Secure; SameSite=Strict`, учётка в памяти процесса.
- Пул `*sql.DB` на сессию. Janitor-горутина раз в минуту закрывает простаивающие сессии
  (idle > SESSION_TTL) и их пулы.
- `DELETE /api/session` = logout: Close пула, удаление сессии.
- Предел сессий на процесс (default 1000, защита памяти) — при превышении выселяется
  старейшая idle.
- Брутфорс: token bucket per-IP на неудачные `POST /api/session` (LOGIN_RATE_LIMIT).

## 6. API (REST/JSON, cookie-сессия)

Единый конверт ошибок:
```json
{ "error": { "code": "driver_error", "message": "...", "sqlState": "42S02",
             "driverCode": 1146, "detail": "Table 'x.y' doesn't exist" } }
```
Коды: `auth_required`, `auth_failed`, `bad_request`, `not_found`, `driver_error`,
`driver_not_supported`, `query_timeout`, `query_cancelled`, `rate_limited`, `internal`.

| Метод и путь | Назначение |
|---|---|
| `GET /api/drivers` | публичный: список поддерживаемых СУБД `[{id, name, defaultPort}]` для экрана логина |
| `POST /api/session` | вход `{driver, host, port, user, password, database?}` |
| `GET /api/session` | инфо сервера + caps + текущая БД |
| `DELETE /api/session` | logout |
| `GET /api/databases` | список БД |
| `POST /api/databases` / `DELETE /api/databases/{db}` / `PATCH /api/databases/{db}` | create / drop / rename |
| `GET /api/databases/{db}/tables` | таблицы+views (TableInfo) |
| `GET /api/databases/{db}/tables/{t}` | TableDef |
| `PUT /api/databases/{db}/tables/{t}` | SaveTable (t=`_new` для CREATE) |
| `DELETE /api/databases/{db}/tables/{t}` | drop (таблица или view по isView) |
| `POST /api/databases/{db}/tables/{t}/truncate` | очистка |
| `GET /api/databases/{db}/tables/{t}/rows` | SelectRows: `?limit&offset&order[col]&filter[col]=op:value...` |
| `POST/PATCH/DELETE /api/databases/{db}/tables/{t}/rows` | insert / update (PK в теле) / delete (список PK) |
| `GET /api/databases/{db}/tables/{t}/create-sql` | ShowCreateTable |
| `POST /api/databases/{db}/tables/{t}/indexes` | SaveIndexes |
| `POST /api/databases/{db}/tables` | bulk: truncate/drop/move/copy списком |
| `POST /api/databases/{db}/query` | консоль `{sql, maxRows, timeoutSec}` → `[]Result` |
| `POST /api/query/{queryID}/cancel` | отмена |
| `POST /api/query` | EXPLAIN |
| `POST /api/databases/{db}/export` | дамп, потоковый ответ (SPA скачивает через fetch → blob; параметры в JSON-body) |
| `POST /api/databases/{db}/import` | multipart: SQL/CSV файл + опции |
| `/api/users`, `/api/views`, `/api/triggers`, `/api/routines`, `/api/events`, `/api/sequences`, `/api/schemas`, `/api/processes`, `/api/processes/{id}/kill`, `/api/status`, `/api/variables` | capability-ресурсы (монтируются при поддержке драйвером) |

Правила: мутации требуют заголовок `X-Requested-With` (анти-CSRF вместе с SameSite=Strict).
Долгие консольные запросы: `queryID` выдаётся клиентом (UUID), таймаут по умолчанию 60с.
PostgreSQL: эндпоинты таблиц/данных принимают query-параметр `?schema=` (по умолчанию —
дефолтная схема юзера).

## 7. Экспорт и импорт

**Экспорт** (MySQL и PG):
- Объекты: вся БД / выбранные таблицы+views / выбранное на экране данных (текущая выборка).
- Форматы: SQL (структура; данные INSERT; опции `IF NOT EXISTS`, `DROP IF EXISTS`, gzip),
  CSV, CSV (`;`), TSV.
- SQL-дамп: потоково (курсор + батчи по 1000 строк), `LOCK TABLES` не используем;
  structure из information_schema-генерации (ShowCreateTable / собственная генерация для PG),
  данные — батчированные INSERT (по 100 строк).
- Импорт: SQL-файл (построчное исполнение с учётом DELIMITER-блоков триггеров/рутин, gzip),
  CSV → INSERT (разделитель, enclosure, перевод строки, пропуск заголовка).
- Прогресс импорта — JSON-стрим `{"done":N,"total":M,"error":...}` по chunked-ответу.

## 8. Фронтенд

**Стек**: Vue 3 (Composition API) + TypeScript + Vite; Pinia; Vue Router; Naive UI;
TanStack Table; CodeMirror 6 (@codemirror/lang-sql + автодополнение из метаданных);
vue-i18n (ru/en).

**Каркас**:
- Левый сайдбар: дерево `БД → группы объектов (Таблицы, Views, Процедуры, Триггеры,
  События, Sequences, Схемы — видимость по caps)`, поиск по дереву, избранное не делаем.
- Центр: **один текущий экран** (без вкладок): навигация — клик по дереву + хлебные крошки
  (`server / db / table / раздел`), назад/вперёд — история браузера (vue-router).
- Верхняя панель: инфо сервера (flavor+версия, юзер), выбор БД, тема, язык, logout.

**Маршруты**: `/login`, `/server` (обзор БД), `/db/:db` (обзор таблиц), `/db/:db/table/:t`
(подразделы через query `?view=data|structure|sql|triggers|partitions`), `/db/:db/console`,
`/server/users`, `/server/processes`, `/db/:db/export`, `/db/:db/import` и т.п. Guard:
нет сессии → `/login`.

**Экраны**:
- **Логин**: выбор СУБД (MySQL/MariaDB/PostgreSQL), хост/порт/юзер/пароль/БД (опц.).
  Хост/юзер/СУБД — в localStorage, пароль — никогда.
- **Обзор сервера**: БД (размер, таблиц), создание БД (collation из драйвера), drop/rename.
- **Данные таблицы**: TanStack-грид, сервер-сайд пагинация/сортировка; строка фильтров
  (операторы из `Operators()`); значение NULL/пустое — различимы; BLOB — hex/превью;
  редактирование строки — модалка с типизированными полями по ColumnDef (NULL-тумблер,
  enum — select, FK — выбор из родительской таблицы); multi-select строк → delete/copy;
  кнопка «экспорт выборки».
- **Структура**: подтабы Колонки/Индексы/FK/Триггеры/Партиции/Опции/SQL. Редактор колонок —
  таблица строк (добавить/удалить/переставить), отправка одним `PUT` (SaveTable).
- **SQL-консоль**: CodeMirror, исполнить всё/выделение, несколько Result-гридов, EXPLAIN,
  форматтер, история в localStorage (per-сервер), счётчик времени.
- **Пользователи (MySQL)**: список; создание (логин/хост/пароль); grants-матрица
  (PrivilegeCatalog: глобальные/на БД/на таблицу, column-level не делаем в v0.1);
  смена пароля; лимиты; drop.
- **Мониторинг**: процессы (kill), переменные, статус.
- **Экспорт/импорт**: диалоги с деревом объектов и опциями §7.

**Сторы**: `session` (инфо+caps), `catalog` (кэш таблиц по БД, инвалидация после DDL),
`notifications`. Единый API-клиент (fetch): конверт ошибок → Naive notification.

## 9. Безопасность

1. Пароль БД — только в памяти процесса (в сессии); не логируется, не возвращается API.
2. Cookie: `HttpOnly; Secure; SameSite=Strict`; ID 256 бит crypto/rand.
3. CSRF: SameSite=Strict + обязательный `X-Requested-With` на мутациях.
4. Инъекции: параметризация значений; экранирование идентификаторов (sqlgen) на 100%
   генерируемого SQL; сырой SQL — только консоль/импорт (назначение инструмента).
5. Брутфорс: token bucket per-IP (LOGIN_RATE_LIMIT).
6. SSRF-паранойя: опциональный TRUSTED_SERVERS (host:port allowlist) — при заданном
   значении коннект только к адресам из списка.
7. Таймауты всех запросов (context); лимит строк консоли (hard cap 100k).
8. Security-заголовки SPA: `X-Content-Type-Options: nosniff`, `Referrer-Policy`,
   `X-Frame-Options: DENY`; CSP — после стабилизации (CodeMirror/Naive инлайн-стили).
9. Логи: адрес сервера, путь, статус, длительность; без тел запросов и учёток.

## 10. Объём v0.1 (чек-лист функций)

- [ ] Вход/выход, сессии с TTL, 3 СУБД (mysql 8.4, mariadb 11.4, postgres 17 в матрице)
- [ ] БД: список/создание/удаление/rename, collation
- [ ] Таблицы: список, создание, правка структуры (колонки/индексы/FK/опции), drop,
      truncate, rename, move/copy между БД
- [ ] Данные: браузинг (пагинация/сортировка/фильтры), insert/update/delete/copy строки,
      upsert, экспорт выборки, история SQL (localStorage)
- [ ] SQL-консоль: мульти-стейтменты, EXPLAIN, отмена, таймаут, форматирование
- [ ] Экспорт: SQL/CSV/TSV (+gzip), объекты и опции §7; импорт SQL/CSV с прогрессом
- [ ] Views, триггеры, процедуры/функции (CRUD, исходники)
- [ ] События (MySQL), sequences (PG), схемы (PG), партиции (MySQL)
- [ ] Пользователи и привилегии MySQL/MariaDB
- [ ] Мониторинг: процессы/kill, переменные, статус
- [ ] UI: тёмная/светлая тема, ru/en, поиск по дереву
- [ ] Сборка: бинарники + docker, CI (lint/unit/integration)

## 11. Тестирование

- **Юнит (Go)**: sqlgen — golden-тесты (экранирование, генерация ALTER/CREATE/SELECT на
  каждый драйвер); session store (TTL, выселение); rate-limit; error-маппинг.
- **Интеграционные (Go, testcontainers-go)**: `mysql:8.4`, `mariadb:11.4`, `postgres:17`;
  каждый метод драйвера — против живого сервера (фикстуры: миграции + seed-данные);
  round-trip: экспорт SQL → импорт в чистую БД → сравнение структуры и данных.
- **API (httptest)**: fake-драйвер; роутинг, caps-гейтинг (404/`driver_not_supported`),
  auth, CSRF-заголовок, конверт ошибок.
- **Фронтенд (Vitest)**: сторы, API-клиент, утилиты (форматирование значений по типу).
- **CI**: golangci-lint, eslint + vue-tsc, go test (unit), go test (integration, services
  в job-контейнерах), сборка. E2E (Playwright) — после v0.1.

## 12. Сборка и релизы

- goreleaser: `linux/darwin/windows × amd64/arm64`, SPA собирается Vite и вшивается
  `go:embed`. CGO_ENABLED=0.
- Docker: distroless/static, non-root, в ghcr.io.
- Semver по тегам; CHANGELOG генерируется из conventional commits.

## 13. Риски и открытые вопросы

1. **Имя продукта** — рабочее `db-manager`; выбрать до релиза (проверить занятость).
2. **Парсинг SHOW GRANTS** (MySQL) — регэкспы как у Adminer хрупки; закладываем
   парсер с тестами на форматы MySQL 8 / MariaDB.
3. **Правка колонок без PK** — обновление по совокупности значений строк (как Adminer);
  документируем ограничение при дубликатах.
4. **Кодировки консоли** — multi-statement с SET NAMES; go-sql-driver и pgx сами
   договариваются о UTF-8; проверяем экзотику (latin1-таблицы) интеграционно.
5. **PG: схемы и пул соединений** — `SET search_path` на общем пуле unsafe при параллельных
   запросах; поэтому схема передаётся параметром `?schema=` и драйвер квалифицирует имена
   таблиц схемой в каждом запросе.
