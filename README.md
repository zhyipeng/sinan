# Sinan

基于 Wails 3 的插件驱动桌面效率平台，对标 uTools，采用 All-in-Plugin 架构。

## 特性
- **全插件架构**：核心业务功能（检索、设置、管理）皆为插件。
- **后端故障隔离**：原生后端运行在 Go 独立进程中；原生 UI 控制器仍与宿主同进程。
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

## 插件中心

`make dev` 会准备统一的插件中心。搜索“插件中心”“插件管理”或“插件商店”均可进入；默认管理已安装插件，在“发现”中浏览、搜索、安装和手动更新。界面复用主窗口页头、返回与拆出。JSON 工具 0.4.1 已上架，提供 macOS Apple Silicon 和 Intel 安装包；其他未发布插件仍显示“待发布”。

已有日常实例使用新版宿主后，会迁移已知未修改的旧包；本地修改或未知版本保留并提示处理。中心包位于 `plugins/system-plugin-manager`。目录契约与信任边界见[插件商店文档](docs/plugin-store.md)。

## 跨平台适配

Windows 与 Linux 同步适配中。可通过 `node scripts/desktop.mjs setup`、`build`、`dev` 使用无需 Make/sh 的开发入口；Linux 使用 GTK3/WebKitGTK 4.1。已增加两端应用检索、原生构建 CI 和 Shell 适配，并实现待实机验收的 Linux GTK3 与 Windows WebView2 隔离内容承载；完整发行安装和目标平台验收仍待完成。依赖、命令和能力边界见[跨平台适配](docs/cross-platform.md)。
