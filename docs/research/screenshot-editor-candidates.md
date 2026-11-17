# 截图后处理组件比较

调查日期：2026-11-17。按用户要求只给选型建议，未安装、执行或替换现有编辑器，也未运行浏览器或真实 WebView。

## 建议

优先考察 **Painterro**。它面向快速截图编辑，提供箭头、矩形、文字、裁剪、像素化和自定义保存，较贴合当前插件；MIT 许可，纯 JavaScript 集成，无须将插件 UI 改为 React。若更看重多对象选择和变换，可再考虑 Filerobot；若接受 Linkware 的署名链接要求，可考虑 marker.js 3。

| 候选 | 许可与能力 | 本项目适配判断 |
| --- | --- | --- |
| [Painterro](https://github.com/devforth/painterro) | [MIT](https://github.com/devforth/painterro/blob/master/LICENSE)；箭头、文字、矩形、裁剪、像素化、撤销重做、自定义 PNG/Blob 保存 | 优先候选，工具与截图场景贴近；需要处理样式、字体图标及图片输入/导出的 CSP 适配 |
| [Filerobot Image Editor](https://github.com/scaleflex/filerobot-image-editor) | MIT；箭头、文字、矩形及多对象选择/变换，另含滤镜等图像编辑能力 | 功能完整，React 包依赖 Konva、组件库，并要求 React/React DOM/react-konva/styled-components；结合依赖结构判断，比当前需求更重，实际产物大小未测 |
| [marker.js 3](https://github.com/ailon/markerjs3) | [Linkware](https://github.com/ailon/markerjs3/blob/main/LICENSE)，须保留界面链接或等效显著公开署名，移除该条件需另获许可；标注内核与工具栏分离 | 适合保留自定义工具栏；不能把 markerjs-ui 的 MIT 许可当成内核许可。实际产物大小未测 |
| [TOAST UI Image Editor](https://github.com/nhn/tui.image-editor) | MIT；裁剪、绘制、文字、形状、滤镜、撤销重做，依赖 Fabric 4.2.0 和配套工具库 | 完整图像编辑器，当前仅截图标注无需其大部分功能；结合功能与依赖结构不作为轻量首选，实际产物大小未测 |

## 实际体积与隔离约束

从 [npm 发布元数据](https://registry.npmjs.org/painterro/1.2.92) 指向的 1.2.92 tarball 读取文件，仅在内存解包，不执行代码、不安装依赖：

| Painterro 1.2.92 发布文件 | 原始字节 | 本机 gzip 字节 |
| --- | ---: | ---: |
| build/painterro.min.js | 301,851（约 295 KiB） | 84,129（约 82 KiB） |
| build/painterro.commonjs2.js | 279,371（约 273 KiB） | 80,141（约 78 KiB） |

gzip 数字仅帮助理解传输体积，宿主实际核验原始文件字节。原包单 JS 在宿主 512 KiB 单文件限制以内，但最终插件仍需核对完整依赖、文件数、2 MiB 总资源预算及实际构建产物。

Painterro 的[构建配置](https://github.com/devforth/painterro/blob/master/webpack.config.js)使用 style-loader 与字体 url-loader；发布 JS 中静态检出了 style 元素创建、@font-face 和 data: 字串。当前宿主禁止内联样式、字体及 data/blob 图片 URL，因此不能把现成发布包视为已兼容：需要抽离 CSS、替换字体图标、检查动态样式及图片加载/导出路径，继续由宿主桥负责复制和保存。具体改造可行性和三端引擎行为尚未验证，不能为接入组件而直接放宽 CSP。

[Painterro 文档](https://github.com/devforth/painterro/blob/master/README.md)宣称轻量原生 JS，但 package.json 仍列有 core-js、caniuse-lite、dom-to-image 等依赖，所以不将它表述为零依赖。[Filerobot React 包声明](https://github.com/scaleflex/filerobot-image-editor/blob/master/packages/react-filerobot-image-editor/package.json)用于核对依赖和许可；重量比较是据依赖结构做出的判断，未冒充实测。

结论：Painterro 最值得先做隔离环境的适配验证；用户后续已确认接入 Painterro，私有隔离适配和本轮结果见 [ADR 0077](../adr/0077-painterro-screenshot-editor.md)；本页体积表仍保留调查时上游发布包的数据。
