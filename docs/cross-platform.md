# 跨平台适配

Windows 与 Linux 同步推进，macOS 保持回归。已增加应用检索、原生构建入口与 Shell 适配，并实现待实机验收的 Linux GTK3 隔离内容承载；还不能将 Windows/Linux 描述为功能完整的平台。

## 构建与开发

需要 Go 1.26+、Node.js 22.12+ 和固定版本 CLI：

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
node scripts/desktop.mjs setup
node scripts/desktop.mjs build
```

Windows 可直接在 PowerShell 中运行，不需要 Make 或 sh。Windows 桌面运行还需要 WebView2 Runtime。Linux 首轮以 Ubuntu 24.04、GTK3/WebKitGTK 4.1 为构建基线，保持 Wails 3.0.0，不升级依赖：

```sh
sudo apt-get install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev
```

Linux 的构建、绑定生成和回归均使用 `gtk3` 标签及 CGO。关闭 CGO 只能检查 CLI、内核和应用检索等非桌面包，不能证明 Linux 桌面可构建。脚本用于目标系统本机开发，不是交叉打包工具。

`node scripts/desktop.mjs dev` 构建后，准备 `.sinan-dev` 中的三个内置插件并运行开发实例；应用检索的 Windows 清单入口为 `backend/plugin.exe`，其他平台为 `backend/plugin`。`prepare` 仅准备开发包，`test` 仅执行无界面回归。已有 `make` 入口继续可用。

`package` 在 macOS 继续生成 `.app`；Windows/Linux 仅生成 `bin/Sinan-<platform>-<arch>` 可执行文件目录，尚无安装器、签名、桌面注册、运行依赖打包及内置插件发行安装流程，不能视为完整发行包。

## 应用检索

业务实现全部保留在 `system.app-search` 插件，复用有界扫描、搜索排名、后台校验、可丢弃缓存及不透明 ID 启动规则。

| 平台 | 默认索引范围 | 启动机制 | 当前限制 |
| --- | --- | --- | --- |
| macOS | 系统与用户 Applications | `/usr/bin/open -a` | 保持原有 Bundle 元数据和图标 |
| Windows | APPDATA、PROGRAMDATA 下开始菜单 Programs；附加目录 | ShellExecute 打开 `.lnk` / `.exe` | 使用文件名；图标、打包应用专用枚举及快捷方式元数据待补 |
| Linux | XDG_DATA_HOME、XDG_DATA_DIRS 的 applications；附加目录 | `gio launch <绝对路径>` | 需要 GIO 和桌面会话；图标待补 |

Linux 解析 `[Desktop Entry]`，支持本地化名称、Hidden、NoDisplay、OnlyShowIn、NotShowIn、TryExec，按根目录优先级用 desktop-file ID 屏蔽低优先级项。`Exec` 参数与字段代码交给 GIO，不交给 shell，也不自行拆分。读取预算为 256 KiB。Linux 显示过滤、本地化和 TryExec 变化沿用现有缓存刷新周期，并非实时系统订阅。

Windows 通过初始化 COM 的固定线程调用 ShellExecute，不拼接 cmd.exe 命令。宿主按 GUI 子系统构建；原生插件按 `.exe` 而非 POSIX 执行位检查，启动时隐藏控制台。后端环境使用显式白名单提供平台系统/应用目录、语言及必要桌面会话变量，不整表继承宿主环境。两端均只接受当前索引中的应用 ID，执行前再次检查条目存在、类型与元数据；不接受外部输入的任意路径。该机制不构成操作系统沙箱。

## 验证与后续顺序

`.github/workflows/platforms.yml` 为 macOS、Windows、Ubuntu 建立原生构建矩阵；macOS/Linux 执行现有无界面回归，Windows 先执行新增平台适配测试和前端测试。历史集成测试的 POSIX 文件模式、链接与进程夹具还需要逐项迁移，不能把这个 Windows 测试范围等同于完整回归。

1. 基础构建、开发入口与应用检索（本轮）。
2. Windows/WebView2 与 Linux/WebKitGTK 的隔离 UI：独立配置、资源协议、窄消息桥、撤销与同窗口承载；不得退化为注入可信主页面。
3. 各平台呼出、失焦、焦点、拆出、输入法、屏幕缩放、系统主题与托盘适配。Linux 需分别确认 X11、Wayland 的能力与降级。
4. Windows 全量测试夹具、安装器、Linux 发行格式、签名与对应平台验收。

Windows HTML 插件 UI 仍明确报告不支持；Linux 的 GTK3 构建现接入独立 WebKit 内容承载，其他构建配置仍报告不支持。可信设置与插件中心不依赖该适配。新增 CI 尚需在远端运行，任何交叉编译或无显示服务器的容器构建均不能代替目标平台运行验收。真实 UI 验收仅在用户明确要求时执行。

## 第二轮进展

原生 renderer 的导航、资源、消息来源与 frame 校验现统一到 Go 会话边界，并已接入 macOS。WebView2 和 WebKitGTK 后续适配器应直接复用该边界；来源和 frame 必须取自引擎事件，不能相信页面自报字段。无窗口回归覆盖拒绝路径、预算与撤销后的迟到响应。

Windows 新增光标屏幕选择，保持 Wails 逻辑工作区转换；Linux GTK3 新增 X11 光标屏幕与工作区查询，Wayland 明确回退。Windows 读取应用主题偏好并同步原生窗口；Linux 使用 GNOME GSettings 读取/观察系统偏好，仅修改应用内 GTK 设置，无对应 Schema 时回退。其他 Linux 桌面的主题来源仍需适配。两端托盘使用普通应用图标。详见 [ADR 0053](adr/0053-shared-native-ui-boundary.md)。

## GTK3 隔离内容承载

Linux UI 要求 WebKitGTK 引擎版本至少 2.40，使用独立 ephemeral Context、命名脚本 world 和同窗口原生内容子视图，复用可信 React 页头。页面 ready、来源转交、资源、消息、调用和撤销仍绑定宿主持有的会话，不接入 Wails Bridge。Windows WebView2 renderer、目标平台真实 UI 验收与完整发行流程仍待完成。详见 [ADR 0054](adr/0054-gtk-isolated-plugin-content.md)。

本轮验证已通过 macOS 全量无界面回归、Linux ARM64 容器内全量 Go 竞态回归/桌面构建/静态检查，以及 Windows amd64/arm64 全仓交叉编译。容器没有显示服务器，未运行桌面或 WebView 交互；远端 CI 与目标系统真实 UI 仍需分别验收。
