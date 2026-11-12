# 插件商店首版

`system.plugin-store` 是随宿主构建的内置插件，负责目录、搜索、详情、安装与手动版本更新。在主窗口打开，沿用插件管理的可信页头、返回和拆出操作。可以搜索“插件商店”或在插件管理中点击“插件商店”。它可禁用、卸载，不增加新的系统保护项。

## 本地使用

`make dev` 会构建并准备四个内置插件（应用检索仅 macOS）。已有日常插件目录可在插件管理中选择本仓库的 `plugins/system-plugin-store`，检查后安装。需要使用本次构建的宿主；只复制 manifest 不会增加旧宿主中不存在的可信组件。

当前远端目录只有 JSON 工具资料，没有已发布资产，所以显示“待发布”。不会用源码版本号生成伪安装包，也不会将本地源码目录当作已发布版本。

## 目录与发行契约

首版目录固定为 `zhyipeng/sinan-plugin-registry`，分支 `main`。商店使用公开 GitHub API 获取目录树，随后按树中固定 blob SHA 读取 `plugins/<plugin-id>/plugin.json`。`submissions`、任意外部索引和用户指定 URL 不作为安装来源。不调用 gh、不读取本机 GitHub 凭据。

目录资料字段为 schemaVersion（1）、id、name、summary、publisher.kind（official/community）、source（repository/path）、status 和 releases。状态支持 pending-release、published、withdrawn。仅 published 中的正式版本允许安装，pending-release 必须无版本。

正式版本记录包含：

| 字段 | 约束 |
| --- | --- |
| schemaVersion、pluginId、name | 记录版本 1，插件 ID 与资料一致，包名称与该发布记录一致 |
| version、minHostVersion | 三段数值版本；当前宿主版本 0.1.0；不支持预发布标签 |
| repository、tag、commit、releaseId | owner/repo，v 加版本，40 位源码提交摘要，正整数 Release ID |
| compatibility | apiVersion 为 1；uiBridgeVersion 为 1 或 2 |
| assets | 当前 OS/架构匹配的发行资产，至少一个、最多八个 |

每项资产包含 os（darwin/linux/windows）、arch（arm64/amd64）、file、sha256、size、assetId、downloadUrl。文件名为 `<id>-<version>-<os>-<arch>.zip`，URL 必须对应记录中的仓库、tag 和文件名，源码地址也须与发行仓库一致。实际下载使用固定 Asset ID 的公开 API；摘要、大小必须匹配。

目录仓库的校验器支持相同字段。模板 submissions 可继续沿用旧字段，审核者纳入正式 releases 时补齐 minHostVersion。维护者应核查真实资产与源码，不应仅凭脚本校验合格就发布。版本记录中的 commit、Release ID 是审核资料；宿主不会据此证明二进制可复现或自动完成源码审核。

## 安装与更新

1. 本地搜索已载入的名称、ID 与摘要，选择插件及版本。
2. “检查安装 / 检查更新”强制核验最新目录。目录修订变化时要求刷新，离线缓存不能安装。
3. 下载并验证归档，受限解压至私有缓存；复用内核包预览，核对 ID、名称和版本。
4. 展示版本变化，原生插件须明确勾选信任。确认时再次读取最新目录，拒绝已撤回或替换的资产，然后用包指纹和已安装包指纹应用。
5. 更新保留既有配置、数据与启用状态；运行中视图按现有升级机制撤销。失败回退复用内核行为。

这是显式选择版本的手动更新，也允许确认降级。没有自动更新、后台下载或根据相同 ID 自动授予信任。开发链接、本地包已变更及受保护插件不能被商店覆盖。目录下架仅阻止后续安装，不自动卸载或停用本地插件。

## 资源和信任边界

目录缓存位于 `os.UserCacheDir()/Sinan/plugin-store`，正常读取五分钟内复用缓存，手动刷新使用条件请求。联网失败且已有缓存时可继续浏览，界面标明缓存状态。GitHub 限流时显示错误并保留已有资料。

目录最多 128 项，每份资料最多 128 KiB、合计最多 2 MiB；版本最多 64 个。包下载及解压内容各限 64 MiB、最多 2048 个 ZIP 条目，拒绝路径越界、符号链接、特殊文件和重复路径。下载仅允许 GitHub 及固定发行资产域名的 HTTPS 重定向。临时预览十分钟有效、单次使用，每服务最多四个待处理操作；取消、失效后的下一次清理与正常退出会释放暂存目录。进程被强制终止可能遗留缓存，未实现跨进程自动清扫。

首版实现 GitHub HTTPS、固定目录审核、资产 SHA-256 与本地指纹检查，尚无独立目录签名、签名密钥轮换及完整撤销体系。哈希不能替代作者签名，原生进程以用户权限运行，不能宣称操作系统沙箱。架构基线中面向公共第三方分发的完整签名及治理要求仍待完成。

验证采用无界面的 HTTP/ZIP 夹具、真实内核安装与更新测试，以及构建和竞态检查。日常验证不启动真实桌面或浏览器，不据此声明焦点、输入法和视觉体验已人工验收。
