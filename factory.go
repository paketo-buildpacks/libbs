/*
 * Copyright 2018-2020, VMware, Inc. All Rights Reserved.
 * Proprietary and Confidential.
 * Unauthorized use, copying or distribution of this source code via any medium is
 * strictly prohibited without the express written consent of VMware, Inc.
 */

package libbs

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/buildpacks/libcnb"
	"github.com/paketo-buildpacks/libpak"
	"github.com/paketo-buildpacks/libpak/effect"
	"github.com/paketo-buildpacks/libpak/sherpa"
)

type ApplicationFactory struct {
	Executor effect.Executor
}

func NewApplicationFactory() *ApplicationFactory {
	return &ApplicationFactory{Executor: effect.NewExecutor()}
}

func (f *ApplicationFactory) NewApplication(
	additionalMetadata map[string]interface{},
	arguments []string,
	artifactResolver ArtifactResolver,
	cache Cache,
	command string,
	bom *libcnb.BOM,
	applicationPath string,
) (Application, error) {

	app := Application{
		ApplicationPath:  applicationPath,
		Arguments:        arguments,
		ArtifactResolver: artifactResolver,
		Cache:            cache,
		Command:          command,
		Executor:         f.Executor,
		BOM:              bom,
	}

	expected, err := f.expectedMetadata(additionalMetadata, app)
	if err != nil {
		return Application{}, fmt.Errorf("failed to generate expected metadata\n%w", err)
	}

	app.LayerContributor = libpak.NewLayerContributor("Compiled Application", expected, libcnb.LayerTypes{
		Cache: true,
	})

	return app, nil
}

func (f *ApplicationFactory) expectedMetadata(additionalMetadata map[string]interface{}, app Application) (map[string]interface{}, error) {
	var err error

	metadata := map[string]interface{}{
		"arguments":        app.Arguments,
		"artifact-pattern": app.ArtifactResolver.Pattern(),
	}

	metadata["files"], err = sherpa.NewFileListing(app.ApplicationPath)
	if err != nil {
		return nil, fmt.Errorf("unable to create file listing for %s\n%w", app.ApplicationPath, err)
	}

	metadata["java-version"], err = f.javaVersion()
	if err != nil {
		return nil, fmt.Errorf("unable to determine java version\n%w", err)
	}

	for k, v := range additionalMetadata {
		metadata[k] = v
	}

	return metadata, nil
}

func (f *ApplicationFactory) javaVersion() (string, error) {
	buf := &bytes.Buffer{}

	if err := f.Executor.Execute(effect.Execution{
		Command: "javac",
		Args:    []string{"-version"},
		Stdout:  buf,
		Stderr:  buf,
	}); err != nil {
		return "", fmt.Errorf("error executing 'javac -version':\n Combined Output: %s: \n%w", buf.String(), err)
	}

	s := strings.Split(strings.TrimSpace(buf.String()), " ")
	switch len(s) {
	case 2:
		return s[1], nil
	case 1:
		return s[0], nil
	default:
		return "unknown", nil
	}
}
