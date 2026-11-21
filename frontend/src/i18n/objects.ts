export const objectMessages = {
 en: { objects: {
  views: 'Views', triggers: 'Triggers', routines: 'Procedures and functions', events: 'Events', partitions: 'Partitions',
  unsupported: 'This object type is not supported by the connected server.', refresh: 'Refresh', create: 'New object', search: 'Filter objects',
  name: 'Name', kind: 'Type', table: 'Table', identity: 'Signature', source: 'SQL source', save: 'Save SQL source', remove: 'Delete', cancel: 'Cancel', confirm: 'Confirm',
  sourceHint: 'SQL source runs with your database account privileges, like the SQL console. Use a full CREATE statement. MySQL routines, triggers and events are recreated; if creation fails, the previous source is restored when possible.',
  empty: 'No objects found.', select: 'Select an object or create one.', parameters: 'Procedure arguments (JSON array)', parameterHint: 'Pass values as a JSON array, including null for OUT parameters. OUT and INOUT values appear in the results.',
  call: 'Call procedure', invalidArgs: 'Enter a JSON array of strings, numbers, booleans or null.', saved: 'Saved.', deleted: 'Deleted.',
  deleteConfirm: 'Delete this object? This operation cannot be undone.', partitionConfirm: 'This operation removes partition data. Continue?',
  addPartition: 'Add partition', partitionSource: 'Partition definition (SQL)', partitionHint: 'Example: PARTITION p2027 VALUES LESS THAN (2028). The table must already use a compatible partitioning scheme.',
  method: 'Method', expression: 'Expression', boundary: 'Boundary', rows: 'Estimated rows', truncate: 'Truncate', selectTable: 'Select a table',
 } },
 ru: { objects: {
  views: 'Представления', triggers: 'Триггеры', routines: 'Процедуры и функции', events: 'События', partitions: 'Партиции',
  unsupported: 'Подключённый сервер не поддерживает этот тип объектов.', refresh: 'Обновить', create: 'Новый объект', search: 'Фильтр объектов',
  name: 'Имя', kind: 'Тип', table: 'Таблица', identity: 'Сигнатура', source: 'Исходный SQL', save: 'Сохранить SQL', remove: 'Удалить', cancel: 'Отмена', confirm: 'Подтвердить',
  sourceHint: 'SQL выполняется с правами вашей учётной записи, как в SQL-консоли. Введите полную команду CREATE. Процедуры, триггеры и события MySQL пересоздаются. При ошибке приложение пытается восстановить предыдущий исходник.',
  empty: 'Объекты не найдены.', select: 'Выберите объект или создайте новый.', parameters: 'Аргументы процедуры (массив JSON)', parameterHint: 'Введите значения как массив JSON. Для OUT-параметров используйте null. Значения OUT и INOUT появятся в результатах.',
  call: 'Вызвать процедуру', invalidArgs: 'Введите массив JSON: строки, числа, логические значения или null.', saved: 'Сохранено.', deleted: 'Удалено.',
  deleteConfirm: 'Удалить объект? Это действие нельзя отменить.', partitionConfirm: 'Операция удалит данные партиции. Продолжить?',
  addPartition: 'Добавить партицию', partitionSource: 'Определение партиции (SQL)', partitionHint: 'Пример: PARTITION p2027 VALUES LESS THAN (2028). Таблица должна использовать совместимую схему партиционирования.',
  method: 'Метод', expression: 'Выражение', boundary: 'Граница', rows: 'Примерное число строк', truncate: 'Очистить', selectTable: 'Выберите таблицу',
 } },
}
