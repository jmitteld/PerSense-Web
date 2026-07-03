# Makefile — Per%Sense (Delphi/Pascal → Go web port)
#
# Common build, run, test, and validation tasks. Run `make` (or `make help`)
# to list targets. Everything shells out to the standard Go toolchain; no
# extra tooling is required for the core targets (build/run/test/vet/fmt).
#
# Optional targets (oracles, sweep, refdata) need Free Pascal (fpc) to compile
# the headless DOS source-oracles used for differential testing — see the
# individual target comments.

# ---- Configuration (override on the command line, e.g. `make run PORT=9090`) -
GO         ?= go
BIN_DIR    ?= bin
BINARY     ?= $(BIN_DIR)/persense
PKG        := ./cmd/persense
PORT       ?= 8080

# Oracle binaries for the differential sweep tests. scripts/build_oracles.sh
# emits them here; the sweep target points the tests at these paths.
ORACLE_DIR ?= legacy/oracle/build

# Differential fuzz volume. PERSENSE_FUZZ_N scales the per-section case count;
# PERSENSE_FUZZ=1 opts into the heavier randomized cubes.
FUZZ_N     ?= 2000

# Version stamp (best-effort; blank outside a git checkout).
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -X main.version=$(VERSION)

.DEFAULT_GOAL := help

# ---- Core ------------------------------------------------------------------

.PHONY: build
build: ## Build the single server binary (REST API + embedded static UI) into bin/
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)
	@echo "built $(BINARY)  ($(VERSION))"

.PHONY: run
run: ## Run the web server (make run PORT=8080)
	$(GO) run $(PKG) -port $(PORT)

.PHONY: install
install: ## go install the server binary into $GOBIN / $GOPATH/bin
	$(GO) install -ldflags '$(LDFLAGS)' $(PKG)

.PHONY: all
all: tidy fmt vet test build ## Tidy, format, vet, test, then build

# ---- Tests -----------------------------------------------------------------

.PHONY: test
test: ## Run the full suite (DOS oracle sweeps SKIP if the oracle isn't built — use `make ci` to require it)
	$(GO) test ./...

# LINUX_ORACLE is where legacy/oracle/build_linux.sh stages the no-root FPC build.
LINUX_ORACLE ?= /tmp/oraclebuild/amort_oracle

.PHONY: oracle-linux
oracle-linux: ## Build the DOS amort oracle with the no-root FPC stager (CI/sandbox) -> /tmp/oraclebuild
	bash legacy/oracle/build_linux.sh

.PHONY: ci
ci: fmt-check vet oracle-linux ## Gated CI check: build the oracle, then run the FULL suite FAIL-CLOSED on a missing/unrunnable oracle
	@echo "running gated suite (PERSENSE_REQUIRE_ORACLE=1 — the DOS differential sweeps FAIL, never skip)"
	PERSENSE_REQUIRE_ORACLE=1 PERSENSE_ORACLE="$(LINUX_ORACLE)" $(GO) test ./... -count=1
	@echo "ci OK — the DOS oracle differential actually ran (a skip would have failed TestOracleGate)"

.PHONY: test-short
test-short: ## Run tests with -short (skips the slow oracle sweeps where honored)
	$(GO) test -short ./...

.PHONY: test-race
test-race: ## Run the test suite with the race detector
	$(GO) test -race ./...

.PHONY: cover
cover: ## Run tests with coverage and write coverage.out (+ HTML report)
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "wrote coverage.out and coverage.html"

.PHONY: bench
bench: ## Run all benchmarks
	$(GO) test -run '^$$' -bench . -benchmem ./...

# ---- Quality ---------------------------------------------------------------

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format all Go source in place (gofmt -w)
	gofmt -w $(shell $(GO) list -f '{{.Dir}}' ./...)

.PHONY: fmt-check
fmt-check: ## Fail if any Go source is not gofmt-clean
	@unformatted=$$(gofmt -l $(shell $(GO) list -f '{{.Dir}}' ./...)); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	else echo "gofmt clean"; fi

.PHONY: tidy
tidy: ## Sync go.mod / go.sum
	$(GO) mod tidy

.PHONY: lint
lint: fmt-check vet ## Format-check + vet (the checks CI enforces)

# ---- DOS differential oracle (optional; requires Free Pascal) --------------

.PHONY: oracles
oracles: ## Build the headless DOS source-oracles (needs fpc) into legacy/oracle/build/
	scripts/build_oracles.sh

.PHONY: sweep
sweep: ## Run the DOS differential sweep tests against the built oracles
	@test -x "$(ORACLE_DIR)/amort_oracle" || { echo "oracles not built — run 'make oracles' first"; exit 1; }
	PERSENSE_ORACLE="$(PWD)/$(ORACLE_DIR)/amort_oracle" \
	PERSENSE_PV_ORACLE="$(PWD)/$(ORACLE_DIR)/pv_oracle" \
	PERSENSE_MTG_ORACLE="$(PWD)/$(ORACLE_DIR)/mtg_oracle" \
	$(GO) test ./internal/finance/... -run TestDOS -count=1

.PHONY: fuzz
fuzz: ## Run the differential fuzzers at volume (make fuzz FUZZ_N=5000)
	@test -x "$(ORACLE_DIR)/amort_oracle" || { echo "oracles not built — run 'make oracles' first"; exit 1; }
	PERSENSE_ORACLE="$(PWD)/$(ORACLE_DIR)/amort_oracle" \
	PERSENSE_PV_ORACLE="$(PWD)/$(ORACLE_DIR)/pv_oracle" \
	PERSENSE_MTG_ORACLE="$(PWD)/$(ORACLE_DIR)/mtg_oracle" \
	PERSENSE_FUZZ=1 PERSENSE_FUZZ_N=$(FUZZ_N) \
	$(GO) test ./internal/finance/... -run 'TestFuzz|TestDOS' -count=1 -timeout 40m -v

.PHONY: refdata
refdata: ## Regenerate legacy/reference-output/refdata.json from the Pascal harness (needs fpc)
	scripts/regen_refdata.sh

# ---- Release cross-compile -------------------------------------------------

.PHONY: release
release: ## Cross-compile static binaries for macOS (arm64/amd64) and Linux (amd64)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 $(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/persense-macos-arm64  $(PKG)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 $(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/persense-macos-amd64  $(PKG)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 $(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/persense-linux-amd64  $(PKG)
	@echo "release binaries in $(BIN_DIR)/"

# ---- Housekeeping ----------------------------------------------------------

.PHONY: clean
clean: ## Remove build artifacts and coverage output
	rm -rf $(BIN_DIR) coverage.out coverage.html
	$(GO) clean

.PHONY: help
help: ## Show this help
	@echo "Per%Sense — make targets:"
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
