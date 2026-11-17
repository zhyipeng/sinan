# ADR 0075：独立截图插件与原生选区任务

日期：2026-11-17。状态：已实现首版，已随截图 0.1.0 与宿主 0.4.0 发布，真实桌面验收待完成。

## 背景与决策

用户要求新增截图插件，并确认首版为“区域截图＋简单标注（箭头、文字、矩形）”。实现放在独立官方仓库 `plugins/screenshot`，ID `tools.screenshot`，版本 0.1.0；不增加内置业务、不改变宿主公共协议、SDK、CSP 或商店格式。

捕获适配在插件后端按 GOOS 分离：macOS 调用系统 screencapture 交互区域模式；Windows 由系统 Windows PowerShell 承载 WinForms/GDI 覆盖层，捕获虚拟桌面后选择区域、处理负坐标和线程 DPI 感知；Linux X11 使用 scrot，Wayland 使用 slurp + grim。Wayland 仅面向支持 screencopy/layer-shell 的合成器，GNOME/KDE Portal 未接入。依赖缺少、授权拒绝、用户取消和超时返回可显示错误，不退回可信页面注入。

标注、颜色、线宽、撤销/重做在隔离页面 Canvas 中完成，坐标及导出保留原图像素。原图不写剪贴板，点击复制时通过既有宿主 clipboard.v1 写 PNG；保存只写系统图片目录下的 Sinan 子目录，不接受页面传入任意目标路径，不覆盖既有文件。Windows 后台及其 PowerShell 子进程避免控制台弹出。原生进程拥有当前用户系统权限，不是截图权限沙箱；manifest 仅使用已有 clipboard.write。最低宿主 0.2.1 取决于真实 clipboard.v1 / UI Bridge 2 能力。

## 生命周期与资源

捕获异步执行，每插件最多一个活动选区，2 分钟预算、15 秒轮询租约。原图和上传缓存过期回收，页面关闭主动取消。stdio 关闭后取消捕获并等待直接子进程结束；不扩展或承诺任意后代进程树回收。原图放在私有临时目录，正常捕获结束后删除；强杀可能残留临时目录。只在显式保存时保留 PNG，不保存标注历史。

单张 PNG 8 MiB、3200 万像素；Windows 为冻结选择先捕获整个虚拟桌面，桌面本身超过此像素预算时明确拒绝。按 36 KiB 二进制分块读写，保持已有 64 KiB command input 和 192 KiB UI result 预算，上传绑定随机任务标识并校验顺序、长度、PNG 完整性和图像尺寸。页面用 Blob/createImageBitmap 解码至 Canvas，不放宽图片来源 CSP。最多 300 个标注、单条文字 240 字符。

## 影响与取舍

以下手动呼回与无热键限制属于初始实现，已被用户后续要求及 [ADR 0076](0076-hidden-window-commands.md) 替代。

主面板延续失焦隐藏，系统选区可能隐藏编辑器，首版不新增宿主自动恢复接口。完成后以呼出快捷键返回，或先拆出窗口；页面保留原图和标注。没有独立全局截图热键，没有窗口/全屏模式、标注二次编辑、保存对话框或 OCR 联动。

官方仓库追加 Node 构建、无界面测试、通用打包与三系统手动 CI；使用当前 SDK 快照，不修改 UPSTREAM。登记仓库没有真实新 Release，因此不新增商店条目或虚假平台资产。

## 验证

验证在 macOS arm64 上进行，不调用真实捕获、剪贴板、原生窗口、授权界面或浏览器。官方仓库 `npm test` 通过 119 项 JS、11 项 Python 及五个插件后台的 Go 竞态检查（OCR 还包含 workerio）；截图新增 6 项页面/绘图检查和 7 项后台用例。`npm run build:all` 与通用 `node scripts/package.mjs` 通过，截图六目标入口及六个 ZIP 已校验；六目标 `go vet ./backend` 通过。临时宿主目录的 CLI inspect 通过。真实本机插件进程在模拟 Host API 下以 1,049,476 字节合成 PNG、29 个分块验证了 clipboard.write 双向 stdio、失败后重试、上传 ID 防重放和关闭 stdio 正常退出；可用官方仓库 `python3 scripts/check-screenshot.py` 复现。所有捕获 API 均未执行，截图七项 Go 用例仅使用合成图和模拟捕获器。交叉编译与合成无界面验证不代表目标平台运行、真实屏幕捕获或正式发行。需补三端系统授权、混合 DPI/多屏、WebView Canvas/createImageBitmap、中文输入法、焦点与隐藏/拆出验收；Linux X11、支持 screencopy 的 Wayland、GNOME/KDE Portal 的未支持状态分别保留。
