import { useEffect, useMemo, useState } from "react";
import {
  Copy,
  Download,
  FilePenLine,
  FolderPlus,
  LoaderCircle,
  RefreshCcw,
  Search,
  Trash2,
  Upload
} from "lucide-react";
import {
  useCD2DownloadURL,
  useCD2FileDetail,
  useCopyCD2Files,
  useCreateCD2Folder,
  useDeleteCD2Files,
  useListCD2Files,
  useMoveCD2Files,
  useRenameCD2File,
  useSearchCD2Files,
  useStatCD2File,
  useUploadCD2Files
} from "../../hooks/useCD2";
import { formatCatalogDate } from "../../lib/catalog-view";
import type {
  CD2CloudAccount,
  CD2DownloadURLInfo,
  CD2FileDetailProperties,
  CD2FileEntry,
  CD2UploadItem
} from "../../types/cd2";

type Props = {
  accounts: CD2CloudAccount[];
};

export function CD2FileOpsTestPanel({ accounts }: Props) {
  const listMutation = useListCD2Files();
  const searchMutation = useSearchCD2Files();
  const statMutation = useStatCD2File();
  const detailMutation = useCD2FileDetail();
  const downloadMutation = useCD2DownloadURL();
  const createFolderMutation = useCreateCD2Folder();
  const renameMutation = useRenameCD2File();
  const moveMutation = useMoveCD2Files();
  const copyMutation = useCopyCD2Files();
  const deleteMutation = useDeleteCD2Files();
  const uploadMutation = useUploadCD2Files();

  const accountPaths = useMemo(
    () =>
      accounts
        .map((item) => item.path?.trim())
        .filter((value): value is string => Boolean(value)),
    [accounts]
  );
  const preferredPath = accountPaths[0] ?? "/115open";

  const [currentPath, setCurrentPath] = useState(preferredPath);
  const [entries, setEntries] = useState<CD2FileEntry[]>([]);
  const [isSearchMode, setIsSearchMode] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState("");
  const [selectedPath, setSelectedPath] = useState("");
  const [selectedEntry, setSelectedEntry] = useState<CD2FileEntry | null>(null);
  const [detail, setDetail] = useState<CD2FileDetailProperties | null>(null);
  const [downloadInfo, setDownloadInfo] = useState<CD2DownloadURLInfo | null>(null);
  const [uploadResults, setUploadResults] = useState<CD2UploadItem[]>([]);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [createFolderName, setCreateFolderName] = useState("");
  const [renameName, setRenameName] = useState("");
  const [targetPath, setTargetPath] = useState(preferredPath);
  const [uploadTargetPath, setUploadTargetPath] = useState(preferredPath);
  const [uploadFiles, setUploadFiles] = useState<File[]>([]);
  const [conflictPolicy, setConflictPolicy] = useState<"overwrite" | "rename" | "skip">("rename");

  useEffect(() => {
    if (!currentPath.trim()) {
      setCurrentPath(preferredPath);
    }
    if (!targetPath.trim()) {
      setTargetPath(preferredPath);
    }
    if (!uploadTargetPath.trim()) {
      setUploadTargetPath(preferredPath);
    }
  }, [currentPath, preferredPath, targetPath, uploadTargetPath]);

  useEffect(() => {
    if (!selectedEntry) {
      return;
    }
    setSelectedPath(selectedEntry.fullPathName);
    setRenameName(selectedEntry.name);
    if (selectedEntry.isDirectory) {
      setUploadTargetPath(selectedEntry.fullPathName);
    }
  }, [selectedEntry]);

  async function handleList(path = currentPath) {
    setNotice(null);
    setError(null);
    setDownloadInfo(null);
    try {
      const response = await listMutation.mutateAsync({ path });
      const resolvedPath = response.currentPath || path;
      setCurrentPath(resolvedPath);
      setEntries(response.entries ?? []);
      setIsSearchMode(false);
      setUploadTargetPath((previous) => (previous.trim() ? previous : resolvedPath));
      setNotice(`已读取 ${resolvedPath} 下的 ${response.entries?.length ?? 0} 个项目。`);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "读取文件列表失败。");
    }
  }

  async function handleSearch() {
    setNotice(null);
    setError(null);
    setDownloadInfo(null);
    try {
      const response = await searchMutation.mutateAsync({
        path: currentPath,
        query: searchKeyword,
        fuzzyMatch: true
      });
      setEntries(response.entries ?? []);
      setIsSearchMode(true);
      setNotice(`搜索完成，共命中 ${response.entries?.length ?? 0} 个项目。`);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "搜索失败。");
    }
  }

  async function handleSelectPath(path: string) {
    setNotice(null);
    setError(null);
    setDownloadInfo(null);
    try {
      const entry = await statMutation.mutateAsync(path);
      setSelectedEntry(entry);
      setSelectedPath(entry.fullPathName);
      setRenameName(entry.name);
      setNotice(`已选中 ${entry.fullPathName}。`);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "读取文件信息失败。");
    }
  }

  async function handleLoadDetail() {
    if (!selectedPath.trim()) {
      setError("请先选择一个文件或目录。");
      return;
    }
    setNotice(null);
    setError(null);
    try {
      const result = await detailMutation.mutateAsync({ path: selectedPath });
      setDetail(result);
      setNotice("文件详情已刷新。");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "读取详情失败。");
    }
  }

  async function handleDownloadURL(path = selectedPath) {
    if (!path.trim()) {
      setError("请先选择一个文件或目录。");
      return;
    }
    setNotice(null);
    setError(null);
    try {
      const result = await downloadMutation.mutateAsync({
        path,
        getDirect: true
      });
      setDownloadInfo(result);
      if (path !== selectedPath) {
        await handleSelectPath(path);
      }
      setNotice("下载链接已获取。");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "获取下载链接失败。");
    }
  }

  async function handleCreateFolder() {
    setNotice(null);
    setError(null);
    try {
      const result = await createFolderMutation.mutateAsync({
        parentPath: currentPath,
        folderName: createFolderName
      });
      setCreateFolderName("");
      setSelectedEntry(result.entry ?? null);
      setSelectedPath(result.entry?.fullPathName ?? "");
      setRenameName(result.entry?.name ?? "");
      setNotice(`目录已创建：${result.entry?.fullPathName ?? currentPath}`);
      await handleList(currentPath);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "创建目录失败。");
    }
  }

  async function handleRename() {
    if (!selectedPath.trim()) {
      setError("请先选择要重命名的文件或目录。");
      return;
    }
    setNotice(null);
    setError(null);
    try {
      const result = await renameMutation.mutateAsync({
        path: selectedPath,
        newName: renameName
      });
      const nextPath = result.resultFilePaths?.[0] ?? selectedPath;
      setNotice(`重命名成功：${nextPath}`);
      await handleSelectPath(nextPath);
      await handleList(currentPath);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "重命名失败。");
    }
  }

  async function handleMove() {
    if (!selectedPath.trim()) {
      setError("请先选择要移动的文件或目录。");
      return;
    }
    setNotice(null);
    setError(null);
    try {
      const result = await moveMutation.mutateAsync({
        paths: [selectedPath],
        destPath: targetPath,
        conflictPolicy
      });
      const nextPath = result.resultFilePaths?.[0] ?? targetPath;
      setNotice(`移动成功：${nextPath}`);
      await handleList(currentPath);
      await handleSelectPath(nextPath);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "移动失败。");
    }
  }

  async function handleCopy() {
    if (!selectedPath.trim()) {
      setError("请先选择要复制的文件或目录。");
      return;
    }
    setNotice(null);
    setError(null);
    try {
      const result = await copyMutation.mutateAsync({
        paths: [selectedPath],
        destPath: targetPath,
        conflictPolicy
      });
      setNotice(`复制成功：${result.resultFilePaths?.[0] ?? targetPath}`);
      await handleList(currentPath);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "复制失败。");
    }
  }

  async function handleDelete() {
    if (!selectedPath.trim()) {
      setError("请先选择要删除的文件或目录。");
      return;
    }
    const confirmed = window.confirm(`确认删除 ${selectedPath} 吗？`);
    if (!confirmed) {
      return;
    }
    setNotice(null);
    setError(null);
    try {
      await deleteMutation.mutateAsync({
        paths: [selectedPath]
      });
      setNotice(`已删除：${selectedPath}`);
      setSelectedPath("");
      setSelectedEntry(null);
      setDetail(null);
      setDownloadInfo(null);
      await handleList(currentPath);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "删除失败。");
    }
  }

  async function handleUpload() {
    if (!uploadTargetPath.trim()) {
      setError("请先填写上传目标目录。");
      return;
    }
    if (uploadFiles.length === 0) {
      setError("请先选择一个或多个本地文件。");
      return;
    }
    setNotice(null);
    setError(null);
    try {
      const response = await uploadMutation.mutateAsync({
        parentPath: uploadTargetPath,
        files: uploadFiles
      });
      setUploadResults(response.uploaded ?? []);
      setUploadFiles([]);
      const firstUploaded = response.uploaded?.[0];
      if (firstUploaded?.entry) {
        setSelectedEntry(firstUploaded.entry);
        setSelectedPath(firstUploaded.entry.fullPathName);
        setRenameName(firstUploaded.entry.name);
      }
      setNotice(`上传完成，共成功上传 ${response.uploaded?.length ?? 0} 个文件。`);
      if (currentPath === uploadTargetPath) {
        await handleList(currentPath);
      }
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "上传失败。");
    }
  }

  return (
    <article className="detail-card">
      <div className="section-head">
        <div>
          <p className="eyebrow">任务 6 / 文件操作测试</p>
          <h4>基于 CD2 的文件与目录操作</h4>
        </div>

        <button
          type="button"
          className="ghost-button"
          onClick={() => void handleList(currentPath)}
          disabled={listMutation.isPending}
        >
          {listMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
          刷新当前目录
        </button>
      </div>

      {notice ? <p className="inline-note">{notice}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}

      <div className="settings-note-card">
        <div>
          <strong>建议先从已接入账号的根路径开始测试</strong>
          <p>这里已经补上了本地文件上传，测试时可以直接选择本地文件，上传到当前 115open 目录或你指定的子目录。</p>
        </div>
      </div>

      {accountPaths.length > 0 ? (
        <div className="action-row">
          {accountPaths.map((path) => (
            <button
              key={path}
              type="button"
              className="ghost-button"
              onClick={() => {
                setCurrentPath(path);
                setTargetPath(path);
                setUploadTargetPath(path);
                void handleList(path);
              }}
            >
              {path}
            </button>
          ))}
        </div>
      ) : null}

      <div className="page-grid settings-layout">
        <article className="detail-card">
          <div className="settings-card-list">
            <div className="settings-action-card">
              <strong>浏览与搜索</strong>

              <div className="field-grid">
                <label className="field">
                  <span>当前目录</span>
                  <input value={currentPath} onChange={(event) => setCurrentPath(event.target.value)} placeholder="/115open" />
                </label>

                <label className="field">
                  <span>搜索关键词</span>
                  <input
                    value={searchKeyword}
                    onChange={(event) => setSearchKeyword(event.target.value)}
                    placeholder="输入文件名关键字"
                  />
                </label>
              </div>

              <div className="action-row">
                <button type="button" className="primary-button" onClick={() => void handleList(currentPath)} disabled={listMutation.isPending}>
                  {listMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
                  浏览目录
                </button>

                <button type="button" className="ghost-button" onClick={() => void handleSearch()} disabled={searchMutation.isPending}>
                  {searchMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Search size={16} />}
                  搜索
                </button>
              </div>

              <div className="scan-summary-grid">
                <SummaryCell label="模式" value={isSearchMode ? "搜索结果" : "目录浏览"} />
                <SummaryCell label="当前目录" value={currentPath || "-"} />
                <SummaryCell label="项目数量" value={String(entries.length)} />
                <SummaryCell label="已选路径" value={selectedPath || "未选择"} />
              </div>

              {entries.length === 0 ? (
                <div className="sync-empty-block">
                  <Search size={18} />
                  <div>
                    <strong>这里会显示当前目录或搜索结果</strong>
                    <p>先点击“浏览目录”或“搜索”，再从结果里选择一个文件或目录继续测试。</p>
                  </div>
                </div>
              ) : (
                <div className="endpoint-grid">
                  {entries.map((entry) => (
                    <article key={entry.fullPathName} className="endpoint-panel">
                      <div className="endpoint-panel-head">
                        <div>
                          <strong>{entry.name || entry.fullPathName}</strong>
                          <p>{entry.fullPathName}</p>
                        </div>
                        <span className={`status-pill ${entry.isDirectory ? "subtle" : "success"}`}>
                          {entry.isDirectory ? "目录" : "文件"}
                        </span>
                      </div>

                      <div className="endpoint-panel-meta">
                        <div>
                          <span>云账号</span>
                          <strong>{entry.cloudName || "-"}</strong>
                        </div>
                        <div>
                          <span>大小</span>
                          <strong>{entry.size}</strong>
                        </div>
                        <div>
                          <span>可搜索</span>
                          <strong>{entry.canSearch ? "支持" : "不支持"}</strong>
                        </div>
                        <div>
                          <span>详情属性</span>
                          <strong>{entry.hasDetailProperties ? "支持" : "不支持"}</strong>
                        </div>
                      </div>

                      <div className="endpoint-panel-actions">
                        <button type="button" className="ghost-button" onClick={() => void handleSelectPath(entry.fullPathName)}>
                          选中
                        </button>

                        {entry.isDirectory ? (
                          <button
                            type="button"
                            className="ghost-button"
                            onClick={() => {
                              setCurrentPath(entry.fullPathName);
                              setTargetPath(entry.fullPathName);
                              setUploadTargetPath(entry.fullPathName);
                              void handleList(entry.fullPathName);
                            }}
                          >
                            进入目录
                          </button>
                        ) : (
                          <button type="button" className="ghost-button" onClick={() => void handleDownloadURL(entry.fullPathName)}>
                            下载链接
                          </button>
                        )}
                      </div>
                    </article>
                  ))}
                </div>
              )}
            </div>
          </div>
        </article>

        <article className="detail-card">
          <div className="settings-card-list">
            <div className="settings-action-card">
              <strong>选中项操作</strong>

              <div className="scan-summary-grid">
                <SummaryCell label="选中路径" value={selectedPath || "未选择"} />
                <SummaryCell label="名称" value={selectedEntry?.name || "-"} />
                <SummaryCell label="类型" value={selectedEntry?.isDirectory ? "目录" : selectedEntry ? "文件" : "-"} />
                <SummaryCell label="更新时间" value={selectedEntry?.writeTime ? formatCatalogDate(selectedEntry.writeTime) : "-"} />
              </div>

              <div className="field-grid">
                <label className="field">
                  <span>上传目标目录</span>
                  <input
                    value={uploadTargetPath}
                    onChange={(event) => setUploadTargetPath(event.target.value)}
                    placeholder="/115open"
                  />
                </label>

                <label className="field">
                  <span>选择本地文件</span>
                  <input type="file" multiple onChange={(event) => setUploadFiles(Array.from(event.target.files ?? []))} />
                </label>

                <label className="field">
                  <span>新建目录名称</span>
                  <input
                    value={createFolderName}
                    onChange={(event) => setCreateFolderName(event.target.value)}
                    placeholder="例如：mam-cd2-test"
                  />
                </label>

                <label className="field">
                  <span>重命名为</span>
                  <input value={renameName} onChange={(event) => setRenameName(event.target.value)} placeholder="新的名称" />
                </label>

                <label className="field">
                  <span>移动/复制目标目录</span>
                  <input value={targetPath} onChange={(event) => setTargetPath(event.target.value)} placeholder="/115open" />
                </label>

                <label className="field">
                  <span>冲突策略</span>
                  <select value={conflictPolicy} onChange={(event) => setConflictPolicy(event.target.value as typeof conflictPolicy)}>
                    <option value="overwrite">覆盖</option>
                    <option value="rename">自动改名</option>
                    <option value="skip">跳过</option>
                  </select>
                </label>
              </div>

              <div className="action-row">
                <button type="button" className="primary-button" onClick={() => void handleUpload()} disabled={uploadMutation.isPending}>
                  {uploadMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Upload size={16} />}
                  上传本地文件
                </button>

                <button type="button" className="primary-button" onClick={() => void handleCreateFolder()} disabled={createFolderMutation.isPending}>
                  {createFolderMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <FolderPlus size={16} />}
                  新建目录
                </button>

                <button type="button" className="ghost-button" onClick={() => void handleLoadDetail()} disabled={detailMutation.isPending}>
                  {detailMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
                  查看详情
                </button>

                <button type="button" className="ghost-button" onClick={() => void handleDownloadURL()} disabled={downloadMutation.isPending}>
                  {downloadMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Download size={16} />}
                  下载链接
                </button>
              </div>

              <div className="action-row">
                <button type="button" className="ghost-button" onClick={() => void handleRename()} disabled={renameMutation.isPending}>
                  {renameMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <FilePenLine size={16} />}
                  重命名
                </button>

                <button type="button" className="ghost-button" onClick={() => void handleMove()} disabled={moveMutation.isPending}>
                  {moveMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
                  移动
                </button>

                <button type="button" className="ghost-button" onClick={() => void handleCopy()} disabled={copyMutation.isPending}>
                  {copyMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Copy size={16} />}
                  复制
                </button>

                <button type="button" className="danger-button" onClick={() => void handleDelete()} disabled={deleteMutation.isPending}>
                  {deleteMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Trash2 size={16} />}
                  删除
                </button>
              </div>

              {uploadFiles.length > 0 ? (
                <label className="field">
                  <span>待上传文件</span>
                  <textarea value={uploadFiles.map((file) => `${file.name} (${file.size} bytes)`).join("\n")} readOnly />
                </label>
              ) : null}

              {uploadResults.length > 0 ? (
                <label className="field">
                  <span>上传结果</span>
                  <textarea value={JSON.stringify(uploadResults, null, 2)} readOnly />
                </label>
              ) : null}

              {detail ? (
                <label className="field">
                  <span>详情结果</span>
                  <textarea value={JSON.stringify(detail, null, 2)} readOnly />
                </label>
              ) : null}

              {downloadInfo ? (
                <label className="field">
                  <span>下载链接结果</span>
                  <textarea value={JSON.stringify(downloadInfo, null, 2)} readOnly />
                </label>
              ) : null}

              {selectedEntry ? (
                <label className="field">
                  <span>当前选中项</span>
                  <textarea value={JSON.stringify(selectedEntry, null, 2)} readOnly />
                </label>
              ) : null}
            </div>
          </div>
        </article>
      </div>
    </article>
  );
}

function SummaryCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="field">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
