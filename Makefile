.PHONY: all build dist print-version clean install test test-e2e test-e2e-smoke \
       test-e2e-zoa test-e2e-zoa-smoke test-e2e-monitoring \
       fmt fmt-check vet lint verify tidy verify-mod \
       image-lambda image-runner image-push-lambda image-push-runner images-push \
       help

BINARY_NAME = zoa
BUILD_DIR   = ./bin
DIST_DIR    ?= ./dist

# Cross-compiled CLI artifacts for GitHub Releases (kubectl-style).
CLI_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
HASH_CMD      := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo "shasum -a 256")

# Container images
IMAGE_REPO        ?= quay.io/rrp-dev-ci/zoa-lambda
RUNNER_IMAGE_REPO ?= quay.io/rrp-dev-ci/zoa-runner
IMAGE_TAG         ?= latest
GIT_COMMIT        = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

CONTAINER_RUNTIME ?= $(shell command -v podman 2>/dev/null || echo docker)

# Tools
TOOLS_DIR     := ./hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin
GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/golangci-lint)

$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go build -tags=tools -o $(abspath $(TOOLS_BIN_DIR))/golangci-lint github.com/golangci/golangci-lint/v2/cmd/golangci-lint

VERSION     = 0.4.0
VERSION_PKG = github.com/openshift-online/rosa-hyperfleet-zoa/internal/version
VERSION_LDFLAGS = -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT) -X $(VERSION_PKG).BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS         = -ldflags "$(VERSION_LDFLAGS)"
DIST_LDFLAGS    = -ldflags "-s -w $(VERSION_LDFLAGS)"

# =============================================================================
# Default
# =============================================================================

all: verify test build

# =============================================================================
# Build
# =============================================================================

build:
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/zoa/
	@echo "✓ $(BUILD_DIR)/$(BINARY_NAME)"

# print-version is the single parser used by CI so we don't grep Makefile by hand.
print-version:
	@echo $(VERSION)

# Cross-compile the CLI for GitHub Releases. Same version ldflags as `build`,
# plus -s -w (stripped) and -trimpath so artifacts are smaller and more reproducible.
dist:
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@for platform in $(CLI_PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		out="$(DIST_DIR)/zoa-$$os-$$arch"; \
		echo "Building $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath $(DIST_LDFLAGS) -o "$$out" ./cmd/zoa/; \
	done
	@cd $(DIST_DIR) && $(HASH_CMD) zoa-* > SHA256SUMS
	@echo "✓ $(DIST_DIR)"
	@ls -lh $(DIST_DIR)

clean:
	@rm -rf $(BUILD_DIR) $(DIST_DIR) coverage.out

install:
	@go install $(LDFLAGS) ./cmd/zoa/

tidy:
	@go mod tidy
	@cd $(TOOLS_DIR) && go mod tidy

verify-mod: tidy
	@git diff --exit-code go.mod go.sum $(TOOLS_DIR)/go.mod $(TOOLS_DIR)/go.sum

# =============================================================================
# Test
# =============================================================================

test:
	@go test -race -coverprofile=coverage.out ./...

# test-e2e drives the built zoa CLI against an already-provisioned RC and/or
# MC environment (ZOA_RC_API_URL / ZOA_MC_API_URL). It is excluded from
# `test` via the `e2e` build tag so `make test` / CI unit tests never need
# live infrastructure. See test/e2e/suite_test.go.
#
# When both RC and MC targets are set, each runs as its own process in
# parallel — sequential within a target, parallel across targets (~2x speedup).
# Falls back to single-process when only one target is configured.
#
# Pass GINKGO_FLAGS for verbose output: GINKGO_FLAGS=-ginkgo.v make test-e2e
GINKGO_FLAGS ?=
ZOA_BIN_ABS   = $(abspath $(BUILD_DIR))/$(BINARY_NAME)
E2E_COMMON    = ZOA_BIN=$(ZOA_BIN_ABS) go test -tags e2e ./test/e2e/... -v $(GINKGO_FLAGS)

define run_e2e_parallel
	@rc_exit=0; mc_exit=0; \
	if [ -n "$(ZOA_RC_API_URL)" ] && [ -n "$(ZOA_MC_API_URL)" ]; then \
		set -o pipefail; \
		echo "Running RC and MC in parallel..."; \
		(ZOA_MC_API_URL= $(E2E_COMMON) $(1) 2>&1 | sed 's/^/[RC] /') & rc_pid=$$!; \
		(ZOA_RC_API_URL= $(E2E_COMMON) $(1) 2>&1 | sed 's/^/[MC] /') & mc_pid=$$!; \
		wait $$rc_pid || rc_exit=$$?; \
		wait $$mc_pid || mc_exit=$$?; \
		if [ $$rc_exit -ne 0 ] || [ $$mc_exit -ne 0 ]; then \
			echo "FAIL: RC=$$rc_exit MC=$$mc_exit"; exit 1; \
		fi; \
		echo "PASS: both RC and MC succeeded"; \
	else \
		$(E2E_COMMON) $(1); \
	fi
endef

test-e2e: build
	$(call run_e2e_parallel,-timeout 20m)
	@echo ""; echo "=== ZOA Monitoring E2E ==="
	@go test -tags e2e_monitoring ./test/e2e-monitoring/... -v -timeout 10m $(GINKGO_FLAGS)

# test-e2e-smoke runs only the specs labeled "smoke" — cheap, --dry-run/read-only
# coverage (discovery + one read TA + one write TA dry-run) meant to be run
# from rosa-hyperfleet/rosa-hyperfleet-api's own e2e jobs so infra/platform
# changes can't silently break ZOA without adding meaningful time to those
# runs. Full validation (including real delete_pod/rollout_restart execution)
# is `make test-e2e`, exercised only from this repo's own on-demand-e2e/nightly.
test-e2e-smoke: build
	$(call run_e2e_parallel,-timeout 5m -ginkgo.label-filter=smoke)
	@echo ""; echo "=== ZOA Monitoring E2E (smoke) ==="
	@go test -tags e2e_monitoring ./test/e2e-monitoring/... -v -timeout 5m -ginkgo.label-filter=smoke $(GINKGO_FLAGS)

# test-e2e-zoa runs only the ZOA functional e2e suite (no monitoring).
test-e2e-zoa: build
	$(call run_e2e_parallel,-timeout 20m)

# test-e2e-zoa-smoke runs only the ZOA functional smoke specs (no monitoring).
test-e2e-zoa-smoke: build
	$(call run_e2e_parallel,-timeout 5m -ginkgo.label-filter=smoke)

# test-e2e-monitoring runs only the observability validation suite.
# Requires RHOBS_API_URL (self-skips if unset). See docs/observability.md.
test-e2e-monitoring:
	@go test -tags e2e_monitoring ./test/e2e-monitoring/... -v -timeout 10m $(GINKGO_FLAGS)

# =============================================================================
# Code Quality
# =============================================================================

fmt:
	@gofmt -w -s .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Run 'make fmt'" && gofmt -l . && exit 1)

vet:
	@go vet ./...

lint: $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) run --timeout=10m ./...

verify: fmt-check vet lint

# =============================================================================
# Container Images
# =============================================================================

image-lambda:
	$(CONTAINER_RUNTIME) build \
		--platform linux/amd64 \
		-t $(IMAGE_REPO):$(IMAGE_TAG) \
		-f Containerfile .

image-runner:
	$(CONTAINER_RUNTIME) build \
		--platform linux/amd64 \
		-t $(RUNNER_IMAGE_REPO):$(IMAGE_TAG) \
		-f Containerfile.runner .

image-push-lambda: image-lambda
	$(CONTAINER_RUNTIME) push $(IMAGE_REPO):$(IMAGE_TAG)
	$(CONTAINER_RUNTIME) tag $(IMAGE_REPO):$(IMAGE_TAG) $(IMAGE_REPO):$(GIT_COMMIT)
	$(CONTAINER_RUNTIME) push $(IMAGE_REPO):$(GIT_COMMIT)

image-push-runner: image-runner
	$(CONTAINER_RUNTIME) push $(RUNNER_IMAGE_REPO):$(IMAGE_TAG)
	$(CONTAINER_RUNTIME) tag $(RUNNER_IMAGE_REPO):$(IMAGE_TAG) $(RUNNER_IMAGE_REPO):$(GIT_COMMIT)
	$(CONTAINER_RUNTIME) push $(RUNNER_IMAGE_REPO):$(GIT_COMMIT)

# Meta target — build + push both images in one command (dev workflow).
# Each image-push-* target depends on the corresponding image-* build target,
# so this single command builds and pushes everything.
images-push: image-push-lambda image-push-runner

# =============================================================================
# Help
# =============================================================================

help:
	@echo "Build:"
	@echo "  build              Build zoa CLI (./bin/zoa)"
	@echo "  dist               Cross-compile CLI for GitHub Releases (./dist)"
	@echo "  print-version      Print the CLI VERSION from this Makefile"
	@echo "  install            Install zoa to GOPATH/bin"
	@echo "  clean              Remove build artifacts"
	@echo ""
	@echo "Test & Quality:"
	@echo "  test               Run unit tests with race detection"
	@echo "  test-e2e           Run full ZOA e2e + monitoring suite (auto-parallel when both RC+MC are set)"
	@echo "  test-e2e-smoke     Run smoke ZOA e2e + monitoring smoke (used by rosa-hyperfleet/-api)"
	@echo "  test-e2e-zoa       Run full ZOA e2e only (no monitoring)"
	@echo "  test-e2e-zoa-smoke Run smoke ZOA e2e only (no monitoring)"
	@echo "  test-e2e-monitoring Run monitoring validation only (requires RHOBS_API_URL)"
	@echo "                     Verbose: GINKGO_FLAGS=-ginkgo.v make test-e2e"
	@echo "  verify             fmt-check + vet + lint"
	@echo "  fmt                Format code"
	@echo ""
	@echo "Images:"
	@echo "  image-lambda       Build zoa-lambda image"
	@echo "  image-runner       Build zoa-runner image"
	@echo "  image-push-lambda  Build + push zoa-lambda (:latest + :commit)"
	@echo "  image-push-runner  Build + push zoa-runner (:latest + :commit)"
	@echo "  images-push        Build + push both images (single command for dev workflow)"
