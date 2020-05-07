/*
 * Copyright 2018-2020, VMware, Inc. All Rights Reserved.
 * Proprietary and Confidential.
 * Unauthorized use, copying or distribution of this source code via any medium is
 * strictly prohibited without the express written consent of VMware, Inc.
 */

package libbs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

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
	Plan             *libcnb.BuildpackPlan
}

func NewApplication(applicationPath string, arguments []string, artifactResolver ArtifactResolver, cache Cache,
	command string, plan *libcnb.BuildpackPlan) (Application, error) {

	l, err := sherpa.NewFileListing(applicationPath)
	if err != nil {
		return Application{}, fmt.Errorf("unable to create file listing for %s\n%w", applicationPath, err)
	}
	expected := map[string][]sherpa.FileEntry{"files": l}

	return Application{
		ApplicationPath:  applicationPath,
		Arguments:        arguments,
		ArtifactResolver: artifactResolver,
		Cache:            cache,
		Command:          command,
		Executor:         effect.NewExecutor(),
		LayerContributor: libpak.NewLayerContributor("Compiled Application", expected),
		Plan:             plan,
	}, nil
}

func (a Application) Contribute(layer libcnb.Layer) (libcnb.Layer, error) {
	a.LayerContributor.Logger = a.Logger

	layer, err := a.LayerContributor.Contribute(layer, func() (libcnb.Layer, error) {
		a.Logger.Bodyf("Executing %s %s", filepath.Base(a.Command), strings.Join(a.Arguments, " "))
		if err := a.Executor.Execute(effect.Execution{
			Command: a.Command,
			Args:    a.Arguments,
			Dir:     a.ApplicationPath,
			Stdout:  a.Logger.InfoWriter(),
			Stderr:  a.Logger.InfoWriter(),
		}); err != nil {
			return libcnb.Layer{}, fmt.Errorf("error running build\n%w", err)
		}

		artifact, err := a.ArtifactResolver.Resolve(a.ApplicationPath)
		if err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to resolve artifact\n%w", err)
		}

		in, err := os.Open(artifact)
		if err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to open %s\n%w", artifact, err)
		}
		defer in.Close()

		file := filepath.Join(layer.Path, "application.zip")
		if err := sherpa.CopyFile(in, file); err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to copy %s to %s\n%w", artifact, file, err)
		}

		layer.Cache = true
		return layer, nil
	})
	if err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to contribute application layer\n%w", err)
	}

	entry, err := a.Cache.AsBuildpackPlanEntry()
	if err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to generate build dependencies\n%w", err)
	}
	a.Plan.Entries = append(a.Plan.Entries, entry)

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

	file := filepath.Join(layer.Path, "application.zip")
	in, err := os.Open(file)
	if err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to open %s\n%w", file, err)
	}
	defer in.Close()

	if err := crush.ExtractZip(in, a.ApplicationPath, 0); err != nil {
		return libcnb.Layer{}, fmt.Errorf("unable to extract %s\n%w", file, err)
	}

	return layer, nil
}

func (Application) Name() string {
	return "application"
}
