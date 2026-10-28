# П5/П6 — автономное завершение v0.1

Spec: `docs/superpowers/specs/2026-10-23-db-manager-design.md`.
User authorization: 2026-10-28 — автономная работа, коммиты после значимых изменений, периодические слияния в master. Дополнительные согласования дизайна и планов не требуются.

Архитектура: сохраняем текущие Driver и request-local schema; новые подсистемы используют capability-интерфейсы. Независимые подсистемы реализуются в отдельных файлах. Общие router/i18n/navigation интегрирует основной агент. Сырой SQL допустим для явно обозначенного редактора исходного SQL объектов, как для существующей консоли; параметры административных форм валидируются и экранируются.

## Задачи

- [x] П5.1 API и UI SQL определения таблицы (отдельный согласованный план).
- [x] П5.2 Views, triggers, routines, MySQL events/partitions: драйверы, API, UI, capability gating и тесты.
- [x] П5.3 MySQL users/password/limits/grants и MySQL/PG monitoring: драйверы, API, UI и тесты.
- [x] П5.4 Database CRUD, collation, table move/copy, metadata; завершение sequences и отдельные замечания PG.
- [x] П5.5 FK editor, parent values, row copy/upsert/no-PK, export selection/views, BLOB preview, SQL formatter.
- [x] П5.6 Полная проверка матрицы mysql:8.4 / mariadb:11.4 / postgres:17, independent review, commit и merge П5.
- [x] П6.1 Dockerfile non-root, GoReleaser, multi-platform releases, документация и changelog.
- [ ] П6.2 Проверка сборки/контейнера/дистрибутива, review, commit и merge П6 (сборка и review выполнены).

## Контракты интеграции

Каждая подсистема определяет собственный `register...Routes(chi.Router)` метод apiServer и экспортирует массив Vue routes; основной агент добавляет вызовы в router.go и Vue Router. Новые интерфейсы не расширяют базовый Driver, чтобы существующие fake-драйверы продолжали компилироваться. Все новые API под существующими auth/CSRF/requestSchema middleware. Языки ru/en через отдельные message-модули, объединяемые основным агентом.

## Проверка

Каждая значимая реализация: наблюдаемый RED → GREEN; Go unit/API, Vitest, linters/typecheck, make build. Драйверные операции проверяются интеграционно в настоящих тестовых контейнерах. Деструктивные сценарии только в созданных тестовых БД. Перед слиянием — независимое ревью и зелёные проверки; после слияния — проверка master. Публикация тегов/образов во внешний registry не входит в локальную подготовку релиза без настроенного remote.

## Решения и результаты

- Существующий чистый worktree П4 переиспользован с новой веткой `codex/p5-p6`, база `201bffa`.
- Рабочее имя `db-manager` сохраняется для локальной подготовки релиза.
- П5 commits: `7221bb1` (основной функционал), `1ec6672` (точные BIGINT), `04d225c` (исправления по ревью), `84d8e92` (обновление FK-теста). Полные API и PostgreSQL интеграции прошли. Все 117 тестов MySQL/MariaDB пакета выполнены в трёх непересекающихся группах с кодом выхода 0; третья группа повторена целиком после исправления устаревшего ожидания FK. Живые тесты ключа BIGINT и обновления view после исправлений прошли на всех нужных движках. Go unit/lint, 191 frontend test, ESLint, проверка типов и сборка прошли.
- П6 проверен в отдельной копии с тестовым remote на исходниках после `04d225c`: шесть архивов, checksum, host HTTP, arm64 ELF и nonroot read-only Docker.
