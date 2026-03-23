export interface ConnectorCapabilities {
  canRead: boolean;
  canWrite: boolean;
  canDelete: boolean;
  canList: boolean;
  canStat: boolean;
  canReadStream: boolean;
  canChangeNotify: boolean;
  canRename?: boolean;
  canMove?: boolean;
  canMakeDirectory?: boolean;
}

export interface ConnectorDescriptor {
  name: string;
  type: string;
  rootPath: string;
  capabilities: ConnectorCapabilities;
}

export interface FileEntry {
  path: string;
  relativePath: string;
  name: string;
  kind: "file" | "directory";
  mediaType: string;
  size: number;
  modifiedAt?: string;
  isDir: boolean;
}

export interface DeviceInfo {
  mountPoint: string;
  volumeLabel: string;
  fileSystem: string;
  volumeSerialNumber: string;
  driveType: number;
  interfaceType: string;
  model: string;
  pnpDeviceId: string;
}

export interface ConnectorTestResponse {
  success: boolean;
  connector: string;
  operation: string;
  healthStatus?: string;
  descriptor?: ConnectorDescriptor;
  entry?: FileEntry;
  entries?: FileEntry[];
  content?: string;
  qrCodeSession?: Cloud115QRCodeSession;
  error?: string;
}

export interface Cloud115QRCodeSession {
  uid: string;
  time: number;
  sign: string;
  appType: string;
  qrCodeUrl: string;
  status: string;
  statusCode: number;
  credential?: string;
}

export interface RemovableDevicesResponse {
  success: boolean;
  devices: DeviceInfo[];
  error?: string;
}
