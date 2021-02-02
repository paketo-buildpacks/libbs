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
	"os"
	"path/filepath"

	"github.com/buildpacks/libcnb"
	"github.com/paketo-buildpacks/libjvm"
	"github.com/paketo-buildpacks/libpak/bard"
)

type Cache struct {
	Logger bard.Logger
	Path   string
}

func (c Cache) Contribute(layer libcnb.Layer) (libcnb.Layer, error) {
	if err := os.MkdirAll(layer.Path, 0755); err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to create layer directory %s\n%w", layer.Path, err)
	}

	file := filepath.Dir(c.Path)
	if err := os.MkdirAll(file, 0755); err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to create directory %s\n%w", file, err)
	}

	if err := os.Symlink(layer.Path, c.Path); os.IsExist(err) {
		c.Logger.Body("Cache already exists")
	} else if err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to link cache from %s to %s\n%w", layer.Path, c.Path, err)
	} else {
		c.Logger.Bodyf("Creating cache directory %s", c.Path)
	}

	layer.Cache = true
	return layer, nil
}

func (c *Cache) AsBOMEntry() (libcnb.BOMEntry, error) {
	d, err := libjvm.NewMavenJARListing(c.Path)
	if err != nil {
		return libcnb.BOMEntry{}, fmt.Errorf("unable to generate dependencies from %s\n%w", c.Path, err)
	}

	return libcnb.BOMEntry{
		Name:     "build-dependencies",
		Metadata: map[string]interface{}{"dependencies": d},
		Build:    true,
	}, nil
}

func (Cache) Name() string {
	return "cache"
}
