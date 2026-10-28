export const foreignKeyMessages = {
 en: { foreignKeys: {
  title: 'Foreign keys', add: 'Add foreign key', name: 'Constraint name', schema: 'Referenced database / schema', table: 'Referenced table', local: 'Local column', remote: 'Referenced column', addMapping: 'Add column mapping', remove: 'Remove', onDelete: 'On delete', onUpdate: 'On update', save: 'Save foreign keys', saved: 'Foreign keys saved.', loadColumns: 'Load referenced columns', defaultAction: 'Server default', empty: 'No foreign keys.', invalid: 'Enter a name, referenced table, and complete unique column mappings for each foreign key.',
  hint: 'Column pairs are matched in the displayed order. PostgreSQL saves changes in a transaction. MySQL commits each DDL step and attempts to restore a constraint when replacement fails; earlier successful steps may remain applied.',
 } },
 ru: { foreignKeys: {
  title: 'Внешние ключи', add: 'Добавить внешний ключ', name: 'Имя ограничения', schema: 'База данных / схема родительской таблицы', table: 'Родительская таблица', local: 'Локальная колонка', remote: 'Колонка родительской таблицы', addMapping: 'Добавить пару колонок', remove: 'Удалить', onDelete: 'При удалении', onUpdate: 'При обновлении', save: 'Сохранить внешние ключи', saved: 'Внешние ключи сохранены.', loadColumns: 'Загрузить колонки родительской таблицы', defaultAction: 'Действие сервера по умолчанию', empty: 'Внешних ключей нет.', invalid: 'Укажите имя, родительскую таблицу и уникальные пары колонок для каждого ключа.',
  hint: 'Колонки сопоставляются в указанном порядке. PostgreSQL сохраняет изменения в транзакции. MySQL фиксирует каждый шаг DDL. При ошибке замены приложение пытается восстановить ограничение; предыдущие успешные шаги могут сохраниться.',
 } },
}
