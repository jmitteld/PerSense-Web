package types

import "strings"

// Result-sanity advisory tiers. See docs/result_warning_layer_spec.md.
//
//	AdvisoryTier ("advisory") — the calc ran and produced a valid number,
//	  but it is almost certainly not what the user intended (e.g. a balloon
//	  that solves to ~0 because the supplied payment already amortizes the
//	  loan). Surfaced amber, non-blocking.
//	NoteTier ("note") — a legitimate but noteworthy situation worth saying
//	  out loud (intentional negative amortization, a final payment that
//	  differs from the regular one). Surfaced grey, non-blocking.
const (
	AdvisoryTier = "advisory"
	NoteTier     = "note"
)

// advisoryPrefix marks a Warnings entry as a structured result-sanity
// advisory. The header that follows is "tier|code|f1,f2@@ " and then the
// human message. The frontend parses the header to choose styling and to
// highlight the named cells, then strips it for display. Plain (un-prefixed)
// Warnings entries still render as default advisories, so existing
// engine warnings are unaffected.
const advisoryPrefix = "@@ADV|"

// FormatAdvisory serializes a result-sanity advisory into a single string
// suitable for a result's Warnings channel.
//
//	tier   — AdvisoryTier or NoteTier
//	code   — stable identifier, e.g. "M-W1" (see the spec's predicate tables)
//	fields — cell identifiers the message refers to (frontend data-field
//	         names / row keys); may be empty
//	msg    — the human-readable, action-oriented message
//
// Carrying the structure inside the Warnings string keeps this layer
// additive: no engine result or API response struct changes shape, and the
// messages flow through the warnings channel the handlers already forward.
func FormatAdvisory(tier, code string, fields []string, msg string) string {
	return advisoryPrefix + tier + "|" + code + "|" + strings.Join(fields, ",") + "@@ " + msg
}
