# ADR 0034：内置插件与官方插件仓库分离

日期：2026-11-11。状态：本地源码分离已实施，远程分发未实现。

## 决策与原因

按用户指令，JSON 工具是可选官方插件，其他三个现有插件属于内置插件。用户确认先创建同级本地独立 Git 仓库，并同时建立目录仓库；不创建或推送 GitHub 远端。

- `sinan` 保留宿主、协议、SDK 与 `system.app-search`、`system.settings`、`system.plugin-manager`。
- `sinan-official-plugins` 保存 `plugins/json-tool` 及独立构建、测试、ZIP 打包入口。
- `sinan-plugin-registry` 登记 `tools.json` 的资料，状态 `pending-release`，源码远端地址为空、版本列表为空；不生成可安装索引。

这样将可选业务插件的源码和构建周期从宿主拆出，内置可信 UI 组件仍随宿主构建，不强行拆分尚不能独立升级的组件。

## SDK 与兼容

宿主当前 Go 模块名为 `sinan`，SDK 尚无可引用的正式发行模块。为避免大规模改写宿主导入路径，也避免官方插件依赖相邻宿主目录，官方仓库携带 `third_party/sinan` 最小模块快照：协议 `protocol.go`、Go SDK `server.go` 与 UI Bridge JS/类型。`UPSTREAM.json` 记录原本体提交与逐文件摘要；将来 SDK 正式发布后替换快照依赖。快照更新须核对指定提交的字节，不自动跟随本体工作区变化。

`tools.json` ID 和 0.1.0 功能保持不变；后端 API 1、UI 桥 2 无变更，不新增宿主管理权限。包构建不需要 Wails 或本体源码。迁移当前源码，不改写本体 Git 历史；原 JSON 开发记录仍可从本体历史追溯。

## 开发与测试影响

默认开发准备脚本只管理三个内置插件；不自动卸载已有 JSON 安装、不删除包外草稿。JSON 在官方仓库显式构建、安装或升级。`make json-plugin` 仅作为委托外部仓库的兼容入口，支持 `OFFICIAL_PLUGINS_DIR`，产物也在外部仓库。

宿主原 JSON 原生集成测试改为读取 `SINAN_JSON_PLUGIN_PACKAGE` 指定的绝对构建目录；无配置则跳过。日常 `make test` 仍强制关闭真实 UI，不需要检出官方插件仓库。官方插件格式化单元测试与浏览器验证脚本随源码迁移。

两个新仓库已初始化 Git，未创建提交、远端或 Release。目录发布、签名、商店 UI、下载校验及正式版本记录协议均待后续实施，不将本地资料登记描述为已上架。

## 验证

- 本体 `make test` 通过，包含前端类型检查/构建、主题 SDK 测试与 Go 竞态测试；未检出外部 JSON 源码也不参与普通宿主测试。
- 本体 `go vet ./...` 与桌面、CLI、UI helper 编译通过；直接 Go 构建出现本机 macOS SDK 目标版本链接警告，未影响构建结果。
- 官方插件 `make test package` 通过，生成 macOS arm64 ZIP 和 SHA-256 文件。
- 核对最小 SDK 每个快照文件与来源提交完全一致；核对 ZIP 根布局与后端执行位。
- 独立后端握手、命令调用和大整数保留验证通过；通过本体 CLI 安装到临时目录成功，临时目录已清理。
- 目录 `python3 scripts/check.py` 通过：1 个插件资料，0 个可安装版本。
- 本体 `git diff --check`、开发脚本语法检查通过。本次未运行真实桌面、原生 WebView 或浏览器交互验证。
