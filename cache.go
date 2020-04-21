/*
 * Copyright 2018-2020, VMware, Inc. All Rights Reserved.
 * Proprietary and Confidential.
 * Unauthorized use, copying or distribution of this source code via any medium is
 * strictly prohibited without the express written consent of VMware, Inc.
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

func (c *Cache) AsBuildpackPlanEntry() (libcnb.BuildpackPlanEntry, error) {
	d, err := libjvm.NewMavenJARListing(c.Path)
	if err != nil {
		return libcnb.BuildpackPlanEntry{}, fmt.Errorf("unable to generate dependencies from %s\n%w", c.Path, err)
	}

	return libcnb.BuildpackPlanEntry{
		Name:     "build-dependencies",
		Metadata: map[string]interface{}{"dependencies": d},
	}, nil
}

func (Cache) Name() string {
	return "cache"
}
