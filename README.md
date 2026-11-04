# Sinan

基于 Wails 3 的插件驱动桌面效率平台，对标 uTools，采用 All-in-Plugin 架构。

应用检索、设置、插件管理等业务功能均通过插件提供，宿主负责插件运行所需的基础机制。

## 项目指导方案

- [架构指导方案](docs/architecture.md)：项目定位、边界、技术选型、实施路线与验收标准。
- [原始规划存档](docs/references/plugin-architecture-planning.md)：来源会话的规划正文，供追溯。
- [开发约定](AGENTS.md)：后续开发及 AI 协作需要遵守的规则。

## 当前进度

Phase 1 最小运行原型已实现，已在本机 macOS arm64 验证：

- Wails 3 + React / TypeScript 命令 Shell。
- Manifest 校验、目录发现、本地目录安装、停止和卸载。
- 声明式插件与按需激活的 Go 独立进程插件。
- JSON-RPC 2.0 stdio 握手、命令调用、超时终止和失败隔离。
- 宿主 CLI、协议类型、最小 Go 插件 SDK 和两个示例插件。

应用检索、全局快捷键、设置、SQLite、第三方插件 UI 和完整插件管理系统插件尚未实现。当前“插件”入口是宿主启动期的本地恢复与验证入口。

## 本地启动

需要 Go 1.26+、Node.js 22.12+、npm、make、Wails CLI 3.0.0；macOS 还需 Xcode Command Line Tools。Go 启动器可通过自动工具链选择下载所需版本。

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
make setup
make demo
```

`make demo` 构建宿主与 CLI，在项目 `.sinan-dev/` 中准备两个示例插件并打开窗口。脚本只对仓库自带的 Go 示例使用信任安装。重复执行保留已有示例；修改示例后需先卸载再安装。

普通启动使用 `make run`，默认插件目录为 `os.UserConfigDir()/Sinan/plugins`；在 macOS 中即 `~/Library/Application Support/Sinan/plugins`。可通过窗口内“插件”入口填入插件目录安装；原生插件需勾选信任。

```sh
make build       # bin/sinan、bin/sinanctl
make package     # macOS: bin/Sinan.app（本地未签名应用包）
make test        # 包含真实子进程的竞态测试
```

前端资源嵌入宿主，修改后用 `make build` 或 `make demo` 重新构建。当前尚未配置 Wails 热重载工作流；仅打开前端 Vite 页面无法调用桌面宿主。

## 独立插件验证

完成一次 `make demo` 构建后，也可退出窗口后使用 CLI：

```sh
./bin/sinanctl -plugins .sinan-dev/plugins list
./bin/sinanctl -plugins .sinan-dev/plugins run example.hello hello
./bin/sinanctl -plugins .sinan-dev/plugins run example.echo echo '你好，司南'
./bin/sinanctl -plugins .sinan-dev/plugins uninstall example.echo
./bin/sinanctl -plugins .sinan-dev/plugins -trust-native install .sinan-dev/packages/echo
```

CLI 所启动的进程在命令结束时回收；桌面内可持续运行并通过“停止”终止。不要让 CLI 与桌面同时修改同一个安装目录，当前无跨进程安装锁。

更多细节见 [当前插件接口](docs/plugin-protocol.md) 和 [ADR 0001](docs/adr/0001-phase-one-runtime.md)。独立进程提供故障隔离，原生插件仍以当前用户权限运行。
