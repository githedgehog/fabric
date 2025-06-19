#
# Version handling
#

version_extra := ""
version_dirty := `[ -z "$(git status -s)" ] || echo "-$(LC_ALL=C tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c 2)$(date +"%H%M")"`
version := `git describe --tags --dirty --always` + version_dirty + version_extra

# Print version that will be used in the build
version:
  @echo "Using version: {{version}}"

_is_clean:
  [ -z "`git status -s`" ] || exit 1

#
# Downloading all tools locally
#

localbin := "bin"
localpath := `pwd`
localbinpath := `pwd`/localbin

_localbin:
  @mkdir -p {{localbin}}

# Download all tools locally
tools: _kustomize _envtest _golangci_lint _butane _addlicense _crd_ref_docs _helm _helmify _oras _actionlint _skopeo _gcov2lcov

# kustomize
kustomize_version := "v5.4.3"
kustomize := localbin / "kustomize" + "-" + kustomize_version
@_kustomize: _localbin
  [ -f {{kustomize}} ] || just _goinstall "sigs.k8s.io/kustomize/kustomize/v5" {{kustomize_version}} "kustomize" {{kustomize}}

# controller-gen
controller_gen := "go tool controller-gen"

# envtest
envtest_k8s_version := "1.31.0"
envtest_version := "release-0.19"
envtest := localbin / "setup-envtest" + "-" + envtest_version
@_envtest: _localbin
  [ -f {{envtest}} ] || just _goinstall "sigs.k8s.io/controller-runtime/tools/setup-envtest" {{envtest_version}} "setup-envtest" {{envtest}}

# golangci-lint, keep version in sync with ci.yaml
golangci_lint_version := "v1.64.8"
golangci_lint := localbin / "golangci-lint" + "-" + golangci_lint_version
@_golangci_lint: _localbin
  [ -f {{golangci_lint}} ] || just _goinstall "github.com/golangci/golangci-lint/cmd/golangci-lint" {{golangci_lint_version}} "golangci-lint" {{golangci_lint}}

# butane
butane_version := "v0.23.0"
butane := localbin / "butane" + "-" + butane_version
@_butane: _localbin
  [ -f {{butane}} ] || just _goinstall "github.com/coreos/butane/internal" {{butane_version}} "internal" {{butane}}

# addlicense
addlicense_version := "v1.1.1"
addlicense := localbin / "addlicense" + "-" + addlicense_version
@_addlicense: _localbin
  [ -f {{addlicense}} ] || just _goinstall "github.com/google/addlicense" {{addlicense_version}} "addlicense" {{addlicense}}

# crd-ref-docs
crd_ref_docs_version := "v0.1.0"
crd_ref_docs := localbin / "crd-ref-docs" + "-" + crd_ref_docs_version
@_crd_ref_docs: _localbin
  [ -f {{crd_ref_docs}} ] || just _goinstall "github.com/elastic/crd-ref-docs" {{crd_ref_docs_version}} "crd-ref-docs" {{crd_ref_docs}}

# helm
helm_version := "v3.16.1"
helm := localbin / "helm" + "-" + helm_version
@_helm: _localbin
  [ -f {{helm}} ] || just _goinstall "helm.sh/helm/v3/cmd/helm" {{helm_version}} "helm" {{helm}}

# helmify
helmify_version := "v0.4.11"
helmify := localbin / "helmify" + "-" + helmify_version
@_helmify: _localbin
  [ -f {{helmify}} ] || just _goinstall "github.com/arttor/helmify/cmd/helmify" {{helmify_version}} "helmify" {{helmify}}

# oras
oras_version := "v1.2.0"
oras := localbin / "oras" + "-" + oras_version
@_oras: _localbin
  [ -f {{oras}} ] || just _goinstall "oras.land/oras/cmd/oras" {{oras_version}} "oras" {{oras}}

# actionlint
actionlint_version := "v1.7.3"
actionlint := localbin / "actionlint" + "-" + actionlint_version
@_actionlint: _localbin
  [ -f {{actionlint}} ] || just _goinstall "github.com/rhysd/actionlint/cmd/actionlint" {{actionlint_version}} "actionlint" {{actionlint}}

# skopeo
skopeo_version := "v1.16.1"
skopeo := localbin / "skopeo" + "-" + skopeo_version
@_skopeo: _localbin
  [ -f {{skopeo}} ] || just _goinstall "github.com/containers/skopeo/cmd/skopeo" {{skopeo_version}} "skopeo" {{skopeo}} "--tags containers_image_openpgp,exclude_graphdriver_btrfs"

# gcov2lcov
gcov2lcov_version := "v1.0.6"
gcov2lcov := localbin / "gcov2lcov" + "-" + gcov2lcov_version
@_gcov2lcov: _localbin
  [ -f {{gcov2lcov}} ] || just _goinstall "github.com/jandelgado/gcov2lcov" {{gcov2lcov_version}} "gcov2lcov" {{gcov2lcov}}

# go install helper
_goinstall PACKAGE VERSION BINNAME TARGET FLAGS="": _localbin
  #!/usr/bin/env bash
  set -euo pipefail

  echo "Installing go package: {{PACKAGE}}@{{VERSION}}..."
  GOBIN=`pwd`/{{localbin}} go install {{FLAGS}} {{PACKAGE}}@{{VERSION}}
  mv {{localbin}}/{{BINNAME}} {{TARGET}}

#
# Common recipes
#

_license_headers: _addlicense
  @{{addlicense}} -l apache -s=only -c Hedgehog -ignore ".github/**" -ignore "config/**" -ignore "**/*.yaml" -ignore "**/*.yml" -ignore "vendor/**" -y 2025 .

# Run linter for GitHub Actions
lint-gha: _actionlint
   {{actionlint}} -verbose

# Run tests
test path="./...": gen _envtest _gcov2lcov
  KUBEBUILDER_ASSETS=`{{envtest}} use {{envtest_k8s_version}} --bin-dir {{localbinpath}} -p path` go test {{go_flags}} `go list {{path}} | grep -v /e2e` -coverprofile cover.out
  {{gcov2lcov}} -infile cover.out -outfile lcov.info

#
# Docker/Helm recipes
#

oci := ""
skopeo_dest_insecure := if oci == "http" { "--dest-tls-verify=false" } else { "" }
oras_insecure := if oci == "http" { "--plain-http" } else { "" }
helm_insecure_push := if oci == "http" { "--plain-http" } else { "" }

_docker-build bin: build
  cp bin/{{bin}} config/docker/{{bin}}/
  cd config/docker/{{bin}}/ && docker build --platform=linux/amd64 -t {{oci_repo}}/{{oci_prefix}}/{{bin}}:{{version}} -f Dockerfile .

skopeo_copy_flags := if env("DOCKER_HOST", "") != "" { "--src-daemon-host " + env_var("DOCKER_HOST") } else { "" }
_docker-push bin: _skopeo
  {{skopeo}} --insecure-policy copy {{skopeo_copy_flags}} {{skopeo_dest_insecure}} docker-daemon:{{oci_repo}}/{{oci_prefix}}/{{bin}}:{{version}} docker://{{oci_repo}}/{{oci_prefix}}/{{bin}}:{{version}}

_helm-build name: _helm
  @rm config/helm/{{name}}-v*.tgz || true
  {{helm}} package config/helm/{{name}} --destination config/helm --version {{version}}
  {{helm}} lint config/helm/{{name}}-{{version}}.tgz

_helm-push name: _helm
  {{helm}} push {{helm_insecure_push}} config/helm/{{name}}-{{version}}.tgz oci://{{oci_repo}}/{{oci_prefix}}/charts

_kube-build name: (_docker-build name) (_helm-build name)

_kube-push name: (_docker-push name) (_helm-push name)
