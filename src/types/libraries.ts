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
