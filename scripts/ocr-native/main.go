// Standalone OCR experiment, deliberately outside the application's Go module.
// Requires the pinned RapidOCRLite v5.0.0a6 C ABI, not a Python runtime.
package main

/*
#include <stdlib.h>
#include "ppocr/ppocr.h"
#ifdef __APPLE__
#include <mach/mach.h>
static unsigned long long current_rss_bytes(void) {
  struct mach_task_basic_info info;
  mach_msg_type_number_t count = MACH_TASK_BASIC_INFO_COUNT;
  if (task_info(mach_task_self(), MACH_TASK_BASIC_INFO, (task_info_t)&info, &count) != KERN_SUCCESS) return 0;
  return info.resident_size;
}
#elif defined(__linux__)
#include <stdio.h>
#include <unistd.h>
static unsigned long long current_rss_bytes(void) {
  unsigned long long total, resident;
  FILE *file = fopen("/proc/self/statm", "r");
  if (!file) return 0;
  int count = fscanf(file, "%llu %llu", &total, &resident);
  fclose(file);
  return count == 2 ? resident * sysconf(_SC_PAGESIZE) : 0;
}
#else
static unsigned long long current_rss_bytes(void) { return 0; }
#endif
#if defined(__APPLE__) || defined(__linux__)
#include <sys/resource.h>
static unsigned long long peak_rss_bytes(void) {
  struct rusage usage;
  if (getrusage(RUSAGE_SELF, &usage) != 0) return 0;
#ifdef __APPLE__
  return usage.ru_maxrss;
#else
  return (unsigned long long)usage.ru_maxrss * 1024;
#endif
}
#else
static unsigned long long peak_rss_bytes(void) { return 0; }
#endif
static int recognize_rgb(ppocr_ocr *ocr, int width, int height,
                         const unsigned char *rgb, ppocr_result *result) {
  ppocr_image image = {width, height, rgb};
  return ppocr_recognize(ocr, &image, result);
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"
	"unsafe"
)

const maxPNGBytes = 8 << 20
const maxPixels = 16_000_000

type fixture struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
}

type line struct {
	Text       string        `json:"text"`
	Confidence float32       `json:"confidence"`
	Box        [4][2]float32 `json:"box"`
}

type caseResult struct {
	Name     string    `json:"name"`
	Expected string    `json:"expected"`
	Actual   string    `json:"actual"`
	Stable   bool      `json:"stable"`
	Times    []float64 `json:"warm_ms"`
	Median   float64   `json:"median_ms"`
	Lines    []line    `json:"lines"`
	Variants []string  `json:"variant_samples,omitempty"`
}

func readRGB(path string) ([]byte, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxPNGBytes {
		return nil, 0, 0, errors.New("PNG must be a regular file of at most 8 MiB")
	}
	cfg, err := png.DecodeConfig(io.LimitReader(f, maxPNGBytes+1))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("invalid PNG: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxPixels/cfg.Height {
		return nil, 0, 0, errors.New("PNG exceeds 16 million pixels")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, 0, 0, err
	}
	img, err := png.Decode(io.LimitReader(f, maxPNGBytes+1))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("invalid PNG: %w", err)
	}
	if img.Bounds().Dx() != cfg.Width || img.Bounds().Dy() != cfg.Height {
		return nil, 0, 0, errors.New("PNG dimensions changed during decoding")
	}
	rgb := make([]byte, cfg.Width*cfg.Height*3)
	for y := 0; y < cfg.Height; y++ {
		for x := 0; x < cfg.Width; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			i := (y*cfg.Width + x) * 3
			// RGBA returns premultiplied components: composite on opaque white.
			rgb[i], rgb[i+1], rgb[i+2] = byte((r+65535-a)>>8), byte((g+65535-a)>>8), byte((b+65535-a)>>8)
		}
	}
	return rgb, cfg.Width, cfg.Height, nil
}

func recognize(handle *C.ppocr_ocr, path string) ([]line, error) {
	rgb, w, h, err := readRGB(path)
	if err != nil {
		return nil, err
	}
	var result C.ppocr_result
	status := C.recognize_rgb(handle, C.int(w), C.int(h), (*C.uchar)(unsafe.Pointer(&rgb[0])), &result)
	runtime.KeepAlive(rgb)
	defer C.ppocr_result_free(&result)
	if status != C.PPOCR_OK {
		return nil, fmt.Errorf("native OCR failed (%d): %s", int(status), C.GoString(C.ppocr_last_error()))
	}
	if result.count > 1000 || (result.count > 0 && result.items == nil) {
		return nil, errors.New("invalid native OCR result")
	}
	lines := []line{}
	for _, item := range unsafe.Slice(result.items, int(result.count)) {
		text := C.GoString(item.text)
		if item.confidence < 0.5 || strings.TrimSpace(text) == "" {
			continue
		}
		out := line{Text: text, Confidence: float32(item.confidence)}
		for i, point := range item.box {
			out.Box[i] = [2]float32{float32(point.x), float32(point.y)}
		}
		lines = append(lines, out)
	}
	return lines, nil
}

func textOf(lines []line) string {
	parts := make([]string, len(lines))
	for i, line := range lines {
		parts[i] = line.Text
	}
	return strings.Join(parts, "\n")
}

func execute() error {
	det := flag.String("det", "", "detector ONNX")
	rec := flag.String("rec", "", "recognizer ONNX")
	dict := flag.String("dict", "", "recognizer dictionary")
	corpus := flag.String("corpus", "", "generated corpus.json")
	one := flag.String("one", "", "optional single fixture name")
	repeats := flag.Int("repeats", 5, "warm runs per fixture")
	flag.Parse()
	if *det == "" || *rec == "" || *dict == "" || *corpus == "" || *repeats < 1 || *repeats > 10000 {
		return errors.New("det, rec, dict, corpus and repeats 1..10000 are required")
	}
	f, err := os.Open(*corpus)
	if err != nil {
		return err
	}
	var fixtures []fixture
	err = json.NewDecoder(io.LimitReader(f, 64<<10)).Decode(&fixtures)
	f.Close()
	if err != nil || len(fixtures) == 0 || len(fixtures) > 100 {
		return errors.New("invalid or oversized fixture corpus")
	}
	selected := []fixture{}
	for _, item := range fixtures {
		if item.Name == "" || strings.ContainsAny(item.Name, `/\`) || item.Name == "." || item.Name == ".." {
			return errors.New("invalid fixture name")
		}
		if *one == "" || *one == item.Name {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		return errors.New("fixture not found")
	}
	// Error messages are thread-local in the C ABI.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var options C.ppocr_options
	C.ppocr_options_init(&options)
	options.backend = C.PPOCR_BACKEND_CPU
	options.det_threshold, options.det_box_threshold, options.det_unclip_ratio = 0.3, 0.5, 1.6
	options.det_limit_side_len, options.det_limit_type = 960, 0
	options.rec_batch_size, options.det_use_dilation = 1, 1
	for i := 0; i < 3; i++ {
		options.det_mean[i], options.det_std[i] = 0.5, 0.5
		options.rec_mean[i], options.rec_std[i] = 0.5, 0.5
	}
	d, r, k := C.CString(*det), C.CString(*rec), C.CString(*dict)
	defer C.free(unsafe.Pointer(d))
	defer C.free(unsafe.Pointer(r))
	defer C.free(unsafe.Pointer(k))
	started := time.Now()
	handle := C.ppocr_create(d, r, k, &options)
	initMS := float64(time.Since(started).Microseconds()) / 1000
	if handle == nil {
		return fmt.Errorf("native initialization failed: %s", C.GoString(C.ppocr_last_error()))
	}
	defer func() {
		if handle != nil {
			C.ppocr_destroy(handle)
		}
	}()
	var cpu C.ppocr_cpu_info
	C.ppocr_query_cpu_info(&cpu)
	imagePath := func(name string) string { return filepath.Join(filepath.Dir(*corpus), "images", name+".png") }
	started = time.Now()
	if _, err = recognize(handle, imagePath(selected[0].Name)); err != nil {
		return err
	}
	firstMS := float64(time.Since(started).Microseconds()) / 1000
	results := []caseResult{}
	for _, item := range selected {
		entry := caseResult{Name: item.Name, Expected: item.Expected, Stable: true}
		variants := map[string]bool{}
		for i := 0; i < *repeats; i++ {
			started = time.Now()
			lines, err := recognize(handle, imagePath(item.Name))
			if err != nil {
				return fmt.Errorf("%s: %w", item.Name, err)
			}
			entry.Times = append(entry.Times, float64(time.Since(started).Microseconds())/1000)
			text := textOf(lines)
			if !variants[text] && len(entry.Variants) < 8 {
				entry.Variants = append(entry.Variants, text)
				variants[text] = true
			}
			if i == 0 {
				entry.Actual, entry.Lines = text, lines
			} else if text != entry.Actual {
				entry.Stable = false
			}
		}
		times := append([]float64{}, entry.Times...)
		sort.Float64s(times)
		entry.Median = times[len(times)/2]
		if len(times)%2 == 0 {
			entry.Median = (times[len(times)/2-1] + times[len(times)/2]) / 2
		}
		results = append(results, entry)
	}
	rssBeforeDestroy := uint64(C.current_rss_bytes())
	C.ppocr_destroy(handle)
	handle = nil
	debug.FreeOSMemory()
	rssAfterDestroy := uint64(C.current_rss_bytes())
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"engine": "RapidOCRLite 5.0.0a6 C ABI", "backend": "CPU", "go": runtime.Version(),
		"os": runtime.GOOS, "arch": runtime.GOARCH, "isa": C.GoString(&cpu.active_isa[0]),
		"native_threads": int(cpu.threads), "init_ms": initMS, "first_call_ms": firstMS,
		"peak_rss_bytes": uint64(C.peak_rss_bytes()), "cases": results,
		"rss_before_destroy_bytes": rssBeforeDestroy, "rss_after_destroy_and_go_gc_bytes": rssAfterDestroy,
	})
}

func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
