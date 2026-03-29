# CD2 Docker 开发测试环境

这套配置用于在 **不影响本机现有 CD2（当前占用 `19798`）** 的前提下，额外拉起一套基于 Docker 的 CD2 测试环境。

## 目标

1. 容器名称独立
2. 端口独立，默认使用 `29798`
3. 数据独立，使用 Docker 命名卷
4. 优先满足 **API / Web 管理界面开发测试**

## 为什么这里没有直接照搬官方 Docker 配置

官方 Docker 指南使用的是 Linux 主机场景，核心特征是：

1. `network_mode: host`
2. `pid: host`
3. `privileged: true`
4. `/dev/fuse`
5. `:shared` 挂载传播

这套方式更适合 Linux / NAS 主机上的“完整挂载模式”，但当前你的环境是 Windows，并且本机已有一个 CD2 正在监听 `19798`。如果继续用 `host` 网络模式，两套实例很容易直接冲突。

因此这里先提供一个 **开发测试优先** 的 `bridge + 端口映射 + 命名卷` 版本，重点是：

1. 让第二套 CD2 和当前本机实例共存
2. 让你先完成后续 API 集成开发
3. 等后面需要验证挂载/FUSE能力时，再单独补 Linux 全量模式

## 文件说明

1. `docker-compose.api-dev.yml`
   - 开发测试版 Compose
2. `.env.example`
   - 环境变量示例

## 端口规划

1. 本机现有 CD2：`19798`
2. Docker 测试 CD2：`29798`

打开方式：

1. 本机现有 CD2：`http://localhost:19798`
2. Docker 测试 CD2：`http://localhost:29798`

## 启动步骤

前提：当前机器必须先安装并启动 Docker / Docker Desktop，并保证 `docker compose` 可用。

### 1. 复制环境变量文件

在当前目录下创建 `.env`，可以直接参考：

```env
CD2_CONTAINER_NAME=clouddrive2-dev
CD2_IMAGE_TAG=latest
CD2_TIMEZONE=Asia/Shanghai
CD2_WEB_PORT=29798
CD2_CONFIG_VOLUME=clouddrive2-dev-config
CD2_CLOUDNAS_VOLUME=clouddrive2-dev-cloudnas
CD2_MEDIA_VOLUME=clouddrive2-dev-media
```

### 2. 启动容器

```powershell
docker compose -f docker/clouddrive2-dev/docker-compose.api-dev.yml up -d
```

### 3. 查看容器状态

```powershell
docker compose -f docker/clouddrive2-dev/docker-compose.api-dev.yml ps
```

### 4. 查看日志

```powershell
docker compose -f docker/clouddrive2-dev/docker-compose.api-dev.yml logs -f
```

### 5. 访问测试实例

```text
http://localhost:29798
```

## 停止与清理

停止容器：

```powershell
docker compose -f docker/clouddrive2-dev/docker-compose.api-dev.yml down
```

如果还要删除测试数据卷：

```powershell
docker volume rm clouddrive2-dev-config clouddrive2-dev-cloudnas clouddrive2-dev-media
```

如果你修改了 `.env` 里的卷名，请用对应的新卷名。

## 当前已知限制

1. 这套配置优先面向 API / Web 测试，不保证完整覆盖官方 Linux Docker 挂载模式
2. 如果后续需要验证 FUSE、本地挂载传播、host network 等能力，需要补一套 Linux 专用配置
3. 当前机器如果没有安装 Docker，本配置无法直接启动

## 与现有本机 CD2 的隔离策略

1. 不复用本机进程
2. 不复用 `19798`
3. 不复用本机 CD2 数据目录
4. 使用独立容器名和独立命名卷
