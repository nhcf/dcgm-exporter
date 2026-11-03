/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package collector

import (
	"fmt"
	"log/slog"
	"maps"
	"strconv"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"

	"github.com/NVIDIA/dcgm-exporter/internal/pkg/appconfig"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/counters"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/dcgmprovider"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/devicemonitoring"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/devicewatchlistmanager"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/nvmlprovider"
)

// gpuInventoryCollector emits physical-GPU compatibility metrics only when selected in the metric source.
// It never initializes or shuts down NVML; the application owns that lifecycle.
type gpuInventoryCollector struct {
	baseExpCollector
	counters counters.CounterList
}

func newGPUInventoryCollector(selected, labels counters.CounterList, hostname string, config *appconfig.Config, watchList devicewatchlistmanager.WatchList) (*gpuInventoryCollector, error) {
	c := &gpuInventoryCollector{
		baseExpCollector: baseExpCollector{hostname: hostname, config: config, deviceWatchList: watchList, labelsCounters: labels},
		counters:         selected,
	}
	needsLabels := false
	for _, counter := range selected {
		if !counters.IsGPUInventoryAggregate(counter.FieldName) {
			needsLabels = true
		}
	}
	if needsLabels && len(labels) > 0 && len(watchList.LabelDeviceFields()) > 0 {
		// Install only this collector's label watches and own their cleanup, including for label-only metric sources.
		c.deviceWatchList.SetDeviceFieldsWithoutLabelWatches(watchList.LabelDeviceFields())
		cleanups, err := c.deviceWatchList.Watch()
		if err != nil {
			return nil, fmt.Errorf("watch physical GPU labels: %w", err)
		}
		c.cleanups = cleanups
		parentCleanups, err := c.deviceWatchList.WatchParentGPULabels()
		if err != nil {
			c.Cleanup()
			return nil, fmt.Errorf("watch parent GPU labels: %w", err)
		}
		c.cleanups = append(c.cleanups, parentCleanups...)
	}
	return c, nil
}

func (c *gpuInventoryCollector) GetMetrics() (MetricsByCounter, error) {
	return c.collect(physicalGPUs(devicemonitoring.GetMonitoredEntities(c.deviceWatchList.DeviceInfo()))), nil
}

// physicalGPUs keeps one parent per selected GPU, including when only MIG instances are selected.
func physicalGPUs(entities []devicemonitoring.Info) []devicemonitoring.Info {
	result := make([]devicemonitoring.Info, 0)
	seen := map[uint]bool{}
	for _, mi := range entities {
		if mi.Entity.EntityGroupId != dcgm.FE_GPU && mi.Entity.EntityGroupId != dcgm.FE_GPU_I {
			continue
		}
		id := mi.DeviceInfo.GPU
		if seen[id] {
			continue
		}
		seen[id] = true
		mi.Entity = dcgm.GroupEntityPair{EntityGroupId: dcgm.FE_GPU, EntityId: id}
		mi.InstanceInfo = nil
		mi.ComputeInstanceInfo = nil
		result = append(result, mi)
	}
	return result
}

func (c *gpuInventoryCollector) collect(gpus []devicemonitoring.Info) MetricsByCounter {
	metrics := make(MetricsByCounter)
	enabled := map[string]counters.Counter{}
	for _, counter := range c.counters {
		enabled[counter.FieldName] = counter
	}
	uuid := "UUID"
	if c.config.UseOldNamespace {
		uuid = "uuid"
	}
	labelCache := make(map[uint]map[string]string)
	emitGPU := func(name string, mi devicemonitoring.Info, value int) {
		counter, ok := enabled[name]
		if !ok {
			return
		}
		base := c.baseExpCollector
		base.counter = counter
		labels, loaded := labelCache[mi.DeviceInfo.GPU]
		if !loaded {
			labels = map[string]string{}
			if len(c.labelsCounters) > 0 && len(c.deviceWatchList.LabelDeviceFields()) > 0 {
				if err := c.getLabelsFromCounters(mi, labels); err != nil {
					slog.Warn("Cannot collect physical GPU metric labels", "gpu_uuid", mi.DeviceInfo.UUID, "error", err)
					labels = nil
				}
			}
			labelCache[mi.DeviceInfo.GPU] = labels
		}
		if labels == nil {
			return
		}
		metrics[counter] = append(metrics[counter], base.createMetric(maps.Clone(labels), mi, uuid, value))
	}
	emitNode := func(name string, value int) {
		counter, ok := enabled[name]
		if !ok {
			return
		}
		metrics[counter] = []Metric{{Counter: counter, Value: strconv.Itoa(value), Hostname: c.hostname}}
	}
	emitNode(counters.GPUTotalCount, len(gpus))
	if _, ok := enabled[counters.NVLinkInactiveStatus]; ok {
		links, err := dcgmprovider.Client().GetNvLinkLinkStatus()
		if err != nil {
			slog.Warn("Cannot collect NVLink inactive counts", "error", err)
		} else {
			down := map[uint]int{}
			for _, link := range links {
				if link.ParentType != dcgm.FE_GPU {
					continue
				}
				id := uint(link.ParentId)
				if _, exists := down[id]; !exists {
					down[id] = 0
				}
				if link.State == dcgm.LS_DOWN {
					down[id]++
				}
			}
			for _, mi := range gpus {
				if count, ok := down[mi.DeviceInfo.GPU]; ok {
					emitGPU(counters.NVLinkInactiveStatus, mi, count)
				}
			}
		}
	}
	needsProcesses := false
	for _, name := range []string{counters.GPUComputeRunningProcesses, counters.GPUGraphicsRunningProcesses, counters.GPUUsedCount, counters.GPUErrorCount} {
		if _, ok := enabled[name]; ok {
			needsProcesses = true
		}
	}
	if !needsProcesses {
		return metrics
	}
	// NVML is local: never label local process counts as belonging to a remote hostengine.
	if c.config.UseRemoteHE || c.config.UseFakeGPUs {
		slog.Debug("Skipping local GPU process counts in remote hostengine or fake GPU mode")
		return metrics
	}
	used, failed := 0, 0
	for _, mi := range gpus {
		compute, graphics, err := nvmlprovider.Client().GetDeviceProcessCounts(mi.DeviceInfo.UUID)
		if err != nil {
			failed++
			slog.Warn("Cannot collect GPU process counts", "gpu_uuid", mi.DeviceInfo.UUID, "error", err)
			continue
		}
		if compute > 0 {
			used++
		}
		emitGPU(counters.GPUComputeRunningProcesses, mi, compute)
		emitGPU(counters.GPUGraphicsRunningProcesses, mi, graphics)
	}
	emitNode(counters.GPUErrorCount, failed)
	// A partial sum would misleadingly report failed devices as idle.
	if failed == 0 {
		emitNode(counters.GPUUsedCount, used)
	}
	return metrics
}
