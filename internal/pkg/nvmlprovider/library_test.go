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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

func TestResolveLibraryPath(t *testing.T) {
	dir := t.TempDir()
	library := filepath.Join(dir, "libnvidia-ml.so.550")
	require.NoError(t, os.WriteFile(library, nil, 0o600))
	encoded, err := json.Marshal([]string{filepath.Join(dir, "missing"), dir})
	require.NoError(t, err)
	for _, tt := range []struct {
		name, explicit, legacy, want string
		fail                         bool
	}{
		{name: "default", want: "libnvidia-ml.so.1"},
		{name: "explicit takes precedence", explicit: "/driver/libnvidia-ml.so.1", legacy: "bad json", want: "/driver/libnvidia-ml.so.1"},
		{name: "legacy directory priority", legacy: string(encoded), want: library},
		{name: "invalid JSON", legacy: "bad", fail: true},
		{name: "empty list", legacy: "[]", fail: true},
		{name: "null list", legacy: "null", fail: true},
		{name: "empty directory", legacy: `[""]`, fail: true},
		{name: "not found", legacy: `["/missing/nvml/library"]`, fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLibraryPaths(tt.explicit, tt.legacy)
			if tt.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, []string{tt.want}, got)
			}
		})
	}
}

func TestResolveLibraryPathRejectsBrokenSymlinkAndLookalike(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "libnvidia-ml.so.backup.txt"), nil, 0o600))
	// Broken links must not be selected.
	require.NoError(t, os.Symlink("missing", filepath.Join(dir, "libnvidia-ml.so")))
	encoded, err := json.Marshal([]string{dir})
	require.NoError(t, err)
	_, err = resolveLibraryPaths("", string(encoded))
	require.Error(t, err)
}

func TestLibraryConfigurationLifecycle(t *testing.T) {
	originalInit, originalShutdown, originalClient := nvml.Init, nvml.Shutdown, nvmlInterface
	t.Cleanup(func() {
		nvml.Init, nvml.Shutdown, nvmlInterface = originalInit, originalShutdown, originalClient
		require.NoError(t, nvml.SetLibraryOptions(nvml.WithLibraryPath("libnvidia-ml.so.1")))
	})
	reset()
	t.Setenv(libraryPathEnv, "")
	t.Setenv(legacyLibraryDirsEnv, "not JSON")
	initCalls, shutdownCalls := 0, 0
	nvml.Init = func() nvml.Return { initCalls++; return nvml.SUCCESS }
	nvml.Shutdown = func() nvml.Return { shutdownCalls++; return nvml.SUCCESS }
	require.ErrorContains(t, Initialize(), legacyLibraryDirsEnv)
	require.Zero(t, initCalls)
	require.Error(t, Client().(nvmlProvider).preCheck())
	t.Setenv(legacyLibraryDirsEnv, "")
	require.NoError(t, Initialize())
	require.NoError(t, Initialize())
	require.Equal(t, 1, initCalls)
	Client().Cleanup()
	require.Equal(t, 1, shutdownCalls)
	require.NoError(t, Initialize())
	require.Equal(t, 2, initCalls)
	Client().Cleanup()
}

func TestTryLibraries(t *testing.T) {
	for _, tt := range []struct {
		name      string
		results   []nvml.Return
		wantCalls int
		wantError bool
	}{
		{"fallback on load failure", []nvml.Return{nvml.ERROR_LIBRARY_NOT_FOUND, nvml.SUCCESS}, 2, false},
		{"stop on success", []nvml.Return{nvml.SUCCESS}, 1, false},
		{"stop on driver error", []nvml.Return{nvml.ERROR_LIB_RM_VERSION_MISMATCH}, 1, true},
		{"all candidates fail", []nvml.Return{nvml.ERROR_LIBRARY_NOT_FOUND, nvml.ERROR_LIBRARY_NOT_FOUND}, 2, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var attempted []string
			calls := 0
			err := tryLibraries([]string{"/first/libnvidia-ml.so.1", "/second/libnvidia-ml.so.1"}, func(path string) error { attempted = append(attempted, path); return nil }, func() nvml.Return {
				ret := tt.results[calls]
				calls++
				return ret
			})
			require.Len(t, attempted, tt.wantCalls)
			require.Equal(t, tt.wantCalls, calls)
			if tt.wantError {
				require.ErrorContains(t, err, attempted[0])
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResolveLibraryPathsContinuesAfterBrokenSymlink(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink("missing", filepath.Join(first, "libnvidia-ml.so")))
	firstLib := filepath.Join(first, "libnvidia-ml.so.1")
	secondLib := filepath.Join(second, "libnvidia-ml.so.1")
	require.NoError(t, os.WriteFile(firstLib, nil, 0o600))
	require.NoError(t, os.WriteFile(secondLib, nil, 0o600))
	encoded, err := json.Marshal([]string{first, second, first})
	require.NoError(t, err)
	paths, err := resolveLibraryPaths("", string(encoded))
	require.NoError(t, err)
	require.Equal(t, []string{firstLib, secondLib}, paths)
}

func TestTryLibrariesStopsOnConfigurationError(t *testing.T) {
	calls := 0
	err := tryLibraries([]string{"first", "second"}, func(string) error {
		calls++
		return os.ErrPermission
	}, func() nvml.Return { t.Fatal("must not initialize after configuration failure"); return nvml.SUCCESS })
	require.ErrorIs(t, err, os.ErrPermission)
	require.Equal(t, 1, calls)
}
