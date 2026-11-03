package cmd

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// startStartupPhase logs both sides of initialization calls. If the kernel kills
// the process inside a native DCGM/NVML call, the last begin event locates it.
// RSS and cgroup usage include native allocations that Go heap statistics omit.
func startStartupPhase(ctx context.Context, stage string, reloadID uint64) func() {
	started := time.Now()
	logStartupPhase(ctx, stage, "begin", reloadID, 0)
	return func() { logStartupPhase(ctx, stage, "end", reloadID, time.Since(started)) }
}

func logStartupPhase(ctx context.Context, stage, event string, reloadID uint64, elapsed time.Duration) {
	attrs := []slog.Attr{
		slog.String("stage", stage), slog.String("event", event),
		slog.Uint64("reload_id", reloadID), slog.Duration("elapsed", elapsed),
	}
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		attrs = append(attrs,
			slog.Uint64("go_heap_alloc_bytes", stats.HeapAlloc),
			slog.Uint64("go_heap_inuse_bytes", stats.HeapInuse),
			slog.Uint64("go_sys_bytes", stats.Sys),
			slog.Int64("cgo_calls", runtime.NumCgoCall()),
			slog.Int("goroutines", runtime.NumGoroutine()),
		)
		attrs = append(attrs, startupProcessMemoryAttrs(os.ReadFile)...)
	}
	slog.Default().LogAttrs(ctx, slog.LevelInfo, "Exporter initialization phase", attrs...)
}

func startupProcessMemoryAttrs(readFile func(string) ([]byte, error)) []slog.Attr {
	var attrs []slog.Attr
	read := func(path string) string {
		data, err := readFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				slog.Debug("Cannot read initialization memory diagnostic", "path", path, "error", err)
			}
			return ""
		}
		return string(data)
	}
	for line := range strings.SplitSeq(read("/proc/self/status"), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "VmRSS:" || fields[2] != "kB" {
			continue
		}
		rss, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil && rss <= math.MaxUint64/1024 {
			attrs = append(attrs, slog.Uint64("process_rss_bytes", rss*1024))
		}
	}
	// Support both cgroup v2 and the standard Kubernetes cgroup v1 memory mount.
	for _, metric := range []struct{ name, v2, v1 string }{
		{"cgroup_memory_current_bytes", "memory.current", "memory.usage_in_bytes"},
		{"cgroup_memory_peak_bytes", "memory.peak", "memory.max_usage_in_bytes"},
		{"cgroup_memory_limit_bytes", "memory.max", "memory.limit_in_bytes"},
	} {
		raw := strings.TrimSpace(read("/sys/fs/cgroup/" + metric.v2))
		if raw == "" {
			raw = strings.TrimSpace(read("/sys/fs/cgroup/memory/" + metric.v1))
		}
		if value, err := strconv.ParseUint(raw, 10, 64); err == nil {
			attrs = append(attrs, slog.Uint64(metric.name, value))
		}
	}
	for line := range strings.SplitSeq(read("/sys/fs/cgroup/memory.events"), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || (fields[0] != "oom" && fields[0] != "oom_kill") {
			continue
		}
		if value, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
			attrs = append(attrs, slog.Uint64("cgroup_"+fields[0], value))
		}
	}
	return attrs
}
