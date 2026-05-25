package api

// FieldError is a structured, machine-readable error that names the
// offending field(s) and row so the frontend can highlight the exact
// cell instead of regex-matching free-text message strings (the old
// explainMtgError approach). It implements the error interface, so it
// can also flow through the existing `error`-typed engine returns.
//
// Proposed in docs/dispatch_gaps.md §4.3.
type FieldError struct {
	// Code is a stable identifier the frontend can dispatch on,
	// e.g. "AMORT_PREPAY_INCOMPLETE".
	Code string `json:"code"`
	// Message is the human-readable, field-named text.
	Message string `json:"message"`
	// Fields lists the affected fields in UI-label form.
	Fields []string `json:"fields,omitempty"`
	// RowIdx is the 1-based row index; 0 means a screen-level error.
	RowIdx int `json:"rowIdx,omitempty"`
	// Block identifies the input block: "mortgage", "lumpsum",
	// "periodic", "prepayment", "balloon", "adjustment", ...
	Block string `json:"block,omitempty"`
}

// Error implements the error interface.
func (e *FieldError) Error() string { return e.Message }

// newFieldError is a small constructor for the row-indexed errors the
// handlers produce for advanced-option blocks.
func newFieldError(code, block string, rowIdx int, fields []string, msg string) *FieldError {
	return &FieldError{Code: code, Block: block, RowIdx: rowIdx, Fields: fields, Message: msg}
}
