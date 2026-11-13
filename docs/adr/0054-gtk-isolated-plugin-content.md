# ADR 0054：GTK3 主窗口内的隔离插件内容

日期：2026-11-13。状态：代码已实现，原生编译与无界面回归通过；真实 UI 未验收。

## 决策

延续用户要求的 Windows/Linux 同步适配，在共用原生会话边界后实现 Linux GTK3 的内容承载。保持真正的同窗口切换、可信页头与独立 WebView，禁止复用可信 Wails WebView 或注入第三方 DOM。Windows WebView2 renderer 仍待实现，本决策不代表两端 UI 均已完成。

## 实现和安全边界

- 每个候选页面创建新的 ephemeral `WebKitWebContext` 和独立 `WebKitUserContentManager`；不继承 Wails 用户脚本或完整 Bridge。仅在当前 GTK 窗口中叠放内容子视图，避开 58 DIP 可信页头。主面板内容最高 542 DIP，拆出后保持同一视图并随原窗口扩展。
- `sinan-plugin` Scheme 的资源逐次经过 Go 会话，统一校验归属、方法、路径、预算及撤销状态。每次响应携带同一 CSP、nosniff 和 no-store；资源拒绝不降级到磁盘或网络。
- 原生消息端口只注册在宿主命名的隔离脚本 world，且宿主脚本只注入初始文档的顶层 frame。页面 world 的兼容 facade 保持现有 `window.webkit.messageHandlers.sinan` / `sinanCall` 形状，但只通过 DOM 消息传递请求，不持有原生端口。
- GTK 消息信号不提供 WKFrameInfo，不能将 WebView 当前 URL 冒充原生消息的 frame 身份。隔离 world 的转交器要求浏览器产生的可信 message 事件、`event.source === window`、顶层窗口及当前精确入口；拒绝来自子 frame 的 postMessage 和 synthetic 事件。页面 envelope 中的任何来源声明均不参与校验。原生侧再检查引擎当前文档和固定会话。公开 facade 的响应不构成授权凭据，页面伪造自己的响应不能获取新宿主能力。
- 仅放行宿主发起的一次初始加载和对应主文档响应，后续导航、重定向、弹窗、下载、文件选择、媒体权限和脚本对话框全部拒绝；关闭 WebRTC、DNS 预取和持久 HTML 存储。资源及业务调用沿用会话预算、白名单、存储和超时。
- 隐藏准备页面，ready 后由现有窗口序列提交，准备失败保留旧页面。引擎退出、加载失败、响应探测超时、会话撤销或一小时寿命触发关闭。异步回复和响应探测持有原生对象引用，关闭时先撤销，再断开信号和释放视图，迟到调用只回复固定错误。
- Esc、Ctrl+, 和已配置的拆出快捷键在 GTK 原生按键路径中转交宿主；字母、数字和逗号按标准 evdev/XKB 物理位置匹配设置录入的 KeyboardEvent.code，覆盖非拉丁布局与 Shift+数字组合，不向页面增加窗口操作权限。主题与快捷键沿用单调修订，同一视图拆出保留页面状态。

## 兼容与验证

Linux UI 构建要求 CGO、`gtk3` 标签、WebKitGTK 4.1 API（引擎版本至少 2.40，支持异步 script-message reply）。普通同窗口承载由 `EmbeddedSupported` 开启；旧独立 helper / ui-probe 不因此获得 Linux 支持。

后端 API 1、UI 桥 1/2 和插件清单不变，现有 SDK 无需新增宿主权限。Node VM 测试覆盖 SDK 兼容、错误/文档/子 frame 拒绝、single-flight 和原生错误清理；这些测试不能替代 WebKit 实际事件与窗口验收。常态验证只编译及执行无窗口回归，不初始化 GTK 显示服务器或创建 WebView。

此实现不提供原生控制器故障隔离、独占 WebContent 进程或操作系统沙箱。GTK 叠放、焦点、输入法、快捷键、主题变化、拆出、渲染故障及 X11/Wayland 行为仍需明确授权后在真实桌面验收，不将静态检查描述为这些体验已通过。


验证记录：macOS `make test` 通过；Debian 12 ARM64 容器内使用 Go 1.26.0、Node.js 22.22.1、GTK 3.24 / WebKitGTK 2.50.6，`go test -tags gtk3 -race -p 1 ./...`、桌面原生构建与 `go vet -tags gtk3 ./...` 通过。Windows amd64/arm64 全仓交叉编译通过。WebKitGTK 2.50 对兼容旧版本的 DNS 预取设置 API 给出弃用警告，不影响编译；未据此宣称获得网络或 OS 沙箱。
