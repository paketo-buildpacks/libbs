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
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/libcnb"
	. "github.com/onsi/gomega"
	"github.com/paketo-buildpacks/libjvm"
	"github.com/paketo-buildpacks/libpak"
	"github.com/paketo-buildpacks/libpak/bard"
	"github.com/paketo-buildpacks/libpak/effect"
	"github.com/paketo-buildpacks/libpak/effect/mocks"
	sbomMocks "github.com/paketo-buildpacks/libpak/sbom/mocks"
	"github.com/sclevine/spec"
	"github.com/stretchr/testify/mock"

	"github.com/paketo-buildpacks/libbs"
)

func testApplication(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect

		cache       libbs.Cache
		ctx         libcnb.BuildContext
		application libbs.Application
		executor    *mocks.Executor
		bom         *libcnb.BOM
		sbomScanner *sbomMocks.SBOMScanner
	)

	it.Before(func() {
		var err error

		ctx.Application.Path, err = ioutil.TempDir("", "application-application")
		Expect(err).NotTo(HaveOccurred())

		ctx.Layers.Path, err = ioutil.TempDir("", "application-layers")
		Expect(err).NotTo(HaveOccurred())

		cache.Path, err = ioutil.TempDir("", "application-cache")
		Expect(err).NotTo(HaveOccurred())

		bom = &libcnb.BOM{}

		artifactResolver := libbs.ArtifactResolver{
			ConfigurationResolver: libpak.ConfigurationResolver{
				Configurations: []libpak.BuildpackConfiguration{{Default: "*"}},
			},
		}

		executor = &mocks.Executor{}
		sbomScanner = &sbomMocks.SBOMScanner{}
		sbomScanner.On("ScanBuild", ctx.Application.Path, libcnb.CycloneDXJSON, libcnb.SyftJSON).Return(nil)

		application = libbs.Application{
			ApplicationPath:  ctx.Application.Path,
			Arguments:        []string{"test-argument"},
			ArtifactResolver: artifactResolver,
			Cache:            cache,
			Command:          "test-command",
			Executor:         executor,
			LayerContributor: libpak.NewLayerContributor(
				"test",
				map[string]interface{}{},
				libcnb.LayerTypes{Cache: true},
			),
			Logger:      bard.Logger{},
			BOM:         bom,
			SBOMScanner: sbomScanner,
		}
	})

	it.After(func() {
		Expect(os.RemoveAll(ctx.Application.Path)).To(Succeed())
		Expect(os.RemoveAll(ctx.Layers.Path)).To(Succeed())
		Expect(os.RemoveAll(cache.Path)).To(Succeed())
	})

	it("contributes layer", func() {
		in, err := os.Open(filepath.Join("testdata", "stub-application.jar"))
		Expect(err).NotTo(HaveOccurred())
		out, err := os.OpenFile(filepath.Join(ctx.Application.Path, "stub-application.jar"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		Expect(err).NotTo(HaveOccurred())
		_, err = io.Copy(out, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(in.Close()).To(Succeed())
		Expect(out.Close()).To(Succeed())
		Expect(ioutil.WriteFile(filepath.Join(cache.Path, "test-file-1.1.1.jar"), []byte{}, 0644)).To(Succeed())

		application.Logger = bard.NewLogger(ioutil.Discard)
		executor.On("Execute", mock.Anything).Return(nil)

		layer, err := ctx.Layers.Layer("test-layer")
		Expect(err).NotTo(HaveOccurred())

		layer, err = application.Contribute(layer)

		Expect(err).NotTo(HaveOccurred())

		Expect(layer.Cache).To(BeTrue())

		e := executor.Calls[0].Arguments[0].(effect.Execution)
		Expect(e.Command).To(Equal("test-command"))
		Expect(e.Args).To(Equal([]string{"test-argument"}))
		Expect(e.Dir).To(Equal(ctx.Application.Path))
		Expect(e.Stdout).NotTo(BeNil())
		Expect(e.Stderr).NotTo(BeNil())

		Expect(filepath.Join(layer.Path, "application.zip")).To(BeARegularFile())
		Expect(filepath.Join(ctx.Application.Path, "stub-application.jar")).NotTo(BeAnExistingFile())
		Expect(filepath.Join(ctx.Application.Path, "fixture-marker")).To(BeARegularFile())

		sbomScanner.AssertCalled(t, "ScanBuild", ctx.Application.Path, libcnb.CycloneDXJSON, libcnb.SyftJSON)
		Expect(bom.Entries).To(HaveLen(1))
		Expect(bom.Entries).To(Equal([]libcnb.BOMEntry{
			{
				Name: "build-dependencies",
				Metadata: map[string]interface{}{
					"layer": "cache",
					"dependencies": []libjvm.MavenJAR{
						{
							Name:    "test-file",
							Version: "1.1.1",
							SHA256:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
						},
					},
				},
				Launch: false,
				Build:  true,
			},
		}))
	})

	context("label-based BOM is suppressed", func() {
		it.Before(func() {
			Expect(os.Setenv("BP_BOM_LABEL_DISABLED", "true")).To(Succeed())
		})

		it.After(func() {
			Expect(os.Unsetenv("BP_BOM_LABEL_DISABLED")).To(Succeed())
		})

		it("contributes layer", func() {
			in, err := os.Open(filepath.Join("testdata", "stub-application.jar"))
			Expect(err).NotTo(HaveOccurred())
			out, err := os.OpenFile(filepath.Join(ctx.Application.Path, "stub-application.jar"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			Expect(err).NotTo(HaveOccurred())
			_, err = io.Copy(out, in)
			Expect(err).NotTo(HaveOccurred())
			Expect(in.Close()).To(Succeed())
			Expect(out.Close()).To(Succeed())
			Expect(ioutil.WriteFile(filepath.Join(cache.Path, "test-file-1.1.1.jar"), []byte{}, 0644)).To(Succeed())

			application.Logger = bard.NewLogger(ioutil.Discard)
			executor.On("Execute", mock.Anything).Return(nil)

			layer, err := ctx.Layers.Layer("test-layer")
			Expect(err).NotTo(HaveOccurred())

			layer, err = application.Contribute(layer)

			Expect(err).NotTo(HaveOccurred())

			Expect(layer.Cache).To(BeTrue())

			e := executor.Calls[0].Arguments[0].(effect.Execution)
			Expect(e.Command).To(Equal("test-command"))
			Expect(e.Args).To(Equal([]string{"test-argument"}))
			Expect(e.Dir).To(Equal(ctx.Application.Path))
			Expect(e.Stdout).NotTo(BeNil())
			Expect(e.Stderr).NotTo(BeNil())

			Expect(filepath.Join(layer.Path, "application.zip")).To(BeARegularFile())
			Expect(filepath.Join(ctx.Application.Path, "stub-application.jar")).NotTo(BeAnExistingFile())
			Expect(filepath.Join(ctx.Application.Path, "fixture-marker")).To(BeARegularFile())

			sbomScanner.AssertCalled(t, "ScanBuild", ctx.Application.Path, libcnb.CycloneDXJSON, libcnb.SyftJSON)
			Expect(bom.Entries).To(HaveLen(0))
		})
	})

	context("contributes layer with ", func() {
		context("folder with multiple files", func() {
			it.Before(func() {
				folder := filepath.Join(ctx.Application.Path, "target", "native-sources")
				os.MkdirAll(folder, os.ModePerm)

				files := []string{"stub-application.jar", "stub-executable.jar"}
				for _, file := range files {
					in, err := os.Open(filepath.Join("testdata", file))
					Expect(err).NotTo(HaveOccurred())

					out, err := os.OpenFile(filepath.Join(folder, file), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
					Expect(err).NotTo(HaveOccurred())

					_, err = io.Copy(out, in)
					Expect(err).NotTo(HaveOccurred())
					Expect(in.Close()).To(Succeed())
					Expect(out.Close()).To(Succeed())
				}
				Expect(os.WriteFile(filepath.Join(ctx.Application.Path, "random-file"), []byte(""), 0644)).To(BeNil())
				Expect(os.Symlink(filepath.Join(ctx.Application.Path, "random-file"), filepath.Join(ctx.Application.Path, "random-link"))).To(BeNil())
			})

			it("matches multiple files", func() {
				artifactResolver := libbs.ArtifactResolver{
					ConfigurationResolver: libpak.ConfigurationResolver{
						Configurations: []libpak.BuildpackConfiguration{{Default: "target/native-sources/*.jar"}},
					},
				}
				application.ArtifactResolver = artifactResolver

				application.Logger = bard.NewLogger(ioutil.Discard)
				executor.On("Execute", mock.Anything).Return(nil)

				layer, err := ctx.Layers.Layer("test-layer")
				Expect(err).NotTo(HaveOccurred())

				layer, err = application.Contribute(layer)

				Expect(err).NotTo(HaveOccurred())

				e := executor.Calls[0].Arguments[0].(effect.Execution)
				Expect(e.Command).To(Equal("test-command"))
				Expect(e.Args).To(Equal([]string{"test-argument"}))
				Expect(e.Dir).To(Equal(ctx.Application.Path))
				Expect(e.Stdout).NotTo(BeNil())
				Expect(e.Stderr).NotTo(BeNil())

				Expect(filepath.Join(layer.Path, "application.zip")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "stub-application.jar")).To(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "stub-executable.jar")).To(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "random-file")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "random-link")).NotTo(BeAnExistingFile())
			})

			it("matches a folder", func() {
				artifactResolver := libbs.ArtifactResolver{
					ConfigurationResolver: libpak.ConfigurationResolver{
						Configurations: []libpak.BuildpackConfiguration{{Default: "target/native-sources"}},
					},
				}
				application.ArtifactResolver = artifactResolver

				application.Logger = bard.NewLogger(ioutil.Discard)
				executor.On("Execute", mock.Anything).Return(nil)

				layer, err := ctx.Layers.Layer("test-layer")
				Expect(err).NotTo(HaveOccurred())

				layer, err = application.Contribute(layer)

				Expect(err).NotTo(HaveOccurred())

				e := executor.Calls[0].Arguments[0].(effect.Execution)
				Expect(e.Command).To(Equal("test-command"))
				Expect(e.Args).To(Equal([]string{"test-argument"}))
				Expect(e.Dir).To(Equal(ctx.Application.Path))
				Expect(e.Stdout).NotTo(BeNil())
				Expect(e.Stderr).NotTo(BeNil())

				Expect(filepath.Join(layer.Path, "application.zip")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-application.jar")).To(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-executable.jar")).To(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "random-file")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "random-link")).NotTo(BeAnExistingFile())
			})

			context("source-removal vars are set", func() {
				it("does not remove the included files", func() {
					artifactResolver := libbs.ArtifactResolver{
						ConfigurationResolver: libpak.ConfigurationResolver{
							Configurations: []libpak.BuildpackConfiguration{{Default: "target/native-sources"}, {Name: "BP_INCLUDE_FILES", Default: "random-*"}},
						},
					}
					application.ArtifactResolver = artifactResolver

					application.Logger = bard.NewLogger(ioutil.Discard)
					executor.On("Execute", mock.Anything).Return(nil)

					layer, err := ctx.Layers.Layer("test-layer")
					Expect(err).NotTo(HaveOccurred())

					layer, err = application.Contribute(layer)

					Expect(err).NotTo(HaveOccurred())

					e := executor.Calls[0].Arguments[0].(effect.Execution)
					Expect(e.Command).To(Equal("test-command"))
					Expect(e.Args).To(Equal([]string{"test-argument"}))
					Expect(e.Dir).To(Equal(ctx.Application.Path))
					Expect(e.Stdout).NotTo(BeNil())
					Expect(e.Stderr).NotTo(BeNil())

					Expect(filepath.Join(layer.Path, "application.zip")).NotTo(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-application.jar")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-executable.jar")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "random-file")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "random-link")).To(BeAnExistingFile())
				})

				it("removes the excluded files", func() {
					artifactResolver := libbs.ArtifactResolver{
						ConfigurationResolver: libpak.ConfigurationResolver{
							Configurations: []libpak.BuildpackConfiguration{
								{Default: "target/native-sources"},
								{Name: "BP_INCLUDE_FILES", Default: "random-*"},
								{Name: "BP_EXCLUDE_FILES", Default: "random-link"},
							},
						},
					}
					application.ArtifactResolver = artifactResolver

					application.Logger = bard.NewLogger(ioutil.Discard)
					executor.On("Execute", mock.Anything).Return(nil)

					layer, err := ctx.Layers.Layer("test-layer")
					Expect(err).NotTo(HaveOccurred())

					layer, err = application.Contribute(layer)

					Expect(err).NotTo(HaveOccurred())

					e := executor.Calls[0].Arguments[0].(effect.Execution)
					Expect(e.Command).To(Equal("test-command"))
					Expect(e.Args).To(Equal([]string{"test-argument"}))
					Expect(e.Dir).To(Equal(ctx.Application.Path))
					Expect(e.Stdout).NotTo(BeNil())
					Expect(e.Stderr).NotTo(BeNil())

					Expect(filepath.Join(layer.Path, "application.zip")).NotTo(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-application.jar")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-executable.jar")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "random-file")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "random-link")).NotTo(BeAnExistingFile())
				})

				it("does not cause issues if all files are included", func() {
					artifactResolver := libbs.ArtifactResolver{
						ConfigurationResolver: libpak.ConfigurationResolver{
							Configurations: []libpak.BuildpackConfiguration{
								{Default: "target/native-sources"},
								{Name: "BP_INCLUDE_FILES", Default: "*"},
							},
						},
					}
					application.ArtifactResolver = artifactResolver

					application.Logger = bard.NewLogger(ioutil.Discard)
					executor.On("Execute", mock.Anything).Return(nil)

					layer, err := ctx.Layers.Layer("test-layer")
					Expect(err).NotTo(HaveOccurred())

					layer, err = application.Contribute(layer)

					Expect(err).NotTo(HaveOccurred())

					e := executor.Calls[0].Arguments[0].(effect.Execution)
					Expect(e.Command).To(Equal("test-command"))
					Expect(e.Args).To(Equal([]string{"test-argument"}))
					Expect(e.Dir).To(Equal(ctx.Application.Path))
					Expect(e.Stdout).NotTo(BeNil())
					Expect(e.Stderr).NotTo(BeNil())

					Expect(filepath.Join(layer.Path, "application.zip")).NotTo(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-application.jar")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-executable.jar")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "random-file")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "random-link")).To(BeAnExistingFile())
				})

				it("keeps interesting files even if all files are excluded", func() {
					artifactResolver := libbs.ArtifactResolver{
						ConfigurationResolver: libpak.ConfigurationResolver{
							Configurations: []libpak.BuildpackConfiguration{
								{Default: "target/native-sources"},
								{Name: "BP_EXCLUDE_FILES", Default: "*"},
							},
						},
					}
					application.ArtifactResolver = artifactResolver

					application.Logger = bard.NewLogger(ioutil.Discard)
					executor.On("Execute", mock.Anything).Return(nil)

					layer, err := ctx.Layers.Layer("test-layer")
					Expect(err).NotTo(HaveOccurred())

					layer, err = application.Contribute(layer)

					Expect(err).NotTo(HaveOccurred())

					e := executor.Calls[0].Arguments[0].(effect.Execution)
					Expect(e.Command).To(Equal("test-command"))
					Expect(e.Args).To(Equal([]string{"test-argument"}))
					Expect(e.Dir).To(Equal(ctx.Application.Path))
					Expect(e.Stdout).NotTo(BeNil())
					Expect(e.Stderr).NotTo(BeNil())

					Expect(filepath.Join(layer.Path, "application.zip")).NotTo(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-application.jar")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-executable.jar")).To(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "random-file")).NotTo(BeAnExistingFile())
					Expect(filepath.Join(ctx.Application.Path, "random-link")).NotTo(BeAnExistingFile())
				})
			})
		})

		context("multiple folders", func() {
			it.Before(func() {
				folder := filepath.Join(ctx.Application.Path, "target", "native-sources")
				os.MkdirAll(folder, os.ModePerm)

				files := []string{"stub-application.jar", "stub-executable.jar"}
				for _, file := range files {
					in, err := os.Open(filepath.Join("testdata", file))
					Expect(err).NotTo(HaveOccurred())

					out, err := os.OpenFile(filepath.Join(folder, file), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
					Expect(err).NotTo(HaveOccurred())

					_, err = io.Copy(out, in)
					Expect(err).NotTo(HaveOccurred())
					Expect(in.Close()).To(Succeed())
					Expect(out.Close()).To(Succeed())
				}

				folder = filepath.Join(ctx.Application.Path, "target", "code-sources")
				os.MkdirAll(folder, os.ModePerm)

				files = []string{"stub-application.jar", "stub-executable.jar"}
				for _, file := range files {
					in, err := os.Open(filepath.Join("testdata", file))
					Expect(err).NotTo(HaveOccurred())

					out, err := os.OpenFile(filepath.Join(folder, fmt.Sprintf("source-%s", file)), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
					Expect(err).NotTo(HaveOccurred())

					_, err = io.Copy(out, in)
					Expect(err).NotTo(HaveOccurred())
					Expect(in.Close()).To(Succeed())
					Expect(out.Close()).To(Succeed())
				}
			})

			it("matches multiple folders", func() {
				artifactResolver := libbs.ArtifactResolver{
					ConfigurationResolver: libpak.ConfigurationResolver{
						Configurations: []libpak.BuildpackConfiguration{{Default: "target/*"}},
					},
				}
				application.ArtifactResolver = artifactResolver

				application.Logger = bard.NewLogger(ioutil.Discard)
				executor.On("Execute", mock.Anything).Return(nil)

				layer, err := ctx.Layers.Layer("test-layer")
				Expect(err).NotTo(HaveOccurred())

				layer, err = application.Contribute(layer)

				Expect(err).NotTo(HaveOccurred())

				e := executor.Calls[0].Arguments[0].(effect.Execution)
				Expect(e.Command).To(Equal("test-command"))
				Expect(e.Args).To(Equal([]string{"test-argument"}))
				Expect(e.Dir).To(Equal(ctx.Application.Path))
				Expect(e.Stdout).NotTo(BeNil())
				Expect(e.Stderr).NotTo(BeNil())

				Expect(filepath.Join(layer.Path, "application.zip")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-application.jar")).To(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "native-sources", "stub-executable.jar")).To(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "code-sources", "source-stub-application.jar")).To(BeAnExistingFile())
				Expect(filepath.Join(ctx.Application.Path, "code-sources", "source-stub-executable.jar")).To(BeAnExistingFile())
			})
		})
	})
}
