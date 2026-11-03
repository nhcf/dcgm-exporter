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

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/dcgm-exporter/internal/pkg/appconfig"
)

func TestInventoryCounterExtraction(t *testing.T) {
	for _, name := range []string{NVLinkInactiveStatus, GPUComputeRunningProcesses, GPUGraphicsRunningProcesses, GPUTotalCount, GPUUsedCount, GPUErrorCount} {
		t.Run(name, func(t *testing.T) {
			set, err := ExtractCounters([][]string{{name, "gauge", "Compatibility metric."}}, &appconfig.Config{})
			require.NoError(t, err)
			require.Empty(t, set.DCGMCounters)
			require.Len(t, set.ExporterCounters, 1)
			id, err := IdentifyMetricType(name)
			require.NoError(t, err)
			require.Equal(t, name, id.String())
			require.Greater(t, uint16(id), uint16(9000))
			_, err = ExtractCounters([][]string{{name, "counter", "Wrong type."}}, &appconfig.Config{})
			require.ErrorContains(t, err, "requires type gauge")
		})
	}
}
