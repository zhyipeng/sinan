# ADR 0038：React 主面板开发模板

日期：2026-11-11。状态：已实施，无弹窗检查通过；真实 UI 未验收。

## 用户选择

用户要求模板界面像插件管理一样默认在主面板中使用，并明确选择保留动态加载与隔离，同时可采用 React；未要求将第三方 React 直接注入可信页面。

## 决策

模板升至 v3，两种选择显示为 Go 后端 + React、React（无后端），保留内部 html/go-html 类型标识兼容现有创建调用。清单仍用 uiView，不改为仅供构建时可信组件注册的 view。

现有控制器默认 Panel:true，已有返回和拆出；本次不重做窗口承载。React 页面只渲染业务表单，不自行创建窗口或复制标题、返回和拆出按钮，复用宿主原生头部、主题变量和 20px 内容间距。拆出保留同一 WebView，因此 React 状态无需序列化迁移。当前实现仍是独立客体的逻辑主面板，不是把页面嵌入 Wails 的可信 WebView。

新项目增加 package.json、package-lock.json、frontend/src 的 React 组件和入口。使用 React 18.3.1（与当前可信管理界面一致）和 esbuild 0.28.2 本地打包，依赖版本与锁文件固定。入口顶层仅发送一次 UI ready，避免 effect/StrictMode 重复握手。JSX 文本输出由 React 转义，受控输入及 revision 存储保留现有边界。

构建输出仍为 dist/plugin/ui 的固定 HTML、JS 和 CSS，生产压缩且不输出 sourcemap，不使用远程脚本、eval、HMR 或完整 Wails Bridge；UI 资源大小预算继续由宿主验证。首次需 npm ci，Release 构建同步安装锁定依赖。开发链接、原生信任、后端 API 1、UI 桥 2 与目录审核 PR 均不变。

旧项目不自动迁移。项目 skill 同步主面板和 React 开发约定。

## 验证

- `make test`、`go vet ./...` 与 `make build` 通过。
- 两种模板实际执行 npm ci、React/esbuild 构建、ZIP 打包、宿主临时安装和隔离 UI 会话准备；Go 后端实际调用通过，资源满足宿主预算。
- 开发链接重复构建、不覆盖已有目录与卸载保留外部产物的回归通过；原有主面板返回/拆出无界面控制器回归通过。
- 新 Release 工作流通过 actionlint，生成项目的 React 开发 skill 通过 quick_validate，`git diff --check` 通过。
- 目标 WebKit 设为 Safari 15，与项目 macOS 12 构建基线对齐；未据此宣称已在所有系统版本实测。
- 按项目约定未启动真实桌面、原生 WebView 或浏览器交互验证；未运行远程 Release/PR。
