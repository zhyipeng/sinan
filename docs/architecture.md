# Sinan 架构指导方案

状态：项目架构基线，Phase 1 最小运行原型已实现；整体规划尚未完成。整理日期：2026-11-04。

来源：[规划插件化工具架构](chatgpt-conversation://6aeaf974-c4d4-83e8-bde5-978c154d8781)。用户已指定将该会话结论作为本项目指导方案。[原始规划存档](references/plugin-architecture-planning.md)用于追溯，本文用于指导实施。

本文保留原会话的架构结论，并区分基线、推荐方案与待决事项。原文涉及的 Wails 能力和版本状态未在本次整理中重新验证，实际开发应针对所选版本验证，不将原文中的“稳定”表述作为保证。

首轮已锁定 Wails 3.0.0、Go 1.26.0，完成 macOS arm64 最小宿主、本地插件目录安装与卸载、声明式和 Go 进程示例、按需激活及 stdio RPC。构建和真实进程测试已通过，桌面手动验证了筛选、执行与停止。详细范围及限制见 [ADR 0001](adr/0001-phase-one-runtime.md) 和[当前插件接口](plugin-protocol.md)。本轮已补充多 Provider 搜索与固定 UI 隔离原型，系统权限服务和公共第三方 UI 加载仍待实现。首版 macOS 应用检索已由可信原生系统插件提供。

按用户后续要求，Shell 已改为聚焦式交互：Command + Space 切换显示与隐藏，空输入框高 50 DIP，按结果调整高度，失焦隐藏，并提供托盘恢复与退出入口。macOS 呼出时选择鼠标所在显示器，使用原生可用区域水平居中，顶边距默认是可用高度的 20%，可在设置插件的「通用」中调整为 0%–50%，首次显示和后续呼出共用该偏好。当前搜索已统一插件声明的静态命令与动态 Search Provider。实现与验证边界见 [ADR 0002](adr/0002-launcher-interaction.md)。

macOS Shell 采用非激活 NSPanel，并配置 floating 层级及跨 Space / 全屏辅助窗口行为，避免呼出时激活宿主而切离全屏应用。该机制及实机验证边界同样记录在 ADR 0002。

本轮搜索契约、运行时并行和取消、定向结果投递以及 UI 隔离原型的边界见 [ADR 0003](adr/0003-search-and-ui-prototype.md)。

可信原生应用检索、直接动作及用途声明的阶段性边界见 [ADR 0004](adr/0004-application-search.md)。当前应用平台调用位于可信插件内，未来受控 Host API 尚未实现；用途声明不构成授权隔离。

应用目录与增量索引保留在应用插件；通用拼音别名工具已抽到 SDK，供应用名称与静态插件命令共同使用。宿主使用历史和有界本地持久化的实现边界见 [ADR 0005](adr/0005-search-refinement.md)。这不是完整的 SQLite / 通用插件存储服务。

配置存储、目录设置 Schema、初始化快照及随宿主构建的设置插件见 [ADR 0006](adr/0006-settings-service-and-view.md)。公共第三方 UI、SQLite 与通用插件数据服务仍未实现。

呼出快捷键支持在设置插件中按键录入、恢复默认和显式保存；宿主负责校验、运行时注册与核心配置持久化。使用保留的 `$launcher` 命名空间，不开放给插件配置接口。新组合先注册，冲突时保留原组合，存储失败时尝试回退注册。录入期间当前呼出热键转为定向录入事件。边界见 [ADR 0008](adr/0008-configurable-launcher-shortcut.md)。

开发期包指纹、重载、升级回退及宿主目录独占约定见 [ADR 0007](adr/0007-plugin-upgrade-and-reload.md)。日常本地管理界面已由 `system.plugin-manager` 提供，宿主保留最小恢复入口及可信视图错误边界，见 [ADR 0010](adr/0010-plugin-management-view.md)。该阶段尚未实现签名、插件市场与远程更新；后续插件商店首版见 ADR 0041。

应用检索 0.4.0 在插件内实现可丢弃的索引快照与后台校验；宿主仅通过可选 `initialize.cacheDir` 提供包外缓存目录。扫描不持有搜索快照锁，无缓存时首查仍需建立索引；已有缓存的结果可能短暂过期，启动前检查应用。兼容性与验证边界见 [ADR 0009](adr/0009-application-index-cache.md)。

插件启用策略由内核持久管理，界面仍在管理插件中。禁用后停止进程、拒绝激活与执行、排除所有搜索贡献，升级和重载保留状态；历史版本允许管理插件禁用自身，再通过宿主恢复入口重新启用；按用户后续指令，所有现有内置插件（应用检索、设置、插件中心）均禁止禁用及卸载，见 ADR 0049。策略保存失败保留原状态，策略损坏时阻止激活而不覆盖原文件。见 [ADR 0011](adr/0011-plugin-enablement.md)。

第三方 UI 边界验证新增只读资源快照构建器与独立的 macOS 原生开发宿主：使用无 Wails Bridge 的 WKWebView、非持久存储、自定义资源 Scheme、导航前拒绝和有预算的窄消息处理。实测和限制见 [ADR 0012](adr/0012-native-ui-boundary-probe.md)。内核现已补齐实例/包指纹绑定的会话和生命周期撤销，原生资源逐次经过会话检查，见 [ADR 0013](adr/0013-ui-session-revocation.md)。开发验证现已接入独立客体进程和私有管道，具备握手、运行超时、撤销通知和强制回收，见 [ADR 0014](adr/0014-ui-guest-supervision.md)。受限 `contributes.uiViews` 声明现已接入内核准备接口，按声明 ID 创建会话，桥版本不兼容和所有非空 UI 权限请求默认拒绝，见 [ADR 0015](adr/0015-declared-ui-views.md)。原生开发客体已补齐调用脚本异常、渲染退出和导航失败的终止处理，以及页面响应超时检测；脚本抛错、死循环和可响应慢页面已有实测，见 [ADR 0016](adr/0016-ui-renderer-failure-probes.md)。另提供 `make ui-acceptance`，在临时根目录准备手动验收页面并在正常关闭回调中清理，见[手动验收](ui-acceptance.md)和 [ADR 0018](adr/0018-disposable-ui-acceptance.md)。桌面现默认启用插件 UI（可用 `-plugin-ui=false` 关闭），在可信管理插件中按声明 ID 打开独立客体窗口；普通客体不执行探测代码，也不接收探测专用动作，见 [ADR 0017](adr/0017-desktop-plugin-windows.md)。独立客体须通过 AppKit 事件取出、派发与窗口更新维持交互，不能只运行 NSRunLoop；修复和回归见 [ADR 0019](adr/0019-native-ui-event-dispatch.md)。独立视图命令可通过 `uiView` 关联声明，搜索回车由内核解析并打开；原有可信组件 `view` 语义不变，见 [ADR 0020](adr/0020-search-opens-independent-views.md)。复用窗口现通过会话绑定的私有聚焦控制保持输入，并支持可读标题和鼠标屏幕初始居中，见 [ADR 0021](adr/0021-window-focus-title-and-placement.md)。第三方 HTML 不进入 Shell；面向公共分发的稳定入口、通用权限授权服务、CPU/内存硬配额及跨平台承载仍待实现。

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
| L1 System Plugins | 应用检索、设置、插件管理及命令业务 | 所有内置插件均禁止禁用和卸载 |
| L2 User Plugins | 剪贴板、翻译、JSON 工具等扩展 | 可安装、禁用和卸载 |

边界约束：

- 设置 UI 属于 `system.settings`；配置读写、校验、迁移和变更通知属于核心服务。
- 插件管理 UI 属于 `system.plugin-manager`；安装、卸载、启停和升级机制属于内核。
- 应用索引与检索属于 `system.app-search`；宿主负责受控系统能力、搜索调度和统一排名。
- 命令面板 Shell 负责输入框、结果容器和导航；业务命令、结果内容来自插件。
- Recovery Shell 属于宿主，提供安全模式、启动错误查看和默认插件恢复；不能依赖系统插件正常运行。

应用检索插件异常时面板仍可打开；设置 UI 与配置服务保持分离；插件管理 UI 或插件进程崩溃不能使宿主失效。内置插件的移除仅作为文件损坏等外部故障恢复场景，不提供正常卸载或禁用入口。

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

插件后台协调的原生快捷键调用采用可取消的主线程投递；退出回调取消并等待协调后清理插件与目录租约，剩余全局快捷键由 Wails 统一释放，避免主线程与后台循环相互等待。见 [ADR 0062](adr/0062-plugin-runtime-shutdown.md)。


### 逻辑主面板与拆出（2026-11-06）

HTML 视图在固定独立客体中以无边框 NSPanel 呈现，与可信搜索 Shell 互斥显示，形成逻辑主面板。此实现没有把跨进程 NSView 嵌入 Wails 窗口。鼠标所在屏幕、横向居中、可用区域顶部 20% 和失焦隐藏延续搜索面板约定。Command + Space 控制当前主面板；返回释放客体并恢复搜索。

Command + Shift + D 或可信原生头部按钮在同一客体进程内迁移同一个 WKWebView 至普通 NSWindow，保留文档状态；父进程解除主面板路由，后续搜索显隐不影响独立窗口。窗口控制仅通过会话私有管道传递，页面桥仍只接受 ready / close。HTML 客体的其他平台承载尚未实现。见 [ADR 0022](adr/0022-main-panel-detach.md)。


### 可信组件页面拆出（2026-11-09）

设置和插件管理现可用同一 Command + Shift + D 或页头按钮拆出。宿主保留当前 Wails 窗口及其 WebView / React 树，将该窗口转换为有标题栏、可调整大小的独立工具窗口，再创建隐藏的新搜索面板接替主窗口角色。无需序列化设置草稿或重新挂载组件。主面板显隐、失焦隐藏与自动高度仅作用于新的主窗口；独立窗口失焦保持显示，关闭会释放。

macOS 保留原 NSPanel 的非激活原生类型，移除浮动、跨 Space 及全屏辅助等主面板特征，避免在现有 NSPanel 上切换激活位造成 WindowServer 焦点标签不一致。Wails Bridge 仅在原有可信组件上下文中保留，第三方 HTML 仍走独立客体。调用者由 Wails 上下文中的真实窗口身份确认，快捷键录入按实际窗口校验焦点；禁用、卸载、重载或升级插件会关闭对应已拆出的可信窗口。见 [ADR 0023](adr/0023-trusted-view-detach.md)。


### 窗口事件投递边界

主窗口呼出与可信视图拆出属于窗口局部事件，使用目标 WebView 的 DispatchWailsEvent 定向投递。Wails WebviewWindow.EmitEvent 走应用事件总线，Sender 仅标识来源，不指定接收窗口，不能用于窗口角色切换。独立页面也以同步角色引用拒绝 launcher:show 重置，防止迟到或误广播事件清空当前工具状态。见 [ADR 0024](adr/0024-window-local-events.md)。


### 内置插件保护

按用户指令，内核固定保护 system.settings、system.plugin-manager 和 system.app-search，拒绝 Uninstall、UninstallChecked 与 SetPluginEnabled(false)，管理界面根据只读 protected 元数据隐藏对应操作。保护由宿主策略决定，不接受插件 manifest 自行开启或关闭。有效历史禁用列表中的这三个 ID 在加载时被忽略，其他插件状态保留，启动不重写原始配置；配置损坏仍沿用默认拒绝策略。启用、重载和升级继续可用，缺失或损坏的包仍需恢复机制。原始决策见 [ADR 0025](adr/0025-protected-system-views.md)，当前全部内置插件保护规则见 [ADR 0049](adr/0049-protect-all-builtin-plugins.md)。


### 自定义窗口独立快捷键

设置插件在呼出快捷键后提供窗口独立快捷键，默认 macOS 为 Command + Shift + D。它是当前插件页的局部按键，不注册系统全局热键；支持按键录入、恢复默认和保存。宿主使用独立的 $window 命名空间及修订号保存，并拒绝与呼出键相同的组合。录入时保持焦点与窗口归属检查。

可信页面按保存后的组合匹配；独立 HTML 客体通过私有初始化数据及 detach-shortcut 配置帧接收更新，不重建页面。更新带有宿主侧单调修订，避免启动或通知并发导致旧值覆盖新值；父进程向管道写入时不持有窗口控制器锁。页面桥不增加任何消息能力，详见 [ADR 0026](adr/0026-configurable-detach-shortcut.md)。


### 插件命令拼音匹配

静态命令检索在原有文字包含匹配之后，使用插件名、命令标题及关键词的拼音别名补充匹配，支持带空格全拼、连续全拼和首字母，原中文标题与命令身份保持不变。通用工具位于 SDK 的 searchtext，不依赖应用插件业务代码；应用索引、目录扫描及动态 Provider 候选仍由各插件维护。宿主保留最多 2048 条文本别名缓存，超长输入不缓存，当前快照中的名称变化自然使用新键。平台转换和多音字限制与原应用检索一致，见 [ADR 0027](adr/0027-plugin-command-pinyin.md)。

## 核心异常恢复补充

退出监督现避免后代继承 stderr 拖住直接进程回收；设置和插件管理提供独立的内容指纹绑定修复事务，普通禁用、卸载限制保持；HTML 主面板在候选准备成功后才替换原会话。实现边界见 [ADR 0028](adr/0028-core-recovery-and-lifecycle.md)。

## UI 业务交互首版

UI 桥 2 增加按会话、实例和清单白名单约束的本插件后端命令调用，以及显式声明的插件私有 JSON 文档；桥 1 保持 ready / close。首个实用 HTML 插件为 tools.json，格式化与压缩由 Go 插件后端执行，草稿由核心按插件持久化。没有增加完整 Wails Bridge、任意路径访问或通用权限授权器，独立进程仍不等同 OS 沙箱。详见 [ADR 0029](adr/0029-ui-calls-private-data-json-tool.md)。

## 插件私有数据管理

可信管理插件提供按需占用列表与二次确认清理，包含卸载后保留的文档；内核只暴露元数据，按文件状态指纹核对后删除固定文档，并撤销旧 HTML 会话阻止自动保存回写。插件包、设置、历史和缓存保持独立，管理接口不进入第三方页面桥。详见 [ADR 0030](adr/0030-plugin-data-management.md)。


## 权限系统暂缓

2026-11-10：按用户指令撤回显式 UI 授权系统。桥 2 继续按清单白名单和 storage 声明限制调用，不提供授权管理或弹窗；原生插件安装信任与隔离机制保持。已有授权记录不再参与运行判断，不自动改写日常配置。权限系统列为延期事项，见 [ADR 0031](adr/0031-defer-ui-permissions.md)。

## 主题与外观首版

2026-11-11：system.settings 增加浅色、深色和跟随系统（默认）；宿主按 `$appearance` 命名空间保存并同步所有可信窗口和隔离 HTML 客体。共享数据主题契约、语义 CSS 变量和被动 SDK 事件，不重载页面或扩大页面桥能力。内置主题先使用统一 Registry，预留后续插件提供主题的入口；主题插件清单与动态加载尚未实现。协议、兼容与验证边界见 [ADR 0032](adr/0032-theme-contract-and-appearance.md)。

## 窗口内设置快捷入口

2026-11-11：Command + ,（macOS）或 Ctrl + ,（其他平台）在可信窗口内进入设置；macOS HTML 客体通过原生私有动作转交宿主。复用已拆出的设置窗口，保持其他独立窗口，释放被替换的附着 HTML 面板。无新增全局热键或页面桥权限，见 [ADR 0033](adr/0033-settings-navigation-shortcut.md)。


## 内置插件与官方插件仓库边界

2026-11-11：按用户指令，本体仓库只保留应用检索、设置、插件管理三个内置插件；JSON 工具 `tools.json` 迁入同级独立 Git 仓库 `sinan-official-plugins`，通过普通插件安装机制使用，不再由本体开发脚本自动安装。独立 `sinan-plugin-registry` 当前仅登记官方 JSON 工具资料，无可安装版本。SDK 尚未正式发布，官方插件暂使用带来源提交和文件摘要的最小 SDK 快照，不依赖宿主 internal 包或本体源码目录。插件 ID、后端 API 1、UI 桥 2、原生信任和隔离边界保持不变；未创建 GitHub 远端，未实现商店、远程更新或目录签名。见 [ADR 0034](adr/0034-official-plugin-repositories.md)。


## 插件项目创建入口

2026-11-11：`system.plugin-manager` 在已有管理能力后增加两种独立开发模板：Go 后端 + HTML、纯 HTML。模板资源和生成逻辑属于管理插件，宿主提供仅供可信组件使用的类型化目录选择/创建适配器；公开协议与 SDK 提供最小源码快照，生成项目不依赖宿主 internal 或源码目录。创建不构建、不安装、不初始化 Git，也不自动注册商店身份。沿用后端 API 1、UI 桥 2 和现有信任策略。原生目录对话框期间暂停其主面板的失焦隐藏和呼出重置，关闭后恢复。实现及限制见 [ADR 0035](adr/0035-plugin-project-templates.md) 和[开发模板](plugin-development.md)。


## 默认启用插件界面

2026-11-11：按用户指令移除普通使用流程中的 UI 启动门槛，`-plugin-ui` 默认 true，`make run` / `make dev` 可直接打开已安装 HTML 插件，`make dev-ui` 保留为兼容别名。`-plugin-ui=false` 显式关闭，隔离探测模式仍不初始化普通插件 UI。仅 macOS 已实现承载；平台不支持或 helper 初始化失败时报告具体原因。默认启用不自动打开插件、不改变按需激活，也不扩大页面桥、资源、原生信任或权限边界。见 [ADR 0036](adr/0036-plugin-ui-default-enabled.md)。


## 开发链接与模板发布工具

2026-11-11：模板 v2 增加单次开发构建与默认/自定义插件目录软链接、标签驱动的 Release 工作流、可选目录审核 PR 和项目级 agent skill。宿主仅在插件根目录允许外部开发链接，仍拒绝包内链接及 system.* 开发链接；原生开发信任由用户在可信插件管理界面确认，绑定规范化目标路径，普通安装信任不能替代该确认。开发插件按指纹显式重载，不使用普通升级事务；卸载只移除链接。目录发布 PR 只进入 submissions 审核记录，不构成已上架、签名或宿主远程分发能力。见 [ADR 0037](adr/0037-plugin-template-dev-release-skill.md)。


## React 主面板模板

2026-11-11：用户确认模板应与插件管理一样默认在主面板使用，保留返回与拆出，同时继续动态加载和隔离。模板 v3 将两种业务页面改为 React（Go + React、无后端 React），使用锁定依赖与本地 esbuild 打包；界面只渲染业务内容，复用宿主头部及主题样式。现有 Panel 默认路由不变，不把第三方 React 注入可信 Shell，不扩大 Wails Bridge。当前仍是 macOS 隔离客体提供逻辑主面板，不宣称同 WebView 内嵌。详见 [ADR 0038](adr/0038-react-main-panel-template.md)。


模板项目的 agent 开发指引按用户后续要求直接生成至根目录 `AGENTS.md`，不再单独生成项目 skill。主面板/React、协议和发布约束保持，已生成项目不自动迁移。沿用 ADR 0037 的后续调整记录。


## 插件页头风格统一

2026-11-11：按用户要求，隔离客体的宿主原生页头对齐可信插件管理的 58 DIP 高度、16 DIP 水平留白、15pt 标题、箭头返回/拆出和主题色。视图标题与插件来源拆分显示，窗口标题保持原语义；拆出时迁移同一内容树并保留页头，将返回切换为关闭、隐藏拆出按钮。第三方页面不绘制宿主导航，也不获得新桥接权限。宿主与 UI helper 需同步构建。见 [ADR 0039](adr/0039-unified-plugin-header.md)。


## 真正的主窗口内插件承载

2026-11-11：用户明确要求与插件管理在同一个主窗口内切换页面，统一焦点、尺寸和返回体验。本轮普通 macOS 路径改为在现有 NSWindow 中添加独立配置、非持久存储的 WKWebView 内容子视图；标题栏直接复用可信 Shell 的 React 页头，返回/拆出/设置和尺寸均走原窗口路径。拆出保留整个 Wails 窗口及原生子视图树，创建新搜索窗口接替主角色。第三方 DOM 与 Wails WebView 分离，不复用其配置、脚本或完整 Bridge。

这明确替代 ADR 0022 的另一个 NSPanel 模拟主面板方案，以及 ADR 0039 普通路径的原生仿制页头。UI 原生控制器现在进入宿主进程，不能继续声称独立 UI 客体进程故障隔离或独占 WebContent 进程；插件后端进程不变。WebKit 导航/CSP/受限桥接、会话撤销、预算和响应检测继续；普通模式不要求 helper。见 [ADR 0040](adr/0040-same-window-plugin-content.md)。


### 内置插件商店（2026-11-12，独立身份已由 ADR 0048 合并）

按用户后续要求，新增 `system.plugin-store`，在同一主窗口提供 GitHub 目录浏览、搜索、详情、安装及手动更新。业务契约和 UI 位于插件层，宿主绑定可信组件，通用资产下载和 ZIP 暂存提供机制，最终安装复用内核的信任确认、指纹与升级回退。目录读取固定 main 分支中经审核的 plugins 版本记录，不消费 submissions；尚无发布资产时显示待发布。当时该插件可禁用、卸载；现已合并为受保护的插件中心，见 ADR 0048。

已实现固定资产 ID、大小及 SHA-256、受限解压、内部安装预览与应用前重新核验（普通二次确认现已取消，见 ADR 0050）。独立目录签名、密钥轮换、完整撤销治理和自动更新仍未实现，公共第三方分发前的签名要求尚未满足。不得把此首版描述为完整公共生态分发方案。边界见 [ADR 0041](adr/0041-builtin-plugin-store.md) 与[插件商店](plugin-store.md)。


### 官方 JSON 工具纯前端替换（2026-11-12）

按用户指令，官方仓库中的 `tools.json` 0.2.0 改为“JSON 工具”，采用 `vanilla-jsoneditor` 文本模式，先提供格式化与压缩。旧 Go 后端已移除，改用 declarative 运行时；数值处理、编辑和校验在隔离页面本地完成，保留原有私有草稿结构和 UI Bridge 2 storage。宿主协议、CSP 与隔离边界不变。插件构建提取组件 CSS、以 CSSOM 挂载 CodeMirror 样式并拆分本地资源，要求 Safari 16.4+ 对应 WebKit。无界面测试及打包通过，真实 UI 未验收，未发布目录版本。详见 [ADR 0042](adr/0042-frontend-json-tool.md)。


### 声明式内容匹配与视图输入（2026-11-12）

命令可通过通用 `inputMatchers` 声明受预算限制的 RE2 正则或 JSON 顶层类型匹配，宿主在本地生成携带原文的候选，不为此启动插件。用户选中后，以新会话打开 HTML 视图；UI 桥 2 的只读 `launch.get` / SDK `readLaunch()` 仅返回该会话的显式打开参数，不提供主输入框订阅。旧无参数打开、名称搜索与动态 Provider 保持兼容，大文本仅参与最多 64 KiB 的本地内容匹配。官方纯前端 JSON 工具 0.3.0 只声明对象/数组，带入内容在实际编辑后保存。具体规则数量、正则复杂度与工作量、兼容及隔离边界见 [ADR 0043](adr/0043-declarative-input-matching.md)。


### 粘贴模式与图片输入（2026-11-12）

空主输入框的显式粘贴现进入预览与结果内筛选模式，退格在筛选词清空后退出，短文本恢复、长文本和图片清空。匹配声明增加桥 2 HTML 视图专用 `kind:image`，图片以限大小、限像素的静态 PNG 在内存验证，并作为选中视图的会话资源提供；`readLaunch()` 返回图片元数据与该会话地址，不开放系统剪贴板订阅、任意文件或网络。Esc 在各插件页面经可信/原生路径回到清空的主输入状态，空主输入再次 Esc 隐藏。图片资源、序列清理、helper 帧预算和兼容边界见 [ADR 0044](adr/0044-paste-mode-and-escape.md)。真实 UI 验收未执行。

插件中心通过宿主注册的可信文本匹配入口，为符合发行命名规则的 ZIP 绝对路径提供「安装插件」结果。搜索协调器沿用结果排序、可用性和取消机制，插件中心复用本地安装界面和包检查。见 [ADR 0087](adr/0087-local-zip-search.md)。


### 本地 ZIP 安装（2026-11-12）

插件管理在目录安装后增加 ZIP 文件选择及路径输入，宿主通过通用受限解压机制为检查和安装分别建立临时快照，操作结束即清理。安装继续由内核核对候选包与已安装包的内容指纹，并复用原生信任确认和升级回退。ZIP 根目录须包含 manifest.json；第三方页面桥、协议、权限和签名能力不变。真实文件对话框交互尚未验收，详见 [ADR 0045](adr/0045-local-zip-install.md)。


### 可选插件图标（2026-11-12）

Manifest 可选 `icon` 指向包内 SVG、PNG、JPEG，宿主限大小读取及静态 SVG 校验后以图片 data URL 提供给可信界面。插件管理、商店、搜索结果及同窗口插件页头显示图标，搜索结果原有项目图标优先，无资源时显示插件名首字。商店目录支持可选内嵌图标；图标不进入可信 DOM、不增加 Bridge 权限。旧清单仍有效，新字段需要支持该字段的宿主。见 [ADR 0046](adr/0046-plugin-icons.md)。


### 默认窗口尺寸（2026-11-12）

主搜索面板宽 800 DIP，空输入高度仍为 50 DIP，结果出现后按内容增长，最高 600 DIP。可信插件页与隔离 HTML 插件页默认使用 800×600 DIP，拆出的窗口也初始化为该尺寸并允许调整；主面板及原生客体继续受屏幕可用区域约束。承载与隔离边界保持不变，见 [ADR 0047](adr/0047-default-window-dimensions.md)。


### 统一插件中心（2026-11-12）

按用户确认，将插件管理与商店合并为 `system.plugin-manager` 0.7.0，名称「插件中心」，保留唯一管理视图与旧名称搜索关键词。默认「已安装」，「发现」按需请求目录；共用详情和管理操作，本地安装与开发 / 数据工具收纳为次要入口。目录服务与业务界面均归管理插件，宿主继续仅做类型化适配，原下载、解压、指纹、信任与升级机制保持。

独立 `system.plugin-store` 不再随构建安装或注册可信视图。普通桌面启动仅按已知旧包完整指纹迁移，先升级中心再核验移除旧商店；未知或修改包保留并提示，配置、私有数据、使用记录和缓存保留。中心沿用管理插件的禁止禁用 / 卸载及宿主恢复规则；第三方隔离、协议与签名治理边界不变。详见 [ADR 0048](adr/0048-unified-plugin-center.md) 与[商店使用文档](plugin-store.md)。


### 所有内置插件受保护（2026-11-12）

按用户指令，禁止禁用和卸载的宿主策略现覆盖全部内置插件：`system.app-search`、`system.settings`、`system.plugin-manager`。内核拒绝普通及带指纹核验的卸载和显式禁用；可信界面使用既有只读 `protected` 元数据隐藏对应操作。历史有效禁用记录中的这些 ID 在加载时被忽略，保留其他插件状态且不主动重写配置；损坏配置仍默认拒绝激活。停止、重载、升级与原有恢复机制保持，不将原生应用检索加入声明式系统视图修复白名单。旧商店身份已退役，不在当前内置保护集合中，继续支持已知包迁移清理。见 [ADR 0049](adr/0049-protect-all-builtin-plugins.md)。


### 安装与更新取消普通二次确认（2026-11-13）

按用户指令，插件中心的商店卡片、详情以及本地目录 / ZIP 安装在单次点击后自动衔接检查与应用，声明式和纯前端插件不再展示普通二次确认。后续按用户指令，原生插件也取消额外确认，单次安装 / 更新点击授权本次已检查候选，见 ADR 0058。服务端 Prepare / Apply、单次票据、目录重新核验、资产摘要、源包及已安装包内容指纹、升级回退保持；恢复系统插件与开发链接信任流程独立。见 [ADR 0050](adr/0050-direct-plugin-installation.md)。


### Windows 与 Linux 同步适配基础（2026-11-13）

按用户指令同步推进 Windows/Linux。首轮将应用检索的目录、条目类型及启动机制从 macOS 假设中拆出，增加开始菜单 `.lnk` / `.exe`、XDG `.desktop` 发现和平台原生启动，业务仍在应用插件。构建与开发准备统一为可供 PowerShell/Make 使用的 Node 入口，Linux 显式采用 GTK3/WebKitGTK 4.1，增加三平台原生构建 CI。Windows 原生入口改为 `.exe` 校验，原生后端仅额外接收显式平台环境白名单以支持目录发现及桌面会话，不整表继承宿主环境。协议、第三方 UI 隔离与权限边界不变；Windows/Linux HTML 客体仍未实现，原生 UI、系统主题、屏幕定位及发行安装流程继续待补。新增 CI 和目标平台运行尚未验证，不将交叉编译等同于平台验收。详见 [ADR 0051](adr/0051-cross-platform-foundation.md) 与[跨平台适配](cross-platform.md)。


### 插件商店 GitHub 镜像加速（2026-11-13）

按用户指令为插件商店增加可选 GitHub 镜像加速。设置项类型由 `directory-list` 扩展出 `select` 与 `text`，插件中心 0.8.0 在系统设置页声明「GitHub 加速服务」与「自定义加速模板」。目录读取与发行资产下载共用 `internal/plugin/ghmirror` 的模板候选：默认官方优先、失败逐个回退内置镜像（`github.chenc.dev`、`gh-proxy.com`），可选仅官方、指定镜像或自定义模板；模板仅支持 `{url}` 且限 HTTPS。为兼容镜像规范化跳转，允许同主机 HTTPS 重定向，跨主机仍拒绝。镜像只改变请求出口，不读取本机凭据，不改变目录校验、资产摘要、ZIP 限制、指纹与信任流程。详见 [ADR 0052](adr/0052-github-mirror-acceleration.md) 与[插件商店](plugin-store.md)。


### 原生 UI 共用边界与 Shell 适配（2026-11-13）

后续将初始导航、GET 资源、实际来源/主 frame、预算及撤销后的调用结果校验统一到 Go 原生 UI 边界，并接入 macOS 普通承载和 helper，为独立 WebView2/WebKitGTK 适配器提供共用机制。Windows 新增光标屏幕和系统主题适配；Linux GTK3 新增 X11 光标屏幕与 GDK 工作区、GNOME 主题偏好读取/观察，Wayland 定位明确回退。两端托盘使用普通应用图标。Windows/Linux 原生插件 UI renderer 仍未实现，不使用可信 Wails WebView 降级承载；边界与验证限制见 [ADR 0053](adr/0053-shared-native-ui-boundary.md)。


### GTK3 同窗口隔离内容承载（2026-11-13）

Linux GTK3 现实现独立 ephemeral WebKitContext、独立用户脚本与命名隔离 world 的原生内容子视图，保留 58 DIP 可信页头和现有准备/提交/撤销/拆出路径。GTK 消息信号没有 WKFrameInfo，来源转交通过仅注入顶层文档的隔离 world 限定可信同窗口消息，并在原生/Go 侧继续绑定精确文档和会话；不将页面声明作为来源证明，不暴露 Wails Bridge。要求 WebKitGTK 2.40+ 的 4.1 API，普通嵌入能力与旧独立 helper 分开判定。Windows WebView2 renderer 仍待实现；无窗口回归不等于 GTK 实机验收。见 [ADR 0054](adr/0054-gtk-isolated-plugin-content.md)。

### Windows 同窗口隔离内容承载（2026-11-14）

Windows amd64/arm64 现接入独立 WebView2 Environment、InPrivate Profile 和原生 Controller，保留可信页头及现有准备/提交/撤销/拆出路径。资源通过每次视图独立的虚拟 HTTPS 源映射到绑定快照，全部资源事件由宿主处理，不映射磁盘目录或启动 HTTP 服务；消息来源取自引擎顶层事件。仅兼容 SDK 窄桥，不注册 Wails Bridge、Host Object 或通用 CDP 接口。临时配置目录可写且可能残留，不将 InPrivate 描述为内存存储或 OS 沙箱。交叉编译、静态检查与无窗口边界测试已通过，Windows 原生 COM 测试仅编译，实际引擎、焦点和缩放仍待实机验收。见 [ADR 0055](adr/0055-webview2-isolated-plugin-content.md)。

Windows 主窗口可接管一个预热的空 WebView2 Controller。每个控制器和随附的独立 Environment、InPrivate Profile 只绑定一次插件会话，并随会话回收。宿主先建立专用子 HWND，通过它管理内容区的层叠及显示。见 [ADR 0083](adr/0083-webview2-warm-controller.md)。

### 官方剪贴板与插件系统输入（2026-11-14）

官方插件仓库新增 `tools.clipboard` 0.1.0，以经信任的原生后台进程提供文本/图片监听、去重、有界本机持久化与隔离选择界面。宿主新增声明式视图热键、原生 `onStartup` 激活、可选 `initialize.dataDir` 和按会话/清单限定的无参数 `input.paste`；页面不能指定输入目标或任意按键。默认 Command/Ctrl + Shift + V 直接打开，选择后恢复原应用并粘贴。剪贴板业务和数据格式留在插件，不自动安装或加入内置保护集合。macOS 自动粘贴需辅助功能权限，缺少时由宿主原生弹窗提供辅助功能设置跳转；Linux 自动输入仅支持 X11 + xdotool，Wayland 可复制。原生目录未纳入现有 UI 文档清理，数据未加密，原生用途声明仍不是 OS 沙箱。无界面测试通过，真实 UI 未验收，见 [ADR 0056](adr/0056-clipboard-plugin-and-system-input.md)。


### 集中配置插件快捷键（2026-11-14）

`system.settings` 0.4.0 在已有设置后增加「插件快捷键」，按有效插件的视图命令热键声明自动列出，支持录入、恢复默认与保存即生效。宿主以保留命名空间按插件/命令保存用户覆盖，复用注册事务、失败回退与定向录入；双向检查呼出、窗口独立及其他插件组合冲突。禁用/卸载释放注册、保留覆盖，恢复启用或重新安装后恢复配置。第三方页面桥无新增配置权限，见 [ADR 0057](adr/0057-configurable-plugin-shortcuts.md)。


### 原生安装与更新取消额外确认（2026-11-14）

`system.plugin-manager` 0.9.0 的商店卡片、详情页及本地目录 / ZIP 安装，对原生包同样在检查通过后直接应用。可信组件根据本次候选类型传递原生信任，点击安装 / 更新即授权本次操作；不再有「信任并安装 / 更新」额外步骤。核心信任参数、包及安装基线指纹、商店票据和目录核验、升级回退继续，开发链接及恢复入口沿用独立流程。协议和隔离页面权限无变化，见 [ADR 0058](adr/0058-direct-native-plugin-installation.md)。

### 桌面自动构建与 Release（2026-11-14）

桌面宿主新增六个 OS/架构原生 runner 的复用构建流程；推送有效 `v*` 版本标签后，全部打包与无界面检查成功才公开 GitHub Release。归档附带 SHA-256，预发布与已公开版本保护独立处理。构建将三个内置插件的最小运行快照嵌入宿主，默认用户目录首次正常启动经既有内核安装缺失插件，信任范围仅限内嵌应用检索原生后端；已有安装不自动覆盖。业务、协议和第三方隔离边界不变，不构成完整签名、安装器或自动更新。远端 Actions 与真实发行待验证，见 [ADR 0059](adr/0059-desktop-release-automation.md) 和[发布流程](releases.md)。

### 桌面宿主自动更新（2026-11-14）

按用户选择，发行实例启动后自动检查稳定新版，由用户在设置中点击下载并重启安装。检查策略、状态与界面留在设置插件；宿主适配生命周期，`internal/hostupdate` 与无 UI 的独立助手仅提供包暂存、校验、目录替换和就绪失败回退机制。源固定为官方仓库的 HTTPS Release，核对平台资产、SHA-256、包元数据和 macOS Bundle 签名；不新增第三方桥权限。已有插件不随宿主自动覆盖，不自动提权，没有独立发布者签名清单或断电恢复保证。真实退出替换、重启与系统策略待实机验收，见 [ADR 0060](adr/0060-host-auto-update.md) 和[自动更新](auto-update.md)。

### 内容匹配分数（2026-11-14）

清单新增可选 `commands[].inputMatchScore`（0～0.95，必须同时声明输入匹配规则），允许宽泛内容入口降低优先级；省略保持 0.95。名称/关键词/拼音命中仍至少 0.85，精确标题为 1；粘贴入口按同一内容分数排序。翻译插件使用 0.2，内核不按插件 ID 编码业务特例。输入、RPC、UI 桥和权限边界不变；旧宿主拒绝该字段，插件需要同步更新宿主。见 [ADR 0061](adr/0061-content-match-ranking.md)。

### core 加密私有存储与 LLM 翻译（2026-11-14）

按用户选择，core 提供 AES-256-GCM 加密插件私有文档与本地主密钥自动解锁，通过 UI Bridge 2 的 `storage.encrypted` 显式权限访问；解密数据仅交给所属授权插件页面，主密钥与密文同时可读时仍可解密。插件中心只管理元数据及清理，LLM 配置、提示词和请求留在独立翻译插件。接口、SDK、跨仓库兼容及静态加密边界见 [ADR 0063](adr/0063-encrypted-plugin-storage-and-llm-translation.md)，不等同于 OS 沙箱或系统钥匙串。

### 全局共享 LLM 能力（2026-11-14）

按用户新指令，LLM 配置、加密凭据与通用请求执行由 core 统一提供；配置 UI 属于设置插件，提示词、输入与业务语义属于调用插件。多套全局配置供声明 `llm` 的插件按请求 ID 选择，未指定时采用插件覆盖或全局默认；Key 不返回插件。任务由会话绑定和生命周期取消管理。翻译 0.3.0 移除自己的配置管理和 LLM HTTP 客户端；开发期直接忽略 0.2.0 私有配置，不实现迁移。此段更新 ADR 0063 的 LLM 业务承载安排，加密存储机制继续。见 [ADR 0064](adr/0064-shared-llm-capability.md)。

### 插件独立版本与发布标签

目录接受 `<plugin-id>-v<version>`，并兼容既有 `v<version>`；版本唯一性按插件 ID 判断，同仓库不同插件可使用相同版本号。字段、资源和权限边界不变；旧宿主需升级才能读取新标签记录，发行前按此顺序部署，不改写历史资产。见 [ADR 0065](adr/0065-independent-plugin-release-tags.md)。

### 独立官方计算插件

`sinan-official-plugins/plugins/calculator` 新增 `tools.calculator` 0.1.0，以现有进程 Search Provider、direct 命令、隔离 UI Bridge 2 和私有 storage 实现主搜索计算与多行计算稿纸。表达式解析、数值运算、稿纸历史与复制业务均留在插件层，宿主协议、SDK、权限和隔离边界不变；未引入新的内核计算能力。三平台依赖与已验证范围见[跨平台适配](cross-platform.md)及[计算插件说明](../../sinan-official-plugins/plugins/calculator/README.md)；当前为本地构建与无界面验证，尚未正式发布或实机 UI 验收。

## 宿主共享系统剪贴板

系统剪贴板文本/PNG 读写现由宿主提供，原生插件通过带清单权限校验的双向 stdio Host API 访问；初始化能力标识为 `clipboard.v1`。计算、剪贴板历史及轮询仍留在独立官方插件，两个插件的 0.2.0 源码已移除重复的平台读写实现。UI Bridge 2 另提供受视图权限与既有预算约束的文本 API；不继承后端权限、不开放完整 Wails Bridge，不构成 OS 沙箱。版本、传输与平台边界见 [ADR 0066](adr/0066-host-clipboard-api.md)。宿主 v0.2.1 与配套插件 0.2.0 已发布，商店最低宿主版本为 0.2.1；实际系统剪贴板与 UI 尚未验收。

## 官方 OCR 私有工作进程

官方仓库本地实现 `tools.ocr` 0.1.0，复用现有剪贴板和 UI Bridge 2，以异步命令及租约承接识别，业务与模型留在插件层。每任务启动独立 native worker，通过私有 stdin 管道感知父进程退出；正常取消后 Wait 回收，后台强杀时 worker 自行退出，并设独立硬超时。没有扩展宿主对任意后代进程的回收能力，没有新增公共协议、SDK 或 Wails Bridge。

macOS arm64 已构建并完成模拟 Host API 的无界面识别与回收验证；完整原生打包暂限 macOS 本机目标，Windows/Linux 明确未适配完成，六目标后台编译不能替代完整插件支持。首轮仅显式识别剪贴板图片；下述会话图片扩展已补充主搜索图片匹配，仍未实现截图捕获。发行状态见下述正式发行记录，实机 UI 仍未验收。详细决策、权限及验证边界见 [ADR 0067](adr/0067-ocr-private-worker-lifecycle.md)。


## 会话图片交付与 OCR 图片入口

宿主 v0.3.1 新增 `ui.image-command.v1`：宿主将当前会话 PNG 快照交给已授权的同插件后台命令，页面只传会话图片 URL；另提供受视图权限约束的剪贴板图片快照及预览资源。OCR 注册 image matcher、带图打开后自动识别，页面图片粘贴使用同一快照完成预览和识别，复制快捷键保留选区。图片校验、替换/撤销、独立后端帧预算与跨仓库兼容见 [ADR 0068](adr/0068-session-image-commands.md)；业务与 worker 留在插件层。新增接口已随 v0.3.1 公开发行，v0.2.1 不支持；OCR 0.1.0 已上架 macOS arm64，真实三端 WebView/剪贴板仍未验收。

### 插件商店后台下载安装

插件中心 catalog 服务现由宿主持有后台安装任务，可信页面提交后立即返回，切换页面和隐藏窗口不取消下载或安装。任务状态、取消与重试 UI 继续属于内置插件中心；保留原有目录、包校验和内核安装事务。最多四个活动任务，同一插件去重，内存保留最近 32 条；退出取消网络工作并等待安装及清理完成，不提供重启恢复或断点续传。不改变公开插件协议、SDK、权限或商店登记格式。见 [ADR 0072](adr/0072-background-plugin-installation.md)。

后台任务 UI 按后续指令改为插件中心第三个「下载安装」Tab；可信快照提供当前候选已下载字节与目录资产总量，下载器在写入缓存时更新，镜像重试归零。进度条只表达下载进度，校验及安装单独显示阶段状态，不扩展公开协议或 SDK，见 ADR 0072 同日补充。


## 启动设置

设置插件在通用分类追加静默启动和开机自启。内核持久化独立 `$startup` 静默偏好；宿主按该偏好在创建主输入框时设为隐藏，并通过 Wails `app.Autostart` 管理当前用户登录注册（原 `internal/startup` 已删除）。注册操作不开放给第三方插件；公开协议、SDK、商店与权限边界不变。平台机制、开发实例限制和迁移边界见 [ADR 0073](adr/0073-startup-preferences.md)。


## 主窗口透明度

设置插件通用分类追加主窗口整体透明度滑块。内核负责独立配置、校验及持久化，宿主通过平台适配设置原生窗口不透明度，覆盖文字与同窗口隔离内容；拆出后恢复独立窗口不透明。第三方隔离及公开插件协议、SDK、权限边界不变，平台能力与失败回退见 [ADR 0074](adr/0074-main-window-opacity.md)。


### 独立截图插件首版

官方仓库新增未发布 `tools.screenshot` 0.1.0：区域截图、箭头/矩形/文字标注、撤销重做、显式复制与 PNG 保存均属于插件。初始实现见 [ADR 0075](adr/0075-screenshot-plugin.md)。后续按用户要求拆为“截图”和“截图后处理”双入口，新增默认 CmdOrCtrl+Shift+A 全局截图热键；前者隐藏窗口后直接选区，完成后自动进入编辑器，后者只打开编辑器。

通用机制由宿主提供：`startHidden` 命令声明、视图级 `window.execute` 权限及 `ui.window-execute.v1` 能力；SDK executeHidden 只执行当前会话已声明的同插件后台命令；截图取得有效图片后恢复后处理，取消或未生成图片时以既有 dismiss 结果清理临时视图，窗口保持隐藏，撤销或替换后不恢复。普通调用预算、CSP 和 All-in-Plugin 边界不变，截图业务仍在独立插件。需配套本轮宿主，已发布 v0.3.1 不支持；未发布或登记新资产。见 [ADR 0076](adr/0076-hidden-window-commands.md)。


### 截图后处理组件

独立截图插件已按用户确认接入固定 Painterro 1.2.92，采用包内源码的可追溯私有构建适配，组件工具与历史留在插件层。样式静态化、图标为 DOM 内静态 SVG、文字直接绘入 Canvas，截图与导出均通过已有桥，不放宽 CSP、不增加宿主协议/SDK/权限。双入口、全局热键与无图片时结束流程不变。适配、许可、缓存预算与无界面验证边界见 [ADR 0077](adr/0077-painterro-screenshot-editor.md)。

截图输出与呈现补充：宿主新增会话限定的 window.show/window.hide、file.savePNG 权限和 ui.image-output.v1 能力；后台仅分块导出 PNG，宿主验证后通过 Wails 原生保存对话框写入用户确认的位置，不暴露路径或任意文件写入接口。window.execute 的 keepHidden 结果支持页面解码成功后再显示，取消/无图继续清理，隐藏页面超时不显示空编辑器。复制成功后的隐藏由插件调用通用窗口能力。见 [ADR 0078](adr/0078-screenshot-output-and-presentation.md)。

截图后处理输入补充：新增会话级 file.openImage 权限及 ui.image-input.v1，宿主原生选图、校验并转为受限 PNG 快照，复用会话资源 URL；业务页面负责导入入口、Command/Ctrl+V 与编辑器载入。图片粘贴沿用 clipboard.readImage，路径与任意文件读取能力不进入页面桥，CSP 不变。视图权限数量上限由 4 调整为 6。见 [ADR 0079](adr/0079-image-import-and-paste.md)。

### 插件间页面交接与依赖检查

用户已选择截图后处理通过跳转 OCR 插件完成识别。已按确认的[交接设计](design/plugin-handoff-and-dependencies.md)实现 ui.plugin-handoff.v1：可选依赖、公开图片入口、会话绑定的图片暂存及准备/提交/激活事务；OCR 识别业务留在独立插件，宿主只协调数据与导航。硬依赖不足时阻止激活，插件中心显示依赖状态；实际权限、兼容与验证边界见 [ADR 0080](adr/0080-plugin-handoff-and-dependencies.md)。尚未提供后台跨插件 RPC，未发布。


### 主界面会话保留

设置插件新增清除输入框延迟与插件 UI 保留时长，默认均为 0 秒。内核只持久化和校验独立主界面偏好，宿主统一隐藏时钟、取消、过期回调防护与主界面唤起决策；Shell 区分输入清空和视图恢复，同窗口插件继续保持隔离。没有新增公开协议、SDK 或权限；生命周期和三端边界见 [ADR 0081](adr/0081-main-window-retention.md)。


### 自启服务复用 Wails

按用户指令，开机自启改用当前锁定 Wails 3 的 `app.Autostart`，删除自行维护的三端原生注册代码。已有注册按库状态复用，写入后复核实际状态；静默启动、设置插件边界和开发实例限制保持，详见 [ADR 0082](adr/0082-wails-autostart.md)。

## 2026-11-17 配套发行

[Sinan 0.4.0](https://github.com/zhyipeng/sinan/releases/tag/v0.4.0)、[截图 0.1.0](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.screenshot-v0.1.0) 和 [OCR 0.3.0](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.ocr-v0.3.0) 已公开发行，三者均提供 macOS/Windows/Linux × arm64/amd64 六目标资产。截图及新 OCR 的最低宿主为 0.4.0，OCR 仍是截图的可选依赖。此前“未发布”描述记录开发阶段，以本节发行状态为准。

宿主[六目标原生构建与无界面回归](https://github.com/zhyipeng/sinan/actions/runs/44070459519)、截图[构建与发布](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44070601312)、OCR[六目标原生验证](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44071389046)及[正式发布](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44072351423)均成功。OCR 首轮 Windows amd64 的测试观测竞态已修复：worker 在 is_running/status 两次查询间退出时按已退出处理，保留存活超时与其他错误的失败判断，新增三项确定性单元用例；修复后的六目标验证全部通过。

宿主包含依赖与页面交接、窗口保留及 Wails 自启迁移，截图包含编辑器、导入/粘贴、原生保存与 OCR 跳转。以上为原生构建、无界面测试和发行完成，尚未执行真实桌面/WebView、截图授权、焦点、多屏、登录自启和实际安装升级验收；独立进程不构成 OS 沙箱。旧版本资产保持不可变。

### macOS 截图自定义选区

官方截图插件开发版 0.2.0 在 macOS 14+ 用包内原生辅助程序捕获各显示器的单帧并显示 AppKit 遮罩，可在任一显示器框选、移动、调整、确认或取消，单次选区限定在开始拖动的显示器。截图与遮罩留在插件层，支持就地标注、移动已有标注、撤销、复制和保存，并复用现有交接机制直接跳转 OCR。独立编辑入口仍走隔离页面，沿用宿主隐藏执行及现有协议和权限。旧版 macOS 延用系统选区，详见 [ADR 0084](adr/0084-macos-screenshot-selection-overlay.md)。整窗与窗口内元素吸附由截图插件实现，元素模式通过 macOS 辅助功能授权获得几何边界，见 [ADR 0085](adr/0085-macos-screenshot-smart-selection.md)。

### Windows 截图遮罩与平台范围

官方截图插件 0.3.0 将原生遮罩能力扩展至 Windows x64 和 ARM64，在插件层使用 Win32、GDI/GDI+ 和 Windows UI Automation。独立后处理入口已移除，隔离网页仅用于隐藏执行和 OCR 交接。新版截图仅支持 macOS 和 Windows，详见 [ADR 0086](adr/0086-windows-screenshot-overlay.md)。
