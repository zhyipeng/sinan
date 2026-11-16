# ADR 0070：Windows 呼出快捷键支持 Alt+Space

- 状态：已实施（Windows 实机 UI 待验收）
- 日期：2026-11-16

## 原因

Windows 把 `Alt+Space` 作为当前窗口的系统菜单快捷键处理：DefWindowProc 会将 `WM_SYSKEYDOWN` 转成 `WM_SYSCOMMAND`/`SC_KEYMENU`，即使 Wails 已用 `RegisterHotKey` 注册了该组合，前台窗口仍会弹出标题栏右键（系统）菜单。结果是设置里无法录入 `Alt+Space`，已配置时在司南窗口聚焦时也会弹出菜单而不是响应热键。macOS 的 `Cmd+Space` 与 Linux 无此问题。

## 决策

- 通过 Wails `application.WindowsOptions.WndProcInterceptor` 在宿主所有窗口的消息处理前拦截 `WM_SYSCOMMAND`（`wParam & 0xFFF0 == SC_KEYMENU` 且 `lParam` 低字为空格）即 `Alt+Space` 的系统菜单激活：
  - 处于快捷键录入状态时，原生捕获该组合并以 `Alt+Space` 投递 `shortcut:recorded`，同时吞掉消息，避免菜单弹出，页面无需收到按键。
  - 当前宿主快捷键（呼出、窗口独立或插件）已配置为 `Alt+Space` 时，仅吞掉系统菜单消息；实际切换仍由已注册全局热键的 `WM_HOTKEY` 完成，避免重复触发。
  - 两者都不满足时原样放行，保留普通窗口的系统菜单行为。
- 消息判定放在与平台无关的纯函数中，便于跨平台无界面测试；只有 Windows 构建安装拦截器，其他平台返回 `nil`。
- 拦截器运行在 UI 消息线程，不能等待快捷键控制器或 `recordMu`：录入状态与"是否配置 Alt+Space"用两个 `atomic.Bool` 暴露，变更时在各快捷键控制器的 `changed` 回调中重新计算，避免与 `Save` 里"持控制器锁再 `InvokeSync` 主线程"形成死锁。
- 未修改 Wails 依赖、全局快捷键注册方式、插件协议、SDK 或设置存储格式。

## 影响及验证边界

`Alt+Space` 一旦作为全局热键注册，司南运行期间该组合会被系统级接管，其他应用的系统菜单也不再响应该组合，这是全局热键的固有代价，不额外提供回退。若注册失败（例如被其他程序占用），司南窗口内仍会吞掉 `Alt+Space` 菜单但不会触发切换，注册错误照常经 `shortcut:changed` 展示。

无界面验证：新增消息判定与 `Alt+Space` 规范化单元测试，快捷键控制器既有录入/切换/冲突/回滚用例保持通过，Windows amd64 本机 `go vet` 与相关用例通过。未执行 Windows 真实桌面 UI、WebView2 键盘、系统菜单与全局热键的实机验收；macOS/Linux 不受本改动影响但真实输入法与焦点仍未实机验证。Windows 全量 desktop 测试仍受既有 `TestLocalZIPNativeRequiresTrust`（原生入口需 `.exe`）夹具限制，与本改动无关。
