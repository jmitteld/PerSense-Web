#!/bin/bash
# Test Go port calculations against DOS reference values
# Run with: bash test_dos_reference.sh
# Requires the server running on localhost:8080

set -e

API="http://localhost:8080"
PASS=0
FAIL=0
ERRORS=""

check() {
  local label="$1" got="$2" want="$3" tol="$4"
  if [ -z "$tol" ]; then tol="0.01"; fi
  local diff=$(python3 -c "print(abs($got - $want))" 2>/dev/null)
  local ok=$(python3 -c "print('yes' if abs($got - $want) <= $tol else 'no')" 2>/dev/null)
  if [ "$ok" = "yes" ]; then
    PASS=$((PASS+1))
  else
    FAIL=$((FAIL+1))
    ERRORS="${ERRORS}\n  FAIL: ${label}: got=${got} want=${want} diff=${diff} tol=${tol}"
  fi
}

echo "============================================"
echo "  DOS Reference vs Go Port — Test Suite"
echo "============================================"
echo ""

# --------------------------------------------------------
echo "--- 1. Mortgage Summation (from refdata.json) ---"
# --------------------------------------------------------

# rate=0, years=30 -> 360
r=$(curl -s -X POST $API/api/mortgage/calc -H 'Content-Type: application/json' \
  -d '{"price":360,"pctDown":0,"years":30,"rate":0,"tax":0}')
v=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['monthly'])")
check "Summation rate=0 yrs=30 monthly" "$v" "1.0" "0.01"

# rate=0.06, years=30, financed=166.523219591779 -> monthly=1
# Instead test: price=100000, 0 down, 30yr, 6% -> monthly=600.516854317036
r=$(curl -s -X POST $API/api/mortgage/calc -H 'Content-Type: application/json' \
  -d '{"price":100000,"pctDown":0,"years":30,"rate":0.06,"tax":0}')
v=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['monthly'])")
check "Mortgage 100k/30yr/6% monthly" "$v" "600.516854317036" "0.001"

# price=200000, 20% down, 30yr, 6% -> monthly=960.826966907258
r=$(curl -s -X POST $API/api/mortgage/calc -H 'Content-Type: application/json' \
  -d '{"price":200000,"pctDown":0.20,"years":30,"rate":0.06,"tax":0}')
v=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['monthly'])")
check "Mortgage 200k/20%/30yr/6% monthly" "$v" "960.826966907258" "0.001"

# price=500000, 10% down, 15yr, 5% -> monthly=3561.0170105101
r=$(curl -s -X POST $API/api/mortgage/calc -H 'Content-Type: application/json' \
  -d '{"price":500000,"pctDown":0.10,"years":15,"rate":0.05,"tax":0}')
v=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['monthly'])")
check "Mortgage 500k/10%/15yr/5% monthly" "$v" "3561.0170105101" "0.001"

# price=120000, 0 down, 10yr, 0% -> monthly=1000
r=$(curl -s -X POST $API/api/mortgage/calc -H 'Content-Type: application/json' \
  -d '{"price":120000,"pctDown":0,"years":10,"rate":0,"tax":0}')
v=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['monthly'])")
check "Mortgage 120k/10yr/0% monthly" "$v" "1000" "0.01"

# --------------------------------------------------------
echo "--- 2. Help Doc Mortgage Examples ---"
# --------------------------------------------------------

# Example 1: $200k, 20yr, 8%, 2pts, 20% down, $200 tax -> total=$1538.30
r=$(curl -s -X POST $API/api/mortgage/calc -H 'Content-Type: application/json' \
  -d '{"price":200000,"pctDown":0.20,"years":20,"rate":0.08,"points":0.02,"tax":200}')
monthly=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['monthly'])")
cash=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['cash'])")
financed=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['financed'])")
check "HelpEx1 monthly total" "$monthly" "1538.30" "5.0"
check "HelpEx1 cash required" "$cash" "43200" "1.0"
check "HelpEx1 financed" "$financed" "160000" "0.01"

# Example 2: $56k cash, $1650/mo, 1.5pts, 8.5%, 30yr, $200 tax
# Help doc text says price=241749.12 (uses standard discrete summation formula).
# DOS source code Summation() uses continuous compounding: e^(-r/12)*(1-e^(-rt))/(1-e^(-r/12))
# Our Go port faithfully matches the DOS source -> price=241233.69
r=$(curl -s -X POST $API/api/mortgage/calc -H 'Content-Type: application/json' \
  -d '{"cash":56000,"monthly":1650,"points":0.015,"years":30,"rate":0.085,"tax":200}')
price=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['price'])")
check "HelpEx2 price (matches DOS code)" "$price" "241233.69" "1.0"

# --------------------------------------------------------
echo "--- 3. Amortization Basic ---"
# --------------------------------------------------------

# $100k, 8%, 30yr monthly, loan 02/12/1998, first 03/01/1998 -> payment=$733.76
r=$(curl -s -X POST $API/api/amortization/calc -H 'Content-Type: application/json' \
  -d '{"amount":100000,"loanDate":"1998-02-12","rate":0.08,"firstDate":"1998-03-01","nPeriods":360,"perYr":12}')
pmt=$(echo "$r" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['schedule'][0]['payment'] if d.get('schedule') else 'ERROR')")
total_int=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin).get('totalInterest','ERROR'))")
check "AmzEx1 payment" "$pmt" "733.76" "1.0"

# $100k, 6%, 30yr monthly -> payment=$599.55
r=$(curl -s -X POST $API/api/amortization/calc -H 'Content-Type: application/json' \
  -d '{"amount":100000,"loanDate":"2024-01-01","rate":0.06,"firstDate":"2024-02-01","nPeriods":360,"perYr":12}')
pmt=$(echo "$r" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['schedule'][0]['payment'] if d.get('schedule') else 'ERROR')")
check "Amz 100k/6%/30yr payment" "$pmt" "599.55" "1.0"

# $100k, 8%, 30yr monthly, standard dates -> payment=$733.76
r=$(curl -s -X POST $API/api/amortization/calc -H 'Content-Type: application/json' \
  -d '{"amount":100000,"loanDate":"2024-01-01","rate":0.08,"firstDate":"2024-02-01","nPeriods":360,"perYr":12}')
pmt=$(echo "$r" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['schedule'][0]['payment'] if d.get('schedule') else 'ERROR')")
sched_len=$(echo "$r" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('schedule',[])))")
check "Amz 100k/8%/30yr payment" "$pmt" "733.76" "1.0"
check "Amz 100k/8%/30yr schedule length" "$sched_len" "360" "0"

# Verify first payment interest portion: $100k * 8% / 12 = $666.67
first_int=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['schedule'][0]['interest'])")
check "Amz 100k/8% first interest" "$first_int" "666.67" "1.0"

# --------------------------------------------------------
echo "--- 4. Present Value ---"
# --------------------------------------------------------

# $10,000 in 1 year at 8% -> PV ~ $9,259 (exact depends on basis)
r=$(curl -s -X POST $API/api/presentvalue/calc -H 'Content-Type: application/json' \
  -d '{"asOfDate":"2024-01-01","rate":0.08,"lumpSums":[{"date":"2025-01-01","amount":10000}]}')
pv=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['sumValue'])")
check "PV $10k/1yr/8%" "$pv" "9259.26" "200"

# $10,000 in 1 year at 6% -> PV ~ $9,417.65
r=$(curl -s -X POST $API/api/presentvalue/calc -H 'Content-Type: application/json' \
  -d '{"asOfDate":"2024-01-01","rate":0.06,"lumpSums":[{"date":"2025-01-01","amount":10000}]}')
pv=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['sumValue'])")
check "PV $10k/1yr/6%" "$pv" "9417.65" "50"

# $10,000 at same date -> PV = $10,000 (no discounting)
r=$(curl -s -X POST $API/api/presentvalue/calc -H 'Content-Type: application/json' \
  -d '{"asOfDate":"2024-01-01","rate":0.06,"lumpSums":[{"date":"2024-01-01","amount":10000}]}')
pv=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['sumValue'])")
check "PV same-date" "$pv" "10000" "0.01"

# $10,000 in past (1 year ago) at 6% -> FV > $10,000
r=$(curl -s -X POST $API/api/presentvalue/calc -H 'Content-Type: application/json' \
  -d '{"asOfDate":"2025-01-01","rate":0.06,"lumpSums":[{"date":"2024-01-01","amount":10000}]}')
fv=$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['sumValue'])")
fv_ok=$(python3 -c "print('yes' if $fv > 10000 else 'no')")
if [ "$fv_ok" = "yes" ]; then PASS=$((PASS+1)); else FAIL=$((FAIL+1)); ERRORS="${ERRORS}\n  FAIL: PV future value > 10000: got=$fv"; fi

# --------------------------------------------------------
echo "--- 5. Interest Math (via Go test) ---"
# --------------------------------------------------------

# Run the existing Go unit tests which cover refdata.json values
go_result=$(cd /Volumes/SSK/persense/persense-port && go test ./internal/finance/interest/... -count=1 2>&1)
if echo "$go_result" | grep -q "^ok"; then
  PASS=$((PASS+1))
  echo "  Go interest tests: PASS"
else
  FAIL=$((FAIL+1))
  ERRORS="${ERRORS}\n  FAIL: Go interest tests failed"
  echo "  Go interest tests: FAIL"
  echo "$go_result"
fi

# --------------------------------------------------------
echo "--- 6. Date Utils (via Go test) ---"
# --------------------------------------------------------

go_result=$(cd /Volumes/SSK/persense/persense-port && go test ./internal/dateutil/... -count=1 2>&1)
if echo "$go_result" | grep -q "^ok"; then
  PASS=$((PASS+1))
  echo "  Go dateutil tests: PASS"
else
  FAIL=$((FAIL+1))
  ERRORS="${ERRORS}\n  FAIL: Go dateutil tests failed"
  echo "  Go dateutil tests: FAIL"
  echo "$go_result"
fi

# --------------------------------------------------------
echo "--- 7. All Go Unit Tests ---"
# --------------------------------------------------------

go_result=$(cd /Volumes/SSK/persense/persense-port && go test ./... -count=1 2>&1)
fail_count=$(echo "$go_result" | grep -c "^FAIL" || true)
if [ "$fail_count" -eq 0 ]; then
  PASS=$((PASS+1))
  echo "  All Go tests: PASS"
else
  FAIL=$((FAIL+1))
  ERRORS="${ERRORS}\n  FAIL: Some Go tests failed"
  echo "$go_result" | grep "FAIL"
fi

# --------------------------------------------------------
echo ""
echo "============================================"
echo "  RESULTS: $PASS passed, $FAIL failed"
echo "============================================"
if [ $FAIL -gt 0 ]; then
  echo ""
  echo "Failures:"
  echo -e "$ERRORS"
fi
echo ""
