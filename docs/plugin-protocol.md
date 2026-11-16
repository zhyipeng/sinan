# 插件接口 v1

本页描述当前实现，完整规划见[架构指导方案](architecture.md)。运行时取舍见 [ADR 0001](adr/0001-phase-one-runtime.md) 和 [ADR 0003](adr/0003-search-and-ui-prototype.md)。

## Manifest

参考 [Schema](../schemas/manifest.schema.json) 和[示例插件](../plugins/)。当前支持 `declarative`、`process`，API 版本为 `1`；未知字段、重复命令 ID、未声明贡献的激活事件会被拒绝。Schema 供编辑器提示，宿主还校验路径、引用和唯一性。

每个插件至少提供一个命令。进程插件可额外声明 `contributes.searchProviders`：

```json
{"searchProviders":[{"id":"demo","prefix":"","minLength":1}]}
```

`prefix` 为不区分大小写的前缀，`minLength` 按 Unicode 字符计数。支持 `onCommand:<id>` 及有搜索贡献时的 `onSearch`；当前通过命令调用或被选中的搜索贡献按需激活，不支持启动激活。`permissions` 对可信原生进程接受 `applications.read`、`applications.launch` 两项不重复的用途声明；声明式插件仍只接受空数组。它们不是系统授权或沙箱检查，尚无系统 Host API。

## 进程入口

`runtime.entry` 指向插件目录内的可执行文件，例如 Unix 的 `backend/plugin` 或 Windows 的 `backend/plugin.exe`；Unix 要求执行位，Windows 要求 `.exe` 后缀。不支持 shell 命令、参数拼接、平台变量或符号链接。跨平台需单独构建。安装来源不能包含安装目录，总大小上限 64 MiB，最多 2048 个条目。

原生插件以当前用户权限运行，安装时必须确认信任。宿主只传入显式语言及平台桌面环境白名单，不整表继承环境变量，stderr 当前丢弃；这些措施不构成安全沙箱。信任记录不防御同一用户改写插件文件。环境白名单包含 Windows 系统/用户应用目录、PATH/TEMP，或 Linux HOME/PATH、XDG、显示服务及会话总线信息；详见 [ADR 0051](adr/0051-cross-platform-foundation.md)。

## JSON-RPC

UTF-8 JSON 对象按换行分帧，stdout 只能输出协议消息。请求使用 `jsonrpc: "2.0"` 和非零数字 ID，响应恰有 `result` 或 `error` 之一。当前不支持插件反向调用 Host API。

宿主先握手：

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"apiVersion":"1","pluginId":"example.search-fast"}}
```

```json
{"jsonrpc":"2.0","id":1,"result":{"apiVersion":"1","pluginId":"example.search-fast","capabilities":["search.query","$/cancelRequest"]}}
```

`capabilities` 可省略以兼容原有命令插件。搜索插件必须同时声明上述两个能力。

命令调用与响应：

```json
{"jsonrpc":"2.0","id":2,"method":"command.execute","params":{"commandId":"echo","input":"你好"}}
```

```json
{"jsonrpc":"2.0","id":2,"result":{"text":"你好"}}
```

搜索与取消：

```json
{"jsonrpc":"2.0","id":3,"method":"search.query","params":{"providerId":"demo","query":"你好","limit":20}}
```

```json
{"jsonrpc":"2.0","id":3,"result":{"items":[{"id":"echo","title":"回声：你好","subtitle":"示例","commandId":"echo","input":"你好","score":0.7}]}}
```

```json
{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":3}}
```

取消为本协议扩展，通知本身无 ID，插件仍需结束原请求并响应，例如 `error.code: -32800`。搜索结果动作只能引用当前插件声明的命令。宿主忽略非法结果、约束分数并生成跨来源唯一键。

单条消息上限约 1 MiB，命令输入上限 64 KiB，动态 Provider 搜索查询上限 4096 字节（本地声明式内容匹配另有 64 KiB 上限），每批搜索最多采用 20 项。搜索预算 2 秒，取消宽限 500 ms；不合作插件会被终止。命令预算仍为 5 秒，调用错误或超时会终止进程。停止后下一次调用可重新激活，卸载则使旧实例失效。

## SDK 与工具

Go 命令插件继续使用 `sdk.Serve`；搜索插件使用 `sdk.ServeWithHandlers`，搜索处理函数接收 `context.Context`，必须响应取消。SDK 允许并行请求、序列化响应；可参考[应用检索插件](../plugins/system-app-search/cmd/main.go)。TypeScript 协议声明位于 `packages/plugin-protocol/index.ts`，尚无完整 TypeScript 运行时 SDK。

`sinanctl search 查询` 按增量输出聚合 JSON，包含 `queryId`、`items`、`pending`、`failures` 和 `done`。CLI 退出时回收进程；桌面保持已激活进程。同一插件根目录由一个宿主独占；运行 CLI 前须先退出使用该目录的桌面实例。

新插件导入后，从桌面 `··· → 刷新` 发现变化，无需重编译宿主。刷新会结束已移除实例，包内容变化则标记待重载并保留当前实例。升级不再需要先卸载。尚未实现心跳、优雅关闭、事件总线、跨插件 RPC 或插件侧逐条流式结果。

## 应用检索阶段的可选扩展

命令可声明 `execution: "direct"` 让用户选择后立即执行；省略时保留命令输入表单。`searchOnly: true` 让命令仅通过动态结果提供入口，不进入静态命令列表。成功响应可带 `dismiss: true` 请求可信 Shell 隐藏窗口。原生应用启动失败时不应返回成功的隐藏请求。

动态结果可带 `icon`，仅支持尺寸不超过 128×128、整个字段不超过 24 KiB 的 PNG base64 data URL；外链、SVG、本地文件地址及非法图片头会被丢弃，结果本身仍保留。

这些字段仍属 API 1 的增量扩展，但旧宿主严格校验 manifest，安装含新字段的插件前需更新宿主。实现取舍与权限用途边界见 [ADR 0004](adr/0004-application-search.md)。

## 宿主使用历史

成功的命令调用按 `(pluginId, commandId, input)` 的摘要记录使用次数和时间，原始输入及查询不落盘。搜索协调器对已返回候选最多增加 0.025 的历史权重；Provider 基础分仍限制为 0.8，最终动态分上限为 0.825。插件不能自行提供此权重。`sinanctl clear-history` 可清除指定插件根目录的历史；当前须先退出使用同一根目录的桌面实例。详见 [ADR 0005](adr/0005-search-refinement.md)。

## 配置与可信视图

应用检索 0.3.0 通过 `contributes.settings` 声明 `[{"id":"directories","title":"额外应用目录","type":"directory-list"}]`。设置项类型支持 `directory-list`（每项最多 16 个已存在的目录绝对路径）、`select`（以 `options` 提供固定选项，保存单个值）和 `text`（单行文本，最长 2048 字符，可带 `placeholder`）。插件中心 0.8.0 用 `select` 与 `text` 声明 GitHub 加速服务及自定义模板。宿主通过 `initialize.params.settings` 提供当前插件的配置，缺失项为空列表；SDK 的 `Handlers.Initialize` 在开始处理命令前读取快照。保存后只停止受影响的进程，下次调用重新激活，无插件侧热更新通知。

声明式命令可使用 `view` 替代 `message`，与 `execution` 互斥。Shell 只显示构建时注册的可信组件，未知键报错；不会将视图键作为文件路径或 URL 加载。`system.settings/settings` 是首个注册组件。此能力不开放第三方 JS/HTML，详见 [ADR 0006](adr/0006-settings-service-and-view.md)。

可信 Shell 的配置保存调用携带 `revision`，过期写入会失败。成功后定向发送 `settings:changed`，只含 `pluginId`、`revision`。CLI 可用 `settings ID` 查看、`set-settings ID revision JSON` 修改；仍不支持同一根目录多个宿主并发写入。

## 升级、重载与单实例目录

`sinanctl inspect 目录` 查看版本和内容变化；`sinanctl [-trust-native] upgrade 目录` 升级，原生包必须确认信任；`sinanctl reload ID` 重载已安装文件。桌面 `system.plugin-manager` 插件支持相同机制。包或安装基线在检查后变化会拒绝应用，需要重新检查。

升级过程中旧调用被退休，成功后保留配置和历史；初始化失败时恢复旧包和信任状态。显式重载不撤销外部磁盘编辑。首次安装仍按需激活，升级握手并不验证全部业务功能。

宿主持有 `.sinan-state/runtime.lock` 目录独占锁，CLI 与桌面（包括只读命令）不能同时打开同一根目录。关闭后 OS 释放锁，文件保留；无须手动删除。详见 [ADR 0007](adr/0007-plugin-upgrade-and-reload.md)。

## 可丢弃的插件缓存目录

`initialize.params.cacheDir` 是 API 1 的可选增量字段，为当前插件提供绝对缓存目录 `.sinan-state/cache/<pluginId>`。目录按需创建，位于插件包之外，升级、重载和卸载不删除它；内容可被丢弃，不用于配置或凭据。目录无法创建时省略字段，插件必须能在没有缓存时工作。旧插件忽略此字段；新插件在旧宿主上回退为无缓存运行。

应用检索 0.4.0 使用此目录保存有版本、数量和大小限制的索引快照。缓存格式和校验属于插件，宿主不解析应用索引。此路径只是原生插件的存储约定，不限制其他文件访问；尚无通用缓存配额或安全沙箱。详见 [ADR 0009](adr/0009-application-index-cache.md)。

## 本地插件管理可信视图

`system.plugin-manager/manager` 与设置视图使用同一静态可信组件注册机制。管理组件只接收列出、检查、安装升级、停止、卸载和重载的类型化适配器；组件不导入 Wails 绑定，不读取安装包内的 JS/HTML。宿主继续负责信任确认、包指纹与生命周期。此适配器仅供内置可信组件，不是向第三方开放的管理权限 API，协议版本不变。

管理插件缺失时，Shell 的 `···` 打开独立恢复入口，仅接受声明式 `system.plugin-manager` 包，沿用检查后应用的指纹验证。管理插件卸载自身后进入恢复入口；可信视图的同步渲染错误由 Shell 错误边界接住。包恢复不能替换编译进宿主的组件代码。详见 [ADR 0010](adr/0010-plugin-management-view.md)。

## 启用策略与激活限制

管理快照 `PluginInfo` 增加布尔 `disabled`；禁用时展示状态为 `disabled`，命令与 Provider 声明仍保留供管理界面检查，搜索协调器在调度和发布时排除禁用插件。此字段及 `SetPluginEnabled(id, enabled)` 属于宿主管理接口，不修改插件进程 API 1。

启用操作持久化到核心设置保留命名空间 `$plugins` 的 `disabled` ID 列表，最多 256 项。先保存再修改运行实例，写入失败不停止当前进程。禁用会终止在途请求并拒绝后续执行和激活；启用只恢复按需激活。停止、配置保存、重载、升级、卸载后重新安装均不会清除显式禁用选择。禁用的原生候选升级/重载只进行静态包校验，不运行初始化，启用后的首次调用才握手。

不存在启用策略时默认启用；策略内容或设置文件无法读取时，保留原文件、返回诊断、将所有插件保持禁用，拒绝更改，需离线修复后重启。管理插件可以禁用自身，由宿主恢复入口或 CLI `enable system.plugin-manager` 恢复。启用不会绕过原生信任或包变更校验，也不是安全沙箱。详见 [ADR 0011](adr/0011-plugin-enablement.md)。

## 内部 UI 会话准备接口

内核 `OpenUIView(ctx, pluginId, viewId)` 供宿主窗口控制器和内部开发验证使用，读取已安装包固定的 `ui/` 子目录；它不是 Wails 方法或进程 RPC，不会因为某个页面提交插件 ID 就授予会话。

会话由宿主对象身份确定归属，绑定已安装实例、指纹、声明的视图 ID 和桥版本。桥 1 仅支持资源读取、单次 `{"kind":"ready","version":1}` 及 `{"kind":"close"}`。额外/重复字段、重复握手、其他动作和页面自报身份被拒绝。消息上限 4 KiB、每会话最多 32 条，超限撤销；最多每插件 4 个、每宿主 16 个活动会话。

生命周期操作和检测到包变化时撤销会话。旧句柄随后不能读取资源、重放消息或因重装/回退到相同指纹而复活。原生验证宿主每次 Scheme 请求都通过该会话，不保存整包资源副本；父进程立即拒绝后续资源与消息，子进程通过异步通知关闭原生入口，超时终止回收。此前交付的字节不能收回。开发验证通过会话专属 stdio 管道接入独立原生客体：管道不接受页面选择归属，支持 hello / ready、逐次资源读取、撤销和退出回收；这不是插件后端 RPC，也未开放执行、设置等 Host API。帧、请求次数、累计资源和运行时长均受预算限制。进程监督细节见 [ADR 0014](adr/0014-ui-guest-supervision.md)，桌面开发入口见 [ADR 0017](adr/0017-desktop-plugin-windows.md)，面向公共分发的 API 仍待完善。stdio API 1 保持不变。详见 [ADR 0013](adr/0013-ui-session-revocation.md)。


## 受限 UI 资源声明

Manifest 可在 `contributes.uiViews` 声明最多 4 个视图。声明只提供资源元数据；Shell 不加载 HTML；插件 UI 可用时，可信管理组件可按声明 ID 请求独立窗口，搜索命令可通过 `uiView` 关联该声明。`commands[].view` 仍只选择宿主构建时注册的可信组件，与声明的 UI ID 同名时拒绝，避免隐式切换承载方式。

```json
{
  "uiViews": [
    {"id": "main", "entry": "pages/index.html", "bridgeVersion": "1", "permissions": []}
  ]
}
```

此片段属于 `contributes`，完整插件仍需遵循已有命令要求。`id` 为插件内唯一的 1–64 字符小写贡献 ID；`entry` 相对于固定 `ui/`，限 256 个 ASCII 字符、最多 8 个目录层级、规范 `.html` 路径，拒绝绝对路径、URL、转义、反斜线、查询、片段和 `.` / `..` 路径段。资源存在性、类型、体积与符号链接由打开会话时检查。资源授权范围仍是同一插件的整个 `ui/` 快照，不是视图级目录隔离，包内敏感文件不得放入该目录。

`bridgeVersion` 必填，支持字符串 `"1"` 与 `"2"`；它独立于后端 `apiVersion` 和私有客体管道版本，不协商降级。桥 1 的 permissions 必须省略或为空，只允许资源读取、ready / close。桥 2 可显式声明本插件 commands 白名单和 storage，具体契约见文末“UI 桥 2”。两者均不继承顶层原生插件用途声明，设置、文件、网络、剪贴板请求继续不受支持。

只有宿主能按声明 ID 创建会话，不能通过入口路径或页面自报身份创建。声明缺失、版本不支持、资源不可用、插件禁用或包改变均拒绝；升级移除视图后旧句柄失效。`packages/plugin-protocol/ui.go` 定义声明类型，`schemas/manifest.schema.json` 提供静态结构约束，Go 校验额外检查 ID 唯一和可信组件冲突。

后端 stdio API 1 保持不变。旧宿主通过未知字段拒绝新声明，不会降级为可信组件；旧插件无需添加该字段。后续桌面开发入口增加仅供可信 Shell 的 `PluginUIEnabled` / `OpenPluginView(pluginId, viewId)`，现默认启用（可显式关闭），不暴露给客体或后端 RPC；它不允许选择入口、可执行文件、脚本或权限。没有通用 UI 权限授权服务。见 [ADR 0015](adr/0015-declared-ui-views.md)。


## 桌面开发窗口入口

插件界面默认启用，无需额外启动参数；`-plugin-ui=false` 可显式关闭，`make dev-ui` 是 `make dev` 的兼容别名。当前 macOS 与 Linux GTK3 构建已有同窗口承载实现，Linux 尚待实机验收；Windows 或未编译对应适配器时返回具体诊断。旧 helper 支持情况独立判定。可信管理组件获取 `PluginUIEnabled()` 后显示包声明视图的打开按钮，调用 `OpenPluginView(pluginId, viewId)`。普通命令执行和可信组件注册不因此改变；页面不会获得这两个 Wails 方法。

宿主只运行自身目录的固定 `sinan-ui-host`，不会运行包内指定的 UI 程序。窗口控制器复用同一插件/视图，首次等待 ready 后返回；关闭后可重新打开，包变更和启用状态仍由内核会话控制。退出宿主时取消启动并等待所有客体回收，再释放内核。helper 缺失或平台不支持时返回诊断并保留 Shell。客体异步退出事件 `ui:closed` 只发送给可信 Shell，字段为 pluginId / viewId / failed，不传播页面错误原文。

桥 1 普通客体只有 ready / close；桥 2 增加按声明授权的命令与私有文档调用，桥接消息依然受限；关闭原生窗口也撤销会话。不执行固定测试函数，不处理 report / revoke 探测动作，没有 8 秒探测期限；开发窗口在父进程侧暂设 1 小时总期限，响应超时策略继续生效。非持久存储、资源归属、CSP、导航拒绝与进程预算沿用此前边界，额外拒绝 JS 对话框、原生文件选择器和媒体采集请求。没有 CPU/内存硬配额或操作系统沙箱。


普通 macOS 开发页面可在初始化完成后发送 ready，由用户操作发送 close：

```js
const send = message => window.webkit.messageHandlers.sinan.postMessage(message);
send({ kind: 'ready', version: 1 });
document.querySelector('#close').addEventListener('click', () => send({ kind: 'close' }));
```

脚本应放在包内独立 `.js` 资源中，HTML 以相对路径引用；固定 CSP 不允许内联脚本或远程脚本。此入口已有 macOS 与 Linux GTK3 实现，兼容相同 SDK 调用形状；Windows 承载与完整跨平台验收仍待完成。


## 搜索命令打开独立视图

命令可用 `uiView` 引用同一插件的声明 ID：

```json
{
  "commands": [{"id": "open", "title": "窗口验收", "keywords": ["window"], "uiView": "input"}],
  "uiViews": [{"id": "input", "entry": "index.html", "bridgeVersion": "1", "permissions": []}]
}
```

片段属于 contributes。uiView 不能与非空 message / view / execution 混用，引用必须存在；declarative / process 均可声明。原有 view 继续只打开可信组件，uiView 不成为导入地址或脚本路径。

用户选择结果后，可信 Shell 调用 `OpenPluginCommand(pluginId, commandId)`。内核在同一变更锁中解析已安装包的当前绑定，并沿用启用状态、包指纹、资源和会话检查；前端不能为命令覆盖视图。普通 Execute（包括 CLI run）拒绝此类命令；打开视图不调用后端 execute。搜索 Provider 的激活规则保持不变。

成功启动才隐藏面板，失败保留搜索；启动期间重新呼出面板后，迟到结果不能隐藏新面板。插件界面默认启用；显式关闭或承载不可用时拒绝打开并说明原因。同一视图的命令别名和管理入口共同去重，重复打开复用并聚焦同一窗口，保留输入和位置。搜索输入参数暂不传给页面；桥 1 只能 ready / close，桥 2 的显式调用见文末。

旧宿主拒绝未知 uiView 字段，后端 API 1 和 UI 桥版本不变。详见 [ADR 0020](adr/0020-search-opens-independent-views.md)。


## 窗口复用与显示信息

`uiViews[].title` 可选，最多 64 个 Unicode 字符，不能是纯空白或含控制字符。原生标题为「插件名 · 视图标题」，省略时使用插件名；管理入口使用该可读标题。它只提供显示信息，不授予任何能力。

宿主解析当前声明后按绑定复用活动客体，启动中的相同请求共享启动结果。私有 focus / focused 控制按待确认编号关联，不进入页面桥，也不携带查询输入。聚焦确认有 2 秒等待预算，超时不关闭健康窗口；尚有未确认请求时拒绝追加。活动窗口名额已满仍可复用，撤销或包变化则拒绝旧句柄。

首次打开在鼠标所在显示器可用区域居中；聚焦已有窗口保持位置，并恢复最小化状态。后端 API 和页面桥仍为 1；私有通道扩展需同步构建宿主与客体。见 [ADR 0021](adr/0021-window-focus-title-and-placement.md)。

## 核心恢复接口

可信 Shell 的 `PreviewSystemRepair(directory)` / `RepairSystemPlugin(directory, fingerprint, installedFingerprint)` 专用于恢复设置和插件管理的声明式组件包。此处 installedFingerprint 表示预览时实际目录字节（允许清单损坏），目录不存在时为空；与普通升级预览中的注册实例基线不同。双方字节改变都必须重新检查。CLI 可在桌面宿主退出、释放目录独占后运行 `sinanctl -plugins 路径 repair-system 本地系统插件目录`。普通卸载和禁用保护不变，HTML 页面桥不获得此接口。

后端与 UI 客体直接进程退出不再等待后代持有的 stderr；不保证回收任意后代进程。HTML 候选页打开失败保留先前附着页面，成功后才撤销原会话。详见 [ADR 0028](adr/0028-core-recovery-and-lifecycle.md)。

## UI 桥 2：本插件命令与私有数据

现同时支持桥 1 和桥 2。旧页面仍使用 ready version 1；新页面显式声明：

```json
{"id":"editor","entry":"index.html","bridgeVersion":"2","commands":["format","compact"],"permissions":["storage"]}
```

commands 必须引用同插件已声明的非视图进程命令，最多 16 项，不可重复。permissions 可省略，唯一支持值为 storage，桥 1 禁止两类能力。宿主不继承原生用途声明为页面权限。ready version 2 后，可用 `packages/plugin-sdk/ui/index.js`：

```js
import { ready, execute, readData, writeData } from './bridge.js';
ready();
const { text } = await execute('format', input);
const doc = await readData();
await writeData(doc.revision, { text: input });
```

SDK 文件需复制到插件 ui/；它只包装 Promise 请求，不包含 Wails Bridge。对应请求为 command.execute / storage.get / storage.put / launch.get，调用不接受插件 ID、路径或额外字段。身份来自宿主 Session；配置、安装、Shell、其他插件和任意后端 RPC 不可访问。每会话在途最多 1 个、每分钟 120 次、500 ms 预算，请求 64 KiB、响应 192 KiB。SDK 最多排队 8 个调用。

storage.get / put 返回 `{revision, value}`；未保存时为 `{revision:0,value:null}`。每插件一个最大 32 KiB JSON 值；put 必须携带当前 revision，冲突要求重新读取，损坏文件拒绝覆盖。包外数据在升级/卸载后保留，清理仅由可信管理入口提供，不向页面开放。页面关闭可能发生写入已提交但回执丢失，重开应读取当前版本。命令超时或撤销沿用强制终止后端，其他会话应重新打开。

实现与限制见 [ADR 0029](adr/0029-ui-calls-private-data-json-tool.md)。原生后端协议 API 仍为 1。

## 私有数据管理（可信宿主）

PluginData() 返回私有文档元数据，不含正文；ClearPluginData(id, token) 仅清理该 ID 的固定文档，需要列表返回的最新文件状态指纹。数据有变化则要求重新检查并确认；成功后撤销该插件旧 HTML 会话，新页面看到空文档，避免旧自动保存重建数据。此接口不开放给 UI 桥 2。

CLI：先退出持有相同根目录的宿主，运行 `sinanctl -plugins 路径 data` 查看，再显式运行 `sinanctl -plugins 路径 clear-data 插件ID 检查指纹`。UI 的清理入口位于插件管理底部，支持已卸载残留和损坏文档；符号链接及目录不会递归清理。设置、使用历史、插件包及缓存不受影响。详情见 [ADR 0030](adr/0030-plugin-data-management.md)。

## 主题数据协议 v1

`packages/plugin-protocol/theme.go` 与 TypeScript 同步定义 `ThemeDefinition` / `ThemeState`。定义字段为 `version: "1"`、命名空间 `id`、`name`、`appearance: "light" | "dark"` 和 `tokens`；状态包含 `mode: "system" | "light" | "dark"`、`revision`、`light` / `dark` 两套定义及可选错误。注册器要求 ID 属于提供者命名空间，每批 1–16 套，整批校验后发布并复制数据。

颜色角色固定为 background、surface、text、muted、placeholder、line、accent、accentHover、onAccent、hover、selected、error、errorBackground、danger、warning、warningBackground、warningLine、outputLine，值均为 `#RRGGBB`。映射为 `--sinan-<kebab-case>`，例如 `--sinan-text`、`--sinan-error-background`；页面可用 `color: var(--sinan-text)` 主动适配。

`packages/plugin-sdk/theme/index.js` 导出 `currentTheme()`（未就绪时 undefined）和 `onThemeChanged(listener)`（返回取消订阅函数）。快照包含 mode、revision、id、appearance 和 tokens，读取时复制；系统模式按当前系统明暗产生快照。宿主在 HTML 文档开始时注入固定呈现运行时，经私有父子通道同步主题，不依赖页面桥 1 / 2 权限，不提供完整 Wails Bridge。

当前仅内置浅/深色可选，未支持 manifest 声明主题或安装主题包。主题契约不允许任意 CSS、网络资源或可执行主题脚本。主程序与客体须同步重建。见 [ADR 0032](adr/0032-theme-contract-and-appearance.md)。

宿主私有窗口控制增加 `window-action/settings`，仅供原生客体把窗口内 Command + , 转交父进程；与 detach/back 不同，它也允许来自独立客体。此动作没有暴露给页面消息桥，页面不能据此获得设置服务访问能力。主程序和 sinan-ui-host 需同步构建，见 [ADR 0033](adr/0033-settings-navigation-shortcut.md)。


## 开发软链接插件

模板 v2 的 `build.mjs --dev` 在选定插件根目录创建 `<id> → <项目>/dist/plugin` 链接，宿主按开发模式注册规范化目标；只允许根目录外的目标且拒绝 system.*。包内链接、特殊文件和大小/数量限制保持。`PluginInfo` 增加 development、developmentFingerprint、needsTrust 管理元数据；`TrustDevelopmentPlugin(id,fingerprint)` 仅供可信管理组件使用，按当前内容核验后将原生开发信任保存在宿主 `.sinan-state/development-trust/` 并绑定目标路径，不执行候选程序。重载前原生首次调用仍拒绝未经信任代码。

同目标重建后按现有指纹标记变化并显式重载；未确认信任时重载只做静态校验，便于刷新信任预览。目标改指其他目录后拒绝沿用旧注册和信任，重启发现新目标也需重新确认。开发链接拒绝普通升级，卸载只 unlink，不删除外部产物。开发缓存仍位于宿主状态目录，不写入外部项目。后端 API 1 和第三方 UI 桥不新增调用能力。详见 [ADR 0037](adr/0037-plugin-template-dev-release-skill.md)。


## 同窗口动态页面承载

普通 macOS 打开路径现将独立配置的 WKWebView 作为主 NSWindow 的原生内容子视图，复用可信 React 页头。`OpenPluginCommand` 在可信窗口上下文中返回是否已在同窗呈现；成功后定向发送 plugin-view:open，页面就绪再提交替换。`ClosePluginView` 仅移除调用窗口的动态内容；返回、设置导航和窗口角色切换均撤销相应旧会话，迟到打开通过窗口序号拒绝。拆出复用同一窗口/内容树并更新窗口归属。

受限页面资源和命令调用直接适配内核会话，保留总计 256 次请求、8 MiB 资源交付预算、1 小时既有寿命上限，以及会话自己的消息/调用预算。没有新增第三方 Wails 方法或 UI 桥动作。UI 原生控制器与宿主同进程，WebKit 页面上下文独立，但不承诺独占渲染进程或 native 故障隔离；后端 stdio API 1、UI 桥 2 不变。


## 声明式输入匹配与打开参数

API 1 的增量可选字段 `commands[].inputMatchers` 用于本地匹配主输入框，多个规则 OR，不激活后端。支持普通命令或桥 2 HTML 视图命令，不支持可信组件 `view` 或 `searchOnly`。例如：

```json
{
  "id": "open",
  "title": "JSON 工具",
  "uiView": "editor",
  "inputMatchers": [
    {"kind": "json", "jsonTypes": ["object", "array"]}
  ]
}
```

通用正则示例：

```json
{"kind":"regex","pattern":"(?i)^https?://[^\\s]+$","minLength":8,"maxLength":2048}
```

`regex` 使用 Go/RE2 语法，默认子串匹配，全串需 `^` / `$`；不支持反向引用、前后查找。`json` 校验整段输入，只允许显式声明的 `object`、`array`、`string`、`number`、`boolean`、`null`。`minLength` / `maxLength` 为 UTF-8 字节数，省略或 0 不增加限制，统一受 64 KiB 上限约束。匹配和传参均使用未裁剪的原文。

每命令最多 8 条、每插件最多 32 条，每次搜索最多评估 256 条。正则最多 256 字节、256 条编译指令；编译缓存最多 256 条、不保存输入。每次查询按输入字节 × 指令数累计最多 8,388,608 工作量，超过则跳过该规则。JSON 每查询最多校验一次；这不是毫秒级硬实时保证。名称/拼音及动态 Provider 仍最多接收 4096 字节去空白查询。

用户选中内容结果后，可信 Shell 经 `OpenPluginCommandWithInput` 创建新会话；HTML 桥 2 在 ready 后读取：

```js
import { ready, readLaunch } from './bridge.js';
ready();
const launch = await readLaunch(); // {commandId?: string, input?: string}
if (launch.input !== undefined) {
  // 作为本次打开输入；普通打开不带 input，可恢复插件自己的草稿。
}
```

请求必须恰为 `{"method":"launch.get"}`，不得添加身份、路径或参数。结果只来自当前会话，重复读取仍为同一份打开参数，不能读取后续主输入框内容或其他会话。无需 storage 声明，不授予其他能力；沿用就绪、实例/指纹、撤销、在途/频率和响应预算。带参数打开仅桥 2 支持；输入最多 64 KiB，编码后超过 192 KiB 时在打开前拒绝。空字符串与省略 input 有区别。

旧宿主会拒绝新清单字段；使用此能力须同步更新宿主与插件。完整约束与验证见 [ADR 0043](adr/0043-declarative-input-matching.md)。


## 粘贴模式与图片输入

可信 Shell 的空主输入框显式粘贴后，宿主只按该内容生成匹配结果；旁边输入关键词仅过滤这组结果，不启动动态 Provider，也不改变将要传入的内容。图片插件声明：

```json
{"id":"open","title":"图片工具","uiView":"editor","inputMatchers":[{"kind":"image"}]}
```

`image` 规则没有 `pattern`、`jsonTypes` 或文本长度参数，必须关联桥 2 HTML 视图。它不匹配文件名或路径字符串。JSON 对象/数组声明与既有行为保持。

用户选中图片结果后，页面在 ready 后调用 `readLaunch()`：

```js
const launch = await readLaunch();
if (launch.image) {
  const { url, mimeType, width, height, bytes } = launch.image;
  // url 是当前会话的 sinan-plugin: 图片资源，可直接赋给 img.src。
}
```

图片为经过验证的静态 PNG，最多 4 MiB、16,777,216 像素；原尺寸保留，不保证动画/原文件元数据。图片地址随机生成，只在当前会话可读，撤销后不可访问，不会在普通打开或其他会话中复用。没有图片时省略 image；图片打开不附带 input。沿用 CSP 和总计 8 MiB 会话资源交付预算，固定包资源预算不变。不要把图片正文塞进普通 `execute` 或草稿请求。

可信 Shell 通过临时 pasteId 选择图片，不向页面开放主输入框、系统剪贴板、任意路径或安装导航接口。页面 `launch.get` 的请求形状、调用频率和 192 KiB 返回预算不变。helper 私有管道帧上限增至 6 MiB，普通后端消息上限不变（新增图片命令的独立预算见下文）；使用图片时须同步更新宿主与 helper。Esc 的统一返回由可信宿主原生路径处理，插件无需声明额外能力。详见 [ADR 0044](adr/0044-paste-mode-and-escape.md)。

## 可选插件图标

Manifest 可加 `"icon": "icon.svg"`，指向包内相对路径的 SVG、PNG 或 JPEG。文件最多 64 KiB，位图宽高各最多 1024 像素；SVG 仅支持静态基础形状及几何/颜色属性，不支持样式、脚本、外部引用、动画或实体。缺省、缺失或无效资源显示插件名首字，不影响插件运行。发布包须包含图标文件。现有搜索结果的 `icon` 优先于插件图标。

商店资料可选 `icon` 为 `data:image/svg+xml;base64,…`（也支持 PNG/JPEG），采用相同资源预算和内容校验；不接受远程图片 URL。API 仍为 1，旧包兼容；旧宿主严格清单解析不支持新增字段，需要升级后再安装带 `icon` 的新包。实现边界见 ADR 0046。


Linux GTK3 使用独立 ephemeral WebKitContext 和命名隔离脚本 world。页面的 WebKit 风格 SDK facade 只传递消息，原生端口不注册到页面 world；顶层、精确入口与可信同窗口事件由宿主注入的隔离脚本检查，再进入共用 Go 会话边界。数据包不能自报来源或请求任意插件身份。普通 GTK3 承载要求 WebKitGTK 2.40+，不增加旧独立 helper / ui-probe 的平台支持。协议、预算及验证限制见 [ADR 0054](adr/0054-gtk-isolated-plugin-content.md)。

## 插件全局热键、后台激活与系统输入（ADR 0056）

命令可添加 `shortcut: "CmdOrCtrl+Shift+V"`，必须关联 `uiView`。桌面按有效插件声明注册，禁用/卸载后释放（协调间隔一秒）；注册冲突进入可信界面诊断，不覆盖已有组合。原生 `activationEvents` 可添加 `onStartup`，桌面持续维持后台进程，显式停止后仍会恢复；禁用可持续停止。

原生进程 `initialize.params.dataDir` 为 API 1 可选增量字段，提供 `.sinan-state/native-data/<pluginId>` 持久目录，升级/重载/卸载保留。创建失败阻止进程激活。它不是缓存，不属于 UI JSON 文档管理服务，当前占用统计和清理不涵盖此目录；不要将此目录描述为凭据安全存储。

UI 桥 2 视图可声明 `permissions: ["input.paste"]`，并通过 SDK `pasteToPreviousApp()` 请求 `{ "method": "input.paste" }`，成功返回 `null`。请求不能包含其他字段；宿主从当前嵌入视图会话解析目标，恢复呼出前应用后发送平台粘贴组合。页面须先通过自己的后端写入系统剪贴板；此 API 不读取或返回剪贴板，不接受 PID、窗口或任意按键。会话撤销、目标不可用、焦点不符或平台权限不足时拒绝/取消。返回成功表示粘贴按键已发送，不保证目标应用接受或完成处理；系统发键失败会恢复页面并返回错误。macOS 需要辅助功能权限；缺少时宿主显示带「打开系统设置 / 稍后」的原生引导，第三方页面不能指定 URL。Linux 自动输入仅支持 X11 + xdotool；旧独立 helper 不提供输入动作。

原生清单 `permissions` 另可声明 `clipboard.read` / `clipboard.write` 用途。与原有应用用途一致，它们不构成 OS 沙箱或通用授权弹窗。


插件声明的 `shortcut` 是默认值：新版宿主允许用户在设置的「插件快捷键」区域录入、恢复默认并显式保存，用户覆盖优先于清单。配置由宿主按插件 ID / 命令 ID 保存，升级、重载及卸载保留；禁用或卸载释放绑定。保存对呼出、窗口独立与其他插件组合做冲突检查，注册/写入失败回退原组合。录入期间已有全局热键转为捕获事件。该配置服务仅提供给可信设置插件，不进入隔离页面桥。见 ADR 0057。

## 内容匹配分数

API 1 清单的可选 `commands[].inputMatchScore` 为 0～0.95 的有限数值，仅可与非空 `inputMatchers` 一起声明。例如宽泛文本入口可配置 `"inputMatchScore": 0.2`；省略为 0.95，显式 0 会保留。仅内容命中时使用该分数；名称/关键词/拼音同时命中仍至少 0.85，精确标题为 1，后续使用历史加分规则不变。粘贴内容结果同样先按此分数降序排序，同分按标题、结果键排序。

旧宿主会拒绝这个新字段，需要支持 [ADR 0061](adr/0061-content-match-ranking.md) 的新版宿主。后端 RPC、SDK、UI Bridge 2、匹配及打开参数预算均无变化。

## 加密私有文档

UI Bridge 2 视图可声明 `permissions: ["storage.encrypted"]`，通过 SDK `readEncryptedData()` / `writeEncryptedData(revision,value)` 请求 `storage.encrypted.get` / `storage.encrypted.put`。请求字段与普通文档相同，结果为 `{revision,value}`，独立于普通 `storage`，明文 JSON 上限 20 KiB。身份由会话绑定，禁止提交插件 ID、路径、主密钥或其他额外字段；沿用就绪、撤销、指纹、在途、频率及 500 ms 预算。

core 自动生成本地主密钥，以 AES-256-GCM 加密落盘；授权页面可以读取自己命名空间的明文，密钥与密文同时可读仍可解密。旧宿主拒绝新权限，SDK 与宿主须配套更新。数据管理返回可选 `encrypted: true`，清理指纹含 `encrypted:` 类别前缀；仅提供元数据，不返回值。详细边界见 [ADR 0063](adr/0063-encrypted-plugin-storage-and-llm-translation.md)。

## 共享 LLM

UI Bridge 2 视图可声明 `llm` 权限，调用 SDK `listLLMConfigurations()` 获取仅含 ID/名称的列表与默认 ID。`startLLM({profileId?,systemPrompt,input})` 对应 `{"method":"llm.start","value":{...}}`，返回 `{id}`；`pollLLM(id)` 和 `cancelLLM(id)` 分别对应 `llm.poll` / `llm.cancel`，字段为 `input`。`llm.config` 无参数。禁止多余字段或自行指定地址、模型、Key、插件身份。

显式配置 ID 优先于插件覆盖和全局默认，允许同一插件的不同服务选择不同配置。Key 由 core 持有，不返回插件。任务仅由创建会话使用，每会话最多 2 个、全局最多 16 个；关闭/撤销或配置变更取消。提示词上限 8 KiB、输入 20 KiB、任务 45 秒，桥本身仍为 500 ms。仅实现 UI 窄桥，不包含原生后台反向 RPC。旧宿主拒绝该权限，须配套更新。详见 [ADR 0064](adr/0064-shared-llm-capability.md)。

## 宿主系统剪贴板（ADR 0066）

此节更新前文「尚无系统 Host API」「剪贴板仅用途声明」的历史限制。初始化可选字段 `hostCapabilities` 包含 `clipboard.v1` 时，原生插件可在相同 stdio 流上请求宿主。反向请求使用标准 JSON-RPC 2.0，ID 在两个方向独立；响应没有 method，扫描循环必须同时处理请求和响应。缺少能力时应明确报告宿主不兼容，不尝试调用旧宿主。协议 API 仍为 1，不代表所有 API 1 宿主都具备此增量能力。

- `clipboard.read` 参数 `{sequence?: number}`，返回 `{content:{kind,data},sequence}`；data 为 base64，kind 为 text / image，空内容为 kind 空字符串、data null。首次可用 sequence 0，后续传上次值；Linux 无系统序列号时调用方仍需去重。未变化、空或受保护类型可返回空内容。
- `clipboard.write` 参数 `{content:{kind:"text"|"image",data:<base64>}}`，成功返回空内容的 ClipboardResult。非空 UTF-8 文本最多 256 KiB，禁止 NUL；图片仅 PNG，最多 8 MiB / 1600 万像素。不提供清空、富文本、文件列表或宿主监控订阅。
- 宿主根据进程清单中的同名权限拒绝未声明操作，参数不能指定其他插件或路径。清单声明不产生操作系统沙箱或新授权弹窗。
- 每进程最多 4 个在途反向请求，重复在途 ID 和超量请求终止进程；两秒预算，进程退出取消等待。已开始的同步原生操作不能保证强制撤销。
- 普通 RPC 的 1 MiB 消息限制不变；反向 clipboard.write 请求和 clipboard.read 响应可使用 12 MiB，扫描器上限相应调整。Go SDK 为新增能力提供有界并发与独立 ID 路由。

Go SDK 用法（命令上下文中）：

```go
host := sdk.HostFromContext(ctx)
err := host.ClipboardWrite(ctx, protocol.ClipboardContent{Kind: "text", Data: []byte("result")})
snapshot, err := host.ClipboardRead(ctx, 0)
```

后台任务可通过 `Handlers.HostReady` 捕获 `*sdk.HostClient`；此回调仅保存客户端，不同步发请求。`Initialize` 应检查 `HostCapabilities` 后再启动后台任务，初始化回调中不能同步等待宿主调用（读循环尚在处理初始化）。既有 SDK 接口与旧插件不需要改动。

页面使用 UI Bridge 2，在对应视图 permissions 声明 `clipboard.read` / `clipboard.write` 后调用 `readClipboardText()` / `writeClipboardText(text)`；请求分别为 `{method:"clipboard.readText"}` / `{method:"clipboard.writeText",input:text}`，成功分别返回 string / null。权限不继承顶层原生清单，声明式视图也可单独申请。最多 32 KiB 文本，维持 64 KiB 请求、192 KiB 响应与 500 毫秒预算，JSON 转义超预算仍拒绝；非文本剪贴板返回错误，图片快照使用下述会话图片接口。页面关闭、包变化或禁用仍按现有会话撤销流程处理。

平台依赖及未验收边界见 [ADR 0066](adr/0066-host-clipboard-api.md) 与[跨平台适配](cross-platform.md)。


## 会话图片命令（工作树新增，未发布）

初始化能力 `ui.image-command.v1` 表示宿主支持以下 UI Bridge 2 扩展。不能将已发布 v0.2.1 或相同 API 1 视为具备此能力；原生插件应检查 `hostCapabilities`，不支持时明确报错。

- `readClipboardImage()` → `{method:"clipboard.readImage"}`，须在当前视图声明 `clipboard.read`。宿主显式读取一次系统剪贴板，验证静态 PNG（最多 4 MiB、16,777,216 像素并完整解码），返回与 `launch.image` 相同的 `{url,mimeType,width,height,bytes}`，不把图片正文返回页面。非图片返回 null（快捷键可保留普通文本粘贴）；平台不可用/无效图片/超限报错。此调用不是剪贴板监听，也不承诺与浏览器 paste event 的 Blob 相同；调用者应在用户粘贴或点击识别时使用，并让预览与后台共同使用返回的快照。
- `executeImage(commandId,url)` → `{method:"command.executeImage",commandId,input:url}`。命令须在当前视图 `commands` 白名单中，URL 必须精确指向当前会话持有的图片；不接受文件路径、其他会话、普通 bundle 资源或页面图片正文。读取主输入框显式带入的 `launch.image` 无须额外剪贴板权限。
- 每会话只保留一份当前图片。新的剪贴板快照成功后替换旧图及随机 URL；失败不替换，旧 URL 在成功替换后不可继续读取或执行，`readLaunch()` 仍仅描述初次打开参数。撤销清空图片。原图预览继续走既有隔离资源策略，256 次请求/8 MiB 累计交付预算不变，耗尽需重新打开视图。
- 宿主把快照经后端 RPC `command.executeImage` 交给同一插件实例，参数为 `{commandId,input:"",image:<PNG base64>}`（Go `CommandParams.Image []byte`）；页面桥请求不含正文，保持 64 KiB/192 KiB、500 ms、单在途及每分钟 120 次限制。仅该后端方法允许小于 6 MiB 的请求帧；SDK 拒绝空图和超过 4 MiB 的图片，普通 `command.execute` 禁止携带图片，普通 RPC 的 1 MiB 限制不变。识别等耗时业务仍应在插件后台异步执行。

UI 与后端权限不互相继承。不增加网络、文件访问或完整 Wails Bridge，不声称独立进程具备安全沙箱。此扩展与 OCR worker 的私有 stdin 管道独立，见 [ADR 0068](adr/0068-session-image-commands.md)。
