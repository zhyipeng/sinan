# Sinan

基于 Wails 3 的插件驱动桌面效率平台，对标 uTools，采用 All-in-Plugin 架构。

## 特性
- **全插件架构**：核心业务功能（检索、设置、管理）皆为插件。
- **故障隔离**：基于 Go 独立进程运行插件，保障宿主稳定。
- **高扩展性**：支持插件 UI 独立承载与隔离。

## 快速开始
需要 Go 1.26+、Node.js 22.12+：
```bash
make setup
make dev
```

## 开发文档
- [架构指导方案](docs/architecture.md)
- [插件开发协议](docs/plugin-protocol.md)
- [AI 协作开发约定](AGENTS.md)

## 插件商店

`make dev` 会准备插件商店。搜索“插件商店”，或从插件管理进入，可浏览、搜索、查看详情、安装和手动更新。界面复用主窗口页头、返回与拆出。当前 JSON 工具没有发布包，显示“待发布”。

已有日常实例需使用本次构建，并在插件管理中安装 `plugins/system-plugin-store`。目录契约与信任边界见[插件商店文档](docs/plugin-store.md)。
