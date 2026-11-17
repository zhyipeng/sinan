# ADR 0077：截图后处理接入 Painterro

日期：2026-11-17。状态：已实现，已随截图 0.1.0 与宿主 0.4.0 发布；真实 WebView 与桌面交互待验收。

## 需求与决定

用户在组件比较后明确要求按推荐接入 Painterro。独立官方 `tools.screenshot` 0.1.0（尚未发布）改用固定 Painterro 1.2.92 的编辑工具、选择/裁剪/像素化和撤销机制，移除原手写绘图逻辑。截图双入口、CmdOrCtrl+Shift+A、有效图片才进入后处理、复制与保存链路均沿用 ADR 0076。实现完全留在插件仓库，不新增宿主 API、协议、权限、SDK 或商店格式。

使用 npm 固定版本和锁文件，从包内 js 源码进行私有构建适配；`painterro-upstream.json` 记录上游地址及每个参与文件的 SHA-256，构建前后核对来源。补丁集中在 `scripts/painterro-csp.mjs`，关键方法以 AST 定位，固定片段替换要求唯一命中；来源或结构不符时失败，不直接修改 node_modules。包内保留 MIT 许可文本 `THIRD_PARTY_NOTICES.txt`。

## 隔离适配

不使用发布包的 style-loader、字体或 dom-to-image，也不放宽现有 CSP：

- 上游基础样式作为独立 painterro.css，主题用静态 CSS 与宿主变量；模板 style 属性改为私有 data 属性，由插件在自身 DOM 中以 CSSOM 属性设置。移除动态 style 元素和配置主题字符串注入。
- 从原字体 SVG 的固定 glyph 路径构建 36 个本地静态 SVG 节点，作为按钮图形并内嵌到插件 DOM；不加载字体或 SVG 图片资源。宿主资源类型不支持外部 SVG，上游棋盘格改用纯 CSS 渐变。
- 截图按原有分块读取，以 Blob/createImageBitmap 解码；Painterro 直接将 ImageBitmap 绘入 Canvas，不加载 data/blob 图片 URL。PNG 导出使用 Canvas.toBlob，复制与保存仍走同插件后台及宿主剪贴板。
- 文字工具保留输入和粗体/斜体/描边控制，渲染改为 Canvas.measureText/fillText/strokeText，支持原图缩放坐标和换行。富文本粘贴转纯文本，最多 2000 个 Unicode 码点、100 行；保留输入法组合期间的输入，组合结束后处理长度。
- 禁用原组件文件/URL 打开、图片粘贴拼接、尺寸扩展、滤镜等当前不开放的入口；删除默认图片 drop/paste 事件及原生 navigator.clipboard 入口。外层 Command/Ctrl+C/S 继续走宿主桥，Z/Shift+Z 走组件历史，文本编辑区保留原生快捷键。
- 每张原图重置历史。最多 20 次撤销，按原图 RGBA 大小以约 64 MiB 历史缓存目标调整，最少保留一次撤销；大图两帧本身可超过该目标，这不是硬内存配额。单张 3200 万像素和 PNG 8 MiB、原桥调用与资源预算不变，临时画布与编码仍有额外内存。

## 平台与兼容

继续面向 WKWebView、WebView2、WebKitGTK，构建目标沿用 Safari 16.4 / Chrome 119 / Firefox 115；不据此宣称三端 WebView 已验收。鼠标滚轮适配 wheelDelta/deltaY，Command/Ctrl 在插件外层归一，退出移除组件文档/窗口事件及观察器。后端和系统选区适配没有改变，Linux GNOME/KDE Wayland Portal 仍未接入。

前端包仅 4 个资源，合计 148,227 字节（约 145 KiB），单文件及总量在现有 512 KiB / 2 MiB / 128 项限制内；构建入口新增预算和资源类型检查。linkedom 0.18.12 只用于 Node 无界面测试，不进入插件包。没有安装到现有用户配置，也没有发布或修改登记仓库资产。仍要求 ADR 0076 的配套宿主，已发布 v0.3.1 不具备 window-execute 能力。

## 验证

本机 macOS arm64 官方仓库 `npm test` 通过 127 项 JS、11 项 Python 及五个后台竞态检查；截图用例共 14 项 JS、10 项 Go。真实适配后的 Painterro 在 linkedom DOM 和模拟 Canvas 下覆盖箭头、矩形、中文/换行/纯文本、撤销重做、裁剪、像素化、导出、历史清空、忙碌状态、组合输入、快捷键和卸载；图片及 Canvas 绘制边界均为合成数据，不是浏览器截图或真实像素渲染验收。

截图六目标构建及通用打包通过；宿主现有 uiview.LoadFS 以严格资源白名单和既有 CSP 接受最终界面，临时宿主 inspect 通过。六个 ZIP 的清单、图标、UI 文件、MIT 许可和 SHA-256 已核对；真实本机后台在模拟 Host API 下通过约 1 MiB 合成 PNG 上传、失败重试与 stdio 退出检查。原生后端本轮无变更，交叉编译不代表 Windows/Linux 运行。

未运行真实屏幕捕获、系统授权、系统剪贴板、桌面、浏览器或三端原生 WebView。SVG 按钮外观、CSSOM/CSP 实际行为、中文字形及输入法、焦点与拆出、多屏/DPI、大图性能和内存仍须用户明确要求后实机验收；CI 仅沿用三平台配置，尚未远端执行。

## 同日工具栏样式修正

根据用户截图，修正上游 32px 棋盘格底板的负边距与插件 24px 颜色按钮混用导致的覆盖；统一标签、数值字段与布尔开关的字号和对齐。颜色装饰底板隐藏，数值框保留 number 语义并用静态 CSS 隐藏系统步进按钮，布尔开关显示明确的未选中边框和选中勾形。工具栏溢出保持横向滚动，焦点环改为内侧，避免被裁切。不改变编辑、复制、保存或截图流程，也未修改宿主安全边界。

14 项现有截图无界面回归及 CSS 解析通过，六目标构建、通用打包和 ZIP 摘要核对通过；当前资源合计 149,935 字节，仍为 4 项。仅验证样式源码、构建和组件逻辑，没有执行三端真实 WebView 的样式渲染、主题或 DPI 验收。
