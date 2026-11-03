# DCGM Exporter

DCGM Exporter exposes NVIDIA GPU telemetry from [NVIDIA Data Center GPU
Manager (DCGM)](https://developer.nvidia.com/dcgm) in the Prometheus text
format. This repository contains the exporter source, container and package
builds, Helm chart, tests, and dashboards.

## Documentation

Use the official NVIDIA documentation for user guidance:

- [Install DCGM Exporter](https://docs.nvidia.com/datacenter/dcgm/latest/installation/install-dcgm-exporter.html)
- [Configure Prometheus for DCGM Exporter](https://docs.nvidia.com/datacenter/dcgm/latest/learn/getting-started-for-system-administrators/configure-prometheus-for-dcgm-exporter.html)
- [`dcgm-exporter` command reference](https://docs.nvidia.com/datacenter/dcgm/latest/reference/command-line-reference/dcgm-exporter.html)
- [DCGM Exporter Metrics](https://docs.nvidia.com/datacenter/dcgm/latest/reference/dcgm-exporter-metrics.html)
- [DCGM Exporter Release Notes](https://docs.nvidia.com/datacenter/dcgm/latest/release-notes/dcgm-exporter.html)

The installation guide covers deployment, configuration, verification,
administration, and troubleshooting.

## Version compatibility

Run DCGM Exporter with the DCGM version paired with that exporter release.
See [Version Compatibility](https://docs.nvidia.com/datacenter/dcgm/latest/installation/install-dcgm-exporter.html#dcgm-exporter-version-compatibility)
for supported package and remote-connection combinations.

## Repository resources

- [Helm chart configuration and troubleshooting](deployment/README.md)
- [Contribution requirements and development workflow](CONTRIBUTING.md)
- [Kubernetes integration and end-to-end tests](tests/k8s/README.md)
- [Container tests and image examples](tests/container/README.md)
- [Default metric collectors](etc/default-counters.csv)

## Dashboards

- [NVIDIA DCGM Exporter dashboard for Grafana](https://grafana.com/grafana/dashboards/12239/)
- Checked-in Grafana dashboard: [`grafana/dcgm-exporter-dashboard.json`](grafana/dcgm-exporter-dashboard.json)
- Community [OpenObserve dashboard](https://github.com/openobserve/dashboards/tree/main/NVIDIA%20GPU%20Monitoring)
  and [integration guide](https://openobserve.ai/blog/how-to-monitor-nvidia-gpu/)

## Contributing and support

Read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a change. For
community support, [open an issue](https://github.com/NVIDIA/dcgm-exporter/issues/new).

Report potential security vulnerabilities through the
[NVIDIA Product Security](https://www.nvidia.com/en-us/security/) process, not
through a public issue. See [SECURITY.md](SECURITY.md) for reporting details.


### Optional wxs compatibility metrics

The optional `etc/wxs-counters.csv` profile retains the six metric names from the
wxs fork and uses current DCGM field names and metadata. Select it with
`-f /etc/dcgm-exporter/wxs-counters.csv`, or add the commented compatibility rows
from `etc/default-counters.csv` to your own complete metric source. These metrics
are exporter-owned gauges, not native DCGM fields; other metric sources do not
enable them implicitly. The profile also enables the old scalar clock, temperature,
ECC, retired-page, profiling and static-label selections. Blob-valued supported-clock
and process-accounting fields from the old `rows_counter.csv` are excluded because
they cannot be rendered as scalar samples. Fan speed and pending retirement use
`gauge`; current NVLink bandwidth metadata is retained. Image, deployment and CI
configuration remain unchanged.

| Metric | Scope and meaning |
| --- | --- |
| `DCGM_FI_DEV_NVLINK_INACTIVE_STATUS` | Per physical GPU: number of NVLink lanes reported `LS_DOWN`. GPUs absent from the link-status response have no sample. |
| `DCGM_FI_DEV_COMP_RUNNING_PROC` | Per physical GPU: number of compute processes reported by NVML. |
| `DCGM_FI_DEV_GRAPH_RUNNING_PROC` | Per physical GPU: number of graphics processes reported by NVML. |
| `DCGM_FI_DEV_TOTAL_COUNT` | Node: number of selected physical GPUs. |
| `DCGM_FI_DEV_USED_COUNT` | Node: number of selected physical GPUs with compute processes. Graphics-only activity does not count as used. |
| `DCGM_FI_DEV_ERROR_COUNT` | Node: number of physical GPUs whose NVML process query failed during this collection. This is not a hardware health diagnosis or a cumulative error count. |

Physical GPUs are deduplicated, including parents of selected MIG instances.
Per-GPU samples use the current GPU labels and `hostname`; they do not repeat
parent process counts under MIG labels. Node counts carry only `hostname` when
hostname output is enabled. Existing queries from the fork using `Hostname`
need to use `hostname`. NVLink inactive counts use `gauge` because links can
recover; do not use `rate()` as though they were cumulative counters.

Failed or unsupported process queries omit that GPU's process samples and count
the GPU once in `DCGM_FI_DEV_ERROR_COUNT`. If any GPU query fails,
`DCGM_FI_DEV_USED_COUNT` is omitted to avoid presenting a partial sum as the
node's utilization. Process metrics and their used/error aggregates are omitted
in remote hostengine and fake-GPU modes because NVML queries local hardware.
NVLink query failures omit only NVLink samples and are logged; they do not fail
the entire scrape. These gauges hold no cumulative state across reloads.

NVML uses the default dynamic loader search (`libnvidia-ml.so.1`) unless configured.
Set `DCGM_EXPORTER_NVML_LIBRARY_PATH` to an explicit library path to override it.
For compatibility, `NVIDIA_LIB_SERACH_DIR` (the original spelling) accepts a JSON
array such as `["/usr/lib/x86_64-linux-gnu", "/usr/local/nvidia/lib64"]`; it searches
directories in order, recursively, and tries the discovered candidates until a
library loads and initializes successfully. Loader failures continue to the next
candidate and are logged with the path; driver initialization errors (such as
a driver/library version mismatch) stop initialization because go-nvml may keep
the library loaded after such failures. The explicit path takes precedence and
is tried alone. Invalid
JSON, empty directory lists and missing libraries produce initialization errors,
subject to the existing startup NVML failure policy. Library selection runs
before NVML initialization and after lifecycle resets; collectors share the
application's NVML lifecycle. `NODE_NAME` remains optional with the current
hostname fallback behavior.

### Startup OOM diagnostics

Initialization emits INFO-level `Exporter initialization phase` records with
`stage`, `event=begin|end`, `elapsed` and `reload_id`. These cover DCGM/NVML
initialization, profiling-group queries, device count/info reads, metric loading,
watch-list discovery and collector initialization. An `end` record indicates
that a call returned; normal error logs still determine whether it succeeded.

Enable `--debug` or `DCGM_EXPORTER_DEBUG=true` to include memory snapshots.
Default INFO logging only records phase boundaries and does not sample memory.
Debug-enabled records include Go heap allocation/in-use/system bytes, cgo call count,
goroutine count, process RSS, and available cgroup memory usage/peak/limit/OOM
counters. Linux cgroup v1 and v2 are supported. Unavailable kernel counters are
omitted. Go system bytes describe runtime-managed memory, rather than process
RSS; native DCGM/NVML allocations are reflected in RSS/cgroup usage and are not
accounted for by Go heap statistics.

For an OOM during startup, inspect the previous container's logs and find the
last `begin` without an `end`. This identifies the interrupted initialization
call even when the kernel kills the process before the HTTP/pprof server starts.
The selected NVML library path is also logged. Logs do not enable pprof or change
collection behavior, memory limits, or GPU configuration.
