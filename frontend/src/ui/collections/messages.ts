import { useI18n } from 'vue-i18n'

export function useCollectionLabels() {
  return useI18n({
    useScope: 'local',
    messages: {
      en: {
        empty: 'No data', loading: 'Loading…', selectPage: 'Select all rows on this page',
        selectRow: 'Select row {number}', pagination: 'Pagination', page: 'Page {number}',
        previous: 'Previous page', next: 'Next page', rowsPerPage: 'Rows per page', size: '{size} / page',
        objects: 'Database objects',
      },
      ru: {
        empty: 'Нет данных', loading: 'Загрузка…', selectPage: 'Выбрать все строки на странице',
        selectRow: 'Выбрать строку {number}', pagination: 'Страницы', page: 'Страница {number}',
        previous: 'Предыдущая страница', next: 'Следующая страница', rowsPerPage: 'Строк на странице',
        size: '{size} / стр.', objects: 'Объекты базы данных',
      },
    },
  })
}
