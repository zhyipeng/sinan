# ADR 0079：图片导入与后处理粘贴

日期：2026-11-17。状态：已实现，未发布，真实桌面交互待验收。

## 需求与决策

截图后处理可直接导入本地图片或使用 Command/Ctrl+V 粘贴图片，无须先截图。图片载入继续使用现有 Painterro 编辑器，取消或失败保留当前内容；替换成功后使用新图建立编辑状态。文字编辑区域保留原生文本粘贴，输入法组合期间不截获快捷键。兼容页面原生 paste 事件，图像剪贴板读取仅由用户粘贴触发，不做后台监听。

- 宿主新增 ui.image-input.v1 能力与 UI Bridge 2 的 file.openImage，SDK openImage()；请求只允许 method 字段，权限为 file.openImage。宿主从活跃会话解析所属窗口、通过 Wails 原生 OpenFile 选择单个文件，使用既有 beginDialog 防止失焦隐藏；不向页面暴露选择路径、任意文件读取或完整 Wails Bridge。
- 文件导入支持 PNG、JPEG、GIF（首帧）。输入须是普通文件，最多 8 MiB，解码前校验 1600 万像素上限，完整解码并规范化为最多 4 MiB PNG，输出编码有写入上限；不保留动画。暂不支持 WebP、HEIC、SVG 等其他格式。
- 规范化图片沿用 Session.AttachLaunchImage：仅当前会话持有的 sinan-plugin URL，替换上一份快照，撤销即不可用，页面通过 Image.decode 与 createImageBitmap 载入。无需放宽 CSP，也不传输 base64 大图片到页面调用响应。
- 粘贴复用已有 clipboard.readImage/readClipboardImage 和视图 clipboard.read 权限，沿用 4 MiB / 1600 万像素校验。未读到图片只提示，不覆盖当前内容，不调用文件导入或系统截图。
- 视图权限白名单增加 file.openImage，单视图声明权限数从 4 增至 6，以容纳截图现有窗口执行/显示/隐藏、保存以及本轮导入/剪贴板读取；不变更其他调用、并发、请求/响应、资源累计预算或 CSP。新增权限仍独立校验，不能借窗口权限读文件或剪贴板。
- file.openImage 使用与 file.savePNG 相同的 125 秒调用预算和 WebView2 127 秒等待；文件选择前后及图片处理后检查取消和会话撤销。Wails 无通用强制取消对话框接口，已打开的系统对话框超时后仍须由用户关闭，返回结果不再附加到会话。

协议保持 API 1 / Bridge 2，旧宿主不具备增量能力。截图后台初始化同时检查 ui.image-input.v1，SDK 来源补丁及摘要同步；登记仓库不添加未发布资产。三平台共用 Go 接口，通过 Wails AppKit、Windows 和 GTK 对话框适配，平台运行支持与原生构建仍按跨平台文档区分。

## 验证

无界面用例覆盖 PNG/JPEG/GIF 规范化、损坏和过大图片、PNG 输出上限、声明权限、额外路径字段拒绝、会话撤销、直接导入、Command/Ctrl+V 与原生 paste、文字编辑与输入法、取消/无图片/解码失败保留内容、页面退出后的迟到结果。最终构建和验证结果见 docs/cross-platform.md。未运行真实桌面、WebView、文件选择对话框或系统剪贴板验证。
