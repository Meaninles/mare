export interface ModuleStatus {
  name: string;
  ready: boolean;
}

export interface DatabaseStatus {
  ready: boolean;
  path: string;
  migrationVersion: string;
}

export interface AppBootstrap {
  appName: string;
  database: DatabaseStatus;
  modules: ModuleStatus[];
}
