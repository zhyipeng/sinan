# /// script
# requires-python = ">=3.12,<3.13"
# dependencies = ["psutil==7.2.2"]
# ///
"""Supervise a standalone native OCR process; Python does not run inference."""
import argparse
import json
import os
from pathlib import Path
import runpy
import signal
import subprocess
import sys
import tempfile
import threading
import time
import struct
import zlib

import psutil


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("binary", "corpus", "det", "rec", "dictionary", "out", "baseline"):
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--sandbox", action="store_true")
    parser.add_argument("--stress-repeats", type=int, default=100)
    parser.add_argument("--threads", type=int, choices=[1, 4], default=4)
    args = parser.parse_args()
    if not 1 <= args.stress_repeats <= 1000:
        parser.error("stress-repeats must be 1..1000")
    if args.sandbox and sys.platform != "darwin":
        parser.error("this OS network-denial wrapper is available only on macOS")
    args.out.parent.mkdir(parents=True, exist_ok=True)
    prefix = ["/usr/bin/sandbox-exec", "-p", "(version 1)(allow default)(deny network*)"] if args.sandbox else []
    command = prefix + [str(args.binary.resolve()), "--det", str(args.det.resolve()),
                        "--rec", str(args.rec.resolve()), "--dict", str(args.dictionary.resolve()),
                        "--corpus", str(args.corpus.resolve())]
    # Native code receives a small, explicit environment, never account credentials.
    env = {"PATH": os.defpath, "PPOCR_BENCH_THREADS": str(args.threads), "LANG": "C", "LC_ALL": "C"}

    def launch(extra, timeout=120):
        started = time.perf_counter()
        p = subprocess.Popen(command + extra, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             env=env, start_new_session=os.name != "nt")
        samples = []
        stop = threading.Event()
        max_children = [0]

        def sample():
            while not stop.wait(0.02):
                try:
                    process = psutil.Process(p.pid)
                    samples.append(process.memory_info().rss)
                    max_children[0] = max(max_children[0], len(process.children(recursive=True)))
                except psutil.Error:
                    pass

        worker = threading.Thread(target=sample, daemon=True)
        worker.start()
        try:
            stdout, stderr = p.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            p.kill()
            p.communicate()
            raise
        finally:
            stop.set()
            worker.join()
        return {"returncode": p.returncode, "stdout": stdout.decode(), "stderr": stderr.decode(),
                "wall_ms": (time.perf_counter() - started) * 1000,
                "sampled_peak_rss_bytes": max(samples, default=0),
                "max_child_processes": max_children[0], "pid_reaped": not psutil.pid_exists(p.pid)}

    measurement = launch([])
    assert measurement["returncode"] == 0, measurement["stderr"]
    assert measurement["pid_reaped"]
    report = json.loads(measurement.pop("stdout"))
    report["supervision"] = measurement
    assert report["native_threads"] == args.threads and report["backend"] == "CPU"
    metrics = runpy.run_path(str(Path(__file__).resolve().parents[1] / "ocr-validation.py"))
    normalize, distance = metrics["normalize"], metrics["distance"]
    original = json.loads(args.baseline.read_text())
    baseline = next(p for p in original["profiles"] if p["profile"] == "v6-small")
    prior_cases = {c["name"]: c for c in baseline["cases"]}
    for case in report["cases"]:
        expected, actual = normalize(case["expected"]), normalize(case["actual"])
        case["edit_distance"] = distance(expected, actual)
        case["reference_chars"] = len(expected)
        case["matches_ort_ignoring_whitespace"] = actual == normalize(prior_cases[case["name"]]["actual"])
    report["all_cases_stable"] = all(case["stable"] for case in report["cases"])
    args.out.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    assert next(c for c in report["cases"] if c["name"] == "blank")["actual"] == ""
    print("Native corpus completed; starting repeated-image memory check", flush=True)
    stress = launch(["--one", "dense", "--repeats", str(args.stress_repeats)], timeout=300)
    assert stress["returncode"] == 0 and stress["pid_reaped"], stress["stderr"]
    stress_data = json.loads(stress.pop("stdout"))
    report["stress"] = {**stress, "repeats": args.stress_repeats,
                        "stable": stress_data["cases"][0]["stable"],
                        "variant_samples": stress_data["cases"][0].get("variant_samples", []),
                        "peak_rss_bytes": stress_data["peak_rss_bytes"],
                        "rss_before_destroy_bytes": stress_data["rss_before_destroy_bytes"],
                        "rss_after_destroy_and_go_gc_bytes": stress_data["rss_after_destroy_and_go_gc_bytes"],
                        "first_ms": stress_data["cases"][0]["warm_ms"][0],
                        "last_ms": stress_data["cases"][0]["warm_ms"][-1]}
    # A deliberately long worker is force-stopped, then reaped. No UI is involved.
    p = subprocess.Popen(command + ["--one", "dense", "--repeats", "10000"],
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env,
                         start_new_session=os.name != "nt")
    time.sleep(0.3)
    assert p.poll() is None, "cancellation worker exited before termination"
    started = time.perf_counter()
    if os.name == "nt":
        p.kill()
    else:
        os.killpg(p.pid, signal.SIGKILL)
    p.communicate(timeout=5)
    report["cancellation"] = {"returncode": p.returncode,
                              "kill_to_reap_ms": (time.perf_counter() - started) * 1000,
                              "pid_reaped": not psutil.pid_exists(p.pid)}
    assert report["cancellation"]["pid_reaped"] and p.returncode != 0
    # Bad files use generated fixture directories; no real user files are opened.
    with tempfile.TemporaryDirectory(prefix="sinan-ocr-invalid-") as temporary:
        root = Path(temporary)
        (root / "images").mkdir()
        (root / "corpus.json").write_text('[{"name":"bad","expected":""}]')
        i = command.index("--corpus") + 1
        old = command[i]
        command[i] = str(root / "corpus.json")
        checks = {}
        for name, data in (("malformed_png", b"not a png"),
                           ("oversized_dimensions", b"\x89PNG\r\n\x1a\n" +
                            struct.pack(">I", 13) + b"IHDR" +
                            struct.pack(">IIBBBBB", 16000001, 1, 8, 2, 0, 0, 0))):
            if name == "oversized_dimensions":
                data += struct.pack(">I", zlib.crc32(data[12:]))
            (root / "images/bad.png").write_bytes(data)
            failed = launch(["--repeats", "1"])
            assert failed["returncode"] == 1 and failed["pid_reaped"]
            checks[name] = {"returncode": failed["returncode"], "error": failed["stderr"].strip()}
        command[i] = old
    report["invalid_input"] = checks
    report["os_network_denied"] = args.sandbox
    args.out.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print("Saved native metrics, memory, cancellation and input-rejection evidence", flush=True)


if __name__ == "__main__":
    main()
