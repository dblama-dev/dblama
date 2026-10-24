export default {
  app: {
    title: 'db-manager',
  },
  login: {
    title: 'Sign in',
    driver: 'DBMS',
    host: 'Host',
    port: 'Port',
    user: 'User',
    password: 'Password',
    database: 'Database',
    databaseOptional: 'optional',
    submit: 'Connect',
    failed: 'Connection failed',
  },
  server: {
    title: 'Server overview',
    refresh: 'Refresh',
    loadFailed: 'Failed to load the database list',
    columns: {
      name: 'Name',
      size: 'Size',
      collation: 'Collation',
    },
  },
  topbar: {
    theme: 'Theme',
    logout: 'Sign out',
  },
  sidebar: {
    placeholder: 'Object tree arrives in P2',
  },
  tree: {
    selectDb: 'Database',
    search: 'Search',
    tables: 'Tables',
    views: 'Views',
    selectTable: 'Select a table',
    loadFailed: 'Failed to load the table list',
  },
  table: {
    data: 'Data',
    structure: 'Structure',
  },
}
