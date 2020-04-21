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
	"strings"

	"github.com/mattn/go-shellwords"
	"github.com/paketo-buildpacks/libpak/bard"
)

// ArgumentResolver provides functionality for resolving build system arguments.
type ArgumentResolver struct {

	// ConfigurationKey is the environment variable key to lookup for user configured arguments.
	ConfigurationKey string

	// DefaultArguments are the default arguments to use if the user has not configured any.
	DefaultArguments []string
}

// NewArgumentResolver creates a new instance, logging the user configuration key and default value.
func NewArgumentResolver(configurationKey string, defaultArguments []string, logger bard.Logger) ArgumentResolver {
	logger.Body(bard.FormatUserConfig(configurationKey, "the arguments passed to the build system", strings.Join(defaultArguments, " ")))

	return ArgumentResolver{
		ConfigurationKey: configurationKey,
		DefaultArguments: defaultArguments,
	}
}

// Resolve resolves the arguments that should be passed to a build system.
func (a *ArgumentResolver) Resolve() ([]string, error) {
	if s, ok := os.LookupEnv(a.ConfigurationKey); ok {
		a, err := shellwords.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("unable to parse arguments from %s\n%w", s, err)
		}

		return a, nil
	}

	return a.DefaultArguments, nil
}
