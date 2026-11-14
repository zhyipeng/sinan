# 桌面自动更新

完整发行版在正常启动后三秒自动检查一次 GitHub 稳定版本，发现新版后在搜索栏显示「有更新」。点击进入设置，已有设置之后的「司南更新」显示当前版本、检查状态和「下载并重启安装」；也可点击「检查更新」重试。后台检查不主动呼出窗口，不自动下载。

点击安装会下载当前系统/架构的发行归档，校验长度和 SHA-256，并检查解压后的版本和平台。准备失败时当前应用继续运行；准备成功后自动退出、替换并重启。新进程未能在 30 秒内到达启动事件时尝试回退旧版。更新不安装或更新官方第三方插件，也不自动覆盖已有内置插件。

## 运行条件

- 必须使用包含 `sinan-updater` 和有效 `release.json` 的完整发行包。`RELEASE_TAG` 在构建时写入可执行文件身份，必须与包元数据一致。开发构建及 `-plugins` 实例不参与自动更新。
- 当前只检查 `zhyipeng/sinan` 的 latest 稳定发行，不接收预发布渠道，不降级。预发布版运行时可升级到版本号更高的稳定版。
- macOS 应先将 `.app` 移到可写的应用目录；Windows/Linux 应先完整解压便携目录。安装目录的父目录必须可写，不自动请求管理员权限。系统库和 WebView 运行依赖仍由系统提供。
- 首个包含更新功能的发行版须手动安装一次。旧版本缺少更新助手时不会被自动改造。
- 使用官方 GitHub HTTPS 和已限定的发行下载域名，不使用插件中心的镜像配置，不需要个人访问令牌。GitHub API 限额或网络失败时可稍后手动重试。

macOS 仍为 ad-hoc 签名，没有 Developer ID 签名、公证；更新会检查 Bundle 签名，但不会绕过 Gatekeeper。Windows 尚无 Authenticode。SHA-256 防止下载损坏或资产不匹配，摘要与包来自同一发布源，未建立独立签名信任链。

## 失败与恢复

旧版完整目录保留在安装目录同级的 `.sinan-update-<随机数>/previous`。确认新版可用后，可以手动删除对应暂存目录；当前不自动清理备份。用户配置与插件数据仍保留在系统用户配置目录。

普通文件替换失败会尝试恢复旧目录；新版未回报就绪时会终止新版、尝试回退并重启旧版。就绪回执只表示应用到达启动事件，不保证所有功能正常。最近一次安装结果保存在系统用户缓存目录 `Sinan/updates/last-result.json`，设置页可读取。

断电、强制结束更新助手或磁盘故障不属于完整事务恢复保证。如果应用目录缺失，请先确认所有实例已退出，再将同级暂存目录中的 `previous` 移回原应用位置；不要覆盖仍在运行或已修改的目录。助手计划和回执位于同一缓存目录下的 `install-*` 私有会话，回退包可能包含旧版完整应用，需保留到确认新版工作正常。

## 无界面验证

```sh
node scripts/desktop.mjs test
node --test scripts/release.test.mjs
```

测试使用虚拟 HTTP 响应、临时应用目录和模拟启动回执，覆盖版本顺序、下载来源、摘要、归档边界、目录替换、保留旧版和回退，不启动真实桌面或浏览器。真实发行归档另有显式的仅解压/签名验证入口：

```sh
RELEASE_TAG=v0.2.0 node scripts/desktop.mjs package
RELEASE_TAG=v0.2.0 node scripts/release.mjs archive
SINAN_UPDATE_TEST_ARCHIVE="$PWD/dist/release/Sinan-0.2.0-macos-arm64.tar.gz" go test ./internal/hostupdate -run TestPackagedArchiveWhenProvided -count=1
```

上例用于 macOS arm64；该验证夹具的候选版本固定为 0.2.0，只写入临时目录，不执行应用替换或启动。其他 OS/架构使用对应归档名。真实宿主退出、进程占用、更新重启、系统策略及多平台 UI 仍需专门实机验收。

具体边界见 [ADR 0060](adr/0060-host-auto-update.md)，发行资产与依赖见[发布流程](releases.md)。GitHub 稳定发行查询使用 [latest release API](https://docs.github.com/en/rest/releases/releases#get-the-latest-release)。

Windows/Linux 便携目录只允许本版发行的四个程序和 `release.json`。若目录混入其他文件，自动更新会拒绝整目录替换；不要将包内容与其他应用或个人数据混放在同一个目录。
