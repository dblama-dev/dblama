export const searchWorkflowMessages = {
  en: { searchWorkflows: {
    title: 'Search database', database: 'Database', tables: 'Tables to search', text: 'Text to find', start: 'Search', stop: 'Stop',
    limits: 'Select up to 20 tables. Search scans text columns with 2 concurrent requests and returns up to 20 matching rows per column. Each request times out after 15 seconds. Large tables can still be expensive to scan.',
    semantics: 'Find literal text, including %, _ and backslash. Case sensitivity follows the database collation. Arrays, binary, numeric and JSON columns are excluded.',
    selected: '{count} of 20 tables selected', progress: '{done} of {total} tables processed; {columns} text columns searched', running: 'Searching…', complete: 'Search complete', cancelled: 'Search cancelled; partial results are shown',
    emptyQuery: 'Enter text to search for.', tooManyTables: 'Select no more than 20 tables.', noTables: 'Select at least one table.', requestTimeout: 'Request timed out.',
    noResults: 'No matches in the searched text columns.', results: 'Matching rows', open: 'Open filtered rows', showing: 'Showing {count} of {total} matches in this column (limit 20).', errors: 'Some searches failed', noCatalog: 'No tables available.',
  } },
  ru: { searchWorkflows: {
    title: 'Поиск в базе данных', database: 'База данных', tables: 'Таблицы для поиска', text: 'Искомый текст', start: 'Найти', stop: 'Остановить',
    limits: 'Выберите не более 20 таблиц. Поиск проверяет текстовые столбцы двумя параллельными запросами и возвращает до 20 строк на столбец. Тайм-аут запроса — 15 секунд. Поиск в больших таблицах может быть затратным.',
    semantics: 'Поиск буквального текста, включая %, _ и обратную косую черту. Регистр зависит от правил сравнения базы. Массивы, двоичные, числовые и JSON-столбцы исключены.',
    selected: 'Выбрано таблиц: {count} из 20', progress: 'Обработано таблиц: {done} из {total}; проверено текстовых столбцов: {columns}', running: 'Идёт поиск…', complete: 'Поиск завершён', cancelled: 'Поиск остановлен; показаны частичные результаты',
    emptyQuery: 'Введите текст для поиска.', tooManyTables: 'Выберите не более 20 таблиц.', noTables: 'Выберите хотя бы одну таблицу.', requestTimeout: 'Истекло время ожидания запроса.',
    noResults: 'В проверенных текстовых столбцах совпадений нет.', results: 'Найденные строки', open: 'Открыть строки с фильтром', showing: 'Показано совпадений в столбце: {count} из {total} (не более 20).', errors: 'Часть запросов завершилась с ошибкой', noCatalog: 'Нет доступных таблиц.',
  } },
}
