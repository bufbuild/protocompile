# See https://tech.davis-hansson.com/p/make/
SHELL := bash
.DELETE_ON_ERROR:
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := all
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-print-directory
BIN := $(abspath .tmp/bin)
COPYRIGHT_YEARS := 2020-2022
LICENSE_IGNORE := -e /testdata/ -e /testprotos/
# Set to use a different compiler. For example, `GO=go1.18rc1 make test`.
GO ?= go
TOOLS_MOD_DIR := ./internal/tools

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
	$(GO) test -vet=off -race -cover ./...

.PHONY: build
build: generate ## Build all packages
	$(GO) build ./...

.PHONY: install
install: ## Install all binaries
	$(GO) install ./...

.PHONY: lint
lint: $(BIN)/golangci-lint ## Lint Go
	$(GO) vet ./...
	$(BIN)/golangci-lint run || : # Don't fail on lint errors initially

.PHONY: lintfix
lintfix: $(BIN)/golangci-lint ## Automatically fix some lint errors
	$(BIN)/golangci-lint run --fix

.PHONY: generate
generate: $(BIN)/license-header $(BIN)/goyacc ## Regenerate code and licenses
	@# We want to operate on a list of modified and new files, excluding
	@# deleted and ignored files. git-ls-files can't do this alone. comm -23 takes
	@# two files and prints the union, dropping lines common to both (-3) and
	@# those only in the second file (-2). We make one git-ls-files call for
	@# the modified, cached, and new (--others) files, and a second for the
	@# deleted files.
	cd parser && \
	PATH=$(BIN):$(PATH) $(GO) generate
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
	test -z "$$(git status --porcelain | tee /dev/stderr)"

$(BIN)/license-header: internal/tools/go.mod internal/tools/go.sum
	@mkdir -p $(@D)
	cd $(TOOLS_MOD_DIR) && \
	$(GO) build -o $@ github.com/bufbuild/buf/private/pkg/licenseheader/cmd/license-header

$(BIN)/golangci-lint: internal/tools/go.mod internal/tools/go.sum
	@mkdir -p $(@D)
	cd $(TOOLS_MOD_DIR) && \
	$(GO) build -o $@ github.com/golangci/golangci-lint/cmd/golangci-lint

$(BIN)/goyacc: internal/tools/go.mod internal/tools/go.sum
	@mkdir -p $(@D)
	cd $(TOOLS_MOD_DIR) && \
	$(GO) build -o $@ golang.org/x/tools/cmd/goyacc
