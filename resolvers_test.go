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

package libbs_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/paketo-buildpacks/libpak"
	"github.com/sclevine/spec"
	"github.com/stretchr/testify/mock"

	"github.com/paketo-buildpacks/libbs"
	"github.com/paketo-buildpacks/libbs/mocks"
)

func testResolvers(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect
	)

	context("AlwaysInterestingFileDetector", func() {
		it("always passes", func() {
			Expect(libbs.AlwaysInterestingFileDetector{}.Interesting("test-path")).To(BeTrue())
		})
	})

	context("JARInterestingFileDetector", func() {
		it("passes for executable JAR", func() {
			Expect(libbs.JARInterestingFileDetector{}.Interesting(filepath.Join("testdata", "stub-executable.jar"))).
				To(BeTrue())
		})

		it("passes for WAR", func() {
			Expect(libbs.JARInterestingFileDetector{}.Interesting(filepath.Join("testdata", "stub-application.war"))).
				To(BeTrue())
		})

		it("fails for non-executable JAR", func() {
			Expect(libbs.JARInterestingFileDetector{}.Interesting(filepath.Join("testdata", "stub-application.jar"))).
				To(BeFalse())
		})
	})

	context("Resolve", func() {
		var (
			detector *mocks.InterestingFileDetector
			path     string
			resolver libbs.ArtifactResolver
		)

		it.Before(func() {
			var err error

			detector = &mocks.InterestingFileDetector{}

			path, err = ioutil.TempDir("", "artifact-resolver")
			Expect(err).NotTo(HaveOccurred())

			resolver = libbs.ArtifactResolver{
				ArtifactConfigurationKey: "TEST_ARTIFACT_CONFIGURATION_KEY",
				ConfigurationResolver: libpak.ConfigurationResolver{
					Configurations: []libpak.BuildpackConfiguration{
						{Name: "TEST_ARTIFACT_CONFIGURATION_KEY", Default: "test-*"},
					},
				},
				ModuleConfigurationKey:  "TEST_MODULE_CONFIGURATION_KEY",
				InterestingFileDetector: detector,
			}
		})

		it.After(func() {
			Expect(os.RemoveAll(path)).To(Succeed())
		})

		it("passes with a single candidate", func() {
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file"), []byte{}, 0644)).To(Succeed())

			Expect(resolver.Resolve(path)).To(Equal(filepath.Join(path, "test-file")))
		})

		it("passes with a single interesting candidate", func() {
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file-1"), []byte{}, 0644)).To(Succeed())
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file-2"), []byte{}, 0644)).To(Succeed())
			detector.On("Interesting", filepath.Join(path, "test-file-1")).Return(false, nil)
			detector.On("Interesting", filepath.Join(path, "test-file-2")).Return(true, nil)

			Expect(resolver.Resolve(path)).To(Equal(filepath.Join(path, "test-file-2")))
		})

		it("fails with zero candidates", func() {
			_, err := resolver.Resolve(path)

			Expect(err).To(MatchError("unable to find single built artifact in test-*, candidates: []"))
		})

		it("fails with multiple candidates", func() {
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file-1"), []byte{}, 0644)).To(Succeed())
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file-2"), []byte{}, 0644)).To(Succeed())
			detector.On("Interesting", mock.Anything).Return(true, nil)

			_, err := resolver.Resolve(path)

			Expect(err).To(MatchError(fmt.Sprintf("unable to find single built artifact in test-*, candidates: [%s %s]",
				filepath.Join(path, "test-file-1"), filepath.Join(path, "test-file-2"))))
		})

		it("fails with multiple candidates and provides extra help", func() {
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file-1"), []byte{}, 0644)).To(Succeed())
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file-2"), []byte{}, 0644)).To(Succeed())
			detector.On("Interesting", mock.Anything).Return(true, nil)

			resolver.AdditionalHelpMessage = "Some more help."

			_, err := resolver.Resolve(path)

			Expect(err).To(MatchError(fmt.Sprintf("unable to find single built artifact in test-*, candidates: [%s %s]. Some more help.",
				filepath.Join(path, "test-file-1"), filepath.Join(path, "test-file-2"))))
		})

		context("$TEST_ARTIFACT_CONFIGURATION_KEY", func() {
			it.Before(func() {
				Expect(os.Setenv("TEST_ARTIFACT_CONFIGURATION_KEY", "another-file")).To(Succeed())
			})

			it.After(func() {
				Expect(os.Unsetenv("TEST_ARTIFACT_CONFIGURATION_KEY")).To(Succeed())
			})

			it("selects candidate from TEST_ARTIFACT_CONFIGURATION_KEY", func() {
				Expect(ioutil.WriteFile(filepath.Join(path, "another-file"), []byte{}, 0644)).To(Succeed())

				Expect(resolver.Resolve(path)).To(Equal(filepath.Join(path, "another-file")))
			})
		})

		context("$TEST_MODULE_CONFIGURATION_KEY", func() {
			it.Before(func() {
				Expect(os.Setenv("TEST_MODULE_CONFIGURATION_KEY", "test-directory")).To(Succeed())
			})

			it.After(func() {
				Expect(os.Unsetenv("TEST_MODULE_CONFIGURATION_KEY")).To(Succeed())
			})

			it("selects candidate from TEST_MODULE_CONFIGURATION_KEY", func() {
				Expect(os.MkdirAll(filepath.Join(path, "test-directory"), 0755)).To(Succeed())
				Expect(ioutil.WriteFile(filepath.Join(path, "test-directory", "test-file"), []byte{}, 0644)).To(Succeed())

				Expect(resolver.Resolve(path)).To(Equal(filepath.Join(path, "test-directory", "test-file")))
			})
		})
	})

	context("ResolveArguments", func() {
		var (
			resolver libpak.ConfigurationResolver
		)

		it.Before(func() {
			resolver = libpak.ConfigurationResolver{
				Configurations: []libpak.BuildpackConfiguration{
					{Name: "TEST_CONFIGURATION_KEY", Default: "test-argument-1 test-argument-2"},
				},
			}
		})

		it("uses default arguments", func() {
			Expect(libbs.ResolveArguments("TEST_CONFIGURATION_KEY", resolver)).
				To(Equal([]string{"test-argument-1", "test-argument-2"}))
		})

		context("$TEST_CONFIGURATION_KEY", func() {

			it.Before(func() {
				Expect(os.Setenv("TEST_CONFIGURATION_KEY", "test-argument-3 test-argument-4")).To(Succeed())
			})

			it.After(func() {
				Expect(os.Unsetenv("TEST_CONFIGURATION_KEY")).To(Succeed())
			})

			it("parses value from $TEST_CONFIGURATION_KEY", func() {
				Expect(libbs.ResolveArguments("TEST_CONFIGURATION_KEY", resolver)).
					To(Equal([]string{"test-argument-3", "test-argument-4"}))
			})
		})
	})

	context("ResolveMany", func() {
		var (
			detector *mocks.InterestingFileDetector
			path     string
			resolver libbs.ArtifactResolver
		)

		it.Before(func() {
			var err error

			detector = &mocks.InterestingFileDetector{}

			path, err = ioutil.TempDir("", "multiple-artifact-resolver")
			Expect(err).NotTo(HaveOccurred())

			resolver = libbs.ArtifactResolver{
				ArtifactConfigurationKey: "TEST_ARTIFACT_CONFIGURATION_KEY",
				ConfigurationResolver: libpak.ConfigurationResolver{
					Configurations: []libpak.BuildpackConfiguration{
						{Name: "TEST_ARTIFACT_CONFIGURATION_KEY", Default: "test-*"},
					},
				},
				ModuleConfigurationKey:  "TEST_MODULE_CONFIGURATION_KEY",
				InterestingFileDetector: detector,
			}
		})

		it.After(func() {
			Expect(os.RemoveAll(path)).To(Succeed())
		})

		it("passes with a single candidate", func() {
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file"), []byte{}, 0644)).To(Succeed())
			Expect(resolver.ResolveMany(path)).To(Equal([]string{filepath.Join(path, "test-file")}))
		})

		it("passes with multiple candidates", func() {
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file"), []byte{}, 0644)).To(Succeed())
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file-1"), []byte{}, 0644)).To(Succeed())

			Expect(resolver.ResolveMany(path)).To(ContainElements(filepath.Join(path, "test-file"), filepath.Join(path, "test-file-1")))
		})

		it("passes with a single folder candidate", func() {
			Expect(os.Mkdir(filepath.Join(path, "test-folder"), os.ModePerm)).To(Succeed())
			Expect(resolver.ResolveMany(path)).To(ContainElement(filepath.Join(path, "test-folder")))
		})

		it("passes with multiple folders", func() {
			Expect(os.Mkdir(filepath.Join(path, "test-folder-1"), os.ModePerm)).To(Succeed())
			Expect(os.Mkdir(filepath.Join(path, "test-folder-2"), os.ModePerm)).To(Succeed())
			Expect(resolver.ResolveMany(path)).To(ContainElements(filepath.Join(path, "test-folder-1"), filepath.Join(path, "test-folder-2")))
		})

		it("passes with a file and a folder", func() {
			Expect(ioutil.WriteFile(filepath.Join(path, "test-file"), []byte{}, 0644)).To(Succeed())
			Expect(os.Mkdir(filepath.Join(path, "test-folder-1"), os.ModePerm)).To(Succeed())
			Expect(resolver.ResolveMany(path)).To(ContainElements(filepath.Join(path, "test-file"), filepath.Join(path, "test-folder-1")))
		})

		it("fails with zero candidates", func() {
			_, err := resolver.ResolveMany(path)

			Expect(err).To(MatchError(HavePrefix("unable to find any built artifacts in test-*, directory contains:")))
		})
	})
}
