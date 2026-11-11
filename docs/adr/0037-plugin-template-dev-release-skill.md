# ADR 0037：模板开发链接、Release 和项目 skill

日期：2026-11-11。状态：已实施，无弹窗检查通过；无真实 UI 或远程发布验收。

## 用户决策

用户要求模板构建增加开发模式并链接默认/可选插件目录，提供自动编译/Release 和可选目录 PR，以及项目级 agent skill。用户明确选择：开发脚本只创建链接，原生信任首次在插件管理界面确认，不由脚本写入信任文件。

## 机制

模板版本升至 2。`build.mjs --dev [--plugins-dir 绝对路径]` 单次构建后链接 `dist/plugin`，保留普通构建。已有安装或不同链接拒绝覆盖，同目标可重复构建；Windows 使用目录 junction，不提供 watch/自动重载。

宿主扫描允许根层级开发链接，解析规范路径到安装目录之外，禁止 system.* 与包内符号链接。注册记录保存目标路径，默认原生不信任。可信管理操作以当前包指纹复核后保存绑定目标的开发信任，不自动运行；普通安装的原生信任记录不继承。信任允许同一路径后续开发修改，但仍须显式重载；链接改指其他目标后拒绝沿用，重启发现也不继承。开发缓存保持在宿主状态目录。开发构建是允许的外部文件编辑例外，不操作宿主配置或突破单实例状态写入约定。

开发链接不支持普通升级事务，避免重命名或回退外部项目；卸载只移除根链接和开发信任，保留源码/产物。包内容仍遵守原有预算和校验。外部修改和原生副作用不在回滚保证内；同用户文件篡改防御及 OS 沙箱仍未实现。

模板工作流使用固定 Actions 提交，标签必须与清单版本对应，在 Linux 交叉编译两个 macOS 架构；默认 CGO_ENABLED=0，需要原生库时由作者调整。上传根布局 ZIP、SHA-256 和版本资料后创建 Release，不覆盖已有版本。跨仓 PR 需单独配置目标和授权，写入目录的 submissions 待审核记录，读取真实 Release/Asset 标识；重复一致记录复用，不覆盖冲突内容、不自动合并。未进行任何真实远程写入。

目录仓库新增该审核记录的本地校验；plugins 中 releases 仍为空，宿主商店、可安装索引、签名及正式宿主版本范围判定均未实现。待审核资料不被描述为可信或已上架。

生成 `.agents/skills/sinan-plugin-dev/SKILL.md`，指导 agent 遵循 API 1、UI 桥 2、私有数据/主题/资源边界与开发和发布流程；该 skill 不赋予远程发布授权。

## 验证

- `make test`、`go vet ./...` 和 `make build` 通过；最后脚本调整另完成模板定向竞态回归及 Node 语法检查。
- 两种生成项目通过独立构建、ZIP 打包、宿主临时安装与隔离 UI 会话准备，未打开窗口；原生后端真实调用通过。
- 开发链接重复构建、已有目录不覆盖、发现/重载、原生默认拒绝、显式指纹信任、目标变更拒绝及重启后不继承信任均通过；卸载保留外部产物，缓存仍在宿主状态目录。
- GitHub 工作流通过 actionlint v1.7.12；生成项目 skill 通过 skill-creator quick_validate（临时 uv 环境提供 PyYAML，不改项目依赖）。
- Release 和目录 PR 脚本用本地 gh 替身离线执行，验证产物摘要、PR 复用及生成记录被目录校验器接受，无远程请求。GitHub hosted runners 与跨仓授权未实跑。
- 真实目录仓库校验通过：1 个插件资料、0 个待审核发布、0 个可安装版本。相关仓库 `git diff --check` 通过。
- 未运行真实桌面、原生 WebView 或浏览器交互验证，也未推送标签、创建 Release 或远程 PR。

## 参考

- [GitHub 自动令牌授权](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication)
- [GitHub Pull requests API](https://docs.github.com/en/rest/pulls/pulls#create-a-pull-request)


## 后续调整：使用项目 AGENTS.md

按用户后续要求，开发指引已合并为模板项目根目录 `AGENTS.md`，保留原有协议、主面板/React、开发链接及发布约束，移除 skill 的 YAML 元数据；不再生成 `.agents/skills/sinan-plugin-dev/`。前文 skill 及其校验记录用于追溯当时实现。该调整不改变运行时、权限或发布流程，不迁移已生成的用户项目。

本次调整的模板定向竞态回归与 `git diff --check` 通过。`make build` 的前端、桌面宿主和 CLI 构建通过，UI helper 在未由本次修改的 `native_darwin.m:139` 遇到 `colorWithDynamicProvider:` 编译错误；没有为此覆盖现有原生 UI 代码。
