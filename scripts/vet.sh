#!/usr/bin/env bash
#
# vet.sh — run the full Per%Sense vetting suite locally.
#
# This is the hand-run replacement for the old .github/workflows/ci.yml (GitHub
# Actions is not in use). It reproduces the same four jobs that workflow ran, in
# the same order, so a green run here means the same thing a green CI run meant:
#
#   1. go            — go build + vet + full `go test ./...` INCLUDING the
#                      Node-backed frontend<->engine differential sweeps
#                      (a frontend render/mapping regression fails here).
#   2. dos-fidelity  — compile the real DOS source-oracles (Free Pascal) and run
#                      the TestDOS differential sweeps against them across the
#                      amortization / present-value / mortgage / interest /
#                      dateutil packages. The DOS engine is the authority.
#   3. actuarial     — cross-check the Go life-contingency math against the live
#                      third-party `actuarialmath` library + SOA SULT table.
#   4. refdata       — regenerate legacy/reference-output/refdata.json from the
#                      Pascal harness and confirm the checked-in copy still matches.
#
# Jobs 2-4 need extra toolchains (Free Pascal `fpc`, Python 3 + pip). If a
# toolchain is missing the job is SKIPPED with a clear notice and the overall
# run still succeeds — UNLESS you pass --strict, which turns any skip into a
# failure (the fail-closed behavior CI had, mirroring PERSENSE_REQUIRE_ORACLE).
#
# Usage:
#   scripts/vet.sh                 # run every job; skip tool-gated jobs if tools absent
#   scripts/vet.sh --quick         # job 1 only (go build/vet/test) — the fast inner loop
#   scripts/vet.sh --strict        # fail (don't skip) if fpc/python are missing
#   scripts/vet.sh --no-oracle     # skip the DOS-fidelity job
#   scripts/vet.sh --no-actuarial  # skip the actuarial cross-check
#   scripts/vet.sh --no-refdata    # skip the refdata-harness check
#   scripts/vet.sh -h | --help
#
# Exit status is non-zero if any job that actually ran failed (or, under
# --strict, if a required toolchain was missing).

set -uo pipefail

cd "$(dirname "$0")/.."          # repo root
REPO_ROOT="$(pwd)"

# ---- options ---------------------------------------------------------------
QUICK=0
STRICT=0
RUN_ORACLE=1
RUN_ACTUARIAL=1
RUN_REFDATA=1

usage() { sed -n '2,/^set -uo/p' "$0" | sed 's/^# \{0,1\}//; s/^#$//' | sed '$d'; }

for arg in "$@"; do
  case "$arg" in
    -q|--quick)        QUICK=1 ;;
    --strict)          STRICT=1 ;;
    --no-oracle)       RUN_ORACLE=0 ;;
    --no-actuarial)    RUN_ACTUARIAL=0 ;;
    --no-refdata)      RUN_REFDATA=0 ;;
    -h|--help)         usage; exit 0 ;;
    *) echo "vet.sh: unknown option '$arg' (try --help)" >&2; exit 2 ;;
  esac
done

# ---- pretty output ---------------------------------------------------------
if [ -t 1 ]; then
  BOLD=$'\033[1m'; RED=$'\033[31m'; GRN=$'\033[32m'; YEL=$'\033[33m'; DIM=$'\033[2m'; RST=$'\033[0m'
else
  BOLD=""; RED=""; GRN=""; YEL=""; DIM=""; RST=""
fi

section() { printf '\n%s==== %s ====%s\n' "$BOLD" "$1" "$RST"; }
have()    { command -v "$1" >/dev/null 2>&1; }

# Results table: parallel arrays of "name" and "PASS|FAIL|SKIP".
NAMES=(); STATES=(); DETAIL=()
record() { NAMES+=("$1"); STATES+=("$2"); DETAIL+=("${3:-}"); }

# Run a command, streaming output; returns its exit status.
run() { echo "${DIM}\$ $*${RST}"; "$@"; }

# ---- preflight -------------------------------------------------------------
if ! have go; then
  echo "${RED}vet.sh: 'go' is not on PATH — the Go toolchain is required.${RST}" >&2
  echo "Install Go (https://go.dev/dl/) and re-run." >&2
  exit 1
fi
echo "Go:  $(go version)"
have node && echo "Node: $(node --version)  ${DIM}(frontend differential sweeps enabled)${RST}" \
          || echo "${YEL}Node: not found — the frontend<->engine sweeps in job 1 will not run.${RST}"

# ===========================================================================
# Job 1 — go build / vet / test (+ Node-backed frontend sweeps)
# ===========================================================================
section "1/4  go build / vet / test"
go_ok=1
run go build ./...            || go_ok=0
run go vet   ./...            || go_ok=0
run go test  ./...            || go_ok=0
if [ "$go_ok" -eq 1 ]; then record "go build/vet/test" PASS; else record "go build/vet/test" FAIL; fi

if [ "$QUICK" -eq 1 ]; then
  echo "\n${DIM}--quick: skipping DOS-fidelity, actuarial, and refdata jobs.${RST}"
  RUN_ORACLE=0; RUN_ACTUARIAL=0; RUN_REFDATA=0
fi

# ===========================================================================
# Job 2 — DOS source-oracle differential sweeps
# ===========================================================================
if [ "$RUN_ORACLE" -eq 1 ]; then
  section "2/4  DOS source-oracle differential sweeps"
  if ! have fpc; then
    msg="Free Pascal (fpc) not installed — install with 'brew install fpc' (macOS) or 'sudo apt-get install -y fpc' (Linux)."
    if [ "$STRICT" -eq 1 ]; then
      echo "${RED}$msg${RST}"; record "dos-fidelity" FAIL "fpc missing (--strict)"
    else
      echo "${YEL}SKIP: $msg${RST}"; record "dos-fidelity" SKIP "fpc missing"
    fi
  else
    dos_ok=1
    run scripts/build_oracles.sh || dos_ok=0
    if [ "$dos_ok" -eq 1 ]; then
      export PERSENSE_ORACLE="$REPO_ROOT/legacy/oracle/build/amort_oracle"
      export PERSENSE_PV_ORACLE="$REPO_ROOT/legacy/oracle/build/pv_oracle"
      export PERSENSE_MTG_ORACLE="$REPO_ROOT/legacy/oracle/build/mtg_oracle"
      run go test -run TestDOS \
        ./internal/finance/amortization/ \
        ./internal/finance/presentvalue/ \
        ./internal/finance/mortgage/ \
        ./internal/finance/interest/ \
        ./internal/dateutil/ || dos_ok=0
    fi
    if [ "$dos_ok" -eq 1 ]; then record "dos-fidelity" PASS; else record "dos-fidelity" FAIL; fi
  fi
fi

# ===========================================================================
# Job 3 — actuarial third-party (actuarialmath) oracle
# ===========================================================================
if [ "$RUN_ACTUARIAL" -eq 1 ]; then
  section "3/4  actuarial third-party (actuarialmath) oracle"
  pip_ok=0
  if have python3 && python3 -m pip --version >/dev/null 2>&1; then pip_ok=1; fi
  if [ "$pip_ok" -eq 0 ]; then
    msg="Python 3 + pip not available — needed to install the 'actuarialmath' cross-check oracle."
    if [ "$STRICT" -eq 1 ]; then
      echo "${RED}$msg${RST}"; record "actuarial" FAIL "python/pip missing (--strict)"
    else
      echo "${YEL}SKIP: $msg${RST}"; record "actuarial" SKIP "python/pip missing"
    fi
  else
    act_ok=1
    run python3 -m pip install --quiet actuarialmath ipython || act_ok=0
    if [ "$act_ok" -eq 1 ]; then
      run go test -run 'TestActuarial|TestSULT' ./internal/finance/actuarial/ || act_ok=0
    fi
    if [ "$act_ok" -eq 1 ]; then record "actuarial" PASS; else record "actuarial" FAIL; fi
  fi
fi

# ===========================================================================
# Job 4 — reference-data harness is current
# ===========================================================================
if [ "$RUN_REFDATA" -eq 1 ]; then
  section "4/4  reference-data harness is current"
  if ! have fpc; then
    msg="Free Pascal (fpc) not installed — needed to regenerate refdata.json from the Pascal harness."
    if [ "$STRICT" -eq 1 ]; then
      echo "${RED}$msg${RST}"; record "refdata-harness" FAIL "fpc missing (--strict)"
    else
      echo "${YEL}SKIP: $msg${RST}"; record "refdata-harness" SKIP "fpc missing"
    fi
  else
    if run scripts/regen_refdata.sh; then record "refdata-harness" PASS; else record "refdata-harness" FAIL; fi
  fi
fi

# ===========================================================================
# Summary
# ===========================================================================
section "summary"
fail=0; skipped=0
for i in "${!NAMES[@]}"; do
  st="${STATES[$i]}"; extra="${DETAIL[$i]:+  (${DETAIL[$i]})}"
  case "$st" in
    PASS) col="$GRN" ;;
    FAIL) col="$RED"; fail=1 ;;
    SKIP) col="$YEL"; skipped=1 ;;
  esac
  printf '  %s%-6s%s %s%s\n' "$col" "$st" "$RST" "${NAMES[$i]}" "$extra"
done

echo
if [ "$fail" -eq 1 ]; then
  echo "${RED}${BOLD}VET FAILED${RST} — one or more jobs did not pass."
  exit 1
fi
if [ "$skipped" -eq 1 ]; then
  echo "${YEL}${BOLD}VET PASSED (with skips)${RST} — install the missing toolchains, or use --strict to treat skips as failures."
else
  echo "${GRN}${BOLD}VET PASSED${RST} — all jobs green."
fi
exit 0
