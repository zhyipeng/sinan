# ADR 0071：OCR 六目标原生包与验证

- 日期：2026-11-17
- 状态：六目标原生验证通过；OCR 0.2.0 已公开发行并登记商店

## 决策

OCR 0.2.0 延续 All-in-Plugin。模型、推理、任务和私有 worker 保留在官方插件；不增加宿主接口、SDK、权限或 manifest 字段，最低宿主仍为 0.3.1。已发布的 OCR 0.1.0 macOS arm64 包保持不变。

完整包在 macOS、Windows、Linux 的 arm64/amd64 runner 原生构建并运行，不用后台交叉编译或重命名产物证明兼容。`ocr.yml` 仅手动触发原生验证与产物收集，不自动发布；普通插件标签发布流程跳过 OCR。另需手动运行 `ocr-publish.yml` 指定成功的原生验证 run ID，独立发布 job 只解析六份实际 ZIP、复核源提交和验证结果，不执行下载的程序，拒绝覆盖公开版本或不匹配的草稿资产。

## 原生适配

- 固定 RapidOCRLite `63bef543c782a0d81db2af056e1dfba550c6dd87` 和原有 PP-OCRv6 small 模型摘要。构建脚本检查原始源码干净，仅复制 Git 跟踪的 CPU 源码到构建私有目录；模型/字典流改用 `std::filesystem::u8path`，不改写原始 checkout。
- 保留 CPU 严格浮点及上游 SIMD 运行时分派，不启用 Metal/Vulkan。macOS 对齐宿主 12.0 构建目标；Linux 以 Ubuntu 24.04/glibc 2.39 为基线，系统 libstdc++ 不复制进包。
- macOS/Linux 的库放在 `lib/`，worker 以相对可执行文件的 RPATH 加载；不依赖 `PATH`、`DYLD_LIBRARY_PATH` 或 `LD_LIBRARY_PATH`。
- Windows 用固定 LLVM-MinGW 20261104 UCRT 工具链，通过 C ABI 与 Go/CGO 连接。按宿主架构选择已核对 SHA-256 的工具链包；原生 DLL 和实际导入的 libc++/libunwind/winpthreads 运行库来自对应目标目录，放在 worker 同目录，附带 LLVM/MinGW-w64 运行库许可。未声明的依赖明确失败，不从任意系统目录猜测复制。
- Windows 清单入口及后台/worker 产物使用 `.exe`；打包校验入口、worker 和原生库的实际 OS/架构与库类型。PNG 私有管道、父进程 EOF、任务租约、取消和输出预算保持 ADR 0067 的语义，不构成 OS 安全沙箱。

## 验证

各 runner 对实际 ZIP 做摘要、清单、执行位及架构检查，并解压到含中文/空格的临时目录。使用相同的 11 张合成 PNG 和原生基准验证完整 OCR、图片快照独立于系统剪贴板、单任务限制、取消与后台强杀后的 worker 回收。合成 PNG 不包含真实截图或用户数据；基准输出不等于完美识别答案。

macOS 额外在系统级禁网下执行；Windows/Linux 未执行 OS 级禁网时明确记录。源码实现、原生运行、无界面测试和正式发行分别记录。三端原生 WebView、真实剪贴板、授权、输入法与多屏仍不在本轮日常回归中，不以引擎成功替代 UI 验收。Linux X11/Wayland 的宿主剪贴板依赖不变。

## 影响

`sinan-official-plugins` 增加固定输入准备、六目标构建和重定位 ZIP 验证入口。`sinan-plugin-registry` 仅在存在已核验的公开资产后更新 0.2.0，不追加虚构平台，也不修改 0.1.0 的摘要或身份。宿主代码和协议无需联动变更。


## 2026-11-17 结果

[六目标原生验证](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44006353531) 全部通过，源码为 `80eeb9c070c8cb48c3fa61a807ff35b89f975445`；[发布 job](https://github.com/zhyipeng/sinan-official-plugins/actions/runs/44006913968) 复核后公开 [OCR 0.2.0](https://github.com/zhyipeng/sinan-official-plugins/releases/tag/tools.ocr-v0.2.0)。六包摘要与公开 SHA256SUMS/资产元数据一致，商店最低宿主仍为 0.3.1，旧版保留。

Windows 需同时以 GUI 子系统构建后台与 worker，并保留 CREATE_NO_WINDOW；仅隐藏控制台仍产生 conhost。打包器校验 PE32+ GUI 子系统，验证保留零残留子进程断言。跨 ISA 不强制复制基准自身的错误：终端图 ARM/x86 均可能漏一个点，位置不同；每图两次输出须稳定，源文本编辑距离和字符差均不得比既有基准更差，不增加额外误差预算。实际差异和发行身份见[核验数据](../research/ocr-cross-platform-validation-results.json)。
