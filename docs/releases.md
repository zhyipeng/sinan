# 自动构建与 GitHub Release

## 触发方式

普通分支 push、Pull Request 或手动运行 `Desktop platform builds`，会执行六目标原生构建、无界面检查、便携打包并保存 Actions artifact。PR 不发布 Release，也不获得仓库写权限。

发布前提交并推送需要发行的代码，然后推送版本标签，例如：

```sh
git tag -a v0.1.0 -m "Sinan v0.1.0"
git push origin v0.1.0
```

标签使用 `v<major>.<minor>.<patch>`，预发布例如 `v0.1.0-rc.1`。不接受前导零和构建元数据。版本来自标签；macOS Bundle 的两个版本字段使用不含预发布后缀的数字版本，完整版本保留在归档文件名中。不会自动提交版本号或创建标签。

`Desktop release` 等待所有构建与检查成功，再核对六个归档和各自 SHA-256。发布任务先建立草稿、上传六个归档与 `SHA256SUMS.txt`，确认远端资产数量和大小后公开；预发布标签标记为 Pre-release，并且不设置为 Latest。上传中断保留草稿，可重跑失败任务继续。已公开版本拒绝覆盖，需要修复时使用新标签。手动运行发布工作流必须选择已有 `v*` 标签，选择分支不会发布。

流程仅使用自动提供的 `GITHUB_TOKEN`，不需要个人访问令牌；只有发布 job 获得 `contents: write`。仓库或组织的 Actions 策略仍须允许工作流创建 Release。新增配置在实际推送后才会生效，本地检查不代表远端运行成功。

## 平台与产物

| 平台 | 架构 | 原生 runner | Release 归档 |
| --- | --- | --- | --- |
| macOS | arm64 / amd64 | macos-15 / macos-15-intel | `Sinan-<版本>-macos-<架构>.tar.gz` |
| Windows | amd64 / arm64 | windows-2022 / windows-11-arm | `Sinan-<版本>-windows-<架构>.zip` |
| Linux | amd64 / arm64 | ubuntu-24.04 / ubuntu-24.04-arm | `Sinan-<版本>-linux-<架构>.tar.gz` |

runner 平台和架构必须与归档目标一致，不能用交叉编译冒充原生验证。macOS/Linux 执行当前全量无界面回归；Windows 继续采用已迁移的平台、原生 COM 边界、首次安装及前端测试，历史 POSIX 夹具尚未全部迁移。

macOS 解压后将 `Sinan.app` 移至 Applications。应用包含桌面入口、CLI、原生 UI helper；归档前执行 ad-hoc 签名及验证，但没有 Developer ID 签名、公证或 DMG 安装器，Gatekeeper 可能阻止打开。

Windows 解压整个目录后运行 `sinan.exe`，需要已安装 WebView2 Runtime 119+。无安装器、Authenticode 签名或自动桌面注册。

Linux 解压整个目录后运行 `sinan`，以 Ubuntu 24.04 为基线，运行依赖为 GTK3、WebKitGTK 4.1（引擎至少 2.40）、GIO；例如 `sudo apt-get install libgtk-3-0t64 libwebkit2gtk-4.1-0 libglib2.0-bin`。可选 X11 自动粘贴需 xdotool；Wayland 不支持该自动输入。无 AppImage/deb/rpm，不打包系统库。

这些是便携发行资产，不代表完整安装、签名、升级或真实 UI 验收。替换应用前退出旧实例，用户配置和插件数据仍在系统用户配置目录，不随应用目录替换。

## 内置插件

统一 Node 构建入口先生成三个内置插件的最小发行快照：清单、图标，以及目标系统的应用检索后端。只将需要运行的文件写入 `apps/desktop/bundled/`，再通过 Go embed 编入桌面宿主；不把源码、开发链接或 `.sinan-dev` 放进包内。Windows 后端入口为 `backend/plugin.exe`。

正常启动使用默认用户目录时，宿主通过既有内核安装 API 安装缺失的应用检索、设置和插件中心，并信任随宿主内嵌的应用检索原生后端。已有插件不自动替换；损坏目录也不覆盖，失败通过既有恢复提示呈现。内置插件后续升级仍沿用已有显式安装/恢复流程及历史迁移规则。`-plugins`、UI 探测和临时验收实例不执行发行初始化。

裸 `go build` 不生成发行快照，正式打包应使用统一入口；生成目录被 Git 忽略，勿手工维护。

## 本地无界面验证

```sh
node --test scripts/release.test.mjs
node scripts/desktop.mjs test
```

在对应平台本机生成发行归档（以下环境变量设置适用于 POSIX shell；PowerShell 使用 `$env:RELEASE_TAG = 'v0.1.0-rc.1'`）：

```sh
RELEASE_TAG=v0.1.0-rc.1 node scripts/desktop.mjs package
RELEASE_TAG=v0.1.0-rc.1 node scripts/release.mjs archive
```

归档位于 `dist/release/`。以上命令不会启动应用、原生 WebView 或浏览器。真正上传需要发布工作流；不要在未准备完整六目标归档时本地执行 `publish`。

GitHub 的 [runner 文档](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)说明目标标签与架构；草稿、标签核验和自动发行说明使用 [GitHub CLI Release 接口](https://cli.github.com/manual/gh_release_create)。

## 后续接入的宿主自动更新

发行包现额外携带 `sinan-updater`。`RELEASE_TAG` 对应完整版本在构建时写入宿主和助手，包内 `release.json` 记录版本、平台及架构，归档前核对一致；macOS 打包禁止附带 AppleDouble 扩展属性旁路文件，以便受限解压器统一验证根目录。

包含该机制的完整发行版启动后检查稳定新版，在用户点击下载并重启安装后消费本流程发布的对应资产和 `SHA256SUMS.txt`。旧版无助手构建仍需手动安装一次。功能、备份、签名与实机验证边界见[自动更新](auto-update.md)。

## v0.2.1

本版增加宿主共享系统剪贴板 API（ADR 0066），配套计算、剪贴板插件 0.2.0；新插件要求司南 0.2.1 与初始化 `clipboard.v1` 能力。[六目标原生构建与无界面检查](https://github.com/zhyipeng/sinan/actions/runs/43806863705) 已通过，[v0.2.1 Release](https://github.com/zhyipeng/sinan/releases/tag/v0.2.1) 已公开；六个归档与 SHA256SUMS 的 GitHub SHA-256 已核验，实机验收仍未执行。macOS 仍为 ad-hoc 签名，Windows/Linux 仍为便携目录，系统依赖和真实桌面验收限制不变。

初始 v0.2.0 标签的发布工作流因商店宿主版本写死为 0.1.0 而取消，未公开 Release；保留标签，不改写历史。v0.2.1 将商店兼容性与展示版本统一取自构建注入的宿主版本，使用 SemVer 比较最低版本（预发行版不能冒充同号正式版），开发构建显式使用当前能力基线。插件中心版本同步为 0.9.2，未增加协议字段。


## v0.3.0 发布准备

新增 `ui.image-command.v1`：会话 PNG 快照交付授权插件后台，配套 OCR 0.1.0 的主输入框图片匹配、原图预览及自动识别。OCR 最低宿主为 0.3.0，已发布 0.2.1 不兼容；源码开发基线和前端版本同步为 0.3.0，插件中心为 0.9.3。宿主仍执行六目标原生构建；OCR 首发资产仅为已验证的 macOS arm64，不增加其他平台的 OCR 支持声明。发布前通过既有无界面检查，真实 UI/系统剪贴板仍未验收，远端发行状态待工作流确认。
