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
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type processDevice interface {
	GetComputeRunningProcesses() ([]nvml.ProcessInfo, nvml.Return)
	GetGraphicsRunningProcesses() ([]nvml.ProcessInfo, nvml.Return)
}

func (n nvmlProvider) GetDeviceProcessCounts(gpuUUID string) (int, int, error) {
	if err := n.preCheck(); err != nil {
		return 0, 0, err
	}
	device, ret := nvml.DeviceGetHandleByUUID(gpuUUID)
	if ret != nvml.SUCCESS {
		return 0, 0, fmt.Errorf("get device %s: %s", gpuUUID, nvml.ErrorString(ret))
	}
	return getProcessCounts(device)
}

func getProcessCounts(device processDevice) (int, int, error) {
	compute, ret := device.GetComputeRunningProcesses()
	if ret != nvml.SUCCESS {
		return 0, 0, fmt.Errorf("get compute processes: %s", nvml.ErrorString(ret))
	}
	graphics, ret := device.GetGraphicsRunningProcesses()
	if ret != nvml.SUCCESS {
		return 0, 0, fmt.Errorf("get graphics processes: %s", nvml.ErrorString(ret))
	}
	return len(compute), len(graphics), nil
}
