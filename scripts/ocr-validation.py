# /// script
# requires-python = ">=3.12,<3.13"
# dependencies = ["rapidocr==3.9.2", "onnxruntime==1.32.0", "pillow==13.0.0", "psutil==7.2.2", "numpy==2.5.4"]
# ///
"""Offline engine experiment; no desktop, clipboard, or application integration.

Run prepare once with networking, then benchmark with networking denied by the OS.
The caller supplies a CJK font so non-macOS hosts can reproduce the same corpus.
"""
import argparse
import hashlib
import importlib.metadata
import json
import platform
import socket
import statistics
import sys
import threading
import time
from pathlib import Path


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def normalize(text):
    # Ignore whitespace only: retain case, punctuation, digits and character width.
    return "".join(text.split())


def distance(a, b):
    row = list(range(len(b) + 1))
    for i, ca in enumerate(a, 1):
        nxt = [i]
        for j, cb in enumerate(b, 1):
            nxt.append(min(nxt[-1] + 1, row[j] + 1, row[j - 1] + (ca != cb)))
        row = nxt
    return row[-1]


def fixtures(out, font_path):
    from PIL import Image, ImageDraw, ImageFont

    root = out / "images"
    root.mkdir(exist_ok=True)
    corpus = []

    def add(name, lines, size=26, bg="#ffffff", fg="#202020", width=1100,
            columns=None, table=False, rotate=0, transparent=False):
        font = ImageFont.truetype(str(font_path), size)
        rows = len(lines) if columns is None else len(columns[0])
        step = size + 22
        height = max(180, 64 + rows * step)
        image = Image.new("RGB", (width, height), bg)
        draw = ImageDraw.Draw(image)
        if columns:
            for col, values in enumerate(columns):
                for row, text in enumerate(values):
                    draw.text((28 + col * width // 2, 26 + row * step), text,
                              font=font, fill=fg)
        elif table:
            for row, text in enumerate(lines):
                for col, cell in enumerate(text.split()):
                    draw.text((28 + col * 250, 26 + row * step), cell,
                              font=font, fill=fg)
        else:
            for row, text in enumerate(lines):
                draw.text((28, 26 + row * step), text, font=font, fill=fg)
        if table:
            for row in range(rows + 1):
                y = 20 + row * step
                draw.line((18, y, width - 18, y), fill="#b0b0b0", width=1)
            for x in (18, 250, 500, width - 18):
                draw.line((x, 20, x, 20 + rows * step), fill="#b0b0b0", width=1)
        if rotate:
            image = image.rotate(rotate, resample=Image.Resampling.BICUBIC,
                                 expand=True, fillcolor=bg)
        if transparent:
            # Fully transparent white background; visible pixels retain original RGB.
            image = image.convert("RGBA")
            image.putalpha(image.convert("L").point(lambda v: 0 if v == 255 else 255))
        image.save(root / (name + ".png"))
        corpus.append({"name": name, "expected": "\n".join(lines),
                       "size": image.size, "font_px": size})

    basic = ["司南 OCR 离线文字识别", "支持中文 English 混排与数字 2026",
             "订单编号 SN-20261116，金额 1234.56 元。", "复制结果后，可以继续编辑。"]
    add("light", basic)
    add("dark", basic, bg="#181a20", fg="#e5e7eb")
    add("small_12px", basic, size=12, width=720)
    add("low_contrast", basic, bg="#f0f0f0", fg="#a0a0a0")
    add("rotated_3deg", basic, rotate=3)
    add("transparent", basic, transparent=True)
    add("terminal", ["构建完成 Build completed", "go test ./...",
                     'const total = price * 1.08;', 'if (count <= 10) { return "OK"; }',
                     "path: /tmp/sinan-ocr/report.json"], size=20,
        bg="#15171b", fg="#e3e8ef")
    add("dense", [f"第{i:02d}行：离线识别测试 OCR sample {i:02d}, total = {i * 12}.50"
                  for i in range(1, 25)], size=18)
    left = ["左栏：项目说明", "第一步准备图片", "第二步识别文字", "第三步复制结果"]
    right = ["右栏：处理状态", "任务已经完成", "图片不会上传", "退出清理内存"]
    add("two_columns", left + right, columns=[left, right], size=24)
    # The table evaluates row-major plain text, not structural table extraction.
    add("table", ["项目               数量               金额",
                  "键盘               2                  399.00",
                  "鼠标               3                  129.50",
                  "合计               5                  1186.50"], size=24,
        table=True, width=960)
    add("blank", [])
    write_json(out / "corpus.json", corpus)
    # Contact sheet is for inspecting synthetic fixtures, not desktop UI testing.
    sheet = Image.new("RGB", (660, 240 * len(corpus)), "#dddddd")
    for i, item in enumerate(corpus):
        img = Image.open(root / (item["name"] + ".png")).convert("RGBA")
        canvas = Image.new("RGBA", img.size, "white")
        canvas.alpha_composite(img)
        canvas.thumbnail((640, 205))
        sheet.paste(canvas.convert("RGB"), (10, i * 240 + 25))
        ImageDraw.Draw(sheet).text((10, i * 240 + 5), item["name"], fill="black")
    sheet.save(out / "contact-sheet.png")


def params(profile, out, rec_batch=6):
    common = {"Global.log_level": "error", "Global.use_cls": False,
              "Global.model_root_dir": str(out / "models"),
              "EngineConfig.onnxruntime.intra_op_num_threads": 4,
              "EngineConfig.onnxruntime.inter_op_num_threads": 1,
              "Det.limit_type": "max", "Det.limit_side_len": 960,
              "Rec.rec_batch_num": rec_batch}
    if profile == "v5-mobile":
        for stage in ("Det", "Rec"):
            common.update({stage + ".ocr_version": "PP-OCRv5",
                           stage + ".model_type": "mobile"})
    elif profile == "v6-tiny":
        for stage in ("Det", "Rec"):
            common[stage + ".model_type"] = "tiny"
    return common


def deny_network():
    def deny(*args, **kwargs):
        raise RuntimeError("OCR validation: network disabled")
    socket.socket.connect = deny
    socket.socket.connect_ex = deny
    socket.create_connection = deny
    socket.getaddrinfo = deny


def create_engine(profile, out, rec_batch=6):
    from rapidocr import ModelType, OCRVersion, RapidOCR
    config = params(profile, out, rec_batch)
    for key, value in list(config.items()):
        if key.endswith(".ocr_version"):
            config[key] = OCRVersion(value)
        elif key.endswith(".model_type"):
            config[key] = ModelType(value)
    return RapidOCR(params=config)


def benchmark(profile, out, repeats, rec_batch):
    import psutil

    deny_network()
    process = psutil.Process()
    baseline = process.memory_info().rss
    samples = [baseline]
    stop = threading.Event()

    def sample_memory():
        while not stop.wait(0.01):
            samples.append(process.memory_info().rss)

    sampler = threading.Thread(target=sample_memory, daemon=True)
    sampler.start()
    started = time.perf_counter()
    from rapidocr import RapidOCR
    import_ms = (time.perf_counter() - started) * 1000
    started = time.perf_counter()
    engine = create_engine(profile, out, rec_batch)
    init_ms = (time.perf_counter() - started) * 1000
    providers = {name: stage.session.session.get_providers() for name, stage in
                 (("det", engine.text_det), ("rec", engine.text_rec))}
    corpus = json.loads((out / "corpus.json").read_text())
    started = time.perf_counter()
    engine(str(out / "images" / "light.png"))
    first_ms = (time.perf_counter() - started) * 1000
    rows = []
    for case in corpus:
        times, outputs = [], []
        result = None
        for _ in range(repeats):
            started = time.perf_counter()
            result = engine(str(out / "images" / (case["name"] + ".png")))
            times.append((time.perf_counter() - started) * 1000)
            outputs.append("\n".join(result.txts) if result.txts is not None else "")
        text = outputs[0]
        expected, actual = normalize(case["expected"]), normalize(text)
        errors = distance(expected, actual)
        rows.append({**case, "actual": text, "edit_distance": errors,
                     "reference_chars": len(expected),
                     "cer": errors / len(expected) if expected else None,
                     "exact": expected == actual, "stable": len(set(outputs)) == 1,
                     "warm_ms": times, "median_ms": statistics.median(times),
                     "scores": [float(v) for v in result.scores] if result.scores is not None else [],
                     "boxes": result.boxes.tolist() if result.boxes is not None else []})
        print(f"{profile} {case['name']}: {errors}/{len(expected)} edits, "
              f"{statistics.median(times):.1f} ms", flush=True)
    stop.set()
    sampler.join()
    peak_bytes = max(samples)
    peak_method = "RSS sampled every 10 ms"
    if sys.platform != "win32":
        import resource
        peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
        peak_bytes = peak if sys.platform == "darwin" else peak * 1024
        peak_method = "getrusage high-water RSS"
    files = sorted((out / "models").glob("*.onnx"))
    report = {"profile": profile, "params": params(profile, out, rec_batch),
              "platform": {"system": platform.system(), "release": platform.release(),
                           "machine": platform.machine(), "python": platform.python_version()},
              "versions": {name: importlib.metadata.version(name) for name in
                           ("rapidocr", "onnxruntime", "opencv-python", "numpy", "pillow")},
              "python_network_denied": True, "repeats": repeats,
              "providers": providers,
              "import_ms": import_ms, "init_ms": init_ms, "first_call_ms": first_ms,
              "cold_import_init_first_ms": import_ms + init_ms + first_ms,
              "baseline_rss_bytes": baseline, "peak_rss_bytes": peak_bytes,
              "peak_method": peak_method,
              "final_rss_bytes": process.memory_info().rss,
              "models": [{"name": f.name, "bytes": f.stat().st_size,
                          "sha256": hashlib.sha256(f.read_bytes()).hexdigest()} for f in files],
              "cases": rows}
    write_json(out / (profile + f"-batch{rec_batch}.json"), report)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=["prepare", "benchmark"])
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--font", type=Path)
    parser.add_argument("--profile", choices=["v5-mobile", "v6-small", "v6-tiny"], default="v5-mobile")
    parser.add_argument("--repeats", type=int, default=5)
    parser.add_argument("--rec-batch", type=int, choices=[1, 6], default=6)
    args = parser.parse_args()
    args.out = args.out.resolve()
    args.out.mkdir(parents=True, exist_ok=True)
    if args.repeats < 1:
        parser.error("repeats must be positive")
    if args.mode == "prepare":
        if not args.font or not args.font.is_file():
            parser.error("prepare requires an existing CJK font")
        fixtures(args.out, args.font)
        create_engine(args.profile, args.out, args.rec_batch)
        print("Prepared synthetic fixtures and local models.")
    else:
        benchmark(args.profile, args.out, args.repeats, args.rec_batch)


if __name__ == "__main__":
    main()
