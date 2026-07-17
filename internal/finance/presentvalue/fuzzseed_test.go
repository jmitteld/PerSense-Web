package presentvalue

import (
	"os"
	"strconv"
)

// fuzzSeed returns base unless PERSENSE_FUZZ_SEED overrides it, so the seeded
// differential fuzzers can probe fresh input batches on demand (e.g. a 500-case
// run with a new seed hunts divergences the default seed's prefix already
// cleared). Deterministic per seed value, so any finding is reproducible.
func fuzzSeed(base int64) int64 {
	if s := os.Getenv("PERSENSE_FUZZ_SEED"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
	}
	return base
}
