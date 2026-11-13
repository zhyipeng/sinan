# ADR 0051：Windows 与 Linux 同步适配的基础设施

日期：2026-11-13。状态：首轮已实现，目标平台原生 CI 及运行验收待执行。

## 决策与原因

用户要求 Windows、Linux 同步推进跨平台适配。保持 Wails 3.0.0 与 All-in-Plugin 边界，先移除应用检索及开发构建的 macOS 独占前提，再实现各平台隔离 UI。

- 公共 Catalog 保留发现、缓存、排名与不透明 ID 校验；默认目录、候选条目和启动交给平台适配。应用检索业务仍在插件层，不进入内核。
- Windows 索引开始菜单中的快捷方式和可执行文件，以 ShellExecute 启动；Linux 索引 XDG 桌面文件，以 GIO 启动，避免自行解析 Exec 为 shell 字符串。
- 以 Node 原生进程 API 提供 Make/PowerShell 共用入口，不通过 shell 拼接路径。Windows 构建 `.exe` 并在开发包中生成对应清单入口；不更改公开 manifest 或 RPC 协议。
- Windows 原生插件入口按 `.exe` 校验，Unix 继续要求执行位；路径边界、普通文件和符号链接检查保持。Windows 宿主使用 GUI 子系统构建，插件进程隐藏控制台窗口。
- 原生后端只接收固定的语言及目标平台桌面环境变量白名单：Windows 的系统、用户应用目录与 PATH/TEMP，Linux 的 HOME/PATH、XDG、显示服务及会话总线地址。不整表继承环境，不转交任意认证变量。macOS 仅转交指定语言变量。白名单供原生桌面集成使用，不是新增页面桥能力或 OS 沙箱。
- Linux 原生构建显式使用 Wails 的 GTK3 标签及 WebKitGTK 4.1，增加三平台构建 CI。Windows 历史 POSIX 测试夹具未完成迁移，先运行新增适配与前端回归。

## 影响和限制

macOS 的 Bundle 解析、图标及启动行为保持。快照 v1 接受新增应用文件后缀，执行仍按当前平台检查条目类型。无新增权限系统或沙箱保证。

Windows/Linux 的图标、打包应用专用发现、隔离插件 UI、主题与光标屏幕原生适配、发行安装流程尚未完成。不得通过复用可信 Wails WebView 或扩大 Bridge 代替隔离 UI。可执行文件目录仅为构建产物，不视作完整安装包。

本地 macOS 自动回归及交叉编译与目标系统实际可用性分开记录，新增 CI 未执行前不声称通过。详细命令、限制和后续顺序见 [跨平台适配](../cross-platform.md)。
