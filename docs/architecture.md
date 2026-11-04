# Sinan 架构指导方案

状态：项目架构基线，Phase 1 最小运行原型已实现；整体规划尚未完成。整理日期：2026-11-04。

来源：[规划插件化工具架构](chatgpt-conversation://6aeaf974-c4d4-83e8-bde5-978c154d8781)。用户已指定将该会话结论作为本项目指导方案。[原始规划存档](references/plugin-architecture-planning.md)用于追溯，本文用于指导实施。

本文保留原会话的架构结论，并区分基线、推荐方案与待决事项。原文涉及的 Wails 能力和版本状态未在本次整理中重新验证，实际开发应针对所选版本验证，不将原文中的“稳定”表述作为保证。

首轮已锁定 Wails 3.0.0、Go 1.26.0，完成 macOS arm64 最小宿主、本地插件目录安装与卸载、声明式和 Go 进程示例、按需激活及 stdio RPC。构建和真实进程测试已通过，桌面手动验证了筛选、执行与停止。详细范围及限制见 [ADR 0001](adr/0001-phase-one-runtime.md) 和[当前插件接口](plugin-protocol.md)。动态 UI 隔离、多 Provider 搜索、权限服务与其他后续能力仍待实现。

按用户后续要求，Shell 已改为聚焦式交互：Command + Space 切换显示与隐藏，空输入框高 50 DIP，按结果调整高度，失焦隐藏，并提供托盘恢复与退出入口。macOS 呼出时选择鼠标所在显示器，水平居中并保留原纵向定位规则。当前搜索仍限于插件声明的命令。实现与验证边界见 [ADR 0002](adr/0002-launcher-interaction.md)。

macOS Shell 采用非激活 NSPanel，并配置 floating 层级及跨 Space / 全屏辅助窗口行为，避免呼出时激活宿主而切离全屏应用。该机制及实机验证边界同样记录在 ADR 0002。

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
| L1 System Plugins | 应用检索、设置、插件管理及命令业务 | 可替换，部分可受保护，具体策略待定 |
| L2 User Plugins | 剪贴板、翻译、JSON 工具等扩展 | 可安装、禁用和卸载 |

边界约束：

- 设置 UI 属于 `system.settings`；配置读写、校验、迁移和变更通知属于核心服务。
- 插件管理 UI 属于 `system.plugin-manager`；安装、卸载、启停和升级机制属于内核。
- 应用索引与检索属于 `system.app-search`；宿主负责受控系统能力、搜索调度和统一排名。
- 命令面板 Shell 负责输入框、结果容器和导航；业务命令、结果内容来自插件。
- Recovery Shell 属于宿主，提供安全模式、启动错误查看和默认插件恢复；不能依赖系统插件正常运行。

禁用应用检索插件后面板仍可打开；移除设置插件后配置服务仍可用；插件管理 UI 或插件进程崩溃不能使宿主失效。

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

首版优先命令面板与独立插件窗口。动态嵌入第三方 UI 待验证，不直接套用 Electron BrowserView 模型。

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
