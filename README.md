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
- 固定测试页的 UI 隔离原型，已在 Chromium 和 WKWebView 进行负向探测；尚未开放默认启用的第三方 UI 加载。

首版 macOS 应用检索插件已实现名称匹配、应用图标与直接启动。设置插件支持目录、历史清除和快捷键；本地插件管理界面由 `system.plugin-manager` 提供。独立插件 UI 已有 macOS 开发入口，默认关闭；SQLite、插件市场和远程更新尚未实现。

## 搜索交互

- macOS 默认使用 **Command + Space** 切换显示与隐藏，其他平台默认 Control + Space，可在设置中修改。
- 空输入仅显示 50 DIP 高的无边框搜索框；输入文字筛选插件命令，窗口随结果数量展开或收缩。
- macOS 在鼠标所在显示器的原生可用区域呼出，水平居中，顶部位于可用高度的 20%。
- 默认选中当前排名第一项；方向键或鼠标手动选择后保持该项，修改查询时恢复默认选择。上下键选择，回车打开命令，Escape 或失焦隐藏；应用继续驻留。
- 点击菜单栏 / 托盘中的司南图标，选择“显示司南”找回窗口，选择“退出司南”结束应用。
- 搜索“插件管理”或 `plugins`，也可点击搜索框右侧 `···` 打开管理插件。管理插件缺失时进入宿主恢复入口；快捷键注册错误也可在恢复入口查看。

三屏定位、20% 纵向位置和全屏呼出已获用户确认。搜索与 UI 原型的实现及验证边界见 [ADR 0003](docs/adr/0003-search-and-ui-prototype.md)。

## 本地启动

需要 Go 1.26+、Node.js 22.12+、npm、make、Wails CLI 3.0.0；macOS 还需 Xcode Command Line Tools。Go 启动器可通过自动工具链选择下载所需版本。

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
make setup
make dev
```

再次运行前先从托盘退出当前实例。`make dev` 构建宿主与 CLI，在项目 `.sinan-dev/` 中准备应用检索（macOS）、设置和插件管理三个系统插件并打开窗口。脚本仅对仓库自带的应用检索插件使用信任安装；重复执行会升级已有系统插件，相同内容不重启。`make demo` 作为兼容别名保留，不再安装 demo 插件。

普通启动使用 `make run`，默认插件目录为 `os.UserConfigDir()/Sinan/plugins`；在 macOS 中即 `~/Library/Application Support/Sinan/plugins`。若尚未安装管理插件，点击 `···`，在恢复入口中检查并恢复本仓库 `plugins/system-plugin-manager` 的绝对路径，再进入插件管理安装其他包；原生插件需勾选信任。

```sh
make build       # bin/sinan、bin/sinanctl
make package     # macOS: bin/Sinan.app（本地未签名应用包）
make test        # 无弹窗回归：前端构建与 Go 竞态测试，包含无界面子进程
```

`make test` 强制关闭真实 WebView 测试，不受外部 `SINAN_NATIVE_UI_TEST=1` 影响。真实桌面 UI、原生探测和浏览器交互回归均移出日常回归，仅在明确要求 UI 验证时单独运行。

按需运行的键盘导航浏览器回归需要本机 `ego-browser` CLI：先运行 `npm --prefix apps/desktop/frontend run dev`，再在另一终端运行 `npm --prefix apps/desktop/frontend run test:keyboard`。固定测试页控制慢结果到达时机，覆盖搜索未结束时的上下键、同批次增量更新、鼠标静止、结果插入和回车打开选中项；不会连接真实插件。

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

macOS 可进一步运行不含 Wails Bridge 的原生边界验证宿主：

```sh
make ui-probe
```

该命令依次检查正常关闭、消息洪泛、禁用插件，以及脚本调用错误、JavaScript 卡死、可响应慢页面六种模式。它把编译进程序的固定测试夹具安装到独立临时根目录，使用非持久 WKWebView、自定义资源 Scheme 和导航决策代理，不读取日常插件目录。父进程持有内核会话，独立客体进程通过会话专属 stdio 管道逐次读取资源，不复制整包资源。启动须完成管道与页面握手，撤销后通知退出，超时强制终止并回收。每次自动关闭验证窗口，只输出固定诊断字段；正常模式接受 close，洪泛模式 `budgetRevoked=true`，禁用模式 `lifecycleRevoked=true`；各模式均应退出 0，且旧资源和消息都被拒绝，`separateProcess`、`childReaped`、`startupHandshake` 和 `eventLoopResponsive` 均为 true。正对照 HTTP 服务可访问，客体的网络和导航尝试不得到达该服务。故障模式额外输出 `failure=script-error` 或 `failure=unresponsive`，检查会话失效后在同一内核打开正常客体；慢页面须完成原生响应探测后正常关闭。故障与响应预算见 [ADR 0016](docs/adr/0016-ui-renderer-failure-probes.md)。命令目前要求 macOS+cgo，基础验证见 [ADR 0012](docs/adr/0012-native-ui-boundary-probe.md)，会话接入见 [ADR 0013](docs/adr/0013-ui-session-revocation.md)，进程监督见 [ADR 0014](docs/adr/0014-ui-guest-supervision.md)。

可复用的资源快照构建器位于 `internal/plugin/uiview`，限制根目录、归属、资源类型、数量和体积，拒绝符号链接及非规范资源 URL。Manifest 现可通过 `contributes.uiViews` 声明入口、桥版本和受限能力（桥 1 无权限，桥 2 可显式声明本插件命令与私有文档）；内核准备接口只接受已声明视图 ID。Shell 不加载第三方 HTML；开发期可通过插件管理打开独立窗口，细节见 [ADR 0015](docs/adr/0015-declared-ui-views.md)。

内核会话准备接口固定读取已安装包的 `ui/` 子目录，绑定插件实例、包指纹、声明视图 ID 和桥版本，资源加载前后检查包内容。停止、禁用、重载、升级、卸载、配置保存及检测到包文件变化时撤销会话；回退相同包也不会让旧句柄复活。外部文件变化在刷新或打开会话等校验点发现，尚无文件监听。每插件最多 4 个、每宿主最多 16 个活动会话，此接口不暴露给 Wails 或插件进程。

## 临时窗口验收

```sh
make ui-acceptance
```

直接进入临时实例的插件管理，找到「窗口验收」并点击「打开 输入测试」。关闭窗口后还可搜索「窗口验收」并回车直接打开。页面可测试中文输入、关闭和模拟卡死；正常退出后清理临时目录，不修改日常插件。为测试 Command + Space，建议先退出日常实例。完整点测步骤见[独立窗口手动验收](docs/ui-acceptance.md)。

## 独立插件窗口（开发期）

```sh
make dev-ui
# 已构建时：./bin/sinan -plugins .sinan-dev/plugins -plugin-ui
```

默认启动不启用。开启后，在插件管理中可打开包内 `contributes.uiViews` 声明的视图，也可搜索通过 `uiView` 关联的命令并回车打开；不会自动安装测试插件。若日常实例正在运行，先通过托盘退出，再启动开发实例。`make build` 构建固定的 `sinan-ui-host`，`make package` 将它一并放入应用包；缺少它时保留 Shell 并显示诊断。

客体窗口可关闭、缩放、输入；重复打开会聚焦并保留输入，新窗口在鼠标所在屏幕居中，标题可由视图声明。关闭、插件停用或宿主退出会撤销并回收。原生自动测试已覆盖页面焦点、普通关闭、拒绝探测动作与运行超过 8 秒；中文输入法和全屏焦点尚需实机验证。没有操作系统沙箱或 CPU/内存硬配额，目前不适合公共第三方分发。见 [ADR 0017](docs/adr/0017-desktop-plugin-windows.md)。

## 应用检索

`make dev` 后搜索 `计算器`、`calculator` 或 `safari`，使用上下键选择并回车直接打开应用。扫描标准系统应用目录、`/Applications` 和 `~/Applications`，同时支持英文文件名、系统本地化名称和英文词首字母（如 `vsc`）。图标随结果展示。

没有缓存时，索引在首次查询建立。有缓存时，重启后先提供快照结果，同时后台校验；超过一分钟后的查询也会触发后台扫描，完成后下一次查询使用新索引。可搜索并执行“刷新应用索引”立即重建。支持拼音全拼、首字母（如 `jisuanqi` / `jsq`），宿主按成功动作的频率和近期使用轻量加权。可在设置中添加额外应用目录。此插件以可信原生进程运行，用途声明不提供操作系统沙箱；具体边界见 [ADR 0004](docs/adr/0004-application-search.md)。

应用索引快照保存在插件根目录的 `.sinan-state/cache/system.app-search/applications-v1.json`，不含查询、使用历史或图标。目录配置变化、缓存格式不兼容或损坏时重新扫描；缓存不可写不影响搜索。后台校验完成前可能短暂显示旧结果，启动前仍会检查应用是否存在。插件升级保留缓存。详见 [ADR 0009](docs/adr/0009-application-index-cache.md)。

自动扫描复用未变化应用的元数据及图标；手动“刷新应用索引”强制重读。使用历史存放在插件根目录的 `.sinan-state/usage.json`，只保存动作摘要、次数和时间，最多 4096 条。退出桌面后可运行以下命令清除开发实例的记录，然后重新启动：

```sh
./bin/sinanctl -plugins .sinan-dev/plugins clear-history
```

拼音、多音字、历史加权和并发写入的限制见 [ADR 0005](docs/adr/0005-search-refinement.md)。

## 设置

搜索“设置”或 `settings`，回车打开。额外应用目录每行一个绝对路径，保存后下一次搜索生效，标准目录保留。可直接清除使用历史，无需退出桌面。

在“呼出快捷键”中点击当前组合键，然后按下新组合，点击“保存快捷键”立即生效并在重启后保留。“恢复默认”填回默认组合，仍需保存。录入期间按当前呼出热键会录入该组合；失焦、离开设置或 30 秒后结束录入。注册冲突时保留原热键；可从托盘重新打开窗口。支持 Command / Ctrl / Alt（可叠加 Shift）与字母、数字、空格、方向键等标准按键。系统保留组合键可能无法录入或注册，其他平台尚未实机验证。机制见 [ADR 0008](docs/adr/0008-configurable-launcher-shortcut.md)。

设置界面属于 `system.settings` 插件，组件随宿主构建；卸载该插件不会删除配置。配置保存在插件根目录的 `.sinan-state/settings.json`，损坏或未知版本的文件保留原样并阻止覆盖。实现边界见 [ADR 0006](docs/adr/0006-settings-service-and-view.md)。

仅用 CLI 时可在退出桌面实例后查看配置：

```sh
./bin/sinanctl -plugins .sinan-dev/plugins settings system.app-search
./bin/sinanctl -plugins .sinan-dev/plugins set-settings system.app-search 0 '{"directories":[]}'
```

第二个命令中的 `0` 需替换为当前读取到的 revision；空列表清空额外目录。当前仅支持目录列表配置，不支持凭据存储。

## 插件管理、升级与重载

打开搜索框右侧 `···`，刷新可发现“文件有变更”，点击对应插件的“重载”应用安装目录中的修改。替换为外部插件包时，填写包目录并点击“检查”，查看版本变化后再安装或升级；原生包需重新勾选信任。配置和使用历史保留，候选初始化失败时恢复旧包。

管理界面属于可信组件插件 `system.plugin-manager`，组件随宿主构建，不加载安装包中的 JavaScript。支持停止和卸载，包括卸载管理插件自身；卸载自身后自动进入恢复入口。管理界面渲染失败时仍可返回搜索或打开恢复入口。宿主恢复入口检查状态、恢复声明式管理包，并可重新启用被禁用的管理插件，不提供日常插件列表操作。编译进宿主的组件代码故障仍须重新构建修复；离线损坏且未注册的安装目录可能需人工修复。详见 [ADR 0010](docs/adr/0010-plugin-management-view.md)。

插件行末的“禁用”会持久停止该插件，移出静态命令和动态搜索，并阻止直接执行；重启、重载、升级及重新安装同 ID 包均保留禁用状态。“启用”恢复按需激活，不立即启动进程。原有“停止”只结束当前进程，下次调用仍可激活。禁用管理插件自身后，点击 `···` 可在恢复入口重新启用。目录设置、缓存、历史和原生信任记录不因禁用而删除。详见 [ADR 0011](docs/adr/0011-plugin-enablement.md)。

退出桌面后也可使用 CLI：

```sh
./bin/sinanctl -plugins .sinan-dev/plugins inspect .sinan-dev/packages/app-search
./bin/sinanctl -plugins .sinan-dev/plugins -trust-native upgrade .sinan-dev/packages/app-search
./bin/sinanctl -plugins .sinan-dev/plugins reload system.app-search
./bin/sinanctl -plugins .sinan-dev/plugins disable system.app-search
./bin/sinanctl -plugins .sinan-dev/plugins enable system.app-search
```

升级回退只覆盖当前升级事务；重载不会撤销直接编辑过的文件，设置等可信组件代码修改仍需重建宿主。完整边界与中断恢复见 [ADR 0007](docs/adr/0007-plugin-upgrade-and-reload.md)。

启用策略位于核心设置文件的保留命名空间 `$plugins`，插件设置接口不可读写。写入失败时保持当前状态；启用策略或整个设置文件损坏、无法读取时阻止插件激活并保留文件，需离线修复后重启，不会自动覆盖或重置为全部启用。

### JSON 格式化插件

使用 `make dev-ui` 在开发实例中安装并打开 `tools.json`，搜索 JSON 即可进入。支持格式化、压缩、错误行列、保留大整数和草稿保存；Command/Ctrl + Enter 格式化，加 Shift 压缩。拆出窗口使用设置中的窗口独立快捷键。

也可先 `make build json-plugin`，退出使用目标目录的宿主后，用 `bin/sinanctl -plugins 插件目录 -trust-native install bin/plugins/json-tool` 安装，再以 `-plugin-ui` 启动桌面宿主。原生后端需显式信任，独立进程不是安全沙箱。草稿显示“已保存”后才确认落盘，单插件文档最大 32 KiB。完整边界见 [ADR 0029](docs/adr/0029-ui-calls-private-data-json-tool.md)。

### 插件私有数据管理

插件管理底部的“插件数据”可查看私有文档占用，清理已安装或卸载后保留的数据。清理须二次确认，会关闭该插件 HTML 页面，保留设置、历史、包和缓存；文件变化后需重新确认。CLI 对应 `data`、`clear-data ID 检查指纹`，先退出使用同目录的桌面宿主。详见 [ADR 0030](docs/adr/0030-plugin-data-management.md)。

设置 → 外观支持浅色、深色和跟随系统（默认），选择自动保存并同步已打开的窗口。JSON 工具已适配；第三方 HTML 可使用主题 CSS 变量和只读 SDK 通知，见[主题协议](docs/plugin-protocol.md#主题数据协议-v1)。当前预留主题扩展接口，尚未支持安装主题插件。

窗口内按 `Command + ,`（macOS）或 `Ctrl + ,`（其他平台）进入设置；设置已拆出时直接聚焦该窗口。macOS 隔离插件窗口也支持此入口。
