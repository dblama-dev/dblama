export default {
  app: {
    title: 'db-manager',
  },
  login: {
    title: 'Вход',
    driver: 'СУБД',
    host: 'Хост',
    port: 'Порт',
    user: 'Пользователь',
    password: 'Пароль',
    database: 'База данных',
    databaseOptional: 'необязательно',
    submit: 'Подключиться',
    failed: 'Не удалось подключиться',
  },
  server: {
    title: 'Обзор сервера',
    refresh: 'Обновить',
    loadFailed: 'Не удалось загрузить список баз данных',
    columns: {
      name: 'Имя',
      size: 'Размер',
      collation: 'Сопоставление',
    },
  },
  topbar: {
    theme: 'Тема',
    logout: 'Выйти',
  },
  sidebar: {
    placeholder: 'Дерево объектов появится в П2',
  },
  tree: {
    selectDb: 'База данных',
    search: 'Поиск',
    tables: 'Таблицы',
    views: 'Views',
    selectTable: 'Выберите таблицу',
    loadFailed: 'Не удалось загрузить список таблиц',
  },
  table: {
    data: 'Данные',
    structure: 'Структура',
  },
}
