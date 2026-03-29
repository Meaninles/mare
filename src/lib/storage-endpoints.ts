export function normalizeCatalogEndpointType(endpointType: string): string {
  switch (endpointType.trim().toUpperCase()) {
    case "LOCAL":
      return "LOCAL";
    case "QNAP":
    case "QNAP_SMB":
      return "QNAP_SMB";
    case "NETWORK":
    case "NETWORK_STORAGE":
      return "NETWORK_STORAGE";
    case "CD2":
    case "CLOUDDRIVE2":
      return "CD2";
    case "REMOVABLE":
    case "REMOVABLE_DRIVE":
      return "REMOVABLE";
    default:
      return endpointType.trim().toUpperCase();
  }
}

export function getCatalogEndpointTypeLabel(endpointType: string): string {
  switch (normalizeCatalogEndpointType(endpointType)) {
    case "LOCAL":
      return "\u672c\u5730";
    case "QNAP_SMB":
      return "QNAP / SMB";
    case "NETWORK_STORAGE":
      return "\u7f51\u7edc\u5b58\u50a8";
    case "CD2":
      return "CD2 云盘目录";
    case "REMOVABLE":
      return "\u53ef\u79fb\u52a8\u8bbe\u5907";
    default:
      return endpointType || "\u672a\u77e5\u7c7b\u578b";
  }
}

export function getStorageRecoveryHint(endpointType: string): string {
  switch (normalizeCatalogEndpointType(endpointType)) {
    case "LOCAL":
      return "\u8bf7\u786e\u8ba4\u5f53\u524d\u673a\u5668\u4e0a\u7684\u672c\u5730\u6839\u8def\u5f84\u662f\u5426\u6b63\u786e\u3002";
    case "QNAP_SMB":
      return "\u8bf7\u786e\u8ba4 SMB \u5171\u4eab\u8def\u5f84\u548c NAS \u53ef\u7528\u6027\u3002";
    case "NETWORK_STORAGE":
      return "\u8bf7\u68c0\u67e5\u7f51\u7edc\u5b58\u50a8\u7684\u767b\u5f55\u72b6\u6001\u3001\u6839\u76ee\u5f55 ID \u4ee5\u53ca\u672c\u673a\u4fdd\u5b58\u7684 115 \u51ed\u8bc1\u662f\u5426\u6709\u6548\u3002";
    case "CD2":
      return "\u8bf7\u68c0\u67e5 CD2 \u8ba4\u8bc1\u72b6\u6001\u3001\u5df2\u63a5\u5165\u4e91\u8d26\u53f7\u4ee5\u53ca\u6240\u9009\u76ee\u5f55\u662f\u5426\u4ecd\u7136\u5b58\u5728\u3002";
    case "REMOVABLE":
      return "\u8bf7\u91cd\u65b0\u63a5\u5165\u540c\u4e00\u5757\u8bbe\u5907\uff0c\u4ee5\u4fbf\u518d\u6b21\u5339\u914d\u8eab\u4efd\u3002";
    default:
      return "\u6821\u9a8c\u524d\u8bf7\u5148\u68c0\u67e5\u8fd9\u4e2a\u7aef\u70b9\u3002";
  }
}

export function getRestoreSourcePriority(endpointType?: string): number {
  switch (normalizeCatalogEndpointType(endpointType ?? "")) {
    case "LOCAL":
      return 0;
    case "REMOVABLE":
      return 1;
    case "QNAP_SMB":
      return 2;
    case "CD2":
      return 3;
    case "NETWORK_STORAGE":
      return 4;
    default:
      return 9;
  }
}
