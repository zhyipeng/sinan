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

`node scripts/desktop.mjs dev` 构建后，准备 `.sinan-dev` 中的三个内置插件并运行开发实例；应用检索的 Windows 清单入口为 `backend/plugin.exe`，其他平台为 `backend/plugin`。`prepare` 仅准备开发包，`test` 仅执行无界面回归。已有 `make` 入口继续可用。Windows 无 make 时使用 `scripts/dev.cmd`（PowerShell 7 包装 `desktop.mjs dev`）：检查 Node.js/Go/wails3、依赖缺失时执行 `setup`、构建前结束仓库内旧 `bin\sinan.exe` 并等待其 WebView2 子进程释放数据目录，并支持 `-Setup`、`-Clean`、`-NoStop`、`-ResetWebView`、`-DryRun`；`-DryRun` 只检查环境不启动桌面。开发实例通过新增的 `-webview-data-dir` 使用 `.sinan-dev\webview2`，与已安装司南的默认 `%APPDATA%\sinan.exe` 隔离，避免 WebView2 `0x800700aa`（用户数据目录被其他 WebView2 进程占用）。

三平台共用的后台协调退出逻辑采用可取消的原生快捷键投递，避免退出主线程等待后台循环、后台循环又等待主线程的死锁。无界面回归模拟注册/注销排队与空闲退出，并验证目录租约可重取；真实 Ctrl+C 和各平台桌面退出未据此验收。目录锁由 OS 随持有进程退出释放，不应通过删除锁文件绕过，见 [ADR 0062](adr/0062-plugin-runtime-shutdown.md)。

`package` 在 macOS 继续生成 `.app`；Windows/Linux 仅生成 `bin/Sinan-<platform>-<arch>` 可执行文件目录，尚无安装器、签名、桌面注册、运行依赖打包及内置插件发行安装流程，不能视为完整发行包。

## 应用检索

业务实现全部保留在 `system.app-search` 插件，复用有界扫描、搜索排名、后台校验、可丢弃缓存及不透明 ID 启动规则。

| 平台 | 默认索引范围 | 启动机制 | 当前限制 |
| --- | --- | --- | --- |
| macOS | 系统与用户 Applications | `/usr/bin/open -a` | 保持原有 Bundle 元数据和图标 |
| Windows | APPDATA、PROGRAMDATA 下开始菜单 Programs；附加目录 | ShellExecute 打开 `.lnk` / `.exe` | 使用文件名；已接入 Shell 图标提取；打包应用专用枚举及其他快捷方式元数据待补 |
| Linux | XDG_DATA_HOME、XDG_DATA_DIRS 的 applications；附加目录 | `gio launch <绝对路径>` | 需要 GIO 和桌面会话；图标待补 |

Linux 解析 `[Desktop Entry]`，支持本地化名称、Hidden、NoDisplay、OnlyShowIn、NotShowIn、TryExec，按根目录优先级用 desktop-file ID 屏蔽低优先级项。`Exec` 参数与字段代码交给 GIO，不交给 shell，也不自行拆分。读取预算为 256 KiB。Linux 显示过滤、本地化和 TryExec 变化沿用现有缓存刷新周期，并非实时系统订阅。

Windows 通过初始化 COM 的固定线程调用 ShellExecute，不拼接 cmd.exe 命令。宿主按 GUI 子系统构建；原生插件按 `.exe` 而非 POSIX 执行位检查，启动时隐藏控制台。后端环境使用显式白名单提供平台系统/应用目录、语言及必要桌面会话变量，不整表继承宿主环境。两端均只接受当前索引中的应用 ID，执行前再次检查条目存在、类型与元数据；不接受外部输入的任意路径。该机制不构成操作系统沙箱。

应用检索 0.5.1 在 Windows 适配内通过 `SHGetFileInfoW` 按实际条目读取 `.exe` 与 `.lnk` 图标，由 Shell 解析快捷方式自定义图标或目标图标；在初始化 COM 的固定后台线程内完成提取，不打开目标应用。离屏 GDI 绘制转为 64×64 PNG data URL，通过黑白背景重建透明度以兼容 alpha 与传统掩码图标，释放 HICON、位图和 DC。复用现有按结果读取和内存缓存，失败返回空图标并沿用默认展示。未增加 SDK、协议、权限或宿主业务，不涉及两个独立插件仓库的格式联动；macOS 图标实现与 Linux 图标待补状态保持。

本次本地验证包括 macOS 应用插件竞态回归、透明度像素转换测试和静态检查，Windows amd64/arm64 插件与测试程序交叉编译、静态检查，以及 Linux amd64/arm64 插件交叉构建。Windows 无窗口原生用例覆盖可执行文件图标、中文路径、透明 PNG 和自定义图标快捷方式，沿用现有 CI 的 `TestPlatform` 筛选；本地未运行这些 Windows 原生用例，远端 CI 结果与实际搜索界面显示仍待验证。未启动桌面 UI 或浏览器。

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

Windows amd64/arm64 默认构建现创建独立 Environment、每视图独立配置目录（优先 `%LOCALAPPDATA%\Sinan\webview2`，不可用时回退系统临时目录）和 InPrivate Profile，要求已安装 WebView2 Runtime（目标版本 119+）。不满足必要接口或策略即关闭，不能回退到可信 Wails 页面。替代 `native_webview2loader` 标签及其他架构暂不支持此承载。

每次视图使用独立虚拟 HTTPS 源，资源直接来自会话快照；不启动 HTTP 服务或映射磁盘目录。SDK 窄桥由引擎顶层消息的精确来源校验，资源、调用和撤销继续经过共有 Go 边界。保留可信页头、主题、原生快捷键和拆出流程；原生视图按 Windows DPI 布局。临时配置目录关闭后尽力清理，不能把 InPrivate 等同于内存存储或操作系统沙箱。详见 [ADR 0055](adr/0055-webview2-isolated-plugin-content.md)。

Windows 主窗口运行时就绪后后台预热一个隐藏的空控制器。一旦被打开请求接管，该控制器及其独立数据目录随插件会话销毁；没有可接管控制器时按需创建。窗口层叠使用宿主显式创建的子 HWND。Windows 页面准备预算为 20 秒，macOS/Linux 恢复 3 秒。macOS arm64 标准无界面回归及 Linux arm64 GTK3 目标测试与构建通过，Windows amd64/arm64 桌面交叉构建、测试程序交叉编译及静态检查通过。Windows 实际加载耗时、可见性、焦点、输入法、DPI、截图隐藏启动及拆出待实机验证。详见 [ADR 0083](adr/0083-webview2-warm-controller.md)。

隔离承载首次接入时完成 macOS 全量无界面回归、Windows amd64/arm64 全仓交叉编译及测试可执行文件编译、Windows 静态检查，以及 Linux ARM64 容器中的 GTK3 UI 边界包竞态回归。原生 COM 回调测试已纳入 Windows CI，尚未本机执行；真实窗口、引擎策略效果、层叠、焦点、输入法、拆出及多屏缩放均待明确安排的实机验收。没有运行桌面 UI 或浏览器交互回归。

## 自动构建与便携发行

已接入 `.github/workflows/desktop-build.yml`，由普通 CI 与标签发布共用，在 macOS、Windows、Ubuntu 各自的 amd64/arm64 runner 上原生构建。`v*` 标签经版本校验、六目标打包及对应无界面检查后发布 GitHub Release；Windows 仍执行已迁移的测试子集。平台与架构在归档前核对，缺失资产或 SHA-256 不符阻止发布。具体触发、依赖、重跑与保护规则见[自动发布](releases.md)。远端实际运行结果仍待验证。

`package` 现将最小内置插件快照编入宿主，默认用户目录正常启动时仅安装缺失插件；已有安装保持原样。macOS `.app` 包含 CLI 和 helper，归档前只做 ad-hoc 签名。Windows/Linux 继续使用便携目录，首次安装内置插件已补齐，但安装器、发行签名、系统依赖打包与真实桌面验收未完成。该段更新前文关于「尚无内置插件发行安装流程」的历史状态，不改变其余平台限制。

本轮本地验证：macOS arm64 打包、ad-hoc 签名验证、归档与 SHA-256、全量无界面回归、新增内置插件初始化竞态测试、桌面静态检查及 actionlint 均通过；Windows arm64 桌面测试可执行文件交叉编译通过，未在 Windows 运行。其他五个目标的原生 Actions 构建和真实发行尚未执行，未进行 UI 验收。

### 首轮远端构建结果与 Linux CLI 安装修复

提交 `4949fc9` 的[首轮 Actions](https://github.com/zhyipeng/sinan/actions/runs/43567782447)中，macOS arm64/amd64、Windows amd64/arm64 的原生构建、对应测试和归档上传通过；Linux 两种架构均在 Wails CLI 安装步骤失败，原因是未带 `gtk3` 标签而查找 GTK4/WebKitGTK 6.0。已修正 Linux 安装参数。使用同一已提交源码快照，在本地 Linux arm64 无显示服务器容器中验证修正后的 CLI 安装、桌面打包、全量无界面竞态回归及发行归档均通过；依赖通过本机只读缓存提供，Go 校验保持开启。修复后的 Ubuntu amd64/arm64 远端任务仍待重新运行，以上均不代表实机 UI 验收。

## 宿主自动更新

设置插件新增发行实例启动检查与显式下载安装入口。macOS 替换整个 `.app`，Windows/Linux 替换便携目录；目标 OS/架构必须同时匹配当前二进制身份、发行元数据和资产名。更新助手在用户缓存运行，等待旧进程退出后进行同卷目录替换，保留旧目录，并以新版启动回执决定是否回退；Windows 通过进程句柄等待并处理文件占用重试，Linux 区分运行依赖但不依赖 X11/Wayland 执行文件更新。应用目录不可写时不自动提权。

无界面目录替换与模拟回执不等同于真实应用重启验收。macOS 的 Gatekeeper、Windows 的文件占用和安全软件、Linux 的系统库兼容，以及真实 UI 重启均待实机验证；备份暂不自动清理，强制中止助手或断电可能需要手动恢复。详见[自动更新](auto-update.md)及 [ADR 0060](adr/0060-host-auto-update.md)。

## 独立官方翻译插件

`sinan-official-plugins/plugins/translate` 新增 `tools.translate`，业务留在独立插件层；复用既有 `select` 设置、声明式输入匹配、UI Bridge 2 打开参数与命令，不改变宿主协议、权限或隔离边界。Google 免费网页接口与 MyMemory 请求由纯 Go 后端执行，页面不放宽 CSP。后台任务/轮询适配现有 500 毫秒页面调用预算。

官方仓库构建入口和 CI 已纳入 macOS、Windows、Linux 的 arm64/amd64 产物；Windows 使用 `.exe` 入口。离线测试使用 HTTP 夹具，不访问用户文本、剪贴板或真实窗口。跨目标编译、服务可达性、原生 WebView、输入法与 X11/Wayland 窗口交互分别验证；尚未完成实机 UI 验收或正式发布。服务限制和使用方式见[翻译插件说明](../../sinan-official-plugins/plugins/translate/README.md)。

翻译插件 0.1.1 在共享 Go 业务层修复单个英文词的自动识别：无重音拉丁字母单词优先按英语处理，手动语言选择优先，其他文本沿用原识别器。已通过离线 HTTP 夹具、Go 竞态回归、静态检查和六目标交叉构建；该修复不新增平台调用或宿主协议，真实 UI 验收仍未执行。

翻译插件 0.1.2 在隔离页面内新增译文复制按钮与 Command/Ctrl + C，无选区时复制完整译文，有选区时保留正常复制。复制在用户操作中使用文档复制命令，并恢复焦点与原选区；引擎拒绝或命令失败时明确提示，不新增原生剪贴板调用、权限或宿主桥。离线 DOM 测试覆盖复制与失败路径；三个目标引擎的实际系统剪贴板行为仍待实机验收。

翻译插件 0.1.3 将宽泛内容入口分数设为 0.2，宿主通过通用 `inputMatchScore` 清单字段承接，普通输入与粘贴内容入口均按此排序；名称命中与特定结构工具的默认优先级保持。变更位于共享 Go 搜索层及平台无关清单，未增加原生 UI 或系统依赖；插件需配合支持该字段的新宿主。兼容边界见 [ADR 0061](adr/0061-content-match-ranking.md)。

## core 加密存储与 LLM 翻译

core 新增加密私有文档和本地主密钥自动解锁，不使用系统凭据服务。共享 Go 标准库 AES-256-GCM、文档 CAS 与会话权限用于三端；macOS/Linux 使用文件模式，Windows 以独立适配设置当前用户专用 DACL。LLM 配置与 OpenAI 兼容 HTTP 请求留在官方翻译插件 0.2.0；HTTPS 远程或 loopback HTTP，不依赖 X11、Wayland 或 D-Bus。授权插件页面可读取其自己的解密配置；主密钥与密文可同时访问时仍可解密。

SDK 与权限枚举同步，旧宿主拒绝新权限；插件中心 0.9.1 显示并清理加密文档元数据。无界面夹具不读取真实凭据，不请求真实 LLM，也不启动 WebView。跨平台构建、目标运行及实机 UI 验收分别记录，见 [ADR 0063](adr/0063-encrypted-plugin-storage-and-llm-translation.md)。

## 全局共享 LLM（更新）

LLM 配置管理已移到设置插件 0.6.0，通用请求与凭据留在 core；翻译 0.3.0 仅生成提示词、选择全局配置并提交输入。全局加密记录使用保留宿主命名空间和已有主密钥，插件只能获取配置 ID/名称，不能从桥读取 Key。三端复用同一 Go 请求和会话取消机制，没有新的平台原生依赖。旧 0.2.0 私有配置不读取、不迁移；见 [ADR 0064](adr/0064-shared-llm-capability.md)。交叉编译、无界面测试与实机验收仍分别记录。

## 官方日历插件

独立官方仓库新增 `tools.calendar` 0.1.2，使用 React DayPicker 10.0.2、React 19.3.1 和 chinese-days 1.5.10 本地打包，只读展示月历、农历、节日、节气和中国大陆放假/补班数据。沿用声明式运行时、隔离 HTML UI、桥 2 ready 和包内图标，不新增宿主协议、权限、系统依赖或持久化，All-in-Plugin 边界不变；首发使用 0.1.2，避免覆盖官方仓库已有的 v0.1.0 / v0.1.1 标签。

公历范围 1901–2099，假期数据覆盖 2004–2027，未收录年份显式降级。目标语法沿用 Safari 16.4 / Chromium 119 / Firefox 115；CSS 静态输出并跟随宿主主题，无 CDN、动态 style 注入、平台路径或 X11/Wayland 调用。六平台/架构复用纯前端包，三系统 CI 追加无界面测试与构建。[三平台 CI](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/43791121005) 无界面测试与构建通过；[0.1.2 Release](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/v0.1.2) 的六目标包及摘要已正式上传。各目标 OS 的插件实机运行、WKWebView/WebView2/WebKitGTK、输入法、焦点、主题和 DPI 的真实 UI 验证仍未执行，发行状态不代表实机交互验收通过。

## 插件独立发布标签

宿主共享 Go 商店校验、插件脚手架与登记仓库 Python 校验同步接受 `<plugin-id>-v<version>`，兼容旧 `v<version>`。官方仓库提供跨平台 Node 标签生成入口，不依赖 shell 拼接或平台命令；包名、平台枚举、Asset ID/摘要和安装语义不变。三端无需新增原生依赖，但都需更新宿主后才能接受新标签记录；旧客户端可能退回整个目录缓存。历史已上架资产不迁移，目标运行及 UI 验收不由元数据测试替代。见 [ADR 0065](adr/0065-independent-plugin-release-tags.md)。

## 搜索空格键打开

设置插件在已有功能后新增「空格键打开」，默认关闭。宿主复用设置存储，在私有 `$search` 命名空间保存布尔值与 revision；保存成功后通知可信窗口立即生效。开启时，主搜索输入框的普通空格复用回车的选中项打开路径，包括粘贴结果筛选；关闭时保留空格输入。输入法组合输入（含 WebKit keyCode 229）、带修饰键的空格和长按重复空格不触发打开，插件页面内的输入不受影响。

macOS、Windows、Linux 共用 Go 配置服务与 React 键盘逻辑，没有新增原生依赖，不涉及 X11/Wayland 差异、公共插件协议、SDK 或登记仓库格式。无界面键盘测试覆盖开关、回车、输入法、修饰键与重复事件，配置测试覆盖默认值、持久化、冲突、损坏和写入失败。三个目标 WebView 的真实输入法与键盘交互仍未实机验证；无界面检查不代表实机验收。

本次本地验证通过 macOS arm64 全量无界面竞态回归、前端类型检查与生产构建、内核及桌面静态检查；Windows/Linux 的 amd64/arm64 内核测试程序交叉编译通过，未在目标系统执行。三平台 CI 已纳入新增键盘用例，Windows 另追加搜索配置用例；远端执行结果尚未验证。

## 独立官方计算插件

官方仓库新增 `tools.calculator` 0.1.0：由进程 Search Provider 识别主输入框的完整表达式并返回结果，direct 命令重新计算后复制；UI Bridge 2 的隔离稿纸支持多行即时计算、结果复制、历史切换、新建/删除和私有 storage 自动保存。页面本地求值，搜索与复制由 Go 后端处理，两端共享表达式校验用例。复用现有协议、SDK、用途声明及 32 KiB 文档 CAS，没有新增内核业务、权限或原生 UI 边界，不需要登记仓库格式联动；未登记尚未发布的资产。

三端的共享解析器使用有界语法与精确有理数处理十进制四则运算；超越函数和非整数乘方使用浮点近似。macOS 通过固定 `/usr/bin/pbcopy` 写文本，Windows 使用 Win32 Unicode 剪贴板与不可见消息窗口，Linux 按 Wayland / X11 分别调用 wl-copy / xclip，不依赖 shell。缺少桌面会话、工具或剪贴板繁忙时返回明确错误，不静默成功。原生后端为纯 Go，Windows 清单使用 `.exe` 入口。

UI 使用本地打包 React、BigInt、静态 CSS 和宿主明暗主题，目标语法沿用 Safari 16.4 / Chromium 119 / Firefox 115，不放宽 CSP、不访问网络。稿纸最多 50 张、每张 100 行且总量不超过现有 32 KiB；容量不足、损坏或多窗口修订冲突不覆盖已有数据，显示失败并保留当前输入。操作系统剪贴板和窗口生命周期仍遵循既有平台限制。

本次本地通过官方仓库前端 100 项、Python 2 项和原生插件 Go 竞态回归、计算后端静态检查、macOS arm64 临时宿主目录安装/搜索、六目标交叉构建及安装包原生头校验。复制路径以模拟函数验证，不读取或修改真实剪贴板；未运行桌面、浏览器或 WebView。Windows/Linux 的目标执行、三端真实复制、输入法、焦点、拆出、主题与 DPI 仍待实机验收；远端 CI 和正式发行未执行。详情见[计算插件说明](../../sinan-official-plugins/plugins/calculator/README.md)。


## 设置页面分类

设置插件按「通用、搜索、快捷键、AI 服务、插件设置、关于与更新」组织现有设置，采用侧栏导航与独立滚动内容区；窄窗口使用横向导航。分类切换保留组件与未保存的表单输入，样式沿用共享明暗主题变量，无新依赖、原生调用、协议或权限变更，其他两个插件仓库无需联动。

三端复用同一 React/CSS 实现。本次仅执行本地无界面回归、前端类型检查和生产构建；未执行真实桌面、WKWebView/WebView2/WebKitGTK、键盘焦点或不同 DPI 的 UI 验收，不据此宣称三端实机验证通过。


## 官方插件图标统一

内置插件与独立官方插件统一使用 40×40 浅色圆角底板与蓝色线条 SVG，规范分别记入各仓库 AGENTS.md；商店内嵌图标与对应源码同步，仅修改资料图标，不改写历史发行资产。图标为静态基础 SVG，不新增平台依赖、协议、权限或主题运行逻辑。

本次 8 个图标通过宿主加载校验，商店资料静态校验、源码一致性及历史发行元数据不变检查通过；官方插件本机 macOS arm64 构建通过。Windows/Linux 构建、三个目标 WebView 的真实显示与 DPI 验收未执行；已安装旧包的图标需等待后续正常版本更新。


### 官方图标更新与计算上架

JSON 工具 0.4.4、剪贴板 0.1.2、翻译 0.3.1、日历 0.1.3 和计算 0.1.0 已发布并登记到商店，每个版本均有三系统 arm64/amd64 六目标包及摘要。独立官方插件的[三平台 CI](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/43799988731) 无界面测试与构建通过；30 个包的清单版本、图标、原生架构和 GitHub 资产摘要已核验，实机 UI 仍未验收。

本体提交 a9aebba 的[六目标 CI](https://github.com/zhyipeng/sinan/actions/runs/43799819453)中，macOS/Linux 两种架构通过；Windows amd64/arm64 在 `TestPlatformWindowsApplicationIcons` 的 WScript.Shell 创建中文快捷方式夹具时失败；后续修复与验证见下文。该失败与独立插件的 CI、发行资产分别记录，不将本体描述为全平台回归通过。


### Windows 图标测试夹具修复

`TestPlatformWindowsApplicationIcons` 的快捷方式夹具改用显式 `IShellLinkW` 与 `IPersistFile.Save`，以 UTF-16 设置目标、图标与保存路径，替代在 Windows 两种架构 CI 中保存中文路径失败的 WScript.Shell 自动化。保留中文 ICO、中文 LNK，并检查快捷方式确实保存到目标中文路径；COM 对象在固定线程初始化并按逆序释放。

自定义图标断言核验输出尺寸、透明角及红色图形的内部像素，拒绝空图标或默认图标；不要求 Shell 渲染的快捷方式图标与直接读取 ICO 的 PNG 字节完全相同，允许覆盖标记或栅格化差异。修复仅涉及无界面测试，不跳过原生用例、不修改生产图标提取或其他平台实现，不启动应用、WebView 或系统授权窗口。

本地 macOS 应用目录竞态回归、Windows amd64/arm64 测试程序交叉编译及对应静态检查通过。提交 `66631a5` 的[六目标 CI](https://github.com/zhyipeng/sinan/actions/runs/43802650469) 全部通过：macOS、Windows、Linux 的 amd64/arm64 原生构建、现有平台无界面回归及归档成功；Windows 两架构实际执行并通过了修复后的图标用例。Windows 仍采用 CI 规定的回归子集，不等同于全仓测试或实机 UI 验收。

## 宿主共享剪贴板 API

系统文本/PNG 剪贴板适配集中到宿主 `internal/clipboard`，独立官方计算和剪贴板 0.2.0（未发布）改用带能力检查与清单权限校验的 stdio Host API。历史、去重、轮询和计算仍属于插件。macOS 需要宿主 Cocoa/CGO，保留受保护类型过滤；Windows 使用 Win32 Unicode/PNG/DIB 与不可见所有者窗口，锁定内存通过类型化 Win32 内存复制 API 访问；Linux 明确区分 X11 xclip 与 Wayland wl-clipboard，无会话或工具时返回错误。非支持构建明确不可用。自动粘贴的辅助功能、焦点与 Wayland 限制不变。

页面 SDK 增加 UI Bridge 2 文本读写，按视图权限、会话撤销和现有预算控制；图片走后端 RPC。三端复用协议、SDK 和 Go 权限检查，不开放完整 Wails Bridge。详情见 [ADR 0066](adr/0066-host-clipboard-api.md)。官方插件后端已去除 Cocoa 依赖，六目标可交叉构建；这不代表不带 CGO 的 macOS 宿主具备剪贴板能力。

本次本地验证：macOS arm64 宿主全量无界面竞态回归（含前端类型检查与生产构建）、全仓 Go 静态检查通过；新增 UI 请求形状、预算及撤销测试、真实 stdio 模拟大图片双向传输、权限拒绝、进程结束取消与错误后继续调用通过。两个官方插件六目标构建、官方仓库前端 100 项/Python 2 项/三个原生插件竞态回归通过。Windows/Linux 的 amd64/arm64 剪贴板、内核与 transport 静态检查及测试程序交叉编译通过，未在目标系统执行；Windows CI 已补无窗口剪贴板和权限检查，但远端结果尚未验证。测试不读取或修改真实剪贴板，没有运行桌面、原生 WebView 或浏览器。三个平台真实系统剪贴板、UI、焦点与自动输入未验收；未发布新版插件，登记仓库无资产变更。

## OCR 引擎预研验证

2026-11-16：在 macOS arm64 上，以独立 Python 环境和 11 张合成图片，对 PP-OCRv5 mobile、PP-OCRv6 small/tiny 进行 CPU 离线识别、耗时、内存与依赖体积验证；正式基准在系统级禁网环境执行，不读取真实剪贴板或屏幕，不启动桌面 UI。v6 small 在本批样本中质量较好，但双栏顺序、内存和原生包体积仍待处理。脚本、模型摘要及数据见 [OCR 验证报告](research/ocr-validation.md)。

这只是本机引擎实验：没有新增插件、宿主能力、协议或发布资产，没有构建原生 OCR 插件，也未执行 Windows/Linux、其他架构、截图选区、Wayland Portal、原生 WebView 或系统授权验证。不能由模型跨平台或 Python 推理成功推断六目标插件已可用。

发布预检补充：插件商店的宿主版本不再写死为 0.1.0，改用各平台构建注入版本做 SemVer 最低版本判定；开发构建使用 0.2.1 能力基线，插件中心 0.9.2 同步展示。修复不增加平台调用、协议或权限。初始 v0.2.0 发布已取消且没有公开资产，本体改发 v0.2.1；插件 0.2.0 最低宿主相应为 0.2.1。远端最终结果在发布后记录。

### 共享剪贴板正式发行结果

[Sinan v0.2.1](https://github.com/zhyipeng/sinan/releases/tag/v0.2.1) 已公开，[六目标原生流水线](https://github.com/zhyipeng/sinan/actions/runs/43806863705) 的 macOS、Windows、Linux arm64/amd64 构建与各自约定的无界面检查全部通过，六个归档的远端 SHA-256 与 SHA256SUMS 一致。初始 v0.2.0 未公开，标签保留。

[剪贴板 0.2.0](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.clipboard-v0.2.0) 与[计算 0.2.0](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.calculator-v0.2.0) 均已公开，各有六目标 ZIP 和摘要；[三平台 CI](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/43806605442) 通过。包内清单、图标（允许 Windows CRLF）、原生架构、大小与远端 SHA-256 已核验，商店使用真实 Release/Asset ID 登记，最低宿主为 0.2.1。

以上发行状态不代替三平台真实系统剪贴板、WebView、焦点、自动输入和安装升级验收；本次未运行这些实机验证。

### OCR 原生程序第二轮实验

2026-11-16：macOS arm64 上从固定 RapidOCRLite v5.0.0a6 源码构建 CPU 库，经独立 Go/CGO 程序完成 PNG 解码及完整 OCR 链路；复用第一轮 small 权重与 11 张合成图。严格浮点构建通过每图 5 次及密集图 100 次文字稳定性检查，原生运行目录 33.28 MiB、ZIP 26.84 MiB，迁移后在禁网环境运行；峰值约 347–349 MiB，销毁引擎未明显回落，进程退出及强制取消已由实验脚本回收。

上游默认 fast-math 在本机出现漏行和输出变化，严格浮点追加选项后本轮不再复现，不能据此断言底层根因或其他编译器已修复。其余五个 OS/架构目标未构建或运行；Windows 仍需模型路径编码、链接和 RSS 适配，Linux 需运行库与 ISA 核验。实验不是安装包，不含插件协议/UI，也未解决宿主对任意后代进程的回收边界；没有新增宿主能力、正式依赖决定或发行资产。详见[原生验证报告](research/ocr-native-validation.md)。

### OCR 插件任务与 worker 接入

官方仓库新增未发布 `tools.ocr` 0.1.0，本地打通剪贴板 PNG、异步命令、隔离页面及文本复制。共享 Go 后台与私有父子管道面向三端，Windows 隐藏子进程控制台；5 秒轮询租约、30 秒任务预算与 worker 的 35 秒硬退出控制生命周期。真实后台被强杀后，macOS arm64 worker 能因 stdin EOF 自行退出；不扩展宿主任意后代进程回收能力，见 [ADR 0067](adr/0067-ocr-private-worker-lifecycle.md)。

本机通过官方仓库 104 项前端/3 项打包测试及四个插件后台竞态回归；OCR 六目标后台交叉编译和静态检查通过，但未在 Windows/Linux 运行。三平台 CI 已纳入相关无界面测试与后台编译，远端未执行。完整原生包仅构建 macOS arm64，约 35.59 MiB / ZIP 27.80 MiB；真实 worker 的 11 图、取消、后台强杀退出在模拟 Host API、系统级禁网下通过，含空格/中文路径亦通过；临时宿主目录检查、安装和搜索通过。macOS 后台、worker 及动态库均显式对齐宿主 12.0 构建目标，已核对 Mach-O，旧系统执行未验证。没有读取真实系统剪贴板或运行桌面 UI。

完整打包入口当前仅允许 macOS 本机目标；macOS amd64 未构建/运行，Windows 因 Unicode 模型路径和 DLL 分发、Linux 因运行库与原生依赖验证未完成而明确不可用。没有发布或上架，不把后台源码/交叉编译描述为六目标 OCR 支持。截图捕获、主搜索会话图片输入和三端 WebView、主题、焦点、拆出及系统剪贴板实机验收仍未完成。


### OCR 会话图片与快捷键扩展

本轮共享宿主/SDK 增加 `ui.image-command.v1` 与页面剪贴板图片快照，OCR 追加 image matcher，主搜索带图打开后自动识别并展示原图；页面通过 Cmd/Ctrl+V 或 image paste event 读取宿主快照，Cmd/Ctrl+C 在无选区时复制当前结果。普通文本粘贴与有选区复制保留原行为。PNG 快照限制统一为 4 MiB，既有会话资源预算不变，三端继续使用既有系统剪贴板适配；Linux X11 和 Wayland 的外部工具/会话依赖仍需分别满足。

本轮仅执行本地无界面检查与 macOS arm64 原生识别验证，不启动桌面、不读真实系统剪贴板。完整 OCR 包仍只验证 macOS arm64；Windows/Linux 完整原生包及三端 WKWebView/WebView2/WebKitGTK 的 paste event、快捷键、焦点和权限尚未实机验收。此接口未发布，官方插件须配套本轮宿主，不能仅凭 v0.2.1 判断兼容。决策见 [ADR 0068](adr/0068-session-image-commands.md)，本轮结果与旧轮次区分记录。

本轮验证结果：宿主 `node scripts/desktop.mjs test`（前端构建、无界面 JS、全量 Go 竞态检查，原生 UI 禁用）及本机桌面构建通过；官方仓库 108 项 JS、3 项打包与四个后台竞态检查通过。OCR 六目标后台 build/vet 通过，仅代表编译/静态检查。macOS arm64 真实 worker 在系统级禁网、模拟宿主下通过 11 图及独立图片快照、不重读剪贴板、取消和后台退出回收验证；更新包约 27.80 MiB，宿主 inspect 通过，SDK 来源补丁可重放。具体摘要与边界见[本轮结果](research/ocr-image-input-validation-results.json)，此前 OCR 报告属于历史轮次，不作为新包摘要。


发行前传输复核：补齐 helper 对大于 512 KiB 会话原图的交付（独立上限 4 MiB，普通 bundle 仍为 512 KiB），并补齐 WebView2 新剪贴板图片/图片命令的 URL 双向转换及重复字段拒绝。新增 4 MiB 真实私有管道和无原生窗口的 WebView2 传输单元测试。v0.3.0 发布已取消且无公开 Release，配套发行改为 v0.3.1，OCR 最低宿主同为 0.3.1。

### Windows 呼出快捷键 Alt+Space

Windows 把 `Alt+Space` 当作窗口系统菜单组合，`RegisterHotKey` 注册后前台窗口仍会收到 `WM_SYSCOMMAND`/`SC_KEYMENU` 并弹出标题栏右键菜单，导致设置无法录入该热键。宿主现通过 Wails `WndProcInterceptor` 在自有窗口内吞掉该系统菜单消息：录入时原生捕获 `Alt+Space` 并投递 `shortcut:recorded`，已配置时交给全局热键的 `WM_HOTKEY` 切换；未配置且未录入时保持系统默认菜单。为避免 UI 消息线程等待控制器锁，录入与配置状态分别用原子布尔值暴露，并在快捷键变更回调中刷新。该实现仅 Windows 构建安装拦截器，不修改全局热键注册、插件协议、SDK 或设置存储格式。新增消息判定与规范化单元测试，Windows amd64 本机 `go vet` 及相关用例通过；Windows 真实桌面、WebView2 键盘与系统菜单、macOS/Linux 真实输入法与焦点仍未实机验收。详见 [ADR 0070](adr/0070-windows-alt-space-shortcut.md)。


### OCR 与宿主正式发行核验（2026-11-17）

[Sinan v0.3.1](https://github.com/zhyipeng/sinan/releases/tag/v0.3.1) 已公开，[六目标原生发布流水线](https://github.com/zhyipeng/sinan/actions/runs/43831907925) 全部成功，六个公开归档的 GitHub SHA-256 与 SHA256SUMS 已逐项核对。首发对应源码 d7fc059；随后源码的自适应管道截止时间、迁移及开发启动修复不据此声称已进入同一发行资产。v0.3.0 未公开，旧标签保留。

[OCR 0.1.0](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.ocr-v0.1.0) 已公开并登记商店，最低宿主为 0.3.1；仅 macOS arm64 完整包，约 27.80 MiB。ZIP 已回下载核对摘要、大小、清单、模型摘要、原生架构、执行位、图标和许可；[插件三平台 CI](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/43830703763) 通过，但不代表 Windows/Linux 完整 OCR 支持。商店使用真实 Release/Asset ID，其余平台不登记。核验数据见[正式发行结果](research/ocr-release-validation-results.json)。以上不代替真实桌面、系统剪贴板、授权与 WebView 实机验收。

### OCR 0.2.0 六目标原生发行（2026-11-17）

[OCR 0.2.0](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.ocr-v0.2.0) 已公开并登记 macOS、Windows、Linux 的 arm64/amd64 六个完整原生包，最低宿主仍为 0.3.1，保留 0.1.0 的既有资产和记录。[原生矩阵](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44006353531) 在六个目标 runner 上通过真实 ZIP 中文/空格路径重定位、11 图各两次、取消和后台退出回收；不是后台交叉编译。独立发布 job 仅解析并复核六份 ZIP、模型、架构、执行位、许可证和报告，公开包摘要与 SHA256SUMS 相符。

Windows 使用固定 LLVM-MinGW UCRT 工具链及包内对应 C++/线程运行库，以 PE GUI 子系统避免后台 conhost；Linux 基线为 Ubuntu 24.04/glibc 2.39 + 系统 libstdc++，X11/xclip、Wayland/wl-clipboard 的宿主依赖不变。模型/字典 UTF-8 路径只在构建私有副本修补，原始上游不变，宿主/SDK/公开协议无改动。跨 ISA 的终端图均可能漏一个点但位置不同，报告保留差异；源文本编辑距离和字符差不劣于既有基准、每图两次稳定，不将合成检查描述为完美准确率。三端真实 WebView、系统剪贴板、输入法、权限、多屏及旧版 macOS 实机仍未验收；Windows/Linux 未做 OS 级禁网。决策见 [ADR 0071](adr/0071-ocr-native-platform-packages.md)，证据见[六目标核验数据](research/ocr-cross-platform-validation-results.json)。

### 剪贴板历史排序与图片预览

官方剪贴板插件新增复制成功后前移并持久保存排序；左侧按可见项加载缩略图，右侧在可用详情区域等比放大，预览最高 1280×960 并自适应满足既有 192 KiB UI 响应预算。复制保留原始图片，写入失败不前移；索引保存失败明确提示且保留原排序。实现位于独立插件，共用 Go、Canvas/createImageBitmap 与静态 CSS，沿用既有三端引擎目标；不改变宿主协议、SDK、权限、原生系统适配或 X11/Wayland 依赖，无登记仓库格式或发行资产变更。

本机通过剪贴板 Go 竞态回归、静态检查、3 项前端无界面测试和 macOS/Windows/Linux arm64、amd64 六目标交叉构建。未执行 Windows/Linux 目标运行及三端 WKWebView/WebView2/WebKitGTK、DPI、真实系统剪贴板与交互验收；当时插件改动尚未发布，交叉构建不代表实机通过。

### 剪贴板 0.2.1 正式发行

[剪贴板 0.2.1](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.clipboard-v0.2.1) 已公开；[发布流水线](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44004573375) 在 macOS runner 通过全量无界面测试、六目标交叉构建与打包。三系统 arm64/amd64 六个 ZIP 已回下载核对清单、原生架构、入口执行位、图标、大小及 SHA-256，摘要与随包文件及 GitHub 资产摘要一致；商店以真实 Release/Asset ID 登记。最低宿主仍为 0.2.1，API 1 / UI Bridge 2 不变。未执行 Windows/Linux 目标运行、三平台真实剪贴板、WebView、焦点、自动输入和 DPI 验收，不将发行与交叉构建视为实机通过。

### 插件商店后台下载安装任务

插件中心的下载、目录复核和包应用现由宿主持有任务，不依赖可信组件挂载或窗口显示；UI 仍在内置插件中，支持读取状态、取消下载和显式重试。最多四个活动任务，同一插件去重；退出取消网络工作并等待安装和暂存清理。任务只在当前宿主进程中保留，不支持退出后恢复或断点续传。共享 Go 实现不新增三端系统依赖、权限或 WebView API；公开协议、SDK 与商店格式不变，独立官方插件和登记仓库无需改动。见 [ADR 0072](adr/0072-background-plugin-installation.md)。

本机 macOS arm64 通过 `node scripts/desktop.mjs test`（前端构建、18 项 JS 无界面用例及全量 Go 竞态回归），标准入口 `node scripts/desktop.mjs build` 的本机桌面及配套程序构建通过。新增任务用例覆盖无页面参与的实际安装、去重、并发上限、校验失败、取消重试及退出收尾。catalog 测试程序的 macOS/Windows/Linux arm64、amd64 六目标交叉编译通过；Windows CI 已追加任务回归，尚未远端执行。未执行 Windows/Linux 目标运行、三端真实窗口切换、隐藏/拆出及 WebView 交互验收，不把交叉编译视为实机通过；改动尚未发行。

同日后续：任务列表移至「已安装」「发现」之后的独立「下载安装」Tab，并用标准 HTML progress 显示实际下载字节、总大小与百分比。总量来自目录资产，缺失 Content-Length 不影响计算；镜像重试归零，校验和安装分别显示阶段，不模拟安装百分比。沿用三端共享 Go 下载器及 WebView 标准控件，无公开协议、SDK、权限或登记格式变化。本机重新通过完整无界面测试及标准构建；分块下载、缺失响应长度、镜像回退与任务进度快照测试通过竞态检查，distribution / catalog 两个测试程序的六目标交叉编译通过。Windows CI 增补任务与进度用例，远端尚未运行；三端真实 WebView 进度条外观、窗口和 Tab 交互未验收。


### 静默启动与开机自启

设置「通用」在外观之后追加两个独立开关，默认关闭。静默启动下次启动生效，以隐藏状态创建主输入框，仍可从托盘和快捷键呼出；开机自启按当前用户登录触发，不强制静默。当前注册已改用 Wails `app.Autostart`：macOS 13+ 有效 app bundle 使用 SMAppService，其余支持场景由库采用 LaunchAgent，Windows 使用 HKCU Run，Linux 使用 XDG Autostart（需要对应桌面会话支持，X11/Wayland 分别待实机核验）。开发自定义插件目录及探测/验收实例禁止修改系统自启；用户移动程序后需重新开启以更新路径。初始方案见 [ADR 0073](adr/0073-startup-preferences.md)，当前 Wails 替代实现见 [ADR 0082](adr/0082-wails-autostart.md)。

普通启动时，搜索框以隐藏状态创建，在窗口运行时就绪后复用快捷键和托盘的定位规则。macOS、Windows 和 Linux X11 优先选择鼠标所在屏幕，在可用区域内水平居中。「设置 → 通用 → 搜索框顶边距」可保存 0%–50% 的整数比例，默认 20%，下次显示时按所选屏幕的可用高度计算。Wayland 沿用 Wails 屏幕回退，实际位置由合成器决定。实际首屏位置、多屏与 Wayland 行为待实机验证。

顶边距设置通过 macOS arm64 标准无界面回归、构建及静态检查，Linux arm64 GTK3 容器定向回归、构建及静态检查，以及 Windows amd64/arm64 桌面和内核测试程序的交叉编译与静态检查。Windows 测试程序尚未在目标系统执行，三端桌面窗口位置尚未实机验证。

搜索框展开时，已取得可用区域的主窗口根据内容高度向上调整，收起后回到首选位置。若可用区域矮于页面，宿主限制窗口高度，可信插件页按实际视口滚动。拆出可信页面时同样限制纵向位置和高度。Wayland 的实际摆放仍由合成器处理。

首次自定义注册实现的历史验证（该自启包现已删除）：macOS arm64 通过标准入口 `node scripts/desktop.mjs test`（前端类型检查、生产构建、18 项 JS 及全量 Go 竞态回归），新增偏好/注册文件/宿主假注册器竞态测试及相关 Go 静态检查通过；`node scripts/desktop.mjs build` 本机桌面及配套程序构建通过。自启包三系统 arm64/amd64 六目标测试程序交叉编译和静态检查通过，Windows 两架构桌面测试程序交叉编译通过；不代表目标系统测试已运行。三平台 CI 沿用原生构建矩阵，Windows 追加启动偏好与自启无界面检查，尚未远端执行。未修改当前用户真实自启项，未执行实际登录、自启进程启动、真实桌面隐藏/呼出及三端 WebView 验收；未发布或调整插件发行资产。


### 主窗口整体透明度

设置「通用」在既有内容后追加透明度滑块，范围 0%–70%，默认 0%，拖动即时应用并持久保存。透明度同时作用于主窗口文字与原生承载的内容；拆出窗口恢复不透明，新主窗口按保存值恢复。macOS 使用 NSWindow alphaValue，Windows 使用 layered HWND 的整体 alpha，Linux GTK3 使用 widget opacity；Linux 检查 screen 合成能力，无合成时明确显示不可用，不以 CSS 背景模拟系统窗口透明。X11/Wayland 的 compositor、WebKit 子视图与 DPI 仍需分别实机核验。详见 [ADR 0074](adr/0074-main-window-opacity.md)。

本轮 macOS arm64 标准 `node scripts/desktop.mjs test`（前端类型检查、生产构建、18 项 JS 及全量 Go 竞态回归）和标准桌面/配套程序构建通过；最终原生改动的定向竞态测试及桌面构建通过，相关 Go 静态检查通过。Linux arm64 无显示服务器、禁网容器中，GTK3 平台/内核/桌面透明度定向竞态回归、静态检查及原生桌面构建通过；平台用例只传 nil 句柄，不创建窗口。Windows amd64/arm64 平台、内核、桌面测试程序交叉编译和静态检查通过，未在 Windows 运行；macOS amd64、Linux amd64 本轮未构建。Windows CI 已追加相关无界面用例，远端未运行。三端真实窗口透明合成、滑块拖动、嵌入/拆出、焦点、DPI 与 Linux X11/Wayland 显示效果未实机验收，不将无界面测试或交叉编译描述为真实 UI 验证；没有发布或修改另外两个插件仓库。


### 截图插件首版（未发布）

独立官方 `tools.screenshot` 0.1.0 新增区域捕获与隔离 Canvas 标注。macOS 使用系统 screencapture 并依赖屏幕录制授权；Windows 使用系统 Windows PowerShell/WinForms/GDI，处理虚拟桌面原点与线程 DPI 感知；Linux X11 需要 scrot，Wayland 仅支持可运行 grim/slurp 的 screencopy/layer-shell 合成器，GNOME/KDE Portal 未接入。缺少桌面、工具、授权或能力时明确失败，不宣称通用 Wayland 支持。

保存使用各平台图片目录的 Sinan 子目录；复制复用宿主剪贴板及其 X11/Wayland 工具依赖。原图/导出限制 8 MiB、3200 万像素，Windows 虚拟桌面自身也受像素预算限制；初始分块沿用当前桥预算与 CSP；随后增加双入口、隐藏窗口和全局热键的配套宿主扩展，见下述 ADR 0076。实现、生命周期和本轮验证见 [ADR 0075](adr/0075-screenshot-plugin.md)。

本轮未运行真实系统截图、屏幕授权、系统剪贴板、三端 WKWebView/WebView2/WebKitGTK、多屏/DPI、焦点和隐藏/拆出验收，未发布或上架。六目标后端交叉编译/包检查不能替代 Windows/Linux 目标运行与三端桌面验收；三平台 CI 仅配置接入，尚未远端执行。

本轮 macOS arm64：官方仓库 119 项 JS、11 项 Python、五个后台竞态检查，以及截图六目标静态检查通过；标准全矩阵构建与通用打包通过，截图六个 ZIP 的入口架构、执行位、图标、UI 源构建与摘要一致。临时宿主目录 inspect 和本机真实 stdio 合成图片（约 1 MiB / 29 块）复制失败重试、标识重放拒绝及退出检查通过，系统剪贴板由模拟 Host API 替代。未运行 Windows/Linux 目标二进制，未执行真实截图或 UI 验收。

### 截图双入口与全局热键（未发布）

截图新增“截图”直接选区和“截图后处理”打开编辑器两个入口，默认 macOS Command+Shift+A、Windows/Linux Ctrl+Shift+A；沿用插件热键配置及平台注册。宿主在加载隐藏命令页面前隐藏主窗口，执行受视图权限限制的 window.execute 后恢复原活动会话窗口；热键不预先显示 Shell；同日后续改为仅有有效截图时恢复后处理，取消、无文件、空图片或无效 PNG 时清理临时页面并保持隐藏，已替换或撤销会话不影响新窗口。仅窗口任务调用采用 125 秒预算，WebView2 对应等待 127 秒，其他方法预算不变；SDK 快照及 manifest Schema 同步。实际依赖 ui.window-execute.v1，已发布 v0.3.1 不支持。详见 [ADR 0076](adr/0076-hidden-window-commands.md)。

共用 Wails 窗口和三端隔离承载，不改变系统选区适配；GNOME/KDE Portal、真实多屏/DPI、系统授权、热键与隐藏/恢复焦点仍未验收。登记仓库格式及已有发行资产不变，本轮未发布。验证结果与第一轮区分记录于 ADR 0076。

本轮续验：macOS arm64 标准无界面入口（19 项 JS、全仓 Go 竞态及前端构建）、最终定向窗口任务竞态检查和标准桌面构建通过；官方仓库 121 项 JS、11 项 Python、五个后台竞态检查与截图六目标构建/静态检查通过。六个截图 ZIP 和模拟 Host API 的真实 stdio 合成图上传/失败重试/退出通过，SDK 来源补丁可重放。Windows 两架构 manifest/uiview/kernel/desktop、Linux 两架构共享 manifest/uiview/kernel 测试程序交叉编译及静态检查通过，未在目标系统运行；Linux 桌面与 macOS amd64 宿主本轮未构建。Windows CI 补充窗口任务、隐藏命令与桥预算回归，但远端未执行。未运行真实截图、热键注册、原生 WebView、系统授权、焦点或多屏验收，未安装到现有用户配置或发布。

截图取消补充：使用现有 CommandResult.dismiss 标志实现无图片时结束截图，宿主共用 Go 路径清理当前会话，主窗口重置搜索但保持隐藏，拆出窗口关闭且不呼出主窗口；隐藏启动的命令通信失败也结束临时页面。没有增加原生平台调用、协议字段、权限或 SDK 方法。组件调研仅解析官方资料和 npm 包，不安装、执行或替换候选组件。

取消分支本轮：macOS arm64 标准无界面回归、本机标准构建及最终窗口任务竞态检查通过；官方 121 项 JS、11 项 Python、五个后台竞态检查（截图 10 项 Go 用例）、六目标截图构建/打包及模拟 Host API 的真实 stdio 合成图片检查通过。Windows 两架构桌面测试程序交叉编译和静态检查、截图六目标静态检查通过，未在 Windows/Linux 运行或执行真实系统截图/窗口验收。


### Painterro 后处理适配（未发布）

截图编辑器改用固定 Painterro 1.2.92，三端共用私有源码适配和静态 CSS；字体图标改为 DOM SVG 节点，棋盘格改为 CSS，文字用 Canvas，原图用 ImageBitmap，不新增 UI 资源类型或 CSP 许可。保留 Command/Ctrl 与输入法组合处理，归一 wheelDelta/deltaY，释放事件和观察器。后端、系统截图、路径、剪贴板与 Linux X11/Wayland 依赖未改变，GNOME/KDE Portal 仍未接入。

本机 macOS arm64：官方 127 项 JS、11 项 Python 和五个后台竞态检查通过；实际 Painterro 以无浏览器 DOM/模拟 Canvas 验证了工具、裁剪/像素化、历史、文字与导出，不能代替 WebView/CSP 或真实图像渲染验收。六目标截图构建/打包、严格宿主 UI 资源加载与临时目录 inspect、ZIP/许可/摘要检查通过。界面 4 个资源合计 148,227 字节，未放宽宿主预算；未在 Windows/Linux 运行后端或执行三端真实 WebView、截图、系统剪贴板、输入法、焦点、多屏及性能验收。没有发布或上架。详见 [ADR 0077](adr/0077-painterro-screenshot-editor.md)。

截图工具栏样式补充：统一颜色按钮与控制栏尺寸，去掉会叠压标签的上游棋盘格负边距装饰；数值输入使用 12px 字号、固定宽度并隐藏原生步进按钮，布尔项统一为 20px 复选框，保留键盘数值编辑和选中状态。三端复用静态 CSS，未修改协议、SDK、权限或平台适配。14 项截图无界面回归、三端目标 CSS 解析、六目标构建及通用打包通过；界面仍为 4 个资源，当前共 149,935 字节。未启动真实桌面或 WebView，深浅主题、原生输入控件、DPI 和窄窗口视觉效果未实机验收。

### 截图复制、原生保存与延迟呈现（ADR 0078）

截图插件“复制并关闭”及 Command/Ctrl+C 共用成功复制后隐藏窗口的流程；保存改为共享 Go 适配调用 Wails SaveFile，对话框绑定原窗口并复用防失焦机制，取消不写文件。原生后台不再探测各平台图片目录或自动写文件，保存路径仅宿主持有。新增 ui.image-output.v1 与视图权限 window.show/window.hide/file.savePNG；PNG 分块和完整校验在弹窗前完成。有效截图返回 keepHidden，页面载入后再显示；取消/无图清理，隐藏页面未启动的超时处理不再打开空编辑器。

macOS arm64 标准无界面回归及桌面构建通过，新增输出/权限/撤销用例另经 desktop/kernel/uiview 定向竞态回归与 WebView2 页面桥测试通过。官方仓库 129 项 JS、11 项 Python、五个后台竞态检查通过；清除旧固定目录实现后截图后台另经竞态回归。截图六目标构建、通用打包、真实 stdio 的合成 PNG 分块导出/复制失败重试/退出检查通过，SDK 来源补丁重放与摘要复核通过。Windows arm64/amd64 desktop/manifest/uiview/kernel 测试程序交叉编译和 vet 通过；Linux arm64/amd64 共享 manifest/uiview/kernel 交叉编译和 vet 通过，未在目标系统执行，Linux 桌面本轮未原生构建。最终 UI 为 4 个资源、150,246 字节。

三端系统截图、复制后隐藏、原生保存对话框、覆盖确认、焦点和多屏/DPI 未实机验证；未启动桌面、浏览器、WebView 或系统对话框，未访问真实屏幕/系统剪贴板，未安装、发布或新增登记仓库资产。Wails 原生对话框无通用强制取消句柄，超时或撤销后用户仍需关闭已有对话框，返回后不写文件。

### 后处理图片导入与粘贴（ADR 0079）

新增宿主通用 file.openImage 与 ui.image-input.v1，通过 Wails AppKit/Windows/GTK 单文件选择对话框导入 PNG/JPEG/GIF 首帧，完整解码转为当前会话私有 PNG；输入 8 MiB、1600 万像素，输出最多 4 MiB。页面使用 Image.decode/createImageBitmap，不放宽 CSP 或资源预算。粘贴复用 clipboard.readImage，只在用户 Command/Ctrl+V 或原生 paste 时读取；文字输入和输入法组合保留原行为。权限分别声明，单视图权限上限由 4 调整为 6；file.openImage 的对话框预算与 file.savePNG 一致。

macOS arm64 标准宿主无界面回归及桌面构建通过；PNG/JPEG/GIF 规范化、损坏/截断、像素/字节限制、输出编码上限与权限/撤销/请求字段均有无界面覆盖。官方仓库无界面测试、截图六目标构建/通用打包、合成 PNG 的真实 stdio 分块检查及 SDK 来源补丁/摘要检查通过；最终页面用例共 13 项，覆盖直接导入、双平台快捷键、原生 paste、无图/取消/解码失败保留当前图片及迟到结果。Windows arm64/amd64 desktop/manifest/uiview/kernel 测试程序交叉编译和 vet 通过；Linux arm64/amd64 共享 manifest/uiview/kernel 交叉编译和 vet 通过，未执行目标测试程序。Windows CI 补充这些无界面用例，远端未执行。

最终截图 UI 为 4 个资源、151,526 字节。Linux 桌面本轮未原生构建；三端原生文件对话框、选图格式/路径、WebView 图片解码、实际剪贴板、输入法与焦点未实机验证。未启动桌面或浏览器、未读取系统剪贴板、未安装或发布；登记仓库无变化。仍须配套本轮新宿主，旧宿主不支持新权限和能力。

### 设计中：截图到 OCR 的跨插件交接

[页面交接与依赖机制](design/plugin-handoff-and-dependencies.md)目前仅为待确认设计。拟在 WKWebView、WebView2、WebKitGTK 同步补齐依赖校验、图片暂存预算、隔离视图准备/激活和交接撤销；当前没有新增可执行能力或平台验证结果。本轮只检查文档差异和链接，未运行构建、真实 UI 或系统剪贴板测试。


### 主界面输入与插件 UI 独立保留

设置「通用」新增清除输入框延迟、插件 UI 保留时长，默认均为 0 秒，整数秒范围 0–86400。主窗口隐藏开始计时，输入到期清除；期限内唤起保留输入或原插件 UI，到期返回主输入框。两项互不影响，拆出的窗口不参与，显式导航及截图/粘贴任务生命周期继续优先。共享 Go 计时、会话序号和 React 导航，三端同窗口原生插件承载复用既有实现；未新增平台 API、公开协议或权限。历史独立进程客体不支持非零插件 UI 保留，保存时明确拒绝。见 [ADR 0081](adr/0081-main-window-retention.md)。

本轮 macOS arm64 标准 `node scripts/desktop.mjs test` 通过前端类型检查、生产构建、20 项 JS 用例与全量 Go 竞态回归；标准 `node scripts/desktop.mjs build` 的桌面和配套程序构建通过，相关 Go 静态检查通过。新增配置及宿主无界面用例覆盖独立截止、过期边界、旧回调取消、隐藏期间修改时长、输入不复活、重启、损坏记录、过期修订和保存失败；计时器用例仅使用未初始化的 Wails 值，没有创建原生窗口或事件循环。Linux arm64 无显示服务器、禁网容器中的 GTK3 内核/桌面定向竞态检查、静态检查及原生桌面构建通过；Windows amd64/arm64 内核和桌面测试程序交叉编译及静态检查通过，没有在 Windows 执行。Windows CI 已增补配置用例，远端未运行。macOS amd64、Linux amd64 未构建；三端热键、失焦、输入法、同窗口插件焦点恢复、真实隐藏和拆出及 Linux X11/Wayland 交互未实机验收。

同期工作区正在实施另一项跨插件交接；上述验证反映运行时的工作区快照，不将并行功能视为本项配置的协议或发行成果。保留并行改动，只修正阻断回归的导入与前端空数组兼容问题；没有发布或修改插件登记资产。

### 插件依赖与截图到 OCR 交接（ADR 0080）

上一节设计已获用户确认，并实现 ui.plugin-handoff.v1。根 dependencies、目标公开 entryPoints 和视图依赖授权由共享协议、Go 校验器和 Schema 同步约束；截图仅将 OCR 作为可选依赖。宿主暂存编辑后的 PNG，校验源会话、目标版本/入口/包指纹，准备目标隔离视图并在同窗口提交后激活 OCR，失败保留源页面。图片数据帧在三端共用 nativeBridge 中独立计数，保留控制调用与资源/CSP 限制；硬依赖失效时拒绝激活并撤销相关视图/进程，插件中心提供依赖与被依赖状态。

macOS arm64 标准宿主无界面回归通过，最终定向竞态检查覆盖声明边界、硬依赖环、可选依赖失效、旧版本/未公开入口、上传重放/跨窗口句柄/配额/超时、目标准备失败与激活/撤销竞态；标准桌面构建通过。官方仓库 139 项 JS、11 项 Python 和五个后台竞态检查通过，真实 SDK 的 1 MiB 分块、清理和超限拒绝有专门用例。截图六目标构建、OCR 后台六目标编译/vet、通用打包和截图合成 PNG 的真实 stdio 检查通过。源码 SDK 来源补丁可重放，摘要同步。

本机 OCR 0.3.0 使用固定上游和已校验模型重新构建，通过原生 ABI probe，并用模拟 Host API 对真实后台/worker 运行 11 组合成图、每组两次的质量/重复稳定性、取消和进程回收检查；未访问真实剪贴板。生成 macOS arm64 OCR 0.3.0 完整包，其余五目标只有后台交叉编译，未生成或宣称 0.3.0 原生完整包。

Windows arm64/amd64 desktop/manifest/uiview/kernel 测试程序交叉编译和 vet 通过；Linux arm64/amd64 共享 manifest/uiview/kernel 交叉编译和 vet 通过，未在目标平台执行。Windows CI 追加依赖与交接的无界面用例，内核 native 测试夹具在 Windows 使用 .exe 入口；远端 CI 未执行，Linux 桌面未在本轮原生构建。截图 UI 仍为 4 个资源，共 153,401 字节。

三端真实 WebView、同窗口/拆出交接、前往插件中心、焦点和大图片传输未实机验收。没有安装、发布或登记新资产，已发布 OCR 0.2.0 资产保持不变。新宿主、截图包和 OCR 0.3.0 需配套使用；正式发布后再登记真实资产和最低宿主兼容性。


### 使用 Wails 自带开机自启（ADR 0082）

按用户指令删除 `internal/startup` 的自定义 LaunchAgent、Windows Run、XDG 注册及对应平台文件测试，改由桌面宿主注入锁定 Wails 3.0.0 的 `app.Autostart`。已有注册通过库实时状态识别并复用，新增注册使用固定 Identifier `dev.sinan.desktop`、无附加参数；设置动作完成后再次复核状态，系统尚待批准时明确返回未生效提示。静默开关、可信设置私有绑定和开发/探测/验收实例限制保持不变，没有公开协议、SDK、manifest 或两个插件仓库的联动。

macOS 13+ 有效 bundle 由 Wails 使用 SMAppService，其他支持场景采用 LaunchAgent；库的 LaunchAgent 分支会尝试 bootstrap/bootout，不能沿用旧代码「不立即加载」的保证。Windows/Linux 继续由库提供当前用户 Run/XDG 自启；Linux 桌面会话与 X11/Wayland 运行依赖、搬迁路径及系统策略仍需实机验证。详见 [ADR 0082](adr/0082-wails-autostart.md)。

本轮 macOS arm64 标准 `node scripts/desktop.mjs test`（前端类型检查、构建、JS 无界面回归与全量 Go 竞态检查）和 `node scripts/desktop.mjs build` 通过；新增 Wails 假服务定向竞态用例与桌面静态检查通过。Linux arm64 无显示服务器、禁网容器中，GTK3 桌面定向竞态测试、静态检查及原生桌面构建通过；Windows amd64/arm64 桌面测试程序交叉编译、静态检查通过，未在 Windows 执行。macOS amd64、Linux amd64 本轮未构建。CI 已移除被删除内部包的引用，远端未运行。没有调用真实 Wails 自启服务、创建 application、修改本机登录项或触发系统授权/原生 UI；三端登录、自启进程、旧注册迁移、系统批准与真实桌面行为尚未验收，未发布。

## 2026-11-17 配套发行

[Sinan 0.4.0](https://github.com/zhyipeng/sinan/releases/tag/v0.4.0)、[截图 0.1.0](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.screenshot-v0.1.0) 和 [OCR 0.3.0](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.ocr-v0.3.0) 已公开发行，三者均提供 macOS/Windows/Linux × arm64/amd64 六目标资产。截图及新 OCR 的最低宿主为 0.4.0，OCR 仍是截图的可选依赖。此前“未发布”描述记录开发阶段，以本节发行状态为准。

宿主[六目标原生构建与无界面回归](https://github.com/zhyipeng/sinan/actions/runs/44070459519)、截图[构建与发布](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44070601312)、OCR[六目标原生验证](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44071389046)及[正式发布](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44072351423)均成功。OCR 首轮 Windows amd64 的测试观测竞态已修复：worker 在 is_running/status 两次查询间退出时按已退出处理，保留存活超时与其他错误的失败判断，新增三项确定性单元用例；修复后的六目标验证全部通过。

宿主包含依赖与页面交接、窗口保留及 Wails 自启迁移，截图包含编辑器、导入/粘贴、原生保存与 OCR 跳转。以上为原生构建、无界面测试和发行完成，尚未执行真实桌面/WebView、截图授权、焦点、多屏、登录自启和实际安装升级验收；独立进程不构成 OS 沙箱。旧版本资产保持不可变。

### macOS 截图自定义选区（ADR 0084）

官方截图插件开发版 0.2.0 在 macOS 14+ 使用 ScreenCaptureKit 分别截取各显示器、AppKit 显示选区遮罩，按所选显示器的快照像素裁剪。可在遮罩直接完成箭头、矩形、文字、马赛克、撤销、复制、保存和 OCR 交接。箭头、矩形和文字共用预设色及十六进制颜色输入，可改色和撤销。Esc 或右键取消。快捷键呼出后可在任一显示器开始框选，可用选区工具拖动已有标注，拖出该显示器时选区停在屏幕边缘。旧版 macOS 延用系统选区。macOS 两架构辅助程序由装有 macOS 14+ SDK 的 Mac 构建，包内需包含匹配架构的 Mach-O；Windows WinForms/GDI 和 Linux X11、受支持 Wayland 合成器原有路径保持。授权归属、安装升级、全屏 Space、焦点、多屏、Retina、文字输入、剪贴板、保存面板和 OCR 跳转待实际桌面验收，尚未发行或登记商店。见 [ADR 0084](adr/0084-macos-screenshot-selection-overlay.md)。

macOS arm64 无界面回归、双屏选区坐标、合成 Retina PNG 标注、OCR 分流和宿主包预览通过；六目标构建与打包通过，核对 macOS 两架构辅助程序的执行权限和临时签名。Windows 和 Linux 本轮仅交叉构建与打包。

隐藏命令的合成器等待由实际隐藏时刻计算，跨平台沿用相同的取消与会话校验。macOS 15.2+ 优先使用直接屏幕矩形截图，尺寸不符或接口失败时回退。遮罩使用非激活面板请求键盘焦点，OCR 交接前恢复宿主窗口并显示跳转状态。初始 Esc、真实双屏、OCR 跳转及响应耗时仍待实机测量。

macOS 截图遮罩新增整窗和窗口内元素吸附。有 macOS 辅助功能授权时默认使用元素吸附，未授权时默认使用整窗吸附。Tab 切换模式，滚轮切换元素候选。候选跨显示器时只取当前显示器的交集。Windows、X11、Wayland 延用原截图路径。见 [ADR 0085](adr/0085-macos-screenshot-smart-selection.md)。

### 截图 0.3.0 平台范围

Windows x64/ARM64 使用独立 Win32 GUI 截图程序，支持多屏选择、整窗和 UIA 元素吸附、遮罩内标注及输出。元素不可读时以整窗为候选。macOS 保留 AppKit 遮罩，macOS 12–13 使用系统框选后直接复制。旧版独立图片后处理已移除，Linux 不再构建或登记新版截图插件。宿主及其他插件的 Linux 目标照旧。macOS 侧无界面验证和四目标打包通过；Windows 只完成交叉构建，UIA、混合 DPI、焦点、输入法及系统输出待目标系统验证。详见 [ADR 0086](adr/0086-windows-screenshot-overlay.md)。
