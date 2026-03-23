# Mare

Mare 是一个桌面端多媒体资产管理客户端，用来把分散在本地、NAS、115 网盘和可移动存储中的图片、视频、音频资产归并为统一 Catalog，并围绕多副本管理、预览、扫描和后续恢复能力构建完整工作流。

当前项目采用 `Tauri + React + TypeScript` 作为客户端层，采用 `Go + SQLite` 作为本地后端与 Catalog 主存储。

## 当前能力

- 桌面客户端主布局与导航
- 资产库页面、筛选排序、分页浏览
- 资产详情页面与多副本展示
- 浅色 / 深色双主题
- Go 后端本地 HTTP 服务
- SQLite Catalog
- Local / QNAP / 115 / Removable 连接器骨架与接入方向
- 全量扫描与单端点重扫
- 图片缩略图生成与缓存
- 视频封面提取
- 音频基础元数据读取
- 图片 / 视频 / 音频基础预览
- 后台任务记录与状态更新
- Go 后端 Air 热重载开发环境

## 项目结构

```text
src/          React + TypeScript 客户端界面
backend/      Go 后端、Catalog、HTTP 服务、Connector、Store
src-tauri/    Tauri 宿主工程
项目文档/      项目介绍、功能设计、技术架构与 MVP 文档
```

## 本地开发

### 1. 启动 Go 后端热重载

在仓库根目录执行：

```powershell
cmd /c npm.cmd run backend:dev
```

这会通过 `backend/dev-air.ps1` 启动 Air，并自动监听 `backend/` 下的 Go 代码变更。

### 2. 启动前端开发服务

```powershell
cmd /c npm.cmd run dev
```

默认会启动 Vite 开发服务器。

### 3. 启动 Tauri 桌面客户端

```powershell
cmd /c npm.cmd run tauri dev
```

或：

```powershell
cmd /c npm.cmd run tauri
```

当前脚本已显式指定 `CARGO_TARGET_DIR=D:\codex_project\mare-tauri-target`，用于避免 Tauri 开发态构建路径与磁盘空间问题。

## 媒体处理说明

- 图片缩略图由 Go 后端直接生成。
- 视频封面提取依赖本机可用的 `FFmpeg`。
- 音频元数据优先通过 `ffprobe` 读取，失败时会回退到 Go 侧基础能力。

如果系统未正确安装或配置 FFmpeg / ffprobe，视频封面与部分音频探测能力会降级，但不会阻断资产基本显示。

## 参考文档

仓库根目录下新增了 [项目文档](./项目文档) 目录，包含：

- 总体架构简介
- 系统功能设计说明书
- 技术架构设计说明书
- MVP 功能需求及任务拆分
- MVP 实现 Prompt（按任务）

这些文档统一以当前代码为准。
