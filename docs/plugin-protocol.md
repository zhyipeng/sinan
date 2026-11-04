# Phase 1 插件接口

本页描述当前实现，完整规划见[架构指导方案](architecture.md)。取舍见 [ADR 0001](adr/0001-phase-one-runtime.md)。

## Manifest

参考 [Schema](../schemas/manifest.schema.json) 和两个[示例](../plugins/)。当前支持 `declarative`、`process`，API 版本为 `1`；未知字段、重复命令 ID、未声明命令的激活事件均会被拒绝。Schema 供编辑器提示，宿主通过 Go 校验器进行运行时校验，包括路径、事件引用和命令唯一性检查。

当前原型按显式命令执行激活；`activationEvents` 可声明 `onCommand:<id>`，不支持启动激活。`permissions` 仅接受空数组，尚无系统 Host API。插件名称和协议内容不会作为 HTML 渲染。

## 进程入口

`runtime.entry` 指向插件目录内的实际可执行文件，例如 `backend/plugin`。不支持 shell 命令、参数拼接、平台变量展开或符号链接。跨平台插件需要为对应平台单独构建并准备插件目录。安装来源不能包含安装目标目录，文件总大小上限 64 MiB，最多 2048 个文件/目录条目。

原生插件以当前用户权限运行，必须可信。本地信任记录不保护插件文件被同一用户改写；此原型不提供公共分发或安全沙箱。

## JSON-RPC

UTF-8 JSON 对象按换行分帧，stdout 只能输出协议消息，stderr 当前丢弃。消息需要 `jsonrpc: "2.0"` 和非零数字 ID，响应必须恰有 `result` 或 `error` 之一。当前为宿主发起的单向请求，不支持插件反向调用 Host API。

宿主先发起握手：

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"apiVersion":"1","pluginId":"example.echo"}}
```

插件响应：

```json
{"jsonrpc":"2.0","id":1,"result":{"apiVersion":"1","pluginId":"example.echo"}}
```

随后执行命令：

```json
{"jsonrpc":"2.0","id":2,"method":"command.execute","params":{"commandId":"echo","input":"你好"}}
```

```json
{"jsonrpc":"2.0","id":2,"result":{"text":"你好"}}
```

Go 插件可复用 `sinan/packages/plugin-sdk` 的 `Serve`，TypeScript 协议声明位于 `packages/plugin-protocol/index.ts`。此时尚无完整的 TypeScript 运行时 SDK。

执行上限 5 秒；超时、取消和停止直接终止进程，下一次命令可重新启动。当前未实现流式结果、协作取消、心跳、自动重启、事件总线和跨插件 RPC。

## 开发工具

`sinanctl` 与桌面宿主使用同一内核；CLI 每次运行退出时关闭所启动的进程。桌面宿主保持已激活进程，用户可通过本地插件入口停止。CLI 与桌面不要同时修改同一个插件目录；首轮尚无跨进程安装锁。

新插件导入安装目录后，桌面点击“插件 → 刷新”发现变化，无需重新编译宿主。刷新会终止已移除或 manifest 发生变化的插件进程；更新现有插件请先停止、卸载，再安装。
