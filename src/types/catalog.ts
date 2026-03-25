export interface CatalogEndpoint {
  id: string;
  name: string;
  note: string;
  endpointType: string;
  rootPath: string;
  roleMode: string;
  identitySignature: string;
  availabilityStatus: string;
  credentialRef?: string;
  credentialHint?: string;
  hasCredential?: boolean;
  connectionConfig: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface CatalogEndpointPayload {
  name: string;
  note: string;
  endpointType: string;
  rootPath: string;
  roleMode: string;
  availabilityStatus: string;
  credentialRef?: string;
  connectionConfig: Record<string, unknown>;
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
  relativePath?: string;
  logicalDirectory?: string;
  resolvedDirectory?: string;
  matchesLogicalPath?: boolean;
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
  canonicalPath?: string;
  canonicalDirectory?: string;
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

export interface CatalogAssetTranscriptInsight {
  text: string;
  language?: string;
  length: number;
  updatedAt: string;
}

export interface CatalogSemanticLabel {
  label: string;
  score: number;
}

export interface CatalogAssetSemanticInsight {
  featureKind: string;
  modelName: string;
  dimensions: number;
  labels: CatalogSemanticLabel[];
  updatedAt: string;
}

export interface CatalogAssetInsights {
  transcript?: CatalogAssetTranscriptInsight;
  semantic?: CatalogAssetSemanticInsight;
  warnings?: string[];
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

export interface CatalogTransferTaskStats {
  generatedAt: string;
  totalTasks: number;
  queuedTasks: number;
  runningTasks: number;
  pausedTasks: number;
  failedTasks: number;
  successTasks: number;
  uploadTasks: number;
  downloadTasks: number;
  syncTasks: number;
  totalItems: number;
  runningItems: number;
  pausedItems: number;
  failedItems: number;
  successItems: number;
  skippedItems: number;
  totalBytes: number;
  completedBytes: number;
}

export interface CatalogTransferTaskRecord {
  id: string;
  taskType: string;
  title: string;
  direction: string;
  status: string;
  sourceLabel?: string;
  targetLabel?: string;
  progressPercent: number;
  progressLabel?: string;
  currentItemName?: string;
  totalItems: number;
  queuedItems: number;
  runningItems: number;
  pausedItems: number;
  failedItems: number;
  successItems: number;
  skippedItems: number;
  totalBytes: number;
  completedBytes: number;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface CatalogTransferTaskItemRecord {
  id: string;
  taskId: string;
  itemIndex: number;
  groupKey: string;
  direction: string;
  displayName: string;
  mediaType: string;
  sourceLabel?: string;
  sourcePath: string;
  targetLabel?: string;
  targetPath: string;
  assetId?: string;
  logicalPathKey?: string;
  status: string;
  phase: string;
  progressPercent: number;
  totalBytes: number;
  transferredBytes: number;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface CatalogTransferTaskListResult {
  generatedAt: string;
  stats: CatalogTransferTaskStats;
  tasks: CatalogTransferTaskRecord[];
}

export interface CatalogTransferTaskDetailRecord {
  task: CatalogTransferTaskRecord;
  items: CatalogTransferTaskItemRecord[];
}

export interface CatalogTransferTaskActionSummary {
  requested: number;
  updated: number;
  taskIds: string[];
  message: string;
}

export interface CatalogDeleteReplicaSummary {
  taskId: string;
  assetId: string;
  displayName: string;
  targetEndpointId: string;
  targetEndpointName: string;
  targetPhysicalPath: string;
  status: string;
  replicaDeleted: boolean;
  assetRemoved: boolean;
  remainingAvailableCopies: number;
  assetStatus: string;
  startedAt: string;
  finishedAt: string;
  error?: string;
}

export interface CatalogSyncEndpointRef {
  id: string;
  name: string;
  endpointType: string;
}

export interface CatalogSyncReplica {
  id: string;
  endpointId: string;
  endpointName: string;
  physicalPath: string;
  relativePath?: string;
  logicalDirectory?: string;
  resolvedDirectory?: string;
  matchesLogicalPath?: boolean;
  replicaStatus: string;
  existsFlag: boolean;
  lastSeenAt?: string;
  version?: CatalogVersion;
}

export interface CatalogSyncAsset {
  id: string;
  displayName: string;
  logicalPathKey: string;
  canonicalPath?: string;
  canonicalDirectory?: string;
  mediaType: string;
  assetStatus: string;
  primaryTimestamp?: string;
  poster?: CatalogAssetPoster;
  availableReplicaCount: number;
  missingReplicaCount: number;
  missingEndpoints: CatalogSyncEndpointRef[];
  consistentEndpoints: CatalogSyncEndpointRef[];
  updatedEndpoints: CatalogSyncEndpointRef[];
  conflictEndpoints: CatalogSyncEndpointRef[];
  recommendedSource?: CatalogSyncEndpointRef;
  replicas: CatalogSyncReplica[];
}

export interface CatalogSyncOverview {
  generatedAt: string;
  recoverableAssets: CatalogSyncAsset[];
  conflictAssets: CatalogSyncAsset[];
  runningTasks: CatalogTask[];
  failedTasks: CatalogTask[];
}

export interface CatalogRestoreAssetSummary {
  taskId: string;
  assetId: string;
  displayName: string;
  sourceEndpointId: string;
  sourceEndpointName: string;
  targetEndpointId: string;
  targetEndpointName: string;
  targetPhysicalPath?: string;
  status: string;
  createdReplica: boolean;
  updatedReplica: boolean;
  skipped: boolean;
  progressPercent?: number;
  progressLabel?: string;
  startedAt: string;
  finishedAt: string;
  error?: string;
}

export interface CatalogBatchRestoreItem {
  assetId: string;
  displayName: string;
  sourceEndpointId?: string;
  targetEndpointId: string;
  status: string;
  skipped: boolean;
  error?: string;
}

export interface CatalogBatchRestoreSummary {
  taskId: string;
  targetEndpointId: string;
  targetEndpointName: string;
  status: string;
  totalAssets: number;
  successCount: number;
  failedCount: number;
  skippedCount: number;
  progressPercent?: number;
  progressLabel?: string;
  items: CatalogBatchRestoreItem[];
  startedAt: string;
  finishedAt: string;
  error?: string;
}

export interface CatalogRetrySyncTaskSummary {
  originalTaskId: string;
  newTaskId?: string;
  taskType: string;
  status: string;
  message: string;
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

export interface CatalogDeleteEndpointSummary {
  endpointId: string;
  endpointName: string;
  endpointType: string;
  removedReplicaCount: number;
  affectedAssetCount: number;
  deletedAssetCount: number;
  updatedImportRuleCount: number;
  deletedAt: string;
}

export interface CatalogDeleteEndpointResponse {
  success: boolean;
  summary?: CatalogDeleteEndpointSummary;
  error?: string;
}

export interface CatalogAssetQueryOptions {
  limit?: number;
  query?: string;
  mediaType?: string;
  assetStatus?: string;
}

export interface CatalogAssetsResponse {
  success: boolean;
  assets?: CatalogAsset[];
  error?: string;
}

export interface CatalogAssetInsightsResponse {
  success: boolean;
  insights?: CatalogAssetInsights;
  error?: string;
}

export interface CatalogTasksResponse {
  success: boolean;
  tasks?: CatalogTask[];
  error?: string;
}

export interface CatalogTransferTasksResponse {
  success: boolean;
  result?: CatalogTransferTaskListResult;
  error?: string;
}

export interface CatalogTransferTaskDetailResponse {
  success: boolean;
  result?: CatalogTransferTaskDetailRecord;
  error?: string;
}

export interface CatalogTransferTaskActionResponse {
  success: boolean;
  summary?: CatalogTransferTaskActionSummary;
  error?: string;
}

export interface CatalogScanResponse {
  success: boolean;
  summary?: FullScanSummary | EndpointScanSummary;
  error?: string;
}

export interface CatalogSyncOverviewResponse {
  success: boolean;
  overview?: CatalogSyncOverview;
  error?: string;
}

export interface CatalogRestoreResponse {
  success: boolean;
  summary?: CatalogRestoreAssetSummary | CatalogBatchRestoreSummary;
  error?: string;
}

export interface CatalogRetryResponse {
  success: boolean;
  summary?: CatalogRetrySyncTaskSummary;
  error?: string;
}

export interface CatalogRestoreAssetResponse {
  success: boolean;
  summary?: CatalogRestoreAssetSummary;
  error?: string;
}

export interface CatalogBatchRestoreResponse {
  success: boolean;
  summary?: CatalogBatchRestoreSummary;
  error?: string;
}

export interface CatalogDeleteReplicaResponse {
  success: boolean;
  summary?: CatalogDeleteReplicaSummary;
  error?: string;
}
