export type CD2AuthMode = "password" | "api_token";

export interface CD2SystemInfo {
  isLogin: boolean;
  userName?: string;
  systemReady: boolean;
  systemMessage?: string;
  hasError: boolean;
}

export interface CD2ClientState {
  enabled: boolean;
  target: string;
  useTls: boolean;
  connected: boolean;
  publicReady: boolean;
  authReady: boolean;
  ready: boolean;
  authBootstrapConfigured: boolean;
  activeAuthMode: string;
  tokenSource?: string;
  tokenRef?: string;
  tokenFriendlyName?: string;
  protoVersion?: string;
  protoDescriptorVersion?: string;
  protoSourceSha256?: string;
  protoSourceUrl?: string;
  serviceName?: string;
  systemInfo: CD2SystemInfo;
  lastCheckedAt?: string;
  lastError?: string;
}

export interface CD2AuthProfile {
  mode: CD2AuthMode;
  serverAddress: string;
  userName?: string;
  tokenRef?: string;
  tokenExpiresAt?: string;
  lastVerifiedAt?: string;
  managedTokenFriendlyName?: string;
  managedTokenRootDir?: string;
  updatedAt: string;
}

export interface CD2AuthStatus {
  configured: boolean;
  profile?: CD2AuthProfile;
  client: CD2ClientState;
}

export interface CD2AuthStatusResponse {
  success: boolean;
  auth?: CD2AuthStatus;
  error?: string;
}

export interface CD2RegisterResult {
  serverAddress: string;
  userName: string;
  success: boolean;
  errorMessage?: string;
  resultFilePaths?: string[];
}

export interface CD2RegisterResponse {
  success: boolean;
  result?: CD2RegisterResult;
  error?: string;
}

export interface UpdateCD2AuthProfilePayload {
  mode: CD2AuthMode;
  serverAddress?: string;
  userName?: string;
  password?: string;
  apiToken?: string;
  managedTokenFriendlyName?: string;
  managedTokenRootDir?: string;
}

export interface RegisterCD2AccountPayload {
  serverAddress?: string;
  userName: string;
  password: string;
}

export interface CD2CloudAccount {
  cloudName: string;
  userName: string;
  nickName?: string;
  displayName: string;
  path?: string;
  isLocked: boolean;
  supportMultiThreadUploading: boolean;
  supportQpsLimit: boolean;
  isCloudEventListenerRunning: boolean;
  hasPromotions: boolean;
  promotionTitle?: string;
  supportHttpDownload: boolean;
}

export interface CD2CloudAccountsResponse {
  success: boolean;
  accounts?: CD2CloudAccount[];
  error?: string;
}

export interface Import115CookiePayload {
  editThisCookie: string;
}

export interface Import115CookieResult {
  success: boolean;
  message?: string;
  accounts?: CD2CloudAccount[];
}

export interface Import115CookieResponse {
  success: boolean;
  result?: Import115CookieResult;
  error?: string;
}

export interface Start115QRCodePayload {
  platform?: string;
}

export interface CD2115QRCodeSession {
  id: string;
  provider: string;
  platform?: string;
  status: string;
  qrCodeImage?: string;
  qrCodeContent?: string;
  lastMessage?: string;
  error?: string;
  addedAccounts?: CD2CloudAccount[];
  startedAt: string;
  updatedAt: string;
  finishedAt?: string;
}

export interface CD2115QRCodeSessionResponse {
  success: boolean;
  session?: CD2115QRCodeSession;
  error?: string;
}

export interface RemoveCD2CloudAccountPayload {
  cloudName: string;
  userName: string;
  permanent?: boolean;
}

export interface RemoveCD2CloudAccountResponse {
  success: boolean;
  error?: string;
}

export interface CD2FileDetailProperties {
  totalFileCount: number;
  totalFolderCount: number;
  totalSize: number;
  isFaved: boolean;
  isShared: boolean;
  originalPath?: string;
}

export interface CD2DownloadURLInfo {
  downloadUrlPath: string;
  expiresIn?: number;
  directUrl?: string;
  userAgent?: string;
  additionalHeaders?: Record<string, string>;
}

export interface CD2FileEntry {
  id: string;
  name: string;
  fullPathName: string;
  size: number;
  fileType: string;
  createTime?: string;
  writeTime?: string;
  accessTime?: string;
  cloudName?: string;
  cloudUserName?: string;
  cloudNickName?: string;
  thumbnailUrl?: string;
  previewUrl?: string;
  originalPath?: string;
  isDirectory: boolean;
  isRoot: boolean;
  isCloudRoot: boolean;
  isCloudDirectory: boolean;
  isCloudFile: boolean;
  isSearchResult: boolean;
  isForbidden: boolean;
  isLocal: boolean;
  canSearch: boolean;
  hasDetailProperties: boolean;
  canContentSearch: boolean;
  canDeletePermanently: boolean;
  detailProperties?: CD2FileDetailProperties;
  downloadUrlPath?: CD2DownloadURLInfo;
}

export interface CD2FileListResponse {
  success: boolean;
  currentPath?: string;
  entries?: CD2FileEntry[];
  error?: string;
}

export interface CD2FileStatResponse {
  success: boolean;
  entry?: CD2FileEntry;
  error?: string;
}

export interface CD2FileDetailResponse {
  success: boolean;
  detail?: CD2FileDetailProperties;
  error?: string;
}

export interface CD2FileOperationResult {
  success: boolean;
  errorMessage?: string;
  resultFilePaths?: string[];
}

export interface CD2CreateFolderPayload {
  parentPath: string;
  folderName: string;
}

export interface CD2CreateFolderResponse {
  success: boolean;
  entry?: CD2FileEntry;
  result?: CD2FileOperationResult;
  error?: string;
}

export interface CD2RenameFilePayload {
  path: string;
  newName: string;
}

export interface CD2MoveFilesPayload {
  paths: string[];
  destPath: string;
  conflictPolicy?: "overwrite" | "rename" | "skip";
  moveAcrossClouds?: boolean;
  handleConflictRecursively?: boolean;
}

export interface CD2CopyFilesPayload {
  paths: string[];
  destPath: string;
  conflictPolicy?: "overwrite" | "rename" | "skip";
  handleConflictRecursively?: boolean;
}

export interface CD2DeleteFilesPayload {
  paths: string[];
  permanent?: boolean;
}

export interface CD2FileOperationResponse {
  success: boolean;
  result?: CD2FileOperationResult;
  error?: string;
}

export interface CD2DownloadURLQuery {
  path: string;
  preview?: boolean;
  lazyRead?: boolean;
  getDirect?: boolean;
}

export interface CD2DownloadURLResponse {
  success: boolean;
  info?: CD2DownloadURLInfo;
  error?: string;
}

export interface CD2UploadItem {
  fileName: string;
  parentPath: string;
  fullPathName: string;
  bytesWritten: number;
  entry?: CD2FileEntry;
}

export interface CD2UploadFilesResponse {
  success: boolean;
  parentPath?: string;
  uploaded?: CD2UploadItem[];
  error?: string;
}

export interface CD2TransferStats {
  generatedAt: string;
  totalTasks: number;
  uploadTasks: number;
  downloadTasks: number;
  copyTasks: number;
  queuedTasks: number;
  runningTasks: number;
  pausedTasks: number;
  failedTasks: number;
  successTasks: number;
  canceledTasks: number;
  skippedTasks: number;
  totalBytes: number;
  finishedBytes: number;
  bytesPerSecond: number;
}

export interface CD2TransferCount {
  uploadCount: number;
  downloadCount: number;
  copyCount: number;
}

export interface CD2TransferTask {
  key: string;
  kind: "upload" | "download" | "copy" | string;
  title: string;
  status: string;
  progressPercent: number;
  sourcePath?: string;
  targetPath?: string;
  filePath?: string;
  operatorType?: string;
  rawStatus?: string;
  paused: boolean;
  canPause: boolean;
  canResume: boolean;
  canCancel: boolean;
  totalBytes: number;
  finishedBytes: number;
  bytesPerSecond: number;
  bufferUsedBytes?: number;
  threadCount?: number;
  totalFiles?: number;
  finishedFiles?: number;
  failedFiles?: number;
  canceledFiles?: number;
  skippedFiles?: number;
  errorMessage?: string;
  detail?: string;
  startedAt?: string;
  finishedAt?: string;
  lastObservedAt: string;
  controlReference?: string;
}

export interface CD2TransferEvent {
  id: string;
  stream: string;
  eventType: string;
  summary?: string;
  message?: string;
  occurredAt: string;
  level?: string;
  target?: string;
  relatedKey?: string;
  counts?: CD2TransferCount;
}

export interface CD2TransferWatcherState {
  started: boolean;
  pushTaskChangeActive: boolean;
  pushMessageActive: boolean;
  lastConnectedAt?: string;
  lastEventAt?: string;
  lastError?: string;
  eventCount: number;
  observedTransferCount: CD2TransferCount;
}

export interface CD2TransfersResult {
  generatedAt: string;
  stats: CD2TransferStats;
  tasks: CD2TransferTask[];
  recentEvents: CD2TransferEvent[];
  watcher: CD2TransferWatcherState;
}

export interface CD2TransfersResponse {
  success: boolean;
  result?: CD2TransfersResult;
  error?: string;
}

export interface CD2TransferActionPayload {
  kind: "upload" | "download" | "copy";
  action: "pause" | "resume" | "cancel";
  keys?: string[];
  sourcePath?: string;
  destPath?: string;
  all?: boolean;
}

export interface CD2TransferActionSummary {
  kind: string;
  action: string;
  requested: number;
  updated: number;
  keys?: string[];
  message: string;
}

export interface CD2TransferActionResponse {
  success: boolean;
  summary?: CD2TransferActionSummary;
  error?: string;
}
