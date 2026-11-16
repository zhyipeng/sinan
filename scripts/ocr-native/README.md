# 原生 OCR 验证程序

这是独立实验模块，不参与宿主构建，不是可安装插件。Go 负责有界 PNG 解码、白底透明合成和 JSON 输出；检测、几何裁切、识别及 CTC 解码由 RapidOCRLite C ABI 完成。运行时不使用 Python、OpenCV 或 ONNX Runtime。

固定上游为 [RapidOCRLite v5.0.0a6](https://github.com/RapidAI/RapidOCRLite/tree/63bef543c782a0d81db2af056e1dfba550c6dd87)，提交 `63bef543c782a0d81db2af056e1dfba550c6dd87`。仅接受 PP-OCRv6 检测与识别权重，不使用其他模型。

## 已验证范围

macOS arm64，Go 1.26.0、AppleClang 21、CMake 4.4.5、Ninja 1.13.2。CPU/NEON 4 线程；Vulkan、Metal、方向分类关闭。使用第一轮同一批合成图片和同一份 small 权重。

`strict-fp.cmake` 在上游编译选项后追加严格浮点选项，不编辑上游源文件。直接使用上游 `-ffast-math` 构建时，本机发现重复输出变化及漏行，必须保留该对照结论。MSVC 分支尚未执行，不代表 Windows 浮点问题已验证。

Windows/Linux 两种架构、macOS amd64 未构建或运行。它们需要各自的 C++ 库、匹配 C ABI 的链接工具链及对应包入口；不能把 macOS dylib 复制过去。Linux 引擎本身不依赖 X11/Wayland，但截图和剪贴板仍需单独适配。Windows 内置 RSS 读取尚未实现（返回 0），外部监督脚本可采样 RSS；本轮只使用 macOS 的高水位值。

## 构建复现

先按[第一轮报告](../../docs/research/ocr-validation.md)准备图片和 PP-OCRv6 small 权重。以下为 macOS 示例，所有依赖及产物放在仓库外。

```sh
git clone --depth 1 --branch v5.0.0a6 https://github.com/RapidAI/RapidOCRLite.git /tmp/sinan-ocr-native-repro/source
uv venv /tmp/sinan-ocr-native-repro/venv --python 3.12
uv pip install --python /tmp/sinan-ocr-native-repro/venv/bin/python cmake==4.4.5 ninja==1.13.2
/tmp/sinan-ocr-native-repro/venv/bin/cmake -S /tmp/sinan-ocr-native-repro/source -B /tmp/sinan-ocr-native-repro/build -G Ninja -DCMAKE_MAKE_PROGRAM=/tmp/sinan-ocr-native-repro/venv/bin/ninja -DCMAKE_BUILD_TYPE=Release -DPPOCR_BUILD_STATIC=OFF -DPPOCR_BUILD_SHARED=ON -DPPOCR_BUILD_EXAMPLES=OFF -DPPOCR_BUILD_TESTS=OFF -DPPOCR_ENABLE_VULKAN=OFF -DPPOCR_ENABLE_METAL=OFF -DCMAKE_PROJECT_INCLUDE="$PWD/scripts/ocr-native/strict-fp.cmake"
/tmp/sinan-ocr-native-repro/venv/bin/cmake --build /tmp/sinan-ocr-native-repro/build --parallel 4
```

然后在仓库根目录运行以下 Python 构建辅助代码。它仅调用 Go 编译器并复制原生库，不参与图片识别：

```python
from pathlib import Path
import os, shutil, subprocess

root = Path('/tmp/sinan-ocr-native-repro')
package = root / 'package'
(package / 'backend').mkdir(parents=True, exist_ok=True)
(package / 'lib').mkdir(exist_ok=True)
shutil.copy2(root / 'build/libppocr.5.0.0.dylib', package / 'lib/libppocr.5.dylib')
env = os.environ.copy()
env.update({
    'CGO_ENABLED': '1',
    'CGO_CFLAGS': f'-I{root}/source/include',
    'CGO_LDFLAGS': f'-L{root}/build -lppocr -Wl,-rpath,@executable_path/../lib',
})
subprocess.run(['go', 'build', '-trimpath', '-ldflags=-s -w',
                '-o', str(package / 'backend/ocr'), '.'],
               cwd='scripts/ocr-native', env=env, check=True)
subprocess.run(['go', 'vet', '.'], cwd='scripts/ocr-native', env=env, check=True)
```

识别字典从同一份识别模型的 `character` 元数据导出，不可拿其他版本字典替代。以下代码在第一轮准备的、装有 ONNX Runtime 的 Python 环境执行一次；原生程序运行时不需要它：

```python
from pathlib import Path
import onnxruntime as ort

model = '/tmp/sinan-ocr-repro/v6-small/models/PP-OCRv6_rec_small.onnx'
session = ort.InferenceSession(model, providers=['CPUExecutionProvider'])
Path('/tmp/sinan-ocr-native-repro/dictionary.txt').write_text(
    session.get_modelmeta().custom_metadata_map['character'])
```

将程序、原生库、两份权重、字典及许可/归属资料组合后，须检查完整目录和 ZIP 都小于 64 MiB。本轮测量的是独立识别运行目录，尚未包含插件 manifest、stdio RPC、SDK 和业务 UI，不可当成已完成的插件发行验证。

## 无界面验证

在仓库根目录运行，按自己的第一轮输出修改路径：

```sh
uv run --python 3.12 --locked --script scripts/ocr-native/validate.py --binary /tmp/sinan-ocr-native-repro/package/backend/ocr --corpus /tmp/sinan-ocr-repro/v6-small/corpus.json --det /tmp/sinan-ocr-repro/v6-small/models/PP-OCRv6_det_small.onnx --rec /tmp/sinan-ocr-repro/v6-small/models/PP-OCRv6_rec_small.onnx --dictionary /tmp/sinan-ocr-native-repro/dictionary.txt --baseline docs/research/ocr-validation-results.json --out /tmp/sinan-ocr-native-repro/results.json --sandbox --stress-repeats 100
```

Python 只监督原生子进程、采样 RSS 和计算文字误差。程序传入明确的环境白名单；识别进程没有网络请求或下载行为，`--sandbox` 进一步用 macOS 系统机制禁网。该测试机制不是插件安全沙箱。

数据包含重复输出稳定性、内存高水位、销毁引擎并回收 Go 内存后的 RSS、连续识别、工作进程强制取消并回收，以及无效 PNG/超像素上限拒绝。稳定性失败会保存在 `all_cases_stable` 和各用例的 `variant_samples` 中，不把测量成功当成候选引擎已通过质量门槛。

详细结果及限制见[原生验证报告](../../docs/research/ocr-native-validation.md)。本次仅实验，无宿主、SDK、清单格式或商店资产变更。
