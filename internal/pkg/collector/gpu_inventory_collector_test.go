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
	"errors"
	"testing"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mockdcgm "github.com/NVIDIA/dcgm-exporter/internal/mocks/pkg/dcgmprovider"
	mockwatcher "github.com/NVIDIA/dcgm-exporter/internal/mocks/pkg/devicewatcher"
	mockmanager "github.com/NVIDIA/dcgm-exporter/internal/mocks/pkg/devicewatchlistmanager"
	mocknvml "github.com/NVIDIA/dcgm-exporter/internal/mocks/pkg/nvmlprovider"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/appconfig"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/counters"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/dcgmprovider"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/deviceinfo"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/devicemonitoring"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/devicewatcher"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/devicewatchlistmanager"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/nvmlprovider"
)

func inventoryCounters(names ...string) counters.CounterList {
	result := counters.CounterList{}
	for _, name := range names {
		result = append(result, counters.Counter{FieldID: dcgm.Short(counters.DCGMFields[name]), FieldName: name, PromType: "gauge"})
	}
	return result
}

func inventoryGPU(id uint, uuid string) devicemonitoring.Info {
	return devicemonitoring.Info{Entity: dcgm.GroupEntityPair{EntityGroupId: dcgm.FE_GPU, EntityId: id}, DeviceInfo: dcgm.Device{GPU: id, UUID: uuid}}
}

func inventoryValues(metrics MetricsByCounter, name string) []string {
	var values []string
	for counter, samples := range metrics {
		if counter.FieldName == name {
			for _, sample := range samples {
				values = append(values, sample.Value)
			}
		}
	}
	return values
}

func TestInventoryPhysicalGPUsDeduplicatesMIG(t *testing.T) {
	gpu := inventoryGPU(2, "GPU-parent")
	mig := gpu
	mig.Entity.EntityGroupId = dcgm.FE_GPU_I
	mig.InstanceInfo = &deviceinfo.GPUInstanceInfo{}
	switchEntity := inventoryGPU(9, "")
	switchEntity.Entity.EntityGroupId = dcgm.FE_SWITCH
	got := physicalGPUs([]devicemonitoring.Info{mig, mig, gpu, switchEntity})
	require.Len(t, got, 1)
	require.Equal(t, gpu.DeviceInfo, got[0].DeviceInfo)
	require.Equal(t, gpu.Entity, got[0].Entity)
	require.Nil(t, got[0].InstanceInfo)
}

func TestInventoryProcessMetrics(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "partial failure"}[failed], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := mocknvml.NewMockNVML(ctrl)
			original := nvmlprovider.Client()
			t.Cleanup(func() { nvmlprovider.SetClient(original) })
			nvmlprovider.SetClient(client)
			// A busy GPU followed by an idle GPU verifies counts do not leak across devices.
			client.EXPECT().GetDeviceProcessCounts("GPU-a").Return(2, 3, nil)
			if failed {
				client.EXPECT().GetDeviceProcessCounts("GPU-b").Return(0, 0, errors.New("lost"))
			} else {
				client.EXPECT().GetDeviceProcessCounts("GPU-b").Return(0, 0, nil)
			}
			c := &gpuInventoryCollector{baseExpCollector: baseExpCollector{config: &appconfig.Config{}, hostname: "node-a"}, counters: inventoryCounters(counters.GPUTotalCount, counters.GPUUsedCount, counters.GPUErrorCount, counters.GPUComputeRunningProcesses, counters.GPUGraphicsRunningProcesses)}
			got := c.collect([]devicemonitoring.Info{inventoryGPU(0, "GPU-a"), inventoryGPU(1, "GPU-b")})
			require.Equal(t, []string{"2"}, inventoryValues(got, counters.GPUTotalCount))
			if failed {
				require.Empty(t, inventoryValues(got, counters.GPUUsedCount))
				require.Equal(t, []string{"1"}, inventoryValues(got, counters.GPUErrorCount))
				require.Equal(t, []string{"2"}, inventoryValues(got, counters.GPUComputeRunningProcesses))
			} else {
				require.Equal(t, []string{"1"}, inventoryValues(got, counters.GPUUsedCount))
				require.Equal(t, []string{"0"}, inventoryValues(got, counters.GPUErrorCount))
				require.Equal(t, []string{"2", "0"}, inventoryValues(got, counters.GPUComputeRunningProcesses))
				require.Equal(t, []string{"3", "0"}, inventoryValues(got, counters.GPUGraphicsRunningProcesses))
			}
		})
	}
}

func TestInventoryNVLinkMetrics(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockdcgm.NewMockDCGM(ctrl)
	original := dcgmprovider.Client()
	t.Cleanup(func() { dcgmprovider.SetClient(original) })
	dcgmprovider.SetClient(client)
	client.EXPECT().GetNvLinkLinkStatus().Return([]dcgm.NvLinkStatus{
		{ParentType: dcgm.FE_GPU, ParentId: 0, State: dcgm.LS_DOWN},
		{ParentType: dcgm.FE_GPU, ParentId: 0, State: dcgm.LS_UP},
		{ParentType: dcgm.FE_GPU, ParentId: 1, State: dcgm.LS_UP},
		{ParentType: dcgm.FE_SWITCH, ParentId: 0, State: dcgm.LS_DOWN},
	}, nil)
	client.EXPECT().GetNvLinkLinkStatus().Return([]dcgm.NvLinkStatus{{ParentType: dcgm.FE_GPU, ParentId: 0, State: dcgm.LS_UP}}, nil)
	client.EXPECT().GetNvLinkLinkStatus().Return(nil, errors.New("unavailable"))
	c := &gpuInventoryCollector{baseExpCollector: baseExpCollector{config: &appconfig.Config{}}, counters: inventoryCounters(counters.NVLinkInactiveStatus, counters.GPUTotalCount)}
	gpus := []devicemonitoring.Info{inventoryGPU(0, "GPU-a"), inventoryGPU(1, "GPU-b"), inventoryGPU(2, "GPU-no-links")}
	require.Equal(t, []string{"1", "0"}, inventoryValues(c.collect(gpus), counters.NVLinkInactiveStatus))
	require.Equal(t, []string{"0"}, inventoryValues(c.collect(gpus), counters.NVLinkInactiveStatus))
	got := c.collect(gpus)
	require.Empty(t, inventoryValues(got, counters.NVLinkInactiveStatus))
	require.Equal(t, []string{"3"}, inventoryValues(got, counters.GPUTotalCount))
}

func TestInventorySelectionAndRemoteModes(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mocknvml.NewMockNVML(ctrl) // No NVML calls expected.
	original := nvmlprovider.Client()
	t.Cleanup(func() { nvmlprovider.SetClient(original) })
	nvmlprovider.SetClient(client)
	for _, config := range []*appconfig.Config{{}, {UseRemoteHE: true}, {UseFakeGPUs: true}} {
		names := []string{counters.GPUTotalCount}
		if config.UseRemoteHE || config.UseFakeGPUs {
			names = append(names, counters.GPUComputeRunningProcesses, counters.GPUErrorCount)
		}
		c := &gpuInventoryCollector{baseExpCollector: baseExpCollector{config: config}, counters: inventoryCounters(names...)}
		got := c.collect([]devicemonitoring.Info{inventoryGPU(0, "GPU-a")})
		require.Len(t, got, 1)
		require.Equal(t, []string{"1"}, inventoryValues(got, counters.GPUTotalCount))
	}
}

func TestInventoryFactoryRegistersOnlySelectedMetrics(t *testing.T) {
	for _, names := range [][]string{nil, {counters.GPUTotalCount, counters.GPUUsedCount}} {
		ctrl := gomock.NewController(t)
		manager := mockmanager.NewMockManager(ctrl)
		if len(names) > 0 {
			manager.EXPECT().EntityWatchList(dcgm.FE_GPU).Return(devicewatchlistmanager.WatchList{}, true)
		}
		factory := InitCollectorFactory(&counters.CounterSet{ExporterCounters: inventoryCounters(names...)}, manager, "node-a", &appconfig.Config{})
		got := factory.NewCollectors()
		if len(names) == 0 {
			require.Empty(t, got)
		} else {
			require.Len(t, got, 1)
			require.Equal(t, dcgm.FE_GPU, got[0].Entity())
			c, ok := got[0].Collector().(*gpuInventoryCollector)
			require.True(t, ok)
			require.Len(t, c.counters, 2)
		}
	}
}

func TestInventoryOwnsLabelWatches(t *testing.T) {
	ctrl := gomock.NewController(t)
	watcher := mockwatcher.NewMockWatcher(ctrl)
	label := counters.Counter{FieldID: dcgm.DCGM_FI_DRIVER_VERSION, FieldName: "DCGM_FI_DRIVER_VERSION", PromType: "label"}
	watchList := *devicewatchlistmanager.NewWatchList(nil, []dcgm.Short{dcgm.DCGM_FI_DEV_GPU_TEMP}, []dcgm.Short{label.FieldID}, watcher, 1000)
	cleaned := false
	watcher.EXPECT().WatchDeviceFieldGroups(gomock.Any(), nil).DoAndReturn(func(groups []devicewatcher.FieldWatchGroup, _ deviceinfo.Provider) ([]dcgm.GroupHandle, []dcgm.FieldHandle, []func(), error) {
		require.Len(t, groups, 1)
		require.Equal(t, []dcgm.Short{label.FieldID}, groups[0].Fields)
		return nil, nil, []func(){func() { cleaned = true }}, nil
	})
	parentCleaned := false
	watcher.EXPECT().WatchDeviceFieldGroupsForParentGPUs(gomock.Any(), nil).Return(nil, nil, []func(){func() { parentCleaned = true }}, nil)
	c, err := newGPUInventoryCollector(inventoryCounters(counters.NVLinkInactiveStatus), counters.CounterList{label}, "node-a", &appconfig.Config{}, watchList)
	require.NoError(t, err)
	c.Cleanup()
	require.True(t, cleaned)
	require.True(t, parentCleaned)
	// Node-only counts need no DCGM label watches.
	_, err = newGPUInventoryCollector(inventoryCounters(counters.GPUTotalCount), counters.CounterList{label}, "node-a", &appconfig.Config{}, watchList)
	require.NoError(t, err)
}

func TestInventoryRollsBackWatchesOnParentFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	watcher := mockwatcher.NewMockWatcher(ctrl)
	label := counters.Counter{FieldID: dcgm.DCGM_FI_DRIVER_VERSION, PromType: "label"}
	watchList := *devicewatchlistmanager.NewWatchList(nil, nil, []dcgm.Short{label.FieldID}, watcher, 1000)
	cleaned := false
	watcher.EXPECT().WatchDeviceFieldGroups(gomock.Any(), nil).Return(nil, nil, []func(){func() { cleaned = true }}, nil)
	watcher.EXPECT().WatchDeviceFieldGroupsForParentGPUs(gomock.Any(), nil).Return(nil, nil, nil, errors.New("watch failed"))
	_, err := newGPUInventoryCollector(inventoryCounters(counters.NVLinkInactiveStatus), counters.CounterList{label}, "node-a", &appconfig.Config{}, watchList)
	require.ErrorContains(t, err, "parent GPU labels")
	require.True(t, cleaned)
}

func TestInventoryReadsLabelsOncePerGPUPerCollection(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failed], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			dcgmClient := mockdcgm.NewMockDCGM(ctrl)
			nvmlClient := mocknvml.NewMockNVML(ctrl)
			oldDCGM, oldNVML := dcgmprovider.Client(), nvmlprovider.Client()
			t.Cleanup(func() { dcgmprovider.SetClient(oldDCGM); nvmlprovider.SetClient(oldNVML) })
			dcgmprovider.SetClient(dcgmClient)
			nvmlprovider.SetClient(nvmlClient)
			label := counters.Counter{FieldID: dcgm.DCGM_FI_DRIVER_VERSION, PromType: "label"}
			watchList := *devicewatchlistmanager.NewWatchList(nil, nil, []dcgm.Short{label.FieldID}, nil, 1000)
			c := &gpuInventoryCollector{baseExpCollector: baseExpCollector{config: &appconfig.Config{}, labelsCounters: counters.CounterList{label}, deviceWatchList: watchList}, counters: inventoryCounters(counters.GPUComputeRunningProcesses, counters.GPUGraphicsRunningProcesses)}
			var labelErr error
			if failed {
				labelErr = errors.New("unavailable")
			}
			// Two metrics share one label query; each new collection retries independently.
			dcgmClient.EXPECT().EntityGetLatestValues(dcgm.FE_GPU, uint(0), []dcgm.Short{label.FieldID}).Return(nil, labelErr).Times(2)
			nvmlClient.EXPECT().GetDeviceProcessCounts("GPU-a").Return(2, 3, nil).Times(2)
			for range 2 {
				got := c.collect([]devicemonitoring.Info{inventoryGPU(0, "GPU-a")})
				if failed {
					require.Empty(t, got)
				} else {
					require.Equal(t, []string{"2"}, inventoryValues(got, counters.GPUComputeRunningProcesses))
					require.Equal(t, []string{"3"}, inventoryValues(got, counters.GPUGraphicsRunningProcesses))
				}
			}
		})
	}
}
