# See https://tech.davis-hansson.com/p/make/
SHELL := bash
.DELETE_ON_ERROR:
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := all
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-print-directory
BIN ?= $(abspath .tmp/bin)
CACHE := $(abspath .tmp/cache)
COPYRIGHT_YEARS := 2020-2025
LICENSE_IGNORE := -E -e "/testdata/|^wellknownimports/google/protobuf/"
# Set to use a different compiler. For example, `GO=go1.18rc1 make test`.
GO ?= go
GO_CMD := GOTOOLCHAIN=local $(GO)
TOOLS_MOD_DIR := ./internal/tools
# We allow the internal/tools module use a newer version of Go, since some of the
# tools we want to install and execute require later versions.
GO_TOOL_CMD := GOTOOLCHAIN=auto $(GO)
UNAME_OS := $(shell uname -s)
UNAME_ARCH := $(shell uname -m)
PATH_SEP ?= ":"

PROTOC_VERSION := $(shell cat ./.protoc_version)
# For release candidates, the download artifact has a dash between "rc" and the number even
# though the version tag does not :(
PROTOC_ARTIFACT_VERSION := $(shell echo $(PROTOC_VERSION) | sed -E 's/-rc([0-9]+)$$/-rc-\1/g')
PROTOC_DIR := $(abspath $(CACHE)/protoc/$(PROTOC_VERSION))
PROTOC := $(PROTOC_DIR)/bin/protoc

LOWER_UNAME_OS := $(shell echo $(UNAME_OS) | tr A-Z a-z)
ifeq ($(LOWER_UNAME_OS),darwin)
	PROTOC_OS := osx
	ifeq ($(UNAME_ARCH),arm64)
		PROTOC_ARCH := aarch_64
	else
		PROTOC_ARCH := x86_64
	endif
else
	PROTOC_OS := $(LOWER_UNAME_OS)
	PROTOC_ARCH := $(UNAME_ARCH)
endif
PROTOC_ARTIFACT_SUFFIX ?= $(PROTOC_OS)-$(PROTOC_ARCH)

.PHONY: help
help: ## Describe useful make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-30s %s\n", $$1, $$2}'

.PHONY: all
all: ## Build, test, and lint (default)
	$(MAKE) test
	$(MAKE) lint

.PHONY: clean
clean: ## Delete intermediate build artifacts
	@# -X only removes untracked files, -d recurses into directories, -f actually removes files/dirs
	git clean -Xdf

.PHONY: test
test: build ## Run unit tests
	$(GO_CMD) test -race -cover ./...
	$(GO_CMD) test -tags protolegacy ./...
	cd internal/benchmarks && SKIP_DOWNLOAD_GOOGLEAPIS=true $(GO_CMD) test -race -cover ./...

.PHONY: benchmarks
benchmarks: build ## Run benchmarks
	cd internal/benchmarks && $(GO_CMD) test -bench=. -benchmem -v ./...

.PHONY: build
build: generate ## Build all packages
	$(GO_CMD) build ./...

.PHONY: install
install: ## Install all binaries
	$(GO_CMD) install ./...

.PHONY: lint
lint: $(BIN)/golangci-lint ## Lint Go
	$(GO_CMD) vet ./... ./internal/benchmarks/...
	$(BIN)/golangci-lint run
	cd internal/benchmarks && $(BIN)/golangci-lint run

.PHONY: lintfix
lintfix: $(BIN)/golangci-lint ## Automatically fix some lint errors
	$(BIN)/golangci-lint run --fix
	cd internal/benchmarks && $(BIN)/golangci-lint run --fix

.PHONY: generate
generate: $(BIN)/license-header $(BIN)/goyacc wellknownimports test-descriptors ext-features-descriptors ## Regenerate code and licenses
	PATH="$(BIN)$(PATH_SEP)$(PATH)" $(GO_CMD) generate ./...
	@# We want to operate on a list of modified and new files, excluding
	@# deleted and ignored files. git-ls-files can't do this alone. comm -23 takes
	@# two files and prints the union, dropping lines common to both (-3) and
	@# those only in the second file (-2). We make one git-ls-files call for
	@# the modified, cached, and new (--others) files, and a second for the
	@# deleted files.
	comm -23 \
		<(git ls-files --cached --modified --others --no-empty-directory --exclude-standard | sort -u | grep -v $(LICENSE_IGNORE) ) \
		<(git ls-files --deleted | sort -u) | \
		xargs $(BIN)/license-header \
			--license-type apache \
			--copyright-holder "Buf Technologies, Inc." \
			--year-range "$(COPYRIGHT_YEARS)"

.PHONY: upgrade
upgrade: ## Upgrade dependencies
	go get -u -t ./... && go mod tidy -v

.PHONY: checkgenerate
checkgenerate:
	@# Used in CI to verify that `make generate` doesn't produce a diff.
	@echo git status --porcelain
	@if [[ -n "$$(git status --porcelain | tee /dev/stderr)" ]]; then \
	  git diff; \
	  false; \
	fi

$(BIN)/license-header: internal/tools/go.mod internal/tools/go.sum
	@mkdir -p $(@D)
	cd $(TOOLS_MOD_DIR) && \
		GOWORK=off $(GO_TOOL_CMD) build -o $@ github.com/bufbuild/buf/private/pkg/licenseheader/cmd/license-header

$(BIN)/golangci-lint: internal/tools/go.mod internal/tools/go.sum
	@mkdir -p $(@D)
	cd $(TOOLS_MOD_DIR) && \
		GOWORK=off $(GO_TOOL_CMD) build -o $@ github.com/golangci/golangci-lint/cmd/golangci-lint

$(BIN)/goyacc: internal/tools/go.mod internal/tools/go.sum
	@mkdir -p $(@D)
	cd $(TOOLS_MOD_DIR) && \
		GOWORK=off $(GO_TOOL_CMD) build -o $@ golang.org/x/tools/cmd/goyacc

$(CACHE)/protoc-$(PROTOC_VERSION).zip:
	@mkdir -p $(@D)
	curl -o $@ -fsSL https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_ARTIFACT_VERSION)-$(PROTOC_ARTIFACT_SUFFIX).zip

.PHONY: protoc
protoc: $(PROTOC)

$(PROTOC): $(CACHE)/protoc-$(PROTOC_VERSION).zip
	@mkdir -p $(@D)
	unzip -o -q $< -d $(PROTOC_DIR) && \
	touch $@

.PHONY: wellknownimports
wellknownimports: $(PROTOC) $(sort $(wildcard $(PROTOC_DIR)/include/google/protobuf/*.proto)) $(sort $(wildcard $(PROTOC_DIR)/include/google/protobuf/*/*.proto))
	@rm -rf wellknownimports/google 2>/dev/null && true
	@mkdir -p wellknownimports/google/protobuf/compiler
	cp -R $(PROTOC_DIR)/include/google/protobuf/*.proto wellknownimports/google/protobuf
	cp -R $(PROTOC_DIR)/include/google/protobuf/compiler/*.proto wellknownimports/google/protobuf/compiler

internal/testdata/all.protoset: $(PROTOC) $(sort $(wildcard internal/testdata/*.proto))
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) --include_imports -I. $(filter-out protoc,$(^F))

internal/testdata/desc_test_complex.protoset: $(PROTOC) internal/testdata/desc_test_complex.proto
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) --include_imports -I. $(filter-out protoc,$(^F))

internal/testdata/desc_test_defaults.protoset: $(PROTOC) internal/testdata/desc_test_defaults.proto
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) --include_imports -I. $(filter-out protoc,$(^F))

internal/testdata/desc_test_proto3_optional.protoset: $(PROTOC) internal/testdata/desc_test_proto3_optional.proto
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) --include_imports -I. $(filter-out protoc,$(^F))

internal/testdata/descriptor_impl_tests.protoset: $(PROTOC) internal/testdata/desc_test2.proto internal/testdata/desc_test_complex.proto internal/testdata/desc_test_defaults.proto internal/testdata/desc_test_proto3.proto internal/testdata/desc_test_proto3_optional.proto
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) --include_imports -I. $(filter-out protoc,$(^F))

internal/testdata/descriptor_editions_impl_tests.protoset: $(PROTOC) internal/testdata/editions/all_default_features.proto internal/testdata/editions/features_with_overrides.proto internal/testdata/editions/file_default_delimited.proto
	cd $(@D)/editions && $(PROTOC) --descriptor_set_out=../$(@F) --include_imports -I. $(filter-out protoc,$(^F))

internal/testdata/editions/all.protoset: $(PROTOC) $(sort $(wildcard internal/testdata/editions/*.proto))
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) --include_imports -I. $(filter-out protoc,$(^F))

internal/testdata/source_info.protoset: $(PROTOC) internal/testdata/desc_test_options.proto internal/testdata/desc_test_comments.proto internal/testdata/desc_test_complex.proto
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) --include_source_info -I. $(filter-out protoc,$(^F))

internal/testdata/options/options.protoset: $(PROTOC) internal/testdata/options/options.proto
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) -I. $(filter-out protoc,$(^F))

internal/testdata/options/test.protoset: $(PROTOC) internal/testdata/options/test.proto
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) -I. $(filter-out protoc,$(^F))

internal/testdata/options/test_proto3.protoset: $(PROTOC) internal/testdata/options/test_proto3.proto
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) -I. $(filter-out protoc,$(^F))

internal/testdata/options/test_editions.protoset: $(PROTOC) internal/testdata/options/test_editions.proto
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) -I. $(filter-out protoc,$(^F))

.PHONY: test-descriptors
test-descriptors: internal/testdata/all.protoset
test-descriptors: internal/testdata/desc_test_complex.protoset
test-descriptors: internal/testdata/desc_test_defaults.protoset
test-descriptors: internal/testdata/desc_test_proto3_optional.protoset
test-descriptors: internal/testdata/descriptor_impl_tests.protoset
test-descriptors: internal/testdata/descriptor_editions_impl_tests.protoset
test-descriptors: internal/testdata/editions/all.protoset
test-descriptors: internal/testdata/source_info.protoset
test-descriptors: internal/testdata/options/options.protoset
test-descriptors: internal/testdata/options/test.protoset
test-descriptors: internal/testdata/options/test_proto3.protoset
test-descriptors: internal/testdata/options/test_editions.protoset

internal/featuresext/cpp_features.protoset: $(PROTOC)
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) google/protobuf/cpp_features.proto
internal/featuresext/java_features.protoset: $(PROTOC)
	cd $(@D) && $(PROTOC) --descriptor_set_out=$(@F) google/protobuf/java_features.proto

.PHONY: ext-features-descriptors
ext-features-descriptors: internal/featuresext/cpp_features.protoset internal/featuresext/java_features.protoset
