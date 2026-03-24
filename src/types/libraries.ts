export interface RegisteredLibrary {
  id: string;
  name: string;
  path: string;
  createdAt: string;
  updatedAt: string;
  lastOpenedAt?: string;
  isActive: boolean;
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
