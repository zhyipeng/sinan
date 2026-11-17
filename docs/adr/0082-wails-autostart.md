# ADR 0082：使用 Wails 自带开机自启服务

状态：已实现，替代 ADR 0073 的自启平台实现，未发行；实际系统登录与授权仍待实机验收。

## 决策与原因

用户指出 Wails 3 已提供跨平台开机自启能力，要求替换自行编写的原生注册代码。核对本项目锁定的本地 `github.com/wailsapp/wails/v3 v3.0.0` 源码，已有 `app.Autostart.IsEnabled`、`EnableWithOptions`、`Disable` 和 `Status`，无需升级或新增依赖。

删除 `internal/startup` 及其自行维护的 LaunchAgent、Windows Run、XDG Desktop Entry 写入和路径转义测试。桌面创建 Wails application 后将 `app.Autostart` 注入宿主的私有窄接口；设置 UI、静默启动偏好与开发/验收实例限制保持原有契约，不向第三方页面暴露完整 Wails Bridge。

只有普通默认插件目录实例可配置系统自启，开发、探测和验收实例不注入服务。每次读取查询 Wails 的实时状态，不另存系统自启布尔值。启用传入固定 Identifier `dev.sinan.desktop`，不追加静默、临时目录或开发参数；是否显示主输入框继续只由静默启动开关控制。

## 平台与兼容性

当前 Wails 实现：macOS 13+ 的有效 `.app` 使用 SMAppService；旧系统、非 bundle 或对应服务不可用时走 LaunchAgent。Windows 由 Wails 管理 HKCU Run，Linux 由 Wails 管理 XDG Autostart；不支持构建返回其 ErrAutostartNotSupported。Linux X11/Wayland 的桌面登录及运行依赖沿用已有要求，不因提供统一 Go API 就宣称各桌面环境均验证。

旧原生实现的注册项可能已经存在。Wails 按当前可执行程序匹配既有 plist、Run 命令或 Desktop Entry；发现已开启时直接复用，不再次 Enable，避免 macOS 同时保留旧 LaunchAgent 和新 SMAppService。关闭交由 Wails Disable 处理，关闭后重新开启再采用库当前策略。不在宿主启动或读取设置时自动迁移、重新启用或写入系统登录项。程序搬迁、异常旧文件、符号链接与系统策略变化仍需实际安装升级验证，不保证任意历史路径均自动修复。

Wails 的 LaunchAgent 分支启用时会 best-effort 调用 launchctl bootstrap，关闭时会尝试 bootout；不再承诺旧实现的「不立即加载」语义。macOS 系统可能要求批准登录项。设置动作完成后再次读取真实状态，若未达到请求状态则明确返回未生效/检查系统设置提示，不能把服务调用返回 nil 直接当作自启已开启。读取、写入和不支持错误转换为固定中文提示，不向 UI 透传底层文件、注册表或原生错误详情。

## 架构影响与验证

仍由设置插件提供 UI，宿主提供机制，内核仅保存静默偏好。没有公开协议、SDK、manifest、商店格式或权限模型变化；sinan-official-plugins、sinan-plugin-registry 不需为本次替换联动。CI 删除已不存在的内部包检查入口，保留桌面假服务测试和已有三平台构建矩阵。

所有新增宿主用例使用假 Wails 服务，验证 Identifier、无附加参数、旧注册复用与重复启用去重、关闭、系统批准未生效、读取/注册失败、不支持平台及静默开关独立性；不创建 application，不读取或修改真实自启项，不触发 launchctl、SMAppService 或系统授权窗口。具体本轮结果记录于跨平台文档，编译不等同于系统自启或登录验收。
