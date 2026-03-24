export interface RegisteredLibrary {
  id: string;
  name: string;
  path: string;
  createdAt: string;
  updatedAt: string;
  lastOpenedAt?: string;
  isActive: boolean;
}

export interface LibraryTaskRecord {
  libraryId: string;
  libraryName: string;
  libraryPath: string;
  libraryIsActive: boolean;
  id: string;
  taskType: string;
  status: string;
  payload: string;
  resultSummary?: string;
  errorMessage?: string;
  retryCount: number;
  createdAt: string;
  updatedAt: string;
  startedAt?: string;
  finishedAt?: string;
  sourceEndpointId?: string;
  sourceEndpointName?: string;
  targetEndpointId?: string;
  targetEndpointName?: string;
}

export interface BackendLibrarySession {
  status: string;
  ready: boolean;
  libraryId?: string;
  path?: string;
  name?: string;
  fileExtension?: string;
  schemaFamily?: string;
  migrationVersion?: string;
  cacheRoot?: string;
  localStateRoot?: string;
  openedAt?: string;
}

export interface BackendLibrarySessionResponse {
  success: boolean;
  library?: BackendLibrarySession;
  error?: string;
}

export interface LibrarySummary {
  assetCount: number;
  replicaCount: number;
  endpointCount: number;
  importRuleCount: number;
  taskCount: number;
}

export interface LegacyCatalogStatus {
  available: boolean;
  sourcePath?: string;
  targetPath?: string;
  manifestPath?: string;
  targetExists: boolean;
  suggestedLibraryName?: string;
  sourceSummary?: LibrarySummary;
  reason?: string;
}

export interface LegacyCatalogStatusResponse {
  success: boolean;
  legacy?: LegacyCatalogStatus;
  error?: string;
}

export interface LegacyCatalogMigrationResult {
  sourcePath: string;
  targetPath: string;
  manifestPath: string;
  suggestedLibraryName: string;
  sourcePreserved: boolean;
  countsMatch: boolean;
  sourceSummary: LibrarySummary;
  targetSummary: LibrarySummary;
  migratedAt: string;
  library: BackendLibrarySession;
}

export interface LegacyCatalogMigrationResponse {
  success: boolean;
  migration?: LegacyCatalogMigrationResult;
  library?: BackendLibrarySession;
  error?: string;
}
