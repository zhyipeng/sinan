# 跨平台适配

Windows 与 Linux 同步推进，macOS 保持回归。已增加应用检索、原生构建入口与 Shell 适配，并实现待实机验收的 Linux GTK3 与 Windows WebView2 隔离内容承载；还不能将 Windows/Linux 描述为功能完整的平台。

## 构建与开发

需要 Go 1.26+、Node.js 22.12+ 和固定版本 CLI。以下 CLI 安装命令适用于 macOS/Windows，Linux 的依赖与 CLI 安装命令见下方：

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
node scripts/desktop.mjs setup
node scripts/desktop.mjs build
```

Windows 可直接在 PowerShell 中运行，不需要 Make 或 sh。Windows 桌面运行还需要 WebView2 Runtime。Linux 首轮以 Ubuntu 24.04、GTK3/WebKitGTK 4.1 为构建基线，保持 Wails 3.0.0，不升级依赖：

```sh
sudo apt-get install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev
go install -tags gtk3 github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
```

Linux 的构建、绑定生成和回归均使用 `gtk3` 标签及 CGO。关闭 CGO 只能检查 CLI、内核和应用检索等非桌面包，不能证明 Linux 桌面可构建。脚本用于目标系统本机开发，不是交叉打包工具。

Linux 安装 Wails CLI 本身也必须带 `-tags gtk3`，因为 CLI 依赖的系统检测包同样通过构建标签选择 GTK 版本。不带标签会查找 `gtk4` / `webkitgtk-6.0`，与本项目的 GTK3/WebKitGTK 4.1 依赖不匹配；CI 的 Linux 安装步骤使用相同参数。

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

Windows amd64/arm64 默认构建现接入独立 WebView2 内容承载；Linux 的 GTK3 构建接入独立 WebKit 内容承载。不满足原生构建条件的配置仍报告不支持。可信设置与插件中心不依赖该适配。新增 CI 尚需在远端运行，任何交叉编译或无显示服务器的容器构建均不能代替目标平台运行验收。真实 UI 验收仅在用户明确要求时执行。

## 第二轮进展

原生 renderer 的导航、资源、消息来源与 frame 校验现统一到 Go 会话边界，并已接入 macOS。WebView2 和 WebKitGTK 后续适配器应直接复用该边界；来源和 frame 必须取自引擎事件，不能相信页面自报字段。无窗口回归覆盖拒绝路径、预算与撤销后的迟到响应。

Windows 新增光标屏幕选择，保持 Wails 逻辑工作区转换；Linux GTK3 新增 X11 光标屏幕与工作区查询，Wayland 明确回退。Windows 读取应用主题偏好并同步原生窗口；Linux 使用 GNOME GSettings 读取/观察系统偏好，仅修改应用内 GTK 设置，无对应 Schema 时回退。其他 Linux 桌面的主题来源仍需适配。两端托盘使用普通应用图标。详见 [ADR 0053](adr/0053-shared-native-ui-boundary.md)。

## GTK3 隔离内容承载

Linux UI 要求 WebKitGTK 引擎版本至少 2.40，使用独立 ephemeral Context、命名脚本 world 和同窗口原生内容子视图，复用可信 React 页头。页面 ready、来源转交、资源、消息、调用和撤销仍绑定宿主持有的会话，不接入 Wails Bridge。目标平台真实 UI 验收与完整发行流程仍待完成。详见 [ADR 0054](adr/0054-gtk-isolated-plugin-content.md)。

本轮验证已通过 macOS 全量无界面回归、Linux ARM64 容器内全量 Go 竞态回归/桌面构建/静态检查，以及 Windows amd64/arm64 全仓交叉编译。容器没有显示服务器，未运行桌面或 WebView 交互；远端 CI 与目标系统真实 UI 仍需分别验收。


## Windows WebView2 隔离内容承载

Windows amd64/arm64 默认构建现创建独立 Environment、临时配置目录和 InPrivate Profile，要求已安装 WebView2 Runtime（目标版本 119+）。不满足必要接口或策略即关闭，不能回退到可信 Wails 页面。替代 `native_webview2loader` 标签及其他架构暂不支持此承载。

每次视图使用独立虚拟 HTTPS 源，资源直接来自会话快照；不启动 HTTP 服务或映射磁盘目录。SDK 窄桥由引擎顶层消息的精确来源校验，资源、调用和撤销继续经过共有 Go 边界。保留可信页头、主题、原生快捷键和拆出流程；原生视图按 Windows DPI 布局。临时配置目录关闭后尽力清理，不能把 InPrivate 等同于内存存储或操作系统沙箱。详见 [ADR 0055](adr/0055-webview2-isolated-plugin-content.md)。

本轮验证为 macOS 全量无界面回归、Windows amd64/arm64 全仓交叉编译及测试可执行文件编译、Windows 静态检查，以及 Linux ARM64 容器中的 GTK3 UI 边界包竞态回归。原生 COM 回调测试已纳入 Windows CI，尚未本机执行；真实窗口、引擎策略效果、层叠、焦点、输入法、拆出及多屏缩放均待明确安排的实机验收。没有运行桌面 UI 或浏览器交互回归。

## 自动构建与便携发行

已接入 `.github/workflows/desktop-build.yml`，由普通 CI 与标签发布共用，在 macOS、Windows、Ubuntu 各自的 amd64/arm64 runner 上原生构建。`v*` 标签经版本校验、六目标打包及对应无界面检查后发布 GitHub Release；Windows 仍执行已迁移的测试子集。平台与架构在归档前核对，缺失资产或 SHA-256 不符阻止发布。具体触发、依赖、重跑与保护规则见[自动发布](releases.md)。远端实际运行结果仍待验证。

`package` 现将最小内置插件快照编入宿主，默认用户目录正常启动时仅安装缺失插件；已有安装保持原样。macOS `.app` 包含 CLI 和 helper，归档前只做 ad-hoc 签名。Windows/Linux 继续使用便携目录，首次安装内置插件已补齐，但安装器、发行签名、系统依赖打包与真实桌面验收未完成。该段更新前文关于「尚无内置插件发行安装流程」的历史状态，不改变其余平台限制。

本轮本地验证：macOS arm64 打包、ad-hoc 签名验证、归档与 SHA-256、全量无界面回归、新增内置插件初始化竞态测试、桌面静态检查及 actionlint 均通过；Windows arm64 桌面测试可执行文件交叉编译通过，未在 Windows 运行。其他五个目标的原生 Actions 构建和真实发行尚未执行，未进行 UI 验收。

### 首轮远端构建结果与 Linux CLI 安装修复

提交 `4949fc9` 的[首轮 Actions](https://github.com/zhyipeng/sinan/actions/runs/43567782447)中，macOS arm64/amd64、Windows amd64/arm64 的原生构建、对应测试和归档上传通过；Linux 两种架构均在 Wails CLI 安装步骤失败，原因是未带 `gtk3` 标签而查找 GTK4/WebKitGTK 6.0。已修正 Linux 安装参数。使用同一已提交源码快照，在本地 Linux arm64 无显示服务器容器中验证修正后的 CLI 安装、桌面打包、全量无界面竞态回归及发行归档均通过；依赖通过本机只读缓存提供，Go 校验保持开启。修复后的 Ubuntu amd64/arm64 远端任务仍待重新运行，以上均不代表实机 UI 验收。
