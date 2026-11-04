# Sinan

基于 Wails 3 的插件驱动桌面效率平台，对标 uTools，采用 All-in-Plugin 架构。

应用检索、设置、插件管理等业务功能均通过插件提供，宿主负责插件运行所需的基础机制。

## 项目指导方案

- [架构指导方案](docs/architecture.md)：项目定位、边界、技术选型、实施路线与验收标准。
- [原始规划存档](docs/references/plugin-architecture-planning.md)：来源会话的规划正文，供追溯。
- [开发约定](AGENTS.md)：后续开发及 AI 协作需要遵守的规则。

当前处于架构规划阶段，尚未初始化应用代码。开发从最小内核与动态插件加载验证开始。
