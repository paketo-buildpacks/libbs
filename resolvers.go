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
	"archive/zip"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"

	"github.com/magiconair/properties"
	"github.com/mattn/go-shellwords"
	"github.com/paketo-buildpacks/libpak"
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

	// ConfigurationResolver is the configuration resolver to use.
	ConfigurationResolver libpak.ConfigurationResolver

	// ModuleConfigurationKey is the environment variable key to lookup for user configured modules.
	ModuleConfigurationKey string

	// InterestingFileDetector is used to determine if a file is a candidate for artifact resolution.
	InterestingFileDetector InterestingFileDetector

	// AdditionalHelpMessage can be used to supply context specific instructions if no matching artifact is found
	AdditionalHelpMessage string
}

// Pattern returns the space separated list of globs that ArtifactResolver will use for resolution.
func (a *ArtifactResolver) Pattern() string {
	pattern, ok := a.ConfigurationResolver.Resolve(a.ArtifactConfigurationKey)
	if ok {
		return pattern
	}
	if module, ok := a.ConfigurationResolver.Resolve(a.ModuleConfigurationKey); ok {
		return filepath.Join(module, pattern)
	}
	return pattern
}

// Resolve resolves the artifact that was created by the build system.
func (a *ArtifactResolver) Resolve(applicationPath string) (string, error) {
	pattern := a.Pattern()
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
	helpMsg := fmt.Sprintf("unable to find single built artifact in %s, candidates: %s", pattern, candidates)
	if len(a.AdditionalHelpMessage) > 0 {
		helpMsg = fmt.Sprintf("%s. %s", helpMsg, a.AdditionalHelpMessage)
	}
	return "", fmt.Errorf("%s", helpMsg)
}

func (a *ArtifactResolver) ResolveMany(applicationPath string) ([]string, error) {
	pattern := a.Pattern()

	patterns, err := shellwords.Parse(pattern)
	if err != nil {
		return []string{}, fmt.Errorf("unable to parse shellwords patterns\n%w", err)
	}

	var candidates []string
	var badPatterns []string
	for _, pattern := range patterns {
		file := filepath.Join(applicationPath, pattern)
		cs, err := filepath.Glob(file)
		if err != nil {
			// err will only be ErrBadPattern / "syntax error in pattern"
			badPatterns = append(badPatterns, pattern)
		}
		candidates = append(candidates, cs...)
	}

	if len(badPatterns) > 0 {
		return []string{}, fmt.Errorf("unable to proceed due to bad pattern(s):\n%s", strings.Join(badPatterns, "\n"))
	}

	if len(candidates) > 0 {
		return candidates, nil
	}

	helpMsg := fmt.Sprintf("unable to find any built artifacts for pattern(s):\n%s", strings.Join(patterns, "\n"))
	if len(a.AdditionalHelpMessage) > 0 {
		helpMsg = fmt.Sprintf("%s. %s", helpMsg, a.AdditionalHelpMessage)
	}
	return []string{}, fmt.Errorf("%s", helpMsg)
}

// ResolveArguments resolves the arguments that should be passed to a build system.
func ResolveArguments(configurationKey string, configurationResolver libpak.ConfigurationResolver) ([]string, error) {
	s, _ := configurationResolver.Resolve(configurationKey)
	w, err := shellwords.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("unable to parse arguments from %s\n%w", s, err)
	}

	return w, nil
}
