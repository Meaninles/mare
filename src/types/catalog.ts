export interface CatalogEndpoint {
  id: string;
  name: string;
  endpointType: string;
  rootPath: string;
  roleMode: string;
  identitySignature: string;
  availabilityStatus: string;
  connectionConfig: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface CatalogVersion {
  id: string;
  size: number;
  mtime?: string;
  createdAt: string;
  scanRevision?: string;
}

export interface CatalogReplica {
  id: string;
  endpointId: string;
  physicalPath: string;
  replicaStatus: string;
  existsFlag: boolean;
  lastSeenAt?: string;
  version?: CatalogVersion;
}

export interface CatalogAssetPoster {
  id: string;
  kind: string;
  url: string;
  mimeType?: string;
  width?: number;
  height?: number;
  createdAt: string;
  updatedAt: string;
}

export interface CatalogAudioMetadata {
  durationSeconds?: number;
  codecName?: string;
  sampleRateHz?: number;
  channelCount?: number;
}

export interface CatalogAsset {
  id: string;
  logicalPathKey: string;
  displayName: string;
  mediaType: string;
  assetStatus: string;
  primaryTimestamp?: string;
  poster?: CatalogAssetPoster;
  previewUrl?: string;
  audioMetadata?: CatalogAudioMetadata;
  createdAt: string;
  updatedAt: string;
  availableReplicaCount?: number;
  missingReplicaCount?: number;
  replicas: CatalogReplica[];
}

export interface CatalogTask {
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
}

export interface EndpointScanSummary {
  taskId: string;
  endpointId: string;
  endpointName: string;
  endpointType: string;
  status: string;
  filesScanned: number;
  batchCount: number;
  assetsCreated: number;
  assetsUpdated: number;
  replicasCreated: number;
  replicasUpdated: number;
  missingReplicas: number;
  startedAt: string;
  finishedAt: string;
  error?: string;
}

export interface FullScanSummary {
  startedAt: string;
  finishedAt: string;
  endpointCount: number;
  successCount: number;
  failedCount: number;
  endpointSummaries: EndpointScanSummary[];
}

export interface CatalogEndpointsResponse {
  success: boolean;
  endpoints?: CatalogEndpoint[];
  endpoint?: CatalogEndpoint;
  error?: string;
}

export interface CatalogAssetsResponse {
  success: boolean;
  assets?: CatalogAsset[];
  error?: string;
}

export interface CatalogTasksResponse {
  success: boolean;
  tasks?: CatalogTask[];
  error?: string;
}

export interface CatalogScanResponse {
  success: boolean;
  summary?: FullScanSummary | EndpointScanSummary;
  error?: string;
}
