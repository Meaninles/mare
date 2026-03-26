import { HardDrive, Upload } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useImportDevices, useSelectImportDeviceRole } from "../hooks/useImport";
import type { ImportDeviceRecord, ImportDeviceRole } from "../types/import";

export function RemovableRoleDialog() {
  const navigate = useNavigate();
  const devicesQuery = useImportDevices();
  const selectRoleMutation = useSelectImportDeviceRole();
  const [pendingIdentity, setPendingIdentity] = useState<string | null>(null);
  const knownIdentitiesRef = useRef<Set<string>>(new Set());
  const dismissedIdentitiesRef = useRef<Set<string>>(new Set());

  const devices = devicesQuery.data ?? [];
  const pendingDevice = useMemo(
    () => devices.find((device) => device.identitySignature === pendingIdentity) ?? null,
    [devices, pendingIdentity]
  );

  useEffect(() => {
    const currentIdentities = new Set(devices.map((device) => device.identitySignature));

    for (const identity of Array.from(dismissedIdentitiesRef.current)) {
      if (!currentIdentities.has(identity)) {
        dismissedIdentitiesRef.current.delete(identity);
      }
    }

    const actionableDevices = devices.filter((device) => {
      if (device.currentSessionRole) {
        return false;
      }
      return !dismissedIdentitiesRef.current.has(device.identitySignature);
    });

    if (pendingIdentity && !currentIdentities.has(pendingIdentity)) {
      setPendingIdentity(null);
    }

    if (!pendingIdentity) {
      const insertedDevice = actionableDevices.find(
        (device) => !knownIdentitiesRef.current.has(device.identitySignature)
      );
      if (insertedDevice) {
        setPendingIdentity(insertedDevice.identitySignature);
      } else if (knownIdentitiesRef.current.size === 0 && actionableDevices.length > 0) {
        setPendingIdentity(actionableDevices[0].identitySignature);
      }
    }

    knownIdentitiesRef.current = currentIdentities;
  }, [devices, pendingIdentity]);

  function closeDialog() {
    if (pendingIdentity) {
      dismissedIdentitiesRef.current.add(pendingIdentity);
    }
    setPendingIdentity(null);
  }

  async function handleSelectRole(role: ImportDeviceRole) {
    if (!pendingDevice) {
      return;
    }

    const result = await selectRoleMutation.mutateAsync({
      identitySignature: pendingDevice.identitySignature,
      role,
      name: pendingDevice.knownEndpoint?.name ?? pendingDevice.device.volumeLabel ?? "移动设备"
    });

    setPendingIdentity(null);

    if (role === "import_source") {
      navigate(`/ingest?device=${encodeURIComponent(result.device.identitySignature)}`);
      return;
    }

    navigate("/storage");
  }

  if (!pendingDevice) {
    return null;
  }

  return (
    <div className="dialog-overlay" role="presentation" onClick={closeDialog}>
      <article
        className="dialog-card removable-role-dialog"
        role="dialog"
        aria-modal="true"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="dialog-header">
          <p className="eyebrow">设备已连接</p>
          <h4>检测到新的可移动设备，需要先确认它在 Mare 中的用途。</h4>
          <p>你可以把它作为长期纳管的管理存储，也可以把它作为一次性的导入源使用。</p>
        </div>

        <div className="dialog-meta">
          <div>
            <span>设备</span>
            <strong>{pendingDevice.device.volumeLabel || "未命名设备"}</strong>
          </div>
          <div>
            <span>位置</span>
            <strong>{pendingDevice.device.mountPoint}</strong>
          </div>
          <div>
            <span>介质信息</span>
            <strong>{getDeviceMeta(pendingDevice)}</strong>
          </div>
          {pendingDevice.knownEndpoint ? (
            <div>
              <span>历史身份</span>
              <strong>
                已识别为已纳管设备“{pendingDevice.knownEndpoint.name}”，当前类型为{" "}
                {getEndpointTypeLabel(pendingDevice.knownEndpoint.endpointType)}。
              </strong>
            </div>
          ) : null}
        </div>

        <div className="role-choice-grid">
          <button
            type="button"
            className="role-choice-card"
            disabled={selectRoleMutation.isPending}
            onClick={() => void handleSelectRole("managed_storage")}
          >
            <div className="role-choice-head">
              <span className="theme-option-icon">
                <HardDrive size={18} />
              </span>
              <span className="status-pill subtle">
                {pendingDevice.knownEndpoint ? "继续纳管" : "长期使用"}
              </span>
            </div>
            <strong>作为管理存储</strong>
            <p>写入存储端点列表，后续参与扫描、同步、恢复和副本状态管理。</p>
          </button>

          <button
            type="button"
            className="role-choice-card"
            disabled={selectRoleMutation.isPending}
            onClick={() => void handleSelectRole("import_source")}
          >
            <div className="role-choice-head">
              <span className="theme-option-icon">
                <Upload size={18} />
              </span>
              <span className="status-pill subtle">临时导入</span>
            </div>
            <strong>作为导入源</strong>
            <p>进入导入中心浏览设备内容，按导入规则把媒体复制到一个或多个管理端点。</p>
          </button>
        </div>

        <div className="dialog-actions">
          <button type="button" className="ghost-button" onClick={closeDialog} disabled={selectRoleMutation.isPending}>
            稍后处理
          </button>
        </div>
      </article>
    </div>
  );
}

function getDeviceMeta(device: ImportDeviceRecord) {
  return [device.device.fileSystem, device.device.model || device.device.interfaceType]
    .filter(Boolean)
    .join(" / ") || "可移动存储";
}

function getEndpointTypeLabel(endpointType: string) {
  switch (endpointType) {
    case "LOCAL":
      return "本地";
    case "QNAP_SMB":
      return "QNAP / SMB";
    case "CLOUD_115":
      return "115 网盘";
    case "ALIST":
      return "AList 网盘";
    case "REMOVABLE":
      return "可移动设备";
    default:
      return endpointType;
  }
}
