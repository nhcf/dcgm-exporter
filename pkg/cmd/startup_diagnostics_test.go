package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartupProcessMemoryAttrs(t *testing.T) {
	for _, tt := range []struct {
		name  string
		files map[string]string
		want  map[string]uint64
	}{
		{name: "cgroup v2 includes native RSS and OOM counters", files: map[string]string{
			"/proc/self/status":             "Name:\tdcgm-exporter\nVmRSS:\t2048 kB\n",
			"/sys/fs/cgroup/memory.current": "419430400\n", "/sys/fs/cgroup/memory.peak": "420000000\n", "/sys/fs/cgroup/memory.max": "419430400\n",
			"/sys/fs/cgroup/memory.events": "low 0\nmax 10\noom 2\noom_kill 1\n",
		}, want: map[string]uint64{"process_rss_bytes": 2097152, "cgroup_memory_current_bytes": 419430400, "cgroup_memory_peak_bytes": 420000000, "cgroup_memory_limit_bytes": 419430400, "cgroup_oom": 2, "cgroup_oom_kill": 1}},
		{name: "cgroup v1 fallback", files: map[string]string{
			"/sys/fs/cgroup/memory/memory.usage_in_bytes": "123", "/sys/fs/cgroup/memory/memory.max_usage_in_bytes": "456", "/sys/fs/cgroup/memory/memory.limit_in_bytes": "1024",
		}, want: map[string]uint64{"cgroup_memory_current_bytes": 123, "cgroup_memory_peak_bytes": 456, "cgroup_memory_limit_bytes": 1024}},
		{name: "missing invalid and unlimited values", files: map[string]string{
			"/proc/self/status": "VmRSS: 18446744073709551615 kB\n", "/sys/fs/cgroup/memory.current": "bad", "/sys/fs/cgroup/memory.max": "max", "/sys/fs/cgroup/memory.events": "oom_kill invalid\noom\n",
		}, want: map[string]uint64{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			attrs := startupProcessMemoryAttrs(func(path string) ([]byte, error) {
				if data, ok := tt.files[path]; ok {
					return []byte(data), nil
				}
				return nil, os.ErrNotExist
			})
			got := map[string]uint64{}
			for _, attr := range attrs {
				got[attr.Key] = attr.Value.Uint64()
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestStartupPhaseLogsBoundariesAtInfoLevel(t *testing.T) {
	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })
	var output bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	finish := startStartupPhase(context.Background(), "dcgm.profiling.metric_groups", 7)
	finish()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 2)
	for i, line := range lines {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.Equal(t, "INFO", record["level"])
		require.Equal(t, "dcgm.profiling.metric_groups", record["stage"])
		require.Equal(t, []string{"begin", "end"}[i], record["event"])
		require.Equal(t, float64(7), record["reload_id"])
		require.NotContains(t, record, "go_heap_alloc_bytes")
		require.NotContains(t, record, "cgo_calls")
	}
}

func TestStartupPhaseMemoryDiagnosticsRequireDebug(t *testing.T) {
	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })
	var output bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	logStartupPhase(context.Background(), "test", "begin", 0, 0)
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	require.Contains(t, record, "go_heap_alloc_bytes")
	require.Contains(t, record, "cgo_calls")
}
