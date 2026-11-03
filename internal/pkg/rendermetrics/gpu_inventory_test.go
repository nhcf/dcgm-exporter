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

package rendermetrics

import (
	"bytes"
	"testing"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/dcgm-exporter/internal/pkg/collector"
	"github.com/NVIDIA/dcgm-exporter/internal/pkg/counters"
)

func TestInventoryAggregateRendering(t *testing.T) {
	for _, name := range []string{counters.GPUTotalCount, counters.GPUUsedCount, counters.GPUErrorCount} {
		counter := counters.Counter{FieldName: name, PromType: "gauge", Help: "Physical GPU count."}
		var output bytes.Buffer
		err := RenderGroup(&output, dcgm.FE_GPU, collector.MetricsByCounter{counter: {{Counter: counter, Value: "2", Hostname: "node-a"}}})
		require.NoError(t, err)
		require.Contains(t, output.String(), name+`{hostname="node-a"} 2`)
		require.NotContains(t, output.String(), `gpu=`)
		require.NotContains(t, output.String(), `UUID=`)
	}
}
