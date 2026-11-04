# 插件接口 v1

本页描述当前实现，完整规划见[架构指导方案](architecture.md)。运行时取舍见 [ADR 0001](adr/0001-phase-one-runtime.md) 和 [ADR 0003](adr/0003-search-and-ui-prototype.md)。

## Manifest

参考 [Schema](../schemas/manifest.schema.json) 和[示例插件](../plugins/)。当前支持 `declarative`、`process`，API 版本为 `1`；未知字段、重复命令 ID、未声明贡献的激活事件会被拒绝。Schema 供编辑器提示，宿主还校验路径、引用和唯一性。

每个插件至少提供一个命令。进程插件可额外声明 `contributes.searchProviders`：

```json
{"searchProviders":[{"id":"demo","prefix":"","minLength":1}]}
```

`prefix` 为不区分大小写的前缀，`minLength` 按 Unicode 字符计数。支持 `onCommand:<id>` 及有搜索贡献时的 `onSearch`；当前通过命令调用或被选中的搜索贡献按需激活，不支持启动激活。`permissions` 仅接受空数组，尚无系统 Host API。

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

Go 命令插件继续使用 `sdk.Serve`；搜索插件使用 `sdk.ServeWithHandlers`，搜索处理函数接收 `context.Context`，必须响应取消。SDK 允许并行请求、序列化响应；可参考两个[搜索示例](../plugins/example-search-fast/manifest.json)。TypeScript 协议声明位于 `packages/plugin-protocol/index.ts`，尚无完整 TypeScript 运行时 SDK。

`sinanctl search 查询` 按增量输出聚合 JSON，包含 `queryId`、`items`、`pending`、`failures` 和 `done`。CLI 退出时回收进程；桌面保持已激活进程。CLI 与桌面不要同时修改同一插件目录，当前无跨进程安装锁。

新插件导入后，从桌面 `··· → 刷新` 发现变化，无需重编译宿主。刷新会终止已移除或 manifest 改变的实例；升级请先卸载再安装。尚未实现心跳、优雅关闭、事件总线、跨插件 RPC 或插件侧逐条流式结果。
