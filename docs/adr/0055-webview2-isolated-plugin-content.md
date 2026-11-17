# ADR 0055：Windows 同窗口隔离 WebView2 插件内容

- 状态：代码已实现；交叉编译及无界面测试已通过，Windows 实机验收待完成
- 日期：2026-11-14
- 关联：[ADR 0053](0053-shared-native-ui-boundary.md)、[ADR 0054](0054-gtk-isolated-plugin-content.md)

## 问题

Windows 已具备应用检索、主题与屏幕适配，HTML 插件 UI 仍报告不支持。继续使用可信 Wails 页面承载第三方 HTML 会破坏既有边界；直接复用完整浏览器包装也会引入宽泛桥接及进程退出等行为。

## 决策

在已有 Wails HWND 中创建独立 WebView2 Controller，以原生内容区保留 58 DIP 可信页头。沿用宿主的隐藏准备、ready 后提交、旧视图保留、撤销和拆出流程；拆出保留原 HWND，不复制客体页面。主窗口内容下缘最多为 600 DIP，拆出后填充剩余客户区。由于隔离 WebView2 与全尺寸 Shell WebView 同属一个父窗口，创建控制器后需把新增的宿主子窗口显式置于 Shell 之上，并在每次布局（含就绪、拆出、DPI 变化）后重新置顶，否则插件内容会被覆盖整个客户区的 Shell 完整盖住，界面表现为"一片空白"。

每个视图创建独立 UserDataFolder、独占 Environment 和 InPrivate Profile，不复用可信 Shell 配置。UserDataFolder 优先放在每用户缓存目录 `%LOCALAPPDATA%\Sinan\webview2` 下的唯一子目录——共享系统临时目录在启用杀毒实时扫描或经组策略限制、定期清理的 Windows 上会让 Environment 创建变慢甚至超时；缓存目录不可用时回退系统临时目录，各视图仍是唯一目录以保持存储隔离。要求已安装的 WebView2 Runtime，环境目标版本至少 119；创建选项、必要接口或宿主策略调用失败即关闭，禁止降级为可信 WebView。首次打开需冷启动独立 Environment，等待采用较宽裕上限并在就绪后立即提交，避免把正常冷启动误报为超时；目录在关闭后重试删除，不承诺内存存储；异常退出及浏览器延迟释放可能留下元数据。未实现启动时残留目录清扫。

仅复用固定版本 `github.com/wailsapp/go-webview2 v1.0.22` 的运行时加载器，不复用其高层 Chromium 宿主或生成的接口包装。加载器会清空 WebView2 环境变量中的参数、路径及调试覆盖，影响当前进程后续 WebView2 创建；不恢复这些覆盖以免客体复用可信配置。薄 COM 封装依据微软 NuGet `Microsoft.Web.WebView2 1.0.2210.55` 的 `build/native/include/WebView2.h` 实现，显式处理句柄、BOOL 值、输出指针、回调 IID、引用计数和固定地址。amd64 的 RECT 值通过指针传递，arm64 通过两个整数寄存器传递；已用离线 C 交叉编译核对该 ABI。所有实例原生操作在 Wails UI 线程执行，无嵌套消息循环，不调用 `os.Exit`。非 amd64/arm64 或启用替代 `native_webview2loader` 构建标签时继续报告不支持。

## 资源与桥接边界

- 每个视图获得随机 `https://<随机值>.sinan.invalid` 虚拟源，不启动 HTTP 服务，不将磁盘目录映射为网站。所有资源事件均由宿主响应；仅精确源、GET 和有效会话快照可返回内容，其余直接返回空 404。读取请求前先设置默认拒绝响应，后续解析或资源流失败仍保留该响应。无法建立拒绝响应时同步停止并关闭 Controller，避免交给默认网络处理。
- 虚拟源在宿主内部映射到绑定的 `sinan-plugin://` 地址。拒绝用户信息、端口变体、查询、片段及非规范转义。CSP 仅将原有资源 Scheme 替换为当前精确源，继续拒绝连接、子 frame、worker、对象、媒体及表单提交；返回 no-store 和 nosniff。
- 内部图片 URL 仅在 `launch.get` 返回的 `image.url` 字段转换；输入、存储和其他字符串保持原值。原始会话、资源与权限协议不变，无新增插件权限。
- 只订阅 CoreWebView2 顶层 `WebMessageReceived`，不订阅 frame 的桥接事件；来源使用引擎 `Source` 并匹配精确入口，页面自报身份无效。兼容现有 SDK 的 `window.webkit.messageHandlers` 形状，内部使用小型、有界消息信封；不注册 Host Object、Wails Bridge 或通用 CDP API。
- 继续复用 Go 会话的 ready/桥版本、命令白名单、存储、单次并行调用、大小/频率/累计预算与撤销校验。迟到结果不返回页面。页面 Promise 有并发上限和超时，宿主错误只返回固定文本。

在导航前关闭默认对话框、上下文菜单、开发者工具、Host Object、状态栏、错误页及浏览器快捷键；拒绝权限、下载、新窗口、frame 和非初始导航。通过宿主私有 `Page.setInterceptFileChooserDialog` 禁用引擎文件选择，完成该策略及启动脚本注册后才允许导航。启动脚本屏蔽 print、execCommand 打印/另存为、File System Access 选择器和 RTC 构造入口，防止额外引擎 UI；这些措施不能替代操作系统沙箱。

快捷键由原生 AcceleratorKeyPressed 处理，固定 Escape / Ctrl+Comma 及配置的拆出组合；字母、数字、逗号按物理扫描码匹配，修饰键严格匹配并排除 AltGr。内容位置按实际 DPI 换算，定期更新边界与父窗口位置。页面心跳失败、渲染终止、准备超时、会话撤销或一小时寿命到期均回收视图。

## 验证与限制

通过 macOS `make test`，新增无引擎的消息、资源、URL 转换、预算、DPI、快捷键和 JS Promise 测试；Windows amd64/arm64 全仓交叉编译及测试可执行文件编译，Windows 静态检查及 Linux ARM64 容器中的 GTK3 包竞态回归通过。新增 COM 回调 GC/引用/IID 测试已加入 Windows CI 范围，本地只编译，未在 Windows 执行。

未创建真实 Windows 窗口或运行 WebView2，因此尚未验证控件层叠、焦点、输入法、拆出、运行时策略实际效果、多显示器缩放及原生崩溃恢复。交叉编译和模拟消息测试不等于这些能力的实机验收。现有远端三平台 CI 尚未在本轮触发。安装器、签名、内置插件发行安装和完整 Windows 测试夹具迁移仍待完成。

独立 Profile、引擎限制及独立渲染进程用于缩小页面能力及隔离故障；不声称具备操作系统安全沙箱、浏览器后台完全无网络或 CPU/内存硬配额。
