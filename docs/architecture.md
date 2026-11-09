# Sinan 架构指导方案

状态：项目架构基线，Phase 1 最小运行原型已实现；整体规划尚未完成。整理日期：2026-11-04。

来源：[规划插件化工具架构](chatgpt-conversation://6aeaf974-c4d4-83e8-bde5-978c154d8781)。用户已指定将该会话结论作为本项目指导方案。[原始规划存档](references/plugin-architecture-planning.md)用于追溯，本文用于指导实施。

本文保留原会话的架构结论，并区分基线、推荐方案与待决事项。原文涉及的 Wails 能力和版本状态未在本次整理中重新验证，实际开发应针对所选版本验证，不将原文中的“稳定”表述作为保证。

首轮已锁定 Wails 3.0.0、Go 1.26.0，完成 macOS arm64 最小宿主、本地插件目录安装与卸载、声明式和 Go 进程示例、按需激活及 stdio RPC。构建和真实进程测试已通过，桌面手动验证了筛选、执行与停止。详细范围及限制见 [ADR 0001](adr/0001-phase-one-runtime.md) 和[当前插件接口](plugin-protocol.md)。本轮已补充多 Provider 搜索与固定 UI 隔离原型，系统权限服务和公共第三方 UI 加载仍待实现。首版 macOS 应用检索已由可信原生系统插件提供。

按用户后续要求，Shell 已改为聚焦式交互：Command + Space 切换显示与隐藏，空输入框高 50 DIP，按结果调整高度，失焦隐藏，并提供托盘恢复与退出入口。macOS 呼出时选择鼠标所在显示器，使用原生可用区域水平居中，纵向固定为可用高度的 20%，不设置像素上限。当前搜索已统一插件声明的静态命令与动态 Search Provider。实现与验证边界见 [ADR 0002](adr/0002-launcher-interaction.md)。

macOS Shell 采用非激活 NSPanel，并配置 floating 层级及跨 Space / 全屏辅助窗口行为，避免呼出时激活宿主而切离全屏应用。该机制及实机验证边界同样记录在 ADR 0002。

本轮搜索契约、运行时并行和取消、定向结果投递以及 UI 隔离原型的边界见 [ADR 0003](adr/0003-search-and-ui-prototype.md)。

可信原生应用检索、直接动作及用途声明的阶段性边界见 [ADR 0004](adr/0004-application-search.md)。当前应用平台调用位于可信插件内，未来受控 Host API 尚未实现；用途声明不构成授权隔离。

拼音与增量索引保留在应用插件；宿主使用历史和有界本地持久化的实现边界见 [ADR 0005](adr/0005-search-refinement.md)。这不是完整的 SQLite / 通用插件存储服务。

配置存储、目录设置 Schema、初始化快照及随宿主构建的设置插件见 [ADR 0006](adr/0006-settings-service-and-view.md)。公共第三方 UI、SQLite 与通用插件数据服务仍未实现。

呼出快捷键支持在设置插件中按键录入、恢复默认和显式保存；宿主负责校验、运行时注册与核心配置持久化。使用保留的 `$launcher` 命名空间，不开放给插件配置接口。新组合先注册，冲突时保留原组合，存储失败时尝试回退注册。录入期间当前呼出热键转为定向录入事件。边界见 [ADR 0008](adr/0008-configurable-launcher-shortcut.md)。

开发期包指纹、重载、升级回退及宿主目录独占约定见 [ADR 0007](adr/0007-plugin-upgrade-and-reload.md)。日常本地管理界面已由 `system.plugin-manager` 提供，宿主保留最小恢复入口及可信视图错误边界，见 [ADR 0010](adr/0010-plugin-management-view.md)。签名、插件市场与远程更新仍未实现。

应用检索 0.4.0 在插件内实现可丢弃的索引快照与后台校验；宿主仅通过可选 `initialize.cacheDir` 提供包外缓存目录。扫描不持有搜索快照锁，无缓存时首查仍需建立索引；已有缓存的结果可能短暂过期，启动前检查应用。兼容性与验证边界见 [ADR 0009](adr/0009-application-index-cache.md)。

插件启用策略由内核持久管理，界面仍在管理插件中。禁用后停止进程、拒绝激活与执行、排除所有搜索贡献，升级和重载保留状态；历史版本允许管理插件禁用自身，再通过宿主恢复入口重新启用；按用户后续指令，设置和插件管理现禁止禁用及卸载。策略保存失败保留原状态，策略损坏时阻止激活而不覆盖原文件。见 [ADR 0011](adr/0011-plugin-enablement.md)。

第三方 UI 边界验证新增只读资源快照构建器与独立的 macOS 原生开发宿主：使用无 Wails Bridge 的 WKWebView、非持久存储、自定义资源 Scheme、导航前拒绝和有预算的窄消息处理。实测和限制见 [ADR 0012](adr/0012-native-ui-boundary-probe.md)。内核现已补齐实例/包指纹绑定的会话和生命周期撤销，原生资源逐次经过会话检查，见 [ADR 0013](adr/0013-ui-session-revocation.md)。开发验证现已接入独立客体进程和私有管道，具备握手、运行超时、撤销通知和强制回收，见 [ADR 0014](adr/0014-ui-guest-supervision.md)。受限 `contributes.uiViews` 声明现已接入内核准备接口，按声明 ID 创建会话，桥版本不兼容和所有非空 UI 权限请求默认拒绝，见 [ADR 0015](adr/0015-declared-ui-views.md)。原生开发客体已补齐调用脚本异常、渲染退出和导航失败的终止处理，以及页面响应超时检测；脚本抛错、死循环和可响应慢页面已有实测，见 [ADR 0016](adr/0016-ui-renderer-failure-probes.md)。另提供 `make ui-acceptance`，在临时根目录准备手动验收页面并在正常关闭回调中清理，见[手动验收](ui-acceptance.md)和 [ADR 0018](adr/0018-disposable-ui-acceptance.md)。桌面现通过默认关闭的 `-plugin-ui` 开关，在可信管理插件中按声明 ID 打开独立客体窗口；普通客体不执行探测代码，也不接收探测专用动作，见 [ADR 0017](adr/0017-desktop-plugin-windows.md)。独立客体须通过 AppKit 事件取出、派发与窗口更新维持交互，不能只运行 NSRunLoop；修复和回归见 [ADR 0019](adr/0019-native-ui-event-dispatch.md)。独立视图命令可通过 `uiView` 关联声明，搜索回车由内核解析并打开；原有可信组件 `view` 语义不变，见 [ADR 0020](adr/0020-search-opens-independent-views.md)。复用窗口现通过会话绑定的私有聚焦控制保持输入，并支持可读标题和鼠标屏幕初始居中，见 [ADR 0021](adr/0021-window-focus-title-and-placement.md)。第三方 HTML 不进入 Shell；面向公共分发的稳定入口、通用权限授权服务、CPU/内存硬配额及跨平台承载仍待实现。

## 1. 定位与原则

构建对标 uTools 的插件驱动桌面效率平台，目标覆盖 Windows、macOS、Linux。

> Everything is a Plugin, but not Everything is in the Kernel.

一切用户可见的业务功能均由插件提供，包括应用检索、设置、插件管理、剪贴板、计算器和其他搜索结果提供者。宿主保留最小交互容器和恢复入口，内核只提供机制。

核心资产依次为：Plugin Manifest → Plugin Runtime → Extension Registry → Plugin SDK。

## 2. 分层与职责

采用微内核 + 插件运行时 + 扩展点架构。

| 层级 | 职责 | 替换与卸载 |
| --- | --- | --- |
| L0 Kernel | 插件注册、加载、生命周期、权限、通信、窗口、存储、搜索协调 | 宿主基础设施，不可卸载 |
| L1 System Plugins | 应用检索、设置、插件管理及命令业务 | 设置、插件管理禁止禁用及卸载；其他插件按普通策略管理 |
| L2 User Plugins | 剪贴板、翻译、JSON 工具等扩展 | 可安装、禁用和卸载 |

边界约束：

- 设置 UI 属于 `system.settings`；配置读写、校验、迁移和变更通知属于核心服务。
- 插件管理 UI 属于 `system.plugin-manager`；安装、卸载、启停和升级机制属于内核。
- 应用索引与检索属于 `system.app-search`；宿主负责受控系统能力、搜索调度和统一排名。
- 命令面板 Shell 负责输入框、结果容器和导航；业务命令、结果内容来自插件。
- Recovery Shell 属于宿主，提供安全模式、启动错误查看和默认插件恢复；不能依赖系统插件正常运行。

禁用应用检索插件后面板仍可打开；设置 UI 与配置服务保持分离；插件管理 UI 或插件进程崩溃不能使宿主失效。设置与插件管理的移除仅作为文件损坏等外部故障恢复场景，不提供正常卸载或禁用入口。

## 3. 插件模型与扩展点

插件可组合提供 Command、Search Provider、UI View 和 Background Service，不要求必须有 UI。

每个插件以 `manifest.json` 声明：

- 唯一 ID、名称、版本、API 版本和宿主兼容范围。
- `contributes`：命令、搜索提供者、视图及后续设置 Schema 等扩展点。
- `runtime`：运行模式和入口。
- `permissions`：所需能力及授权范围。
- `activationEvents`：按需激活事件。

支持启动、搜索、命令、视图、剪贴板、文件拖入及命名空间事件触发。区分 Installed、Activated、Running、Suspended、Deactivated，并设计异常隔离、重启和禁用处理；具体状态机在协议设计阶段确定。

默认按需激活，空闲插件按策略休眠或退出。后续可增加上下文菜单、widgets、文件处理器、工作流和 AI 工具扩展点。

插件包采用 ZIP 类封装，包含 manifest、资源、可选 UI、按平台区分的后端文件和签名信息。`.apkg` 仅为原会话暂定名称，尚未确定。

## 4. 运行时与协议

Wails 服务绑定不能代替动态插件运行时；宿主与插件之间建立稳定的 Plugin Host API。

| 运行模式 | 实施安排 |
| --- | --- |
| Declarative | MVP：声明式命令、静态配置和快捷链接 |
| Native Process | MVP：独立进程，原生插件优先 Go，协议保持语言无关 |
| JavaScript 沙箱 | 后续：受控后台 JS 运行环境，具体技术待验证 |

不使用 Go 原生 `plugin` 包，不在首版引入 Node.js 兼容或 uTools 插件兼容层。UI JavaScript、后台 JavaScript 与 Node.js 运行时分开设计。

Native Process 由 Supervisor 管理，通过 **JSON-RPC 2.0 over stdio** 通信。正式协议应包含标准 `jsonrpc: "2.0"` 字段；原文中的请求、响应示例为概念草图，不能直接作为完整规范。

协议设计需覆盖版本协商、请求和响应、错误、超时、取消、流式结果、心跳、背压和消息大小限制。具体方法、消息分帧与错误码在 `packages/plugin-protocol`、`schemas/protocol/` 中定义后实施。

插件间采用受控 Service RPC 和 Event Bus：服务声明、版本兼容及调用权限均须检查。事件使用 `core.*`、`system.*` 等命名空间，敏感事件订阅受权限控制。持久任务与后台调度使用 Scheduler Service。

## 5. 搜索扩展点

Search Coordinator 统一组织插件结果：

1. 根据声明的触发条件筛选 Provider。
2. 优先返回静态命令索引结果，无需唤醒插件。
3. 并行查询适用的动态 Provider，逐批合并结果。
4. 输入变化时取消旧查询，避免过时结果覆盖当前结果。
5. 限制每个 Provider 的结果量，由宿主结合相关性与使用频率统一排名。

不将每次输入广播给全部插件。Provider 提供评分与元数据，不能自行决定最终排名。

| 指标 | 初期验收目标 |
| --- | --- |
| 命令面板唤出 | P95 ≤ 100ms |
| 静态命令首批结果 | P95 ≤ 30ms |
| 本地应用结果 | P95 ≤ 100ms |
| 输入交互 | 不阻塞 UI |
| 插件崩溃 | 不影响 Shell |

这些是规划目标，尚无实测数据；实施时补充测试机器、数据规模、冷暖启动条件和采样方法。

## 6. UI 承载

| UI | 方案 |
| --- | --- |
| Shell | Wails 主 WebView，承载面板和统一导航 |
| 内置插件 | 经宿主认证的可信组件，仍遵循统一插件协议 |
| 第三方插件 | 独立安全上下文与受限消息桥，承载方式由原型验证决定 |
| 独立工具窗口 | 由宿主管理的原生窗口 |

2026-11-06 用户明确调整目标交互：插件界面默认在主窗口中使用，通过快捷键将当前视图拆为独立窗口。拆出应保留当前输入与页面状态；主窗口随后可继续搜索和打开其他插件，主窗口的呼出/隐藏不联动拆出的窗口。宿主退出或插件被禁用等生命周期撤销仍作用于对应客体。

已采用逻辑主面板与同进程 WebView 迁移实现 HTML 视图默认呈现和拆出，见 ADR 0022；自动回归通过，人工交互验收待补充。主面板继续保持可信 Shell 与插件页面的安全上下文、资源和消息桥分离，不把第三方 HTML 注入可信主页面。此方案不属于跨进程原生视图嵌入。

禁止把任意第三方 HTML 注入可信主页面，禁止暴露完整 Wails Bridge。需要限制资源来源、导航、CSP 和消息桥权限；分别验证 WKWebView、WebView2 及 Linux 目标环境的资源加载、焦点、输入法、拖放和窗口表现。

## 7. 设置、存储与安全

设置由四部分组成：内核 Settings Storage、插件 Settings Schema、设置插件 Settings UI、配置变更 Settings Events。通用 Schema 自动生成设置表单，复杂插件可提供专用页面。

首版推荐 SQLite：`core.db` 保存插件注册表、配置、授权、命令索引和历史记录；插件资源位于 `plugins/`，数据位于 `plugin-data/<plugin-id>/`，另设 `cache/` 和 `logs/`。

插件数据按命名空间隔离，由核心服务负责配额、事务和清理。敏感凭据通过系统 Keychain / Credential Manager 等安全存储能力管理。

权限覆盖私有存储、剪贴板读写、文件读写、应用读取与启动、网络请求、快捷键和进程执行。文件权限尽量限定目录，网络权限尽量限定域名。

区分 Sandboxed Plugin 与 Trusted Native Plugin。独立进程只提供故障隔离，普通原生进程仍可能拥有当前用户的系统权限；manifest 授权不等于操作系统沙箱。未建立可信边界前，不对普通第三方原生插件宣称安全隔离。

基础权限边界从最小运行时开始设计；公共分发前补齐签名、哈希校验、更新回滚、审核与恶意插件处置。

## 8. 技术与目录基线

| 模块 | 技术方向 |
| --- | --- |
| 桌面宿主 / 内核 | Wails 3 / Go |
| Shell UI | React + TypeScript |
| 样式 / 状态 | 推荐 Tailwind CSS / Zustand |
| 本地存储与索引 | 推荐 SQLite / FTS5 + 自定义排序 |
| 插件通信 | JSON-RPC 2.0 over stdio |
| 插件 SDK | TypeScript，第一阶段定义接口，后续完善工具链 |
| 插件 CLI | Go 或 Node.js，待定 |

版本在项目初始化时验证并锁定。第一阶段不引入 Redis、独立数据库服务、消息队列或微服务框架。

推荐按下列职责组织 Monorepo，按实际开发需要创建目录：

```text
apps/desktop/                 # Wails 入口及 frontend/
internal/kernel/              # 核心服务组合
internal/plugin/              # registry、supervisor、lifecycle、permissions、transport、activation
internal/search/              # 搜索协调、聚合与排名
internal/windows/             # 窗口机制
internal/storage/             # 存储底座
internal/platform/            # darwin、windows、linux 平台适配
packages/plugin-sdk/          # 插件开发接口
packages/plugin-protocol/     # 跨语言协议契约
packages/ui-components/       # 可复用 UI
packages/plugin-cli/          # 开发与打包工具
plugins/system-settings/
plugins/system-plugin-manager/
plugins/system-app-search/
plugins/system-calculator/
plugins/system-clipboard/
schemas/manifest.schema.json
schemas/protocol/
docs/adr/                     # 架构决策记录
```

## 9. 实施路线与验收

| 阶段 | 范围 | 验收 |
| --- | --- | --- |
| Phase 1 | 最小 Wails 宿主、Manifest、Registry、stdio RPC、生命周期、两个示例插件 | 独立插件无需重新编译宿主即可安装、执行、停止和卸载 |
| Phase 2 | 全局快捷键、搜索协调、静态命令索引、应用检索、设置、插件管理插件 | 应用搜索与配置管理由插件完成 |
| Phase 3 | 隔离 UI、插件窗口、SDK、调试、打包、热重载 | 外部开发者可独立编写带界面插件 |
| Phase 4 | 权限体系完善、签名、自动更新、跨平台验证、插件市场 | 支持可控的第三方插件分发与运行 |

优先完成三个技术验证：

1. **动态插件加载**：宿主启动后发现用户目录插件、启动进程并调用 RPC，无需重建宿主。
2. **动态 UI 加载**：验证第三方 UI 安全上下文、受控桥接、焦点及跨平台行为，确定承载方式。
3. **搜索扩展点**：两个独立插件提供结果，宿主完成并行查询、排序、取消和展示。

原会话未给出完整首版功能清单，不在本次文档整理中擅自补定。先验证机制，再扩展应用检索、设置等系统插件。

## 10. 待决事项与演进

- 完整 MVP 功能清单；首轮示例已确定为声明式问候与 Go 进程回声，首个验证平台为 macOS arm64。
- Wails 3.0.0 在其他目标平台的能力验证结果。
- 第三方 UI 最终承载方式和沙箱执行方案。
- Manifest 正式 Schema、协议分帧、API 兼容与版本迁移策略。
- 插件包后缀、CLI 实现语言、系统插件保护与替换策略。

后台 JS 沙箱作为后续运行时扩展；插件市场、Node.js 兼容和 AI Agent 不属于首版实施范围。AI 能力未来通过普通插件和既有权限、调度、结果协议接入。

架构变化通过 `docs/adr/` 记录，并同步更新本文。以用户最新明确指令为准；原始规划中的代码示例仅作为设计参考。


### 逻辑主面板与拆出（2026-11-06）

HTML 视图在固定独立客体中以无边框 NSPanel 呈现，与可信搜索 Shell 互斥显示，形成逻辑主面板。此实现没有把跨进程 NSView 嵌入 Wails 窗口。鼠标所在屏幕、横向居中、可用区域顶部 20% 和失焦隐藏延续搜索面板约定。Command + Space 控制当前主面板；返回释放客体并恢复搜索。

Command + Shift + D 或可信原生头部按钮在同一客体进程内迁移同一个 WKWebView 至普通 NSWindow，保留文档状态；父进程解除主面板路由，后续搜索显隐不影响独立窗口。窗口控制仅通过会话私有管道传递，页面桥仍只接受 ready / close。HTML 客体的其他平台承载尚未实现。见 [ADR 0022](adr/0022-main-panel-detach.md)。


### 可信组件页面拆出（2026-11-09）

设置和插件管理现可用同一 Command + Shift + D 或页头按钮拆出。宿主保留当前 Wails 窗口及其 WebView / React 树，将该窗口转换为有标题栏、可调整大小的独立工具窗口，再创建隐藏的新搜索面板接替主窗口角色。无需序列化设置草稿或重新挂载组件。主面板显隐、失焦隐藏与自动高度仅作用于新的主窗口；独立窗口失焦保持显示，关闭会释放。

macOS 保留原 NSPanel 的非激活原生类型，移除浮动、跨 Space 及全屏辅助等主面板特征，避免在现有 NSPanel 上切换激活位造成 WindowServer 焦点标签不一致。Wails Bridge 仅在原有可信组件上下文中保留，第三方 HTML 仍走独立客体。调用者由 Wails 上下文中的真实窗口身份确认，快捷键录入按实际窗口校验焦点；禁用、卸载、重载或升级插件会关闭对应已拆出的可信窗口。见 [ADR 0023](adr/0023-trusted-view-detach.md)。


### 窗口事件投递边界

主窗口呼出与可信视图拆出属于窗口局部事件，使用目标 WebView 的 DispatchWailsEvent 定向投递。Wails WebviewWindow.EmitEvent 走应用事件总线，Sender 仅标识来源，不指定接收窗口，不能用于窗口角色切换。独立页面也以同步角色引用拒绝 launcher:show 重置，防止迟到或误广播事件清空当前工具状态。见 [ADR 0024](adr/0024-window-local-events.md)。


### 系统界面插件保护

按用户指令，内核固定保护 system.settings 和 system.plugin-manager，拒绝 Uninstall 与 SetPluginEnabled(false)，管理界面根据只读 protected 元数据隐藏对应操作。保护由宿主策略决定，不接受插件 manifest 自行开启或关闭。有效历史禁用列表中的这两个 ID 在加载时被忽略，其他插件状态保留，启动不重写原始配置；配置损坏仍沿用默认拒绝策略。启用、重载和升级继续可用，缺失或损坏的包仍需恢复机制。见 [ADR 0025](adr/0025-protected-system-views.md)。


### 自定义窗口独立快捷键

设置插件在呼出快捷键后提供窗口独立快捷键，默认 macOS 为 Command + Shift + D。它是当前插件页的局部按键，不注册系统全局热键；支持按键录入、恢复默认和保存。宿主使用独立的 $window 命名空间及修订号保存，并拒绝与呼出键相同的组合。录入时保持焦点与窗口归属检查。

可信页面按保存后的组合匹配；独立 HTML 客体通过私有初始化数据及 detach-shortcut 配置帧接收更新，不重建页面。更新带有宿主侧单调修订，避免启动或通知并发导致旧值覆盖新值；父进程向管道写入时不持有窗口控制器锁。页面桥不增加任何消息能力，详见 [ADR 0026](adr/0026-configurable-detach-shortcut.md)。
