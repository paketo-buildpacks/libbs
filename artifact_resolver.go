/*
 * Copyright 2018-2020, VMware, Inc. All Rights Reserved.
 * Proprietary and Confidential.
 * Unauthorized use, copying or distribution of this source code via any medium is
 * strictly prohibited without the express written consent of VMware, Inc.
 */

package libbs

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/magiconair/properties"
	"github.com/paketo-buildpacks/libpak/bard"
)

//go:generate mockery -name InterestingFileDetector -case=underscore

// InterestingFileDetector is an interface for types that determine whether a given file is interesting.
type InterestingFileDetector interface {

	// Interesting determines if a path is an interesting path for consideration.
	Interesting(path string) (bool, error)
}

// AlwaysInterestingFileDetector is an implementation of InterestingFileDetector that always returns true, indicating
// that all files are interesting.
type AlwaysInterestingFileDetector struct{}

func (AlwaysInterestingFileDetector) Interesting(path string) (bool, error) {
	return true, nil
}

// JARInterestingFileDetector is an implementation of InterestingFileDetector that returns true if the path represents
// a JAR file with a Main-Class manifest entry or a WAR file with a WEB-INF/ directory.
type JARInterestingFileDetector struct{}

func (j JARInterestingFileDetector) Interesting(path string) (bool, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return false, fmt.Errorf("unable to open %s\n%w", path, err)
	}
	defer z.Close()

	for _, f := range z.File {
		if i, err := j.entry(f); err != nil {
			return false, fmt.Errorf("unable to investigate entry %s/%s\n%w", path, f.Name, err)
		} else if i {
			return true, nil
		}
	}

	return false, nil
}

func (JARInterestingFileDetector) entry(f *zip.File) (bool, error) {
	if f.Name == "WEB-INF/" && f.FileInfo().IsDir() {
		return true, nil
	}

	if f.Name == "META-INF/MANIFEST.MF" {
		m, err := f.Open()
		if err != nil {
			return false, fmt.Errorf("unable to open %s\n%w", f.Name, err)
		}
		defer m.Close()

		b, err := ioutil.ReadAll(m)
		if err != nil {
			return false, fmt.Errorf("unable to read %s\n%w", f.Name, err)
		}

		p, err := properties.Load(b, properties.UTF8)
		if err != nil {
			return false, fmt.Errorf("unable to parse properties in %s\n%w", f.Name, err)
		}

		if _, ok := p.Get("Main-Class"); ok {
			return true, nil
		}
	}

	return false, nil
}

// ArtifactResolver provides functionality for resolve build system built artifacts.
type ArtifactResolver struct {

	// ArtifactConfigurationKey is the environment variable key to lookup for user configured artifacts.
	ArtifactConfigurationKey string

	// ModuleConfigurationKey is the environment variable key to lookup for user configured modules.
	ModuleConfigurationKey string

	// DefaultArtifact is the default artifact pattern to use if the user has not configured any.
	DefaultArtifact string

	// InterestingFileDetector is used to determine if a file is a candidate for artifact resolution.
	InterestingFileDetector InterestingFileDetector
}

// NewArtifactResolver creates a new instance, logging the user configuration keys and default values.  The instance
// uses the AlwaysInterestingFileDetector.
func NewArtifactResolver(artifactConfigurationKey string, moduleConfigurationKey string, defaultArtifact string, logger bard.Logger) ArtifactResolver {
	logger.Body(bard.FormatUserConfig(moduleConfigurationKey, "the module to find application artifact in", "<ROOT>"))
	logger.Body(bard.FormatUserConfig(artifactConfigurationKey, "the built application artifact", defaultArtifact))

	return ArtifactResolver{
		ArtifactConfigurationKey: artifactConfigurationKey,
		ModuleConfigurationKey:   moduleConfigurationKey,
		DefaultArtifact:          defaultArtifact,
		InterestingFileDetector:  AlwaysInterestingFileDetector{},
	}
}

// Resolve resolves the artifact that was created by the build system.
func (a *ArtifactResolver) Resolve(applicationPath string) (string, error) {
	pattern := a.DefaultArtifact
	if s, ok := os.LookupEnv(a.ModuleConfigurationKey); ok {
		pattern = filepath.Join(s, pattern)
	}
	if s, ok := os.LookupEnv(a.ArtifactConfigurationKey); ok {
		pattern = s
	}

	file := filepath.Join(applicationPath, pattern)
	candidates, err := filepath.Glob(file)
	if err != nil {
		return "", fmt.Errorf("unable to find files with %s\n%w", pattern, err)
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	var artifacts []string
	for _, c := range candidates {
		if ok, err := a.InterestingFileDetector.Interesting(c); err != nil {
			return "", fmt.Errorf("unable to investigate %s\n%w", c, err)
		} else if ok {
			artifacts = append(artifacts, c)
		}
	}

	if len(artifacts) == 1 {
		return artifacts[0], nil
	}

	sort.Strings(artifacts)
	return "", fmt.Errorf("unable to find built artifact in %s, candidates: %s", pattern, candidates)
}
