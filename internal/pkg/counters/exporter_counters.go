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

package counters

import "fmt"

type ExporterCounter uint16

const (
	DCGMFIUnknown                       ExporterCounter = 0
	DCGMXIDErrorsCount                  ExporterCounter = iota + 9000
	DCGMClockEventsCount                ExporterCounter = iota + 9000
	DCGMGPUHealthStatus                 ExporterCounter = iota + 9000
	DCGMP2PStatus                       ExporterCounter = iota + 9000
	DCGMXIDErrorsTotal                  ExporterCounter = iota + 9000
	DCGMClockEventsTotal                ExporterCounter = iota + 9000
	ExporterNVLinkInactiveStatus        ExporterCounter = iota + 9000
	ExporterGPUComputeRunningProcesses  ExporterCounter = iota + 9000
	ExporterGPUGraphicsRunningProcesses ExporterCounter = iota + 9000
	ExporterGPUTotalCount               ExporterCounter = iota + 9000
	ExporterGPUUsedCount                ExporterCounter = iota + 9000
	ExporterGPUErrorCount               ExporterCounter = iota + 9000
)

// String method to convert the enum value to a string
func (enm ExporterCounter) String() string {
	switch enm {
	case DCGMXIDErrorsCount:
		return DCGMExpXIDErrorsCount
	case DCGMClockEventsCount:
		return DCGMExpClockEventsCount
	case DCGMGPUHealthStatus:
		return DCGMExpGPUHealthStatus
	case DCGMP2PStatus:
		return DCGMExpP2PStatus
	case DCGMXIDErrorsTotal:
		return DCGMExpXIDErrorsTotal
	case DCGMClockEventsTotal:
		return DCGMExpClockEventsTotal
	case ExporterNVLinkInactiveStatus:
		return NVLinkInactiveStatus
	case ExporterGPUComputeRunningProcesses:
		return GPUComputeRunningProcesses
	case ExporterGPUGraphicsRunningProcesses:
		return GPUGraphicsRunningProcesses
	case ExporterGPUTotalCount:
		return GPUTotalCount
	case ExporterGPUUsedCount:
		return GPUUsedCount
	case ExporterGPUErrorCount:
		return GPUErrorCount
	default:
		return "DCGM_FI_UNKNOWN"
	}
}

// DCGMFields maps DCGMExporterMetric String to enum
var DCGMFields = map[string]ExporterCounter{
	NVLinkInactiveStatus:        ExporterNVLinkInactiveStatus,
	GPUComputeRunningProcesses:  ExporterGPUComputeRunningProcesses,
	GPUGraphicsRunningProcesses: ExporterGPUGraphicsRunningProcesses,
	GPUTotalCount:               ExporterGPUTotalCount,
	GPUUsedCount:                ExporterGPUUsedCount,
	GPUErrorCount:               ExporterGPUErrorCount,

	DCGMXIDErrorsCount.String():   DCGMXIDErrorsCount,
	DCGMClockEventsCount.String(): DCGMClockEventsCount,
	DCGMGPUHealthStatus.String():  DCGMGPUHealthStatus,
	DCGMP2PStatus.String():        DCGMP2PStatus,
	DCGMXIDErrorsTotal.String():   DCGMXIDErrorsTotal,
	DCGMClockEventsTotal.String(): DCGMClockEventsTotal,
	DCGMFIUnknown.String():        DCGMFIUnknown,
}

func IdentifyMetricType(s string) (ExporterCounter, error) {
	mv, ok := DCGMFields[s]
	if !ok {
		return mv, fmt.Errorf("unknown ExporterCounter field '%s'", s)
	}
	return mv, nil
}

// IsGPUInventoryMetric identifies optional compatibility metrics collected on physical GPUs.
func IsGPUInventoryMetric(name string) bool {
	switch name {
	case NVLinkInactiveStatus:
		return true
	case GPUComputeRunningProcesses:
		return true
	case GPUGraphicsRunningProcesses:
		return true
	case GPUTotalCount:
		return true
	case GPUUsedCount:
		return true
	case GPUErrorCount:
		return true
	default:
		return false
	}
}

// IsGPUInventoryAggregate identifies node-wide counts with no device labels.
func IsGPUInventoryAggregate(name string) bool {
	return name == GPUTotalCount || name == GPUUsedCount || name == GPUErrorCount
}
