/*
 * Copyright 2018-2020, VMware, Inc. All Rights Reserved.
 * Proprietary and Confidential.
 * Unauthorized use, copying or distribution of this source code via any medium is
 * strictly prohibited without the express written consent of VMware, Inc.
 */

package libbs_test

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/paketo-buildpacks/libbs"
	"github.com/sclevine/spec"
)

func testArgumentResolver(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect

		resolver libbs.ArgumentResolver
	)

	it.Before(func() {
		resolver = libbs.ArgumentResolver{
			ConfigurationKey: "TEST_CONFIGURATION_KEY",
			DefaultArguments: []string{"test-argument-1", "test-argument-2"},
		}
	})

	it("uses default arguments", func() {
		Expect(resolver.Resolve()).To(Equal([]string{"test-argument-1", "test-argument-2"}))
	})

	context("$TEST_CONFIGURATION_KEY", func() {

		it.Before(func() {
			Expect(os.Setenv("TEST_CONFIGURATION_KEY", "test-argument-3 test-argument-4")).To(Succeed())
		})

		it.After(func() {
			Expect(os.Unsetenv("TEST_CONFIGURATION_KEY")).To(Succeed())
		})

		it("parses value from $TEST_CONFIGURATION_KEY", func() {
			Expect(resolver.Resolve()).To(Equal([]string{"test-argument-3", "test-argument-4"}))
		})
	})

}
