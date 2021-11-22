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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/paketo-buildpacks/libpak/sbom"

	"github.com/buildpacks/libcnb"
	"github.com/paketo-buildpacks/libpak/bard"

	. "github.com/onsi/gomega"
	"github.com/paketo-buildpacks/libpak"
	"github.com/paketo-buildpacks/libpak/effect"
	"github.com/paketo-buildpacks/libpak/effect/mocks"
	"github.com/paketo-buildpacks/libpak/sherpa"
	"github.com/sclevine/spec"
	"github.com/stretchr/testify/mock"

	"github.com/paketo-buildpacks/libbs"
)

func testFactory(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect             = NewWithT(t).Expect
		applicationFactory *libbs.ApplicationFactory
		executor           *mocks.Executor
	)

	it.Before(func() {
		executor = &mocks.Executor{}
		applicationFactory = &libbs.ApplicationFactory{
			Executor: executor,
		}
	})

	context("NewApplication", func() {
		var (
			application libbs.Application
			appDir      string
			appFilePath string
		)

		it.Before(func() {
			var err error
			appDir, err = ioutil.TempDir("", "application-application")
			Expect(err).NotTo(HaveOccurred())

			// mock javac version
			executor.On("Execute", mock.Anything).Run(func(args mock.Arguments) {
				execution := args.Get(0).(effect.Execution)
				_, err := execution.Stdout.Write([]byte("javac some-version"))
				Expect(err).NotTo(HaveOccurred())
			}).Return(nil)

			// create app dir and add test file
			appDir, err = ioutil.TempDir("", "application-application")
			Expect(err).NotTo(HaveOccurred())

			file := filepath.Join(appDir, "some-file")
			Expect(ioutil.WriteFile(file, []byte{}, 0644)).To(Succeed())
			appFilePath, err = filepath.EvalSymlinks(file)
			Expect(err).NotTo(HaveOccurred())

			resolver := libbs.ArtifactResolver{
				ConfigurationResolver: libpak.ConfigurationResolver{
					Configurations: []libpak.BuildpackConfiguration{{Default: "*"}},
				},
			}
			bomScanner := sbom.NewSyftCLISBOMScanner(libcnb.Layers{}, executor, bard.Logger{})
			buildpackAPI := "0.7"

			application, err = applicationFactory.NewApplication(
				map[string]interface{}{"addl-key": "addl-value"},
				[]string{"test-argument"},
				resolver,
				libbs.Cache{},
				"",
				nil,
				appDir,
				bomScanner,
				buildpackAPI,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		it.After(func() {
			Expect(os.RemoveAll(appDir)).To(Succeed())
		})

		context("metadata", func() {
			var metadata map[string]interface{}

			it.Before(func() {
				metadata = application.LayerContributor.ExpectedMetadata.(map[string]interface{})
			})

			it("adds file list", func() {
				fileEntries, ok := metadata["files"].([]sherpa.FileEntry)
				Expect(ok).To(BeTrue())
				Expect(fileEntries[0].Path).To(Equal(appFilePath))
			})

			it("adds args", func() {
				Expect(metadata["arguments"]).To(Equal([]string{"test-argument"}))
			})

			it("adds artifact pattern", func() {
				Expect(metadata["artifact-pattern"]).To(Equal("*"))
			})

			it("adds java version", func() {
				Expect(metadata["java-version"]).To(Equal("some-version"))
			})

			it("accepts arbitrary metadata", func() {
				Expect(metadata["addl-key"]).To(Equal("addl-value"))
			})
		})
	})
}
