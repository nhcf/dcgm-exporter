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

package nvmlprovider

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

type fakeProcessDevice struct {
	compute, graphics       []nvml.ProcessInfo
	computeRet, graphicsRet nvml.Return
	graphicsCalls           int
}

func (d *fakeProcessDevice) GetComputeRunningProcesses() ([]nvml.ProcessInfo, nvml.Return) {
	return d.compute, d.computeRet
}

func (d *fakeProcessDevice) GetGraphicsRunningProcesses() ([]nvml.ProcessInfo, nvml.Return) {
	d.graphicsCalls++
	return d.graphics, d.graphicsRet
}

func TestGetProcessCounts(t *testing.T) {
	for _, tt := range []struct {
		name                      string
		device                    fakeProcessDevice
		wantCompute, wantGraphics int
		wantError                 string
		graphicsCalls             int
	}{
		{name: "idle", graphicsCalls: 1},
		{name: "independent counts", device: fakeProcessDevice{compute: make([]nvml.ProcessInfo, 2), graphics: make([]nvml.ProcessInfo, 3)}, wantCompute: 2, wantGraphics: 3, graphicsCalls: 1},
		{name: "compute failure stops queries", device: fakeProcessDevice{computeRet: nvml.ERROR_GPU_IS_LOST}, wantError: "compute"},
		{name: "unsupported is not idle", device: fakeProcessDevice{computeRet: nvml.ERROR_NOT_SUPPORTED}, wantError: "compute"},
		{name: "graphics failure", device: fakeProcessDevice{graphicsRet: nvml.ERROR_NO_PERMISSION}, wantError: "graphics", graphicsCalls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			compute, graphics, err := getProcessCounts(&tt.device)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantCompute, compute)
			require.Equal(t, tt.wantGraphics, graphics)
			require.Equal(t, tt.graphicsCalls, tt.device.graphicsCalls)
		})
	}
}

func TestProcessCountsRequireNVMLInitialization(t *testing.T) {
	_, _, err := (nvmlProvider{}).GetDeviceProcessCounts("GPU-test")
	require.ErrorContains(t, err, "not initialized")
}

func TestProcessCountsHandleFailureUsesUUID(t *testing.T) {
	original := nvml.DeviceGetHandleByUUID
	t.Cleanup(func() { nvml.DeviceGetHandleByUUID = original })
	nvml.DeviceGetHandleByUUID = func(uuid string) (nvml.Device, nvml.Return) {
		require.Equal(t, "GPU-requested", uuid)
		return nil, nvml.ERROR_GPU_IS_LOST
	}
	_, _, err := (nvmlProvider{initialized: true}).GetDeviceProcessCounts("GPU-requested")
	require.ErrorContains(t, err, "GPU-requested")
}
