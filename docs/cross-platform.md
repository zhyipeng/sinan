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

Windows amd64/arm64 默认构建现创建独立 Environment、临时配置目录和 InPrivate Profile，要求已安装 WebView2 Runtime（目标版本 119+）。不满足必要接口或策略即关闭，不能回退到可信 Wails 页面。替代 `native_webview2loader` 标签及其他架构暂不支持此承载。

每次视图使用独立虚拟 HTTPS 源，资源直接来自会话快照；不启动 HTTP 服务或映射磁盘目录。SDK 窄桥由引擎顶层消息的精确来源校验，资源、调用和撤销继续经过共有 Go 边界。保留可信页头、主题、原生快捷键和拆出流程；原生视图按 Windows DPI 布局。临时配置目录关闭后尽力清理，不能把 InPrivate 等同于内存存储或操作系统沙箱。详见 [ADR 0055](adr/0055-webview2-isolated-plugin-content.md)。

本轮验证为 macOS 全量无界面回归、Windows amd64/arm64 全仓交叉编译及测试可执行文件编译、Windows 静态检查，以及 Linux ARM64 容器中的 GTK3 UI 边界包竞态回归。原生 COM 回调测试已纳入 Windows CI，尚未本机执行；真实窗口、引擎策略效果、层叠、焦点、输入法、拆出及多屏缩放均待明确安排的实机验收。没有运行桌面 UI 或浏览器交互回归。

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

本体提交 a9aebba 的[六目标 CI](https://github.com/zhyipeng/sinan/actions/runs/43799819453)中，macOS/Linux 两种架构通过；Windows amd64/arm64 在 `TestPlatformWindowsApplicationIcons` 的 WScript.Shell 创建中文快捷方式夹具时失败，尚未解决。该失败与独立插件的 CI、发行资产分别记录，不将本体描述为全平台回归通过。
