/*
 * Copyright 2018-2020 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package libbs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/paketo-buildpacks/libpak/sbom"

	"github.com/buildpacks/libcnb"
	"github.com/paketo-buildpacks/libpak"
	"github.com/paketo-buildpacks/libpak/bard"
	"github.com/paketo-buildpacks/libpak/crush"
	"github.com/paketo-buildpacks/libpak/effect"
	"github.com/paketo-buildpacks/libpak/sherpa"
)

type Application struct {
	ApplicationPath  string
	Arguments        []string
	ArtifactResolver ArtifactResolver
	Cache            Cache
	Command          string
	Executor         effect.Executor
	LayerContributor libpak.LayerContributor
	Logger           bard.Logger
	BOM              *libcnb.BOM
	SBOMScanner      sbom.SBOMScanner
}

func (a Application) Contribute(layer libcnb.Layer) (libcnb.Layer, error) {
	a.LayerContributor.Logger = a.Logger

	layer, err := a.LayerContributor.Contribute(layer, func() (libcnb.Layer, error) {
		// Build
		a.Logger.Bodyf("Executing %s %s", filepath.Base(a.Command), strings.Join(a.Arguments, " "))
		if err := a.Executor.Execute(effect.Execution{
			Command: a.Command,
			Args:    a.Arguments,
			Dir:     a.ApplicationPath,
			Stdout:  bard.NewWriter(a.Logger.Logger.InfoWriter(), bard.WithIndent(3)),
			Stderr:  bard.NewWriter(a.Logger.Logger.InfoWriter(), bard.WithIndent(3)),
		}); err != nil {
			return libcnb.Layer{}, fmt.Errorf("error running build\n%w", err)
		}

		// In some cases, process output does not end with a clean line of output
		// This resets the cursor to the beginningo of the next line so indentation lines up
		a.Logger.Info()

		// Persist Artifacts
		artifacts, err := a.ArtifactResolver.ResolveMany(a.ApplicationPath)
		if err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to resolve artifacts\n%w", err)
		}
		a.Logger.Debugf("Found artifacts: %s", artifacts)

		if len(artifacts) == 1 {
			artifact := artifacts[0]

			fileInfo, err := os.Stat(artifact)
			if err != nil {
				return libcnb.Layer{}, fmt.Errorf("unable to resolve artifact %s\n%w", artifact, err)
			}

			if fileInfo.IsDir() {
				if err := copyDirectory(artifact, filepath.Join(layer.Path, filepath.Base(artifact))); err != nil {
					return libcnb.Layer{}, fmt.Errorf("unable to copy the directory\n%w", err)
				}
			} else {
				file := filepath.Join(layer.Path, "application.zip")
				if err := copyFile(artifact, file); err != nil {
					return libcnb.Layer{}, fmt.Errorf("unable to copy the file %s to %s\n%w", artifact, file, err)
				}
			}
		} else {
			for _, artifact := range artifacts {
				fileInfo, err := os.Stat(artifact)
				if err != nil {
					return libcnb.Layer{}, fmt.Errorf("unable to resolve artifact %s\n%w", artifact, err)
				}

				if fileInfo.IsDir() {
					if err := copyDirectory(artifact, filepath.Join(layer.Path, filepath.Base(artifact))); err != nil {
						return libcnb.Layer{}, fmt.Errorf("unable to copy a directory\n%w", err)
					}
				} else {
					dest := filepath.Join(layer.Path, fileInfo.Name())
					if err := copyFile(artifact, dest); err != nil {
						return libcnb.Layer{}, fmt.Errorf("unable to copy a file %s to %s\n%w", artifact, dest, err)
					}
				}
			}
		}

		return layer, nil
	})
	if err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to contribute application layer\n%w", err)
	}

	// Create SBOM
	if err := a.SBOMScanner.ScanBuild(a.ApplicationPath, libcnb.CycloneDXJSON, libcnb.SyftJSON); err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to create Build SBoM \n%w", err)
	}

	entry, err := a.Cache.AsBOMEntry()
	if err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to generate build dependencies\n%w", err)
	}
	entry.Metadata["layer"] = a.Cache.Name()
	a.BOM.Entries = append(a.BOM.Entries, entry)

	// Purge Workspace
	a.Logger.Header("Removing source code")
	cs, err := ioutil.ReadDir(a.ApplicationPath)
	if err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to list children of %s\n%w", a.ApplicationPath, err)
	}
	for _, c := range cs {
		file := filepath.Join(a.ApplicationPath, c.Name())
		if err := os.RemoveAll(file); err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to remove %s\n%w", file, err)
		}
	}

	// Restore compiled artifacts
	file := filepath.Join(layer.Path, "application.zip")
	if _, err := os.Stat(file); err == nil {
		a.Logger.Header("Restoring application artifact")
		in, err := os.Open(file)
		if err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to open %s\n%w", file, err)
		}
		defer in.Close()

		if err := crush.ExtractZip(in, a.ApplicationPath, 0); err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to extract %s\n%w", file, err)
		}
	} else if err != nil && os.IsNotExist(err) {
		a.Logger.Header("Restoring multiple artifacts")
		err := copyDirectory(layer.Path, a.ApplicationPath)
		if err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to restore multiple artifacts\n%w", err)
		}
	} else {
		return libcnb.Layer{}, fmt.Errorf("unable to restore artifacts\n%w", err)
	}

	return layer, nil
}

func (Application) Name() string {
	return "application"
}

func copyDirectory(from, to string) error {
	files, err := ioutil.ReadDir(from)
	if err != nil {
		return err
	}

	for _, file := range files {
		sourcePath := filepath.Join(from, file.Name())
		destPath := filepath.Join(to, file.Name())

		fileInfo, err := os.Stat(sourcePath)
		if err != nil {
			return err
		}

		if fileInfo.IsDir() {
			if err := copyDirectory(sourcePath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(sourcePath, destPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(from string, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return fmt.Errorf("unable to open file%s\n%w", from, err)
	}
	defer in.Close()

	if err := sherpa.CopyFile(in, to); err != nil {
		return fmt.Errorf("unable to copy %s to %s\n%w", from, to, err)
	}

	return nil
}
