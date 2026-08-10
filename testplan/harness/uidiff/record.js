'use strict';
// record.js — ROUND 48, ITEM 0y. WHERE A RUN'S RESULTS FILE IS ALLOWED TO LAND.
//
// WHY THIS IS ITS OWN MODULE. Round 48's first attempt at item 0y put the
// filename choice inline in run.js and pinned it with a Go source-scanning
// guard. The round's own adversarial audit then killed that guard with FOUR
// mutants it could not see, because a source scanner checks that TEXT is
// present, not that BEHAVIOUR holds:
//
//	U6  fs.writeFileSync(path.join(__dirname, 'uidiff_results.json'), ...)
//	    split across lines, bypassing the guarded selection entirely — the
//	    "baseline must not be reachable unconditionally" check was a LINE
//	    window, which is R59's condemned shape inside the file condemning it.
//	U8  const partial = false
//	U9  const errored = []          <- ROUND 47'S EXACT BUG, GUARD GREEN
//	U10 the ternary inverted: baseline on error, quarantine otherwise
//
// All four leave every needle present as text while the gates are dead. A Go
// test can never execute run.js, so no Go guard can close this. The decision is
// therefore a PURE FUNCTION, exercised by selftest.js on EVERY uidiff run —
// which is a gate that runs, not a description of one.
//
// 🚨 THE RULE, AND WHY IT IS THIS RULE:
//   - any case errored  -> quarantine. An errored case compared NOTHING, so a
//     results file containing it is a different population wearing the
//     baseline's filename (R54, R58).
//   - a tier filter set -> quarantine. Same argument: `--only=plain` scores 3
//     cases. Round 48 destroyed the committed 258-case baseline with exactly
//     this, INSIDE the fix meant to prevent it (audit F4).
//   - otherwise         -> the baseline.
// Errors take precedence over partial, so an errored partial run is reported as
// errored: it is the more serious fault and the one that must not be silent.

const BASELINE = 'uidiff_results.json';
const ERRORED = 'uidiff_results.ERRORED.json';
const PARTIAL = 'uidiff_results.PARTIAL.json';

function chooseResultsFile({ erroredCount, only }) {
  if (!Number.isInteger(erroredCount) || erroredCount < 0) {
    throw new Error(`chooseResultsFile: erroredCount must be a non-negative integer, got ${erroredCount}`);
  }
  if (erroredCount > 0) return ERRORED;
  if (only) return PARTIAL;
  return BASELINE;
}

// A run may write the baseline only when it errored on nothing AND scored every
// tier. Kept as a separate predicate so the exit-code decision in run.js cannot
// drift away from the filename decision.
function isBaselineRun({ erroredCount, only }) {
  return chooseResultsFile({ erroredCount, only }) === BASELINE;
}

module.exports = { chooseResultsFile, isBaselineRun, BASELINE, ERRORED, PARTIAL };
