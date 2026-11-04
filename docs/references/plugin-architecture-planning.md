# 原始规划存档

来源：[规划插件化工具架构](chatgpt-conversation://6aeaf974-c4d4-83e8-bde5-978c154d8781)  
归档日期：2026-11-04

本文为原会话规划正文的存档，移除了无法在项目 Markdown 中解析的搜索引用标记，以文字标注图片组。原文的建议、草图和占位内容保持其原有性质；不表示能力已实现，也不表示外部技术信息已重新验证。实施以[架构指导方案](../architecture.md)及用户最新指令为准。

---

# 基于 Wails 3 的 All-in-Plugin 桌面效率工具架构规划

(Wails 3) (Go + TypeScript) (Plugin First) (Windows / macOS / Linux)

我建议把这个项目定位为：

**一个插件驱动的桌面效率平台，而不只是一个支持插件的快捷启动器。**

对标 uTools，但在架构上更进一步：应用检索、文件搜索、剪贴板历史、计算器、设置、插件管理，甚至命令面板内的搜索结果提供者，都由插件实现。

宿主只负责提供运行这些插件所必需的基础能力。

整个项目最重要的设计原则是：

> **Everything is a Plugin, but not Everything is in the Kernel.**
>
> 一切用户可见的功能都是插件，内核只提供机制，不直接实现具体业务。

## 一、整体架构

我建议采用 **Microkernel（微内核）+ Plugin Runtime（插件运行时）+ Extension Points（扩展点）** 的架构。

**用户交互层**

全局快捷键 · 命令面板 · 插件 UI · 独立窗口 · 系统托盘

插件层 · Built-in &amp; Third-party Plugins

**Plugin Runtime · 插件运行时**

**Core Services · Go 微内核**

**Wails 3 / Native OS**

Go Runtime · WebView · Windows · macOS · Linux

架构中最关键的边界是：**插件可以被卸载、更换或禁用，而内核依然能够正常运行。**

比如：

- 禁用应用搜索插件后，命令面板仍然能打开。
- 卸载设置插件后，内核依然能够读取和写入配置。
- 插件管理界面崩溃，不会导致插件运行时崩溃。
- 第三方插件不能通过普通前端代码绕过权限系统直接调用 Go 的系统能力。

Wails 3 已具备稳定的桌面多窗口、Go Service、事件系统和系统托盘等能力，适合用作这个项目的桌面宿主。

---

## 二、明确内核和插件的边界

这一步决定整个架构能否真正做到 All-in-Plugin。

我建议将功能分成三个层级。

| 层级 | 职责 | 是否可卸载 |
|--------|--------|--------|
| L0：Kernel | 插件加载、权限、通信、窗口管理、存储底座 | 否 |
| L1：System Plugins | 应用搜索、设置、插件管理、命令系统 | 可替换，部分受保护 |
| L2：User Plugins | 剪贴板、翻译、JSON 工具等 | 是 |

这里需要特别强调：**设置界面是插件，但配置存储服务不是插件。**

例如 `system.settings` 插件提供设置页面，而 `core.config` 提供底层配置接口，允许其他插件通过权限受控的 API 访问自己的配置。

同理，应用搜索插件负责索引和搜索应用，内核只提供调用插件、接收结果和启动系统资源的基础机制。

### 必须保留的最小 Shell

即使所有系统插件都损坏，宿主仍然需要一个极简应急界面，用来恢复默认插件、进入安全模式、查看启动错误。

这个 Recovery Shell 属于内核，而不是常规业务 UI。这是为了防止插件系统出现循环依赖或无法恢复的问题。

---

## 三、插件系统设计

插件系统是整个项目最核心、最值得提前设计的部分。

### 3.1 插件不应该只是一张网页

我建议一个插件可以独立提供以下四种能力：

| 能力 | 用途 | 典型插件 |
|--------|--------|--------|
| Command | 注册可执行命令 | 计算器、格式化 JSON |
| Search Provider | 向全局搜索提供结果 | 应用搜索、文件搜索 |
| UI View | 提供交互式页面 | 设置中心、剪贴板历史 |
| Background Service | 后台监听、索引、任务 | 剪贴板监听、文件索引 |

一个插件可以只实现其中一种，也可以同时拥有四种。

例如应用检索插件：

- 注册 `apps` 命令。
- 提供搜索建议和应用列表。
- 在后台维护已安装应用的索引。
- 提供一个应用管理界面（可选）。

这种设计比“每个插件对应一个 Web 页面”更通用。

### 3.2 插件包结构

建议使用可独立安装的插件包，例如 `.apkg`（暂定名称）。

```text
system.app-search/
├── manifest.json
├── icon.png
├── dist/
│   ├── index.html
│   ├── index.js
│   └── assets/
├── backend/
│   ├── darwin-arm64/
│   ├── darwin-amd64/
│   └── windows-amd64/
└── signatures/
```

其中 `manifest.json` 定义插件元数据、权限和扩展点。

```json

{
  "id": "system.app-search",
  "name": "Application Search",
  "version": "1.0.0",
  "apiVersion": "1",
  "type": "system",
  "engines": {
    "host": ">=0.1.0"
  },
  "contributes": {
    "commands": [
      {
        "id": "open",
        "title": "Search Applications",
        "keywords": ["app", "应用", "启动"]
      }
    ],
    "searchProviders": [
      {
        "id": "applications",
        "priority": 100
      }
    ],
    "views": [
      {
        "id": "manager",
        "entry": "dist/index.html"
      }
    ]
  },
  "runtime": {
    "kind": "process",
    "entry": "backend/${platform}/plugin"
  },
  "permissions": [
    "applications.read",
    "applications.launch"
  ],
  "activationEvents": [
    "onSearch",
    "onCommand:open"
  ]
}
```

这里的 `contributes` 是整个架构的关键。

插件声明自己能提供什么，内核负责把能力挂载到对应的扩展点上。

后续不只是 Search Provider，还可以扩展：

- `contextMenus`：右键菜单
- `settings`：插件设置项
- `widgets`：首页小组件
- `fileHandlers`：文件处理器
- `workflowActions`：工作流节点
- `aiTools`：AI 可调用工具

### 3.3 插件生命周期

插件生命周期示意，异常状态可触发隔离、重启或禁用

建议支持按需激活，而不是启动宿主时加载所有插件。

触发机制可以包括：

```typescript
type ActivationEvent =
  | "onStartup"
  | "onSearch"
  | "onClipboardChange"
  | "onFileDrop"
  | `onCommand:${string}`
  | `onView:${string}`
  | `onEvent:${string}`;
```

默认插件保持未激活，只有命中触发条件才加载相应逻辑。

另外要区分：

- **Installed**：已经安装。
- **Activated**：已经初始化。
- **Running**：后台任务或 UI 正在运行。
- **Suspended**：后台工作暂停，但保留运行状态。
- **Deactivated**：插件已停止。

这样才能避免安装几十个插件后，整个工具变成另一个高内存占用的 Electron 大应用。

---

## 四、插件运行时：整个项目最大的技术难点

这里有一个很重要的事实：

**Wails 3 的 Go Service 并不等于动态插件运行时。**

Wails 的 Go Service 和 TypeScript 绑定通常在构建时通过静态分析生成。安装一个新的插件，并不会自动把它的 Go 方法注册成新的 Wails Service。

因此，应该在 Wails 和插件之间自行实现一套稳定的 Plugin Host API。

### 4.1 推荐三种运行模式

| 模式 | 适用场景 | 方案 |
|--------|--------|--------|
| Declarative | 简单命令、静态配置、快捷链接 | 无独立执行环境 |
| JavaScript | 普通工具、UI、轻量计算 | 受控 JS 沙箱 |
| Native Process | 文件索引、系统监听、重型计算 | 独立进程 + RPC |

我的建议是 **三种都支持，但分阶段实现**。

**MVP 先实现 Declarative + Native Process，第二阶段再增加真正的 JS 后台沙箱。**

原因是 UI JavaScript、后台 JavaScript 和 Node.js JavaScript 是三种不同的问题。

Wails 并不自带 Node.js Runtime。如果为了兼容 uTools 插件而直接引入 Node.js，会大幅增加运行时复杂度。

建议原生插件优先采用 Go，但协议保持语言无关，为 Python、Rust、Node.js 等后续扩展留下空间。

### 4.2 不建议使用 Go 原生 `plugin` 包

Go 的 `plugin` 包存在跨平台、版本兼容、构建环境和崩溃隔离方面的限制。

对于需要同时运行在 Windows 和 macOS 的桌面工具，**进程外插件比 Go 动态链接插件更合适**。

推荐：

```text
Wails Host
    |
    +-- Plugin Supervisor (Go)
           |
           +-- app-search process
           |
           +-- clipboard process
           |
           +-- file-indexer process
```

插件可以通过 stdin/stdout JSON-RPC 或其他本地 IPC 协议通信。

第一版建议使用 **JSON-RPC 2.0 over stdio**：

- 无需分配本地端口。
- 易于调试。
- 支持不同语言。
- 容易实现超时、取消、错误处理。
- 后续可以添加消息流或更高性能的传输层。

不过需要注意：独立进程只提供故障隔离，并不自动提供安全沙箱。插件进程仍可能拥有当前用户的文件和系统权限。第三方原生代码必须采用更严格的信任、签名和操作系统沙箱策略，不能仅靠 manifest 声明权限。

### 4.3 插件通信协议

先定义统一的协议：

```typescript
interface PluginRequest {
  id: string;
  method: string;
  params?: unknown;
}

interface PluginResponse {
  id: string;
  result?: unknown;
  error?: {
    code: number;
    message: string;
  };
}
```

例如调用应用检索插件：

```json
{
  "id": "req-001",
  "method": "search.query",
  "params": {
    "query": "chrome",
    "limit": 20
  }
}
```

插件返回结果：

```json
{
  "id": "req-001",
  "result": {
    "items": [
      {
        "id": "com.google.Chrome",
        "title": "Google Chrome",
        "subtitle": "Applications",
        "score": 0.95,
        "icon": "plugin://system.app-search/icons/chrome",
        "actions": [
          {
            "id": "launch",
            "title": "Open Application"
          }
        ]
      }
    ]
  }
}
```

插件运行时应该额外支持超时、请求取消、流式结果、心跳、背压和数据大小限制，而不仅仅是普通 request/response。

### 4.4 前端与插件的关系

这里建议设计一个自己的 SDK：

```typescript
import { definePlugin } from "@app/plugin-sdk";

export default definePlugin({
  async activate(ctx) {
    ctx.commands.register("hello", async () => {
      ctx.ui.showNotification("Hello!");
    });
  },

  async deactivate() {}
});
```

这只是希望提供给插件开发者的接口形态，并不意味着它直接运行在 Wails 绑定环境里。

对于内置 UI 插件，可以有可信的宿主集成路径；对于第三方 UI 插件，建议使用独立安全上下文，通过受限消息桥通信。

尤其不能让不可信插件 UI 获得宿主前端的完整 Wails Bridge 权限。

---

## 五、全局搜索应该是一种扩展点，而不是一个应用搜索功能

这是我认为最值得重点设计的地方。

uTools 的核心体验就是通过一个输入框进入不同功能。其插件机制也支持命令、正则、文本、图像、文件、活动窗口等入口匹配方式。

你的项目可以抽象得更彻底。

### 5.1 命令面板只提供 UI 容器

## chrome

ESC

---

**Google Chrome**

应用 · 打开浏览器

(最佳匹配)

**Chrome Profile**

文件 · \~/Library/Application Support/...

**Search Chrome on Web**

命令 · 浏览器搜索

---

3 results · 3 providers

Enter 执行

这个界面由 Shell 提供，结果由各个插件提供。

例如：

```text
用户输入 chrome
       |
       v
  Search Coordinator
       |
       +-- App Search Plugin
       +-- File Search Plugin
       +-- Command Plugin
       +-- Browser History Plugin
       |
       v
   Result Aggregator
       |
       v
   Ranking & Display
```

### 5.2 Search Provider API

```typescript
interface SearchProvider {
  id: string;
  priority: number;

  search(
    query: string,
    context: SearchContext,
    signal: AbortSignal
  ): AsyncIterable<SearchBatch>;
}
```

建议支持两种搜索：

**Static Search**

插件在注册时把命令、关键词等信息提交给内核。搜索时内核直接查找，无须唤醒插件。

例如：

- 打开设置
- JSON 格式化
- 打开剪贴板历史
- 执行指定工作流

**Dynamic Search**

需要插件运行时处理的动态检索。

例如：

- 搜索本地应用
- 文件搜索
- 浏览器历史
- AI 搜索
- 远程 API 检索

搜索时必须可以异步返回、取消过时请求、限制每个 Provider 的结果数量。

### 5.3 搜索性能

不建议把每次键盘输入广播给所有插件进程。

推荐：

1. 根据插件声明的触发条件筛选 Provider。
2. 首先返回本地静态命令结果。
3. 并行查询低延迟 Provider。
4. 逐步合并动态结果。
5. 取消上一轮未完成查询。
6. 根据相关性和使用频率排序。

可以先设定这些工程目标：

| 指标 | 初期目标 |
|--------|--------|
| 唤出命令面板 | P95 ≤ 100ms |
| 静态命令首批结果 | P95 ≤ 30ms |
| 本地应用结果 | P95 ≤ 100ms |
| 搜索输入响应 | 不阻塞 UI |
| 插件崩溃影响 | 不影响 Shell |
| 空闲插件 | 按策略休眠或退出 |

这些是建议的性能验收目标，并非 Wails 3 已承诺的实测指标。

另一个重要设计是：搜索结果排名应该由内核统一负责，插件只提供相关性评分和结果元数据，避免插件通过随意提高分数长期霸占顶部。

---

## 六、插件 UI：建议采用双层 UI 架构

这里有一个需要尽早验证的技术问题：

**Wails 3 的多窗口能力不等于 Electron BrowserView 那样可以任意组合多个嵌入视图。**

因此，不建议直接照搬 uTools 的 BrowserView 架构。uTools 当前采用 Electron、BrowserView 与 Node.js preload 桥接，Wails 3 的宿主机制不同。

我建议：

| UI 类型 | 承载方式 | 使用场景 |
|--------|--------|--------|
| Shell UI | Wails 主 WebView | 搜索框、结果列表、导航 |
| Built-in Plugin UI | 宿主内可信组件 | 设置、插件管理 |
| Third-party Plugin UI | 隔离文档 / 独立 WebView | 第三方插件页面 |
| Detached Window | Wails 原生窗口 | 常驻工具、独立使用 |

其中，内置插件也应遵循统一插件协议，区别只是它们经过宿主认证，可以使用可信组件渲染。

第三方插件建议独立源、独立安全上下文，并限制导航、CSP、资源访问及消息桥。不能简单地把任意插件 HTML 注入主页面。

在 macOS 的 WKWebView 与 Windows 的 WebView2 上，需要分别验证插件资源加载、焦点管理、输入法、拖放、窗口透明度等行为。

### 推荐的用户体验

[原会话此处为界面参考图组，未归档图片]

**模式 A：命令面板**

默认唤出形态，轻量、快速。用于搜索和执行无需复杂交互的命令。

---

[原会话此处为界面参考图组，未归档图片]

**模式 B：嵌入式工具界面**

选择某个工具后，搜索面板扩展为插件 UI。顶部保留统一的返回、关闭和分离窗口操作。

---

[原会话此处为界面参考图组，未归档图片]

**模式 C：独立插件窗口**

用于剪贴板管理、设置中心、Markdown 编辑器等需要持续交互的场景。

对首版来说，我建议先实现模式 A 和模式 C。模式 B 涉及动态嵌入第三方网页、Bridge 隔离等问题，可以在插件 UI 原型验证后再决定最终方案。

## 七、设置也应该是插件

假设有三个插件：

```text
system.settings
system.app-search
system.clipboard
```

每个插件都可以贡献自己的设置项。

例如：

```typescript
ctx.settings.register({
  id: "app-search",
  title: "应用搜索",
  fields: [
    {
      key: "includeSystemApps",
      type: "boolean",
      title: "搜索系统应用",
      default: true
    }
  ]
});
```

设置插件读取各插件声明的设置 Schema，自动生成通用的设置界面。

复杂插件可以提供自己的设置页面。

我建议把设置能力分为：

- **Settings Storage**：内核负责保存、读取、校验和迁移。
- **Settings Schema**：插件声明配置结构。
- **Settings UI**：设置中心插件负责显示和编辑。
- **Settings Events**：配置变更时通知相关插件。

这样未来替换设置页面的 React UI，甚至整个设置插件，都不会影响已保存的配置。

插件管理同样采用这个模式：内核提供安装、卸载、启停、升级 API，`system.plugin-manager` 负责交互界面。

---

## 八、插件之间的通信

建议提供两种通信机制。

### 8.1 Service RPC

适合明确的一对一能力调用。

例如，工作流插件希望调用 JSON 格式化插件：

```typescript
const result = await ctx.plugins.call(
  "tools.json",
  "format",
  { text: input }
);
```

需要有服务声明、版本兼容性校验和访问权限检查。

### 8.2 Event Bus

适合广播事件。

```typescript
ctx.events.on("clipboard.changed", event => {
  // 处理剪贴板变更
});
```

可以用于：

- 剪贴板变更
- 当前窗口变更
- 系统主题切换
- 插件安装与卸载
- 用户执行某个命令

建议统一采用命名空间：

```text
core.theme.changed
core.plugin.installed
system.clipboard.changed
system.app-search.index.updated
```

跨插件事件需经权限检查，不允许所有插件无条件订阅敏感信息。

对于持久任务、后台作业和定时调度，另外提供 Scheduler Service，而不是依赖前端的 `setInterval`。

---

## 九、存储和安全模型

我建议首版使用 SQLite，辅以插件自己的文件目录。

```text
AppData/
├── core.db
├── plugins/
│   ├── system.settings/
│   └── system.app-search/
├── plugin-data/
│   ├── system.settings/
│   └── system.app-search/
├── cache/
└── logs/
```

`core.db` 保存插件注册表、配置、权限授权、命令索引、历史记录等。

每个插件有独立的逻辑存储命名空间；核心服务负责配额、事务和清理。插件的敏感凭据可以通过系统 Keychain / Credential Manager 保存。

### 权限模型

| 权限 | 用途 | 风险 |
|--------|--------|--------|
| `storage.private` | 插件私有存储 | 低 |
| `clipboard.read` | 读取剪贴板 | 高 |
| `clipboard.write` | 写入剪贴板 | 中 |
| `filesystem.read` | 文件读取 | 高 |
| `filesystem.write` | 文件写入 | 高 |
| `applications.launch` | 启动应用 | 中 |
| `network.fetch` | 网络请求 | 中到高 |
| `system.hotkey` | 注册全局快捷键 | 中 |
| `process.execute` | 执行系统进程 | 极高 |

权限应尽量做细，例如文件权限限制到用户选定目录、网络权限限制到授权域名。

这里需要把插件划分为两种信任等级：

**Sandboxed Plugin**：适用于普通第三方插件，只能访问受控 Host API。

**Trusted Native Plugin**：允许运行原生可执行文件，需要明确告知用户风险。不能声称用户批准 manifest 权限后，普通操作系统进程就获得了完整隔离。

后续如果需要开放公共插件市场，还要加入包签名、哈希校验、更新回滚、恶意插件处理和审核机制。

---

## 十、推荐的技术选型

| 模块 | 推荐技术 | 理由 |
|--------|--------|--------|
| Desktop Host | Wails 3 | Go + 原生 WebView |
| Core Runtime | Go | 并发、系统集成、进程管理 |
| Shell UI | React + TypeScript | 组件生态成熟 |
| Styling | Tailwind CSS | 快速构建统一组件 |
| State | Zustand | 轻量状态管理 |
| Storage | SQLite | 本地数据、索引和事务 |
| Search Index | SQLite FTS5 + 自定义排序 | 命令和本地数据检索 |
| IPC | JSON-RPC over stdio | 易于跨语言 |
| Plugin SDK | TypeScript | 插件开发体验 |
| Plugin CLI | Go 或 Node.js | 开发、打包和调试 |
| Packaging | ZIP 类插件包 + manifest | 易分发和升级 |

第一阶段没必要引入 Redis、独立数据库服务、消息队列或微服务框架。

### 推荐 Monorepo 结构

```text
project/
├── apps/
│   └── desktop/
│       ├── main.go
│       └── frontend/
│
├── internal/
│   ├── kernel/
│   ├── plugin/
│   │   ├── registry/
│   │   ├── supervisor/
│   │   ├── lifecycle/
│   │   ├── permissions/
│   │   ├── transport/
│   │   └── activation/
│   ├── search/
│   ├── windows/
│   ├── storage/
│   └── platform/
│       ├── darwin/
│       ├── windows/
│       └── linux/
│
├── packages/
│   ├── plugin-sdk/
│   ├── plugin-protocol/
│   ├── ui-components/
│   └── plugin-cli/
│
├── plugins/
│   ├── system-settings/
│   ├── system-plugin-manager/
│   ├── system-app-search/
│   ├── system-calculator/
│   └── system-clipboard/
│
├── schemas/
│   ├── manifest.schema.json
│   └── protocol/
│
└── docs/
    ├── architecture/
    ├── plugin-api/
    └── adr/
```

建议从第一天就把插件协议、SDK 和宿主代码分开，否则随着内置插件增加，很容易退化成普通的单体桌面应用。

---

## 十一、分阶段实施路线

**Phase 1：最小内核与插件运行验证**

(MVP 核心)

完成 Wails 宿主、插件 Manifest、Plugin Registry、JSON-RPC 通信、生命周期以及两个示例插件。

验收：一个独立插件能够在不重新编译宿主的情况下安装、执行、停止和卸载。

---

**Phase 2：全局搜索与系统插件**

实现快捷键唤出、Search Coordinator、静态命令索引、应用检索插件、设置插件和插件管理器。

验收：可以完全通过插件完成应用搜索和配置管理。

---

**Phase 3：插件 UI 与开发者 SDK**

实现可隔离的插件 UI、插件窗口管理、TypeScript SDK、开发调试命令、插件打包和热重载。

验收：外部开发者可以独立编写一个带界面的插件。

---

**Phase 4：稳定性与生态**

补齐权限授权、插件签名、自动更新、跨平台验证、插件市场和更多 SDK API。

验收：支持可控的第三方插件分发和运行。

### 第一版建议只实现的功能

**MVP 功能范围** / 项

[按选定范围细化 MVP]

## 十二、我建议优先做的三个技术验证

在正式开发大量功能前，最好先用最简单的演示插件验证以下三个问题。

**验证 1：动态插件加载**

宿主启动后，从用户目录发现插件、启动插件进程、调用 RPC，且无需重新构建 Wails 宿主。这是整个架构的基础。

**验证 2：动态 UI 加载**

验证是否可以在 Wails WebView 中安全加载外部插件 UI，是否能独立管理权限与焦点，并处理跨平台差异。这决定未来 UI 插件采用独立窗口、隔离文档还是混合方案。

**验证 3：搜索扩展点**

让两个完全独立的插件同时提供搜索结果，宿主负责并行查询、排序、取消与展示。如果这一步能跑通，基本就打通了产品最核心的使用路径。

## 十三、几个值得提前确定的设计决策

| 问题 | 我的建议 |
|--------|--------|
| 插件是否必须有 UI？ | 不必须 |
| 插件是否可以有后台任务？ | 必须支持 |
| 内置插件和第三方插件是否使用相同协议？ | 是 |
| 是否允许插件之间调用？ | 是，但要权限控制 |
| 是否允许第三方 Go 插件？ | 是，独立进程 |
| 是否一开始兼容 Node.js？ | 否，作为后续可选运行时 |
| 是否需要统一 SDK？ | 是，第一阶段就定义 |
| 是否需要统一 UI 组件？ | 推荐，但不强制 |
| 是否支持插件热安装？ | 是 |
| 是否需要插件市场？ | 后期实现 |
| 是否要支持 AI Agent？ | 预留扩展点，暂不实现 |

---

## 最终建议

这个项目的关键不在于实现多少个工具，而是建立一套足够稳定的**插件协议、扩展点模型和运行时**。

我会优先把下面四个部分当成项目真正的核心资产：

**Plugin Manifest → Plugin Runtime → Extension Registry → Plugin SDK**

而 Wails 3、React、SQLite，都是支撑这套机制的基础设施。

还有一个方向值得特别考虑：**未来的 AI Agent 也可以只是普通插件。** 每个插件声明的 Command 或 Action 可以选择暴露为机器可调用工具，AI 通过同一套权限、调度和结果协议工作，而不必在内核中另起一套 AI 系统。

接下来最值得深入的是 **Plugin Runtime + Plugin SDK 的详细技术设计**，尤其是动态插件 UI、跨进程 RPC、权限隔离以及内置插件和第三方插件如何真正共享同一套接口。把这一层设计好，之后开发应用搜索、设置、剪贴板等功能就会变成相对独立的插件开发工作。

