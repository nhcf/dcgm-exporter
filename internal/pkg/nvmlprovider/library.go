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
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const (
	libraryPathEnv       = "DCGM_EXPORTER_NVML_LIBRARY_PATH"
	legacyLibraryDirsEnv = "NVIDIA_LIB_SERACH_DIR"
)

// initializeLibrary retries candidates only when dlopen failed. A driver-level Init
// failure can leave go-nvml's library loaded, so changing paths then is unsafe.
func initializeLibrary() error {
	paths, err := resolveLibraryPaths(os.Getenv(libraryPathEnv), os.Getenv(legacyLibraryDirsEnv))
	if err != nil {
		return err
	}
	return tryLibraries(paths, func(path string) error {
		return nvml.SetLibraryOptions(nvml.WithLibraryPath(path))
	}, nvml.Init)
}

func tryLibraries(paths []string, configure func(string) error, init func() nvml.Return) error {
	var failures []error
	for _, path := range paths {
		if err := configure(path); err != nil {
			return errors.Join(append(failures, fmt.Errorf("configure NVML library %q: %w", path, err))...)
		}
		slog.Info("Attempting to initialize NVML library", "library_path", path)
		ret := init()
		if ret == nvml.SUCCESS {
			slog.Info("NVML library initialized", "library_path", path)
			return nil
		}
		err := fmt.Errorf("initialize NVML library %q: %s", path, nvml.ErrorString(ret))
		failures = append(failures, err)
		slog.Warn("NVML library candidate failed", "library_path", path, "error", err)
		if ret != nvml.ERROR_LIBRARY_NOT_FOUND {
			// The library loaded but the driver rejected initialization. Do not replace
			// a live library or call Shutdown after an unsuccessful initialization.
			return errors.Join(failures...)
		}
	}
	return fmt.Errorf("all NVML library candidates failed: %w", errors.Join(failures...))
}

// resolveLibraryPaths preserves the dynamic loader default unless explicitly configured.
func resolveLibraryPaths(explicitPath, legacyDirs string) ([]string, error) {
	if explicitPath != "" {
		return []string{explicitPath}, nil
	}
	if legacyDirs == "" {
		return []string{"libnvidia-ml.so.1"}, nil
	}
	var dirs []string
	if err := json.Unmarshal([]byte(legacyDirs), &dirs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", legacyLibraryDirsEnv, err)
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("%s must contain at least one directory", legacyLibraryDirsEnv)
	}
	var searchErrors []string
	var paths []string
	seen := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" {
			return nil, fmt.Errorf("%s contains an empty directory", legacyLibraryDirsEnv)
		}
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				searchErrors = append(searchErrors, walkErr.Error())
				return nil
			}
			suffix, versioned := strings.CutPrefix(entry.Name(), "libnvidia-ml.so.")
			validVersion := versioned && suffix != "" && suffix[0] >= '0' && suffix[0] <= '9' && strings.Trim(suffix, "0123456789.") == ""
			if !entry.IsDir() && (entry.Name() == "libnvidia-ml.so" || validVersion) {
				info, err := os.Stat(path)
				if err != nil {
					searchErrors = append(searchErrors, err.Error())
					return nil
				}
				if info.Mode().IsRegular() && !seen[path] {
					paths = append(paths, path)
					seen[path] = true
				}
			}
			return nil
		})
		if err != nil {
			searchErrors = append(searchErrors, err.Error())
		}
	}
	if len(paths) > 0 {
		return paths, nil
	}
	return nil, fmt.Errorf("no NVML library found in %s directories %v (search errors: %s)", legacyLibraryDirsEnv, dirs, strings.Join(searchErrors, "; "))
}
