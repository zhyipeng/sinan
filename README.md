# Sinan

基于 Wails 3 的插件驱动桌面效率平台，对标 uTools，采用 All-in-Plugin 架构。

应用检索、设置、插件管理等业务功能均通过插件提供，宿主负责插件运行所需的基础机制。

## 项目指导方案

- [架构指导方案](docs/architecture.md)：项目定位、边界、技术选型、实施路线与验收标准。
- [原始规划存档](docs/references/plugin-architecture-planning.md)：来源会话的规划正文，供追溯。
- [开发约定](AGENTS.md)：后续开发及 AI 协作需要遵守的规则。
- [下一轮开发计划](docs/next-iteration.md)：多 Provider 搜索、运行时并行与取消、插件 UI 隔离验证。

## 当前进度

Phase 1 最小运行原型已实现，已在本机 macOS arm64 验证：

- Wails 3 + React / TypeScript 命令 Shell。
- Manifest 校验、目录发现、本地目录安装、停止和卸载。
- 包内容变更检测、显式重载、保留配置的升级与初始化失败回退。
- 声明式插件与按需激活的 Go 独立进程插件。
- JSON-RPC 2.0 stdio 握手、命令调用、超时终止和失败隔离。
- 宿主 CLI、协议类型、Go 插件 SDK，以及应用检索与设置系统插件。
- 宿主搜索协调、静态命令索引、动态来源并行合并、请求取消及来源失败隔离。
- 配置服务、可信设置组件插件、额外应用目录及桌面历史清除入口。
- 固定测试页的 UI 隔离原型，已在 Chromium 和 WKWebView 进行负向探测；尚未开放第三方 UI 加载。

首版 macOS 应用检索插件已实现名称匹配、应用图标与直接启动。设置插件首版支持额外应用目录和历史清除；SQLite、第三方插件 UI 和完整插件管理系统插件尚未实现。当前“插件”入口是宿主启动期的本地恢复与验证入口。

## 搜索交互

- macOS 使用 **Command + Space** 切换显示与隐藏，其他平台映射为 Control + Space。
- 空输入仅显示 50 DIP 高的无边框搜索框；输入文字筛选插件命令，窗口随结果数量展开或收缩。
- macOS 在鼠标所在显示器的原生可用区域呼出，水平居中，顶部位于可用高度的 20%。
- 默认选中当前排名第一项；方向键或鼠标手动选择后保持该项，修改查询时恢复默认选择。上下键选择，回车打开命令，Escape 或失焦隐藏；应用继续驻留。
- 点击菜单栏 / 托盘中的司南图标，选择“显示司南”找回窗口，选择“退出司南”结束应用。
- 搜索框右侧 `···` 打开本地插件入口。快捷键注册失败时，该入口会提示占用错误。

三屏定位、20% 纵向位置和全屏呼出已获用户确认。搜索与 UI 原型的实现及验证边界见 [ADR 0003](docs/adr/0003-search-and-ui-prototype.md)。

## 本地启动

需要 Go 1.26+、Node.js 22.12+、npm、make、Wails CLI 3.0.0；macOS 还需 Xcode Command Line Tools。Go 启动器可通过自动工具链选择下载所需版本。

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
make setup
make dev
```

再次运行前先从托盘退出当前实例。`make dev` 构建宿主与 CLI，在项目 `.sinan-dev/` 中准备应用检索（macOS）和设置插件并打开窗口。脚本仅对仓库自带的应用检索插件使用信任安装；重复执行会升级已有系统插件，相同内容不重启。`make demo` 作为兼容别名保留，不再安装 demo 插件。

普通启动使用 `make run`，默认插件目录为 `os.UserConfigDir()/Sinan/plugins`；在 macOS 中即 `~/Library/Application Support/Sinan/plugins`。可通过窗口内 `···` 的本地插件入口填入插件目录安装；原生插件需勾选信任。

```sh
make build       # bin/sinan、bin/sinanctl
make package     # macOS: bin/Sinan.app（本地未签名应用包）
make test        # 包含真实子进程的竞态测试
```

键盘导航的浏览器回归需要本机 `ego-browser` CLI：先运行 `npm --prefix apps/desktop/frontend run dev`，再在另一终端运行 `npm --prefix apps/desktop/frontend run test:keyboard`。固定测试页控制慢结果到达时机，覆盖搜索未结束时的上下键、同批次增量更新、鼠标静止、结果插入和回车打开选中项；不会连接真实插件。

前端资源嵌入宿主，修改后用 `make build` 或 `make dev` 重新构建。当前尚未配置 Wails 热重载工作流；仅打开前端 Vite 页面无法调用桌面宿主。

## 独立插件验证

完成一次 `make dev` 构建后，从托盘退出司南，再使用 CLI：

```sh
./bin/sinanctl -plugins .sinan-dev/plugins list
./bin/sinanctl -plugins .sinan-dev/plugins search calculator
./bin/sinanctl -plugins .sinan-dev/plugins settings system.app-search
./bin/sinanctl -plugins .sinan-dev/plugins reload system.app-search
```

CLI 所启动的进程在命令结束时回收；桌面内可持续运行并通过“停止”终止。CLI 与桌面不能同时打开同一个插件根目录；宿主生命周期内持有目录独占锁，即使只读 CLI 命令也需先退出桌面。

更多细节见 [当前插件接口](docs/plugin-protocol.md) 和 [ADR 0001](docs/adr/0001-phase-one-runtime.md)。独立进程提供故障隔离，原生插件仍以当前用户权限运行。

## UI 隔离原型

```sh
./bin/sinan -ui-probe -plugins .sinan-dev/probe-plugins
```

此开发模式展示固定隔离页并输出 `ISOLATION_PROBE` 布尔报告，不注册热键或托盘。原型验证逻辑隔离与受限消息桥，不提供任意插件 HTML 加载、完整导航防护或渲染进程隔离。

## 应用检索

`make dev` 后搜索 `计算器`、`calculator` 或 `safari`，使用上下键选择并回车直接打开应用。扫描标准系统应用目录、`/Applications` 和 `~/Applications`，同时支持英文文件名、系统本地化名称和英文词首字母（如 `vsc`）。图标随结果展示。

索引在首次查询建立，超过一分钟后的下一次查询自动重建；也可搜索并执行“刷新应用索引”。支持拼音全拼、首字母（如 `jisuanqi` / `jsq`），宿主按成功动作的频率和近期使用轻量加权。可在设置中添加额外应用目录。此插件以可信原生进程运行，用途声明不提供操作系统沙箱；具体边界见 [ADR 0004](docs/adr/0004-application-search.md)。

自动扫描复用未变化应用的元数据及图标；手动“刷新应用索引”强制重读。使用历史存放在插件根目录的 `.sinan-state/usage.json`，只保存动作摘要、次数和时间，最多 4096 条。退出桌面后可运行以下命令清除开发实例的记录，然后重新启动：

```sh
./bin/sinanctl -plugins .sinan-dev/plugins clear-history
```

拼音、多音字、历史加权和并发写入的限制见 [ADR 0005](docs/adr/0005-search-refinement.md)。

## 设置

搜索“设置”或 `settings`，回车打开。额外应用目录每行一个绝对路径，保存后下一次搜索生效，标准目录保留。可直接清除使用历史，无需退出桌面。

设置界面属于 `system.settings` 插件，组件随宿主构建；卸载该插件不会删除配置。配置保存在插件根目录的 `.sinan-state/settings.json`，损坏或未知版本的文件保留原样并阻止覆盖。实现边界见 [ADR 0006](docs/adr/0006-settings-service-and-view.md)。

仅用 CLI 时可在退出桌面实例后查看配置：

```sh
./bin/sinanctl -plugins .sinan-dev/plugins settings system.app-search
./bin/sinanctl -plugins .sinan-dev/plugins set-settings system.app-search 0 '{"directories":[]}'
```

第二个命令中的 `0` 需替换为当前读取到的 revision；空列表清空额外目录。当前仅支持目录列表配置，不支持凭据存储。

## 插件升级与重载

打开搜索框右侧 `···`，刷新可发现“文件有变更”，点击对应插件的“重载”应用安装目录中的修改。替换为外部插件包时，填写包目录并点击“检查”，查看版本变化后再安装或升级；原生包需重新勾选信任。配置和使用历史保留，候选初始化失败时恢复旧包。

退出桌面后也可使用 CLI：

```sh
./bin/sinanctl -plugins .sinan-dev/plugins inspect .sinan-dev/packages/app-search
./bin/sinanctl -plugins .sinan-dev/plugins -trust-native upgrade .sinan-dev/packages/app-search
./bin/sinanctl -plugins .sinan-dev/plugins reload system.app-search
```

升级回退只覆盖当前升级事务；重载不会撤销直接编辑过的文件，设置等可信组件代码修改仍需重建宿主。完整边界与中断恢复见 [ADR 0007](docs/adr/0007-plugin-upgrade-and-reload.md)。
