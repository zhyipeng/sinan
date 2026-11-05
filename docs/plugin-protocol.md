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

`runtime.entry` 指向插件目录内的可执行文件，例如 `backend/plugin`。不支持 shell 命令、参数拼接、平台变量或符号链接。跨平台需单独构建。安装来源不能包含安装目录，总大小上限 64 MiB，最多 2048 个条目。

原生插件以当前用户权限运行，安装时必须确认信任。宿主不继承环境变量，stderr 当前丢弃；这些措施不构成安全沙箱。信任记录不防御同一用户改写插件文件。

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

单条消息上限约 1 MiB，命令输入上限 64 KiB，搜索查询上限 4096 字节，每批搜索最多采用 20 项。搜索预算 2 秒，取消宽限 500 ms；不合作插件会被终止。命令预算仍为 5 秒，调用错误或超时会终止进程。停止后下一次调用可重新激活，卸载则使旧实例失效。

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

应用检索 0.3.0 通过 `contributes.settings` 声明 `[{"id":"directories","title":"额外应用目录","type":"directory-list"}]`。当前每插件最多 16 项，每项最多 16 个目录，写入时验证存在的目录绝对路径。宿主通过 `initialize.params.settings` 提供当前插件的配置，缺失项为空列表；SDK 的 `Handlers.Initialize` 在开始处理命令前读取快照。保存后只停止受影响的进程，下次调用重新激活，无插件侧热更新通知。

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
