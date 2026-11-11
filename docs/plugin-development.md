# 插件开发模板 v3

插件管理页点击“创建插件”，填写插件名，确认可编辑 ID，选择模板和父目录后创建。项目写入 `<父目录>/<插件 ID>/`，不复用已有目录。取消目录选择保留当前输入。

## 两种模板

| 模板 | 运行模式 | 示例 | 构建要求 |
| --- | --- | --- | --- |
| Go 后端 + React 界面 | process，后端 API 1、UI 桥 2 | 页面调用 echo 后端，并保存私有草稿 | Node.js 22.12+、Go 1.26+ |
| React 界面（无后端） | declarative，UI 桥 2 | 页面处理输入并保存私有草稿 | Node.js 22.12+ |

无后端 React 模板不携带 Go 模块、后端或后端命令。两种模板都以 React 编写业务界面，默认在主面板内使用，返回与拆出由宿主管理。动态页面仍保持隔离，不进入可信 React 树。两种模板均使用本插件 storage 声明和受限 UI Bridge，没有完整 Wails Bridge、任意文件/网络 Host API 或显式 UI 授权弹窗。原生进程仍不等于安全沙箱。

## 项目结构

```text
<plugin-id>/
  manifest.json
  README.md
  .gitignore
  sinan-template.json
  package.json
  package-lock.json
  frontend/
    index.html
    src/
      main.jsx
      App.jsx
      style.css
      bridge.js
  scripts/build.mjs
  scripts/package.py
  scripts/release.mjs
  scripts/registry-pr.mjs
  .github/workflows/release.yml
  AGENTS.md
  cmd/main.go                # 仅 Go 模板
  go.mod                     # 仅 Go 模板
  third_party/sinan/         # 仅 Go 模板：协议与 SDK 快照
```

插件名为 1–64 个 Unicode 字符，不允许控制字符。名称只进入 JSON 编码后的清单，不插入 HTML 或可执行代码。ID 遵守现有清单规则，例如 `local.my-tool`，不能使用 `system.*`。英文名自动生成 ID；中文等无法提取英文片段时默认 `local.my-plugin`，可手动修改。选定 ID 也是目录名和插件数据身份。

SDK 快照来自当前宿主内嵌的公开协议定义、Go SDK 和 UI Bridge，不引用宿主 internal。`sinan-template.json` 记录模板版本、类型和相关文件 SHA-256；它是开发追溯信息，不是签名或商店可信身份。SDK 正式发布后可演进依赖方式。

## 构建与安装

在生成项目内执行：

```sh
npm ci
npm run build
```

首次构建需安装锁定的 React/esbuild 依赖。JSX、CSS 和 UI SDK 打包为本地 JS/CSS，不使用 CDN、内联脚本或热更新服务器；ready 握手位于 main.jsx 顶层，避免 React effect 重复发送。构建保持压缩且不输出 sourcemap，确保模板满足当前资源预算。构建输出 `dist/plugin`，其中包含清单、UI，以及 Go 模板的后端。Go 模板按当前 Go 目标平台构建，Windows 目标自动使用 `backend/plugin.exe`；构建目标不等于该平台 UI 已获得运行验证。构建脚本只重新生成本项目 `dist/plugin`。

在插件管理现有“安装 / 升级插件”入口填写 `dist/plugin` 的绝对路径，先检查再安装；Go 模板需确认原生信任。源码目录不是可安装包。HTML 插件界面默认启用，可用 `-plugin-ui=false` 显式关闭；仅 macOS 已实现承载。修改后重新构建，并通过现有检查/升级流程应用。

创建项目不会执行构建、下载依赖、安装插件、初始化 Git 或向插件目录注册内容。项目与官方/第三方身份无关，不自动登记到商店目录。

## 实现边界

模板和表单属于 `system.plugin-manager`，宿主只提供可信页面的类型化目录选择和创建适配器。第三方页面桥及 stdio API 不增加项目创建方法。

目录选择附着到发起的可信窗口；对话框期间主面板暂停失焦隐藏和呼出重置，关闭后恢复。未运行真实 UI 验证，目录框交互及输入法等仍需按需人工验收。

文件生成先验证请求，再以排他方式新建项目目录；通过根目录约束及排他文件创建写入固定模板路径。已有目录、文件或同名符号链接均拒绝复用。写入中途失败保留未完成的目录并明确报错，不自动递归清理未知文件；用户处理后重试。不声称防御同一用户进程的任意并发文件篡改。


## 开发链接

```sh
node scripts/build.mjs --dev
node scripts/build.mjs --dev --plugins-dir /absolute/path/to/plugins
```

这是单次构建，不是 watch。默认链接目标位于当前系统配置目录下 `Sinan/plugins/<id>`，与普通宿主默认插件目录一致；使用开发实例时需指定其 `.sinan-dev/plugins` 绝对路径。链接指向本项目 `dist/plugin`，不复制产物、不写宿主信任文件。已存在的普通安装目录或其他链接会拒绝覆盖。开发模式只允许本机操作系统的 Go 后端，交叉编译用于发行构建。

宿主刷新会发现开发链接；Go 插件首次在插件管理中勾选允许原生运行并确认“信任开发插件”，信任绑定规范化的目标目录及当次检查内容。后续在同一目标目录构建后刷新、显式重载，保留开发信任；更换目标不继承信任。修改中的产物可能暂时不可用，不保证旧资源快照或业务副作用回退，Windows 占用文件时可能需先停止插件。

只有插件根目录这一层允许开发链接，包内部符号链接仍拒绝；内置 system.* 插件不接受开发链接。普通安装继续复制和校验包，开发链接不能走普通升级事务。卸载只删除链接和开发信任，保留外部源码与产物；再用普通安装即可切回发行包。开发脚本可在宿主运行时建立新链接，不写宿主状态或取得其目录锁；宿主状态和管理操作仍由单实例内核串行处理。所有原生代码仍以当前用户权限运行。

## 自动发布与目录 PR

标签 `vX.Y.Z` 必须对应 manifest.version。工作流在 Ubuntu 上以 CGO_ENABLED=0 交叉编译 darwin/arm64 和 darwin/amd64，生成 ZIP、SHA-256 及 release.json；仅标签触发发布，手动执行也须选择版本标签。Go 源码先在运行器本机执行测试与静态检查，不在 Linux 运行 macOS 二进制。原生库依赖或其他平台需自行调整运行器/矩阵，不把交叉编译描述为实机验证。

GitHub Actions 已固定第三方动作提交，构建任务只有读取仓库权限，发布任务才有 contents 写权限。已存在 Release 不覆盖；部分发布失败需人工处理，不能在同版本静默换包。

可选配置仓库变量 `SINAN_REGISTRY_REPOSITORY=owner/repo` 和 GitHub Secret `SINAN_REGISTRY_TOKEN`。跨仓凭据需要目标目录仓库 Contents/Pull requests 写权限；私有源仓库还需读取源 Release。未配目录变量跳过 PR，配置后无凭据则失败并保留已发布 Release。默认 GITHUB_TOKEN 不可替代跨仓授权。自动化只创建/复用 PR，不自动合并。

目录 PR 写入 `submissions/<id>/<version>.json`：固定源码提交、Release/Asset 标识、下载地址、兼容协议、平台、大小和摘要，作为待审核记录。目录仓库当前校验该格式，但不生成安装索引或授予官方身份；第一次提交可针对未收录插件。审核人需要核实归属与真实字节，正式宿主兼容范围仍须在纳入安装索引前确认。

项目级 agent 开发指引位于 `AGENTS.md`，覆盖运行协议、主题和存储边界、开发链接、测试及发布授权。创建模板或构建产物不授权推送标签、发布 Release 或发起远程 PR。


## 模板 v3 的呈现边界

当前 macOS 动态插件内容使用独立配置的 WKWebView，作为现有 Wails 主窗口的原生子视图呈现在内容区；搜索、设置、插件管理和动态页面在同一个 NSWindow 中切换，页头直接使用可信 Shell 的同一 React 组件。返回移除动态内容并恢复搜索；拆出复用现有 Wails 窗口和整棵原生视图树，再创建新的搜索主窗口。页面不进入可信 WebView 的 DOM 或 React 树，也不获得 Wails Bridge。UI 原生控制器现位于宿主进程，不再声称具备独立 UI 客体进程故障隔离。无后端 React 和 Go + React 模板均沿用该机制，前端不自行打开窗口，不重复绘制宿主标题栏和返回/拆出按钮。

模板选择器显示 React 名称，内部模板类型 html/go-html 保留以兼容已有调用；新项目元数据的 templateVersion 为 3。旧项目不会自动迁移，新项目需先 npm ci 再构建。发布工作流也先执行 npm ci，开发软链接和审核 PR 机制保持。


插件的宿主页头与插件管理统一使用 58 DIP 高度、箭头返回/拆出、标题/来源分层和主题色；拆出后保留页头并切换为关闭按钮。业务组件只维护下方内容区，不复制导航栏。普通插件页头直接复用可信主窗口组件，现有 HTML 插件同样适用；独立 UI helper 仅保留给探测及历史原生验证，不参与普通页面打开。
