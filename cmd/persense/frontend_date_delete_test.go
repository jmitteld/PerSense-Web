package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Tests for CORRECTING a date field — backspace, forward-delete, and retyping a
// highlighted segment (static/index.html: maskDateInput's non-autoComplete
// branch, plus dateValidity).
//
// Client UI report #8 (2026-08-10 triage, reproduced at r48 and re-measured at
// r59 against HEAD d143ee2): correcting a date scrambled the field. The mask's
// delete / mid-string branch discarded the user's segment boundaries — it
// concatenated every digit in the field and re-sliced at fixed 2/2/4 positions
// — so with any digit count other than 8 every digit after the edit point
// shifted one segment left:
//
//	12/15/2026, backspace the "5"        -> 12/12/026
//	12/15/2026, highlight "15", type "1" -> 12/12/026
//	11/15/2026, backspace a month digit  -> 11/52/026
//
// Measured breadth before the fix, over THIS TEST'S OWN population (150 single
// digit deletions + 54 single-digit segment retypes = 204 corrections over the
// 11 field states in FIELDS below): 148 of the 204 relocated a digit into the
// wrong segment. On a complete field EVERY deletion in the month or the day did
// it, by backspace and by forward-delete alike.
//
// THE INVARIANT THIS FILE PINS: a single-character edit never moves a digit
// ACROSS a delimiter. Deleting one digit from the day changes only the day.
// Expressed as: split the field on its delimiter, keep the digits of each
// segment, and that list must equal the list you get from the raw string with
// that one character removed.
//
// Deleting a DELIMITER is the deliberate exception — the remaining digits
// legitimately re-flow back into MM/DD/YYYY — and is pinned separately by name
// rather than swept, so the exception cannot silently widen.
//
// Like frontend_dob_year_test.go and frontend_date_delim_test.go this extracts
// the SHIPPED functions from index.html and runs them under Node, so the code
// under test is the shipped code and not a copy. Skips when node is absent.
//
// extractJSFunc is defined in frontend_dob_year_test.go (same package).
func TestFrontendDateFieldCorrectionJS(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend JS test")
	}
	htmlBytes, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(htmlBytes)

	harness := `
'use strict';
function saveStateSoon() {}
// Deterministic clock: the DOB century pivot must not depend on the wall date.
const RealDate = Date;
Date = class extends RealDate { getFullYear() { return 2026; } };

` + extractJSFunc(t, html, "maskDateInput") + `

` + extractJSFunc(t, html, "dateValidity") + `

` + extractJSFunc(t, html, "inferAmzDateFromLoan") + `

` + extractJSFunc(t, html, "parseDate") + `

// amzPeriodMonths reads a <select> on the live page; the default worksheet is
// monthly, which is the case this pin cares about.
function amzPeriodMonths() { return 1; }

` + extractJSFunc(t, html, "computeDefaultFirstPayment") + `

` + extractJSFunc(t, html, "fmtDateDisplay") + `

` + extractJSFunc(t, html, "amzAddMonths") + `

` + extractJSFunc(t, html, "maybeFillFirstPaymentDefault") + `

// mkEl is a stand-in for the date <input>: the mask reads value, selectionStart
// and dataset, and writes value plus the caret. Nothing else of the DOM is
// touched by the function under test.
function mkEl(value, caret, id, delim) {
  return {
    id: id || 'amz-loan-date',
    value: value,
    placeholder: 'MM/DD/YYYY',
    readOnly: false,
    selectionStart: caret === null || caret === undefined ? value.length : caret,
    dataset: delim ? { delim: delim } : {},
    setSelectionRange: function (a) { this.selectionStart = a; },
  };
}

// applyEdit performs the BROWSER's own edit first and then calls the mask
// exactly as the shipped capture-phase listener does (index.html, the
// document 'input' handler: isDelete from inputType, and a single non-digit
// insertion passed through as the separator character).
//   backspace  - collapsed caret, deleteContentBackward
//   del        - collapsed caret, deleteContentForward
//   seldelete  - {a,b}: a highlighted range deleted, deleteContentBackward
//   selreplace - {a,b,ch}: selection replaced by a typed character, insertText
//   type       - a character inserted at the caret, insertText
// Returns null when the keystroke is a no-op at that caret (nothing to delete).
function applyEdit(el, kind, arg) {
  const v = el.value, p = el.selectionStart;
  let isDelete = false, data = null;
  if (kind === 'backspace') {
    if (p === 0) return null;
    el.value = v.slice(0, p - 1) + v.slice(p); el.selectionStart = p - 1; isDelete = true;
  } else if (kind === 'del') {
    if (p >= v.length) return null;
    el.value = v.slice(0, p) + v.slice(p + 1); el.selectionStart = p; isDelete = true;
  } else if (kind === 'seldelete') {
    if (arg.a >= arg.b) return null;
    el.value = v.slice(0, arg.a) + v.slice(arg.b); el.selectionStart = arg.a; isDelete = true;
  } else if (kind === 'selreplace') {
    el.value = v.slice(0, arg.a) + arg.ch + v.slice(arg.b);
    el.selectionStart = arg.a + arg.ch.length; data = arg.ch;
  } else if (kind === 'type') {
    el.value = v.slice(0, p) + arg + v.slice(p); el.selectionStart = p + 1; data = arg;
  }
  const sepChar = (!isDelete && typeof data === 'string' &&
                   data.length === 1 && /\D/.test(data)) ? data : '';
  maskDateInput(el, isDelete, sepChar);
  return { value: el.value, caret: el.selectionStart };
}

// segDigits is the invariant's shape: the digits of each delimiter-separated
// segment, in order. TRAILING EMPTY segments are trimmed off both sides of the
// comparison, because the mask deliberately drops them (deleting a year leaves
// "12/15", not "12/15/") and that is a change of shape, not of digits — the
// thing this invariant is about is whether a digit crossed a delimiter.
function segDigits(s, d) {
  const parts = s.split(d).map(function (x) { return x.replace(/\D/g, ''); });
  let last = parts.length;
  while (last > 1 && parts[last - 1] === '') last--;
  return parts.slice(0, last).join('|');
}

// ---- the swept population -------------------------------------------------
// Field states a user can actually be looking at when they reach for backspace:
// padded and unpadded months and days, both delimiters, a leap day, a
// far-future year, and two partially-entered fields.
const FIELDS = ['12/15/2026', '01/05/2026', '1/15/2026', '11/15/2026',
                '03/31/2099', '02/29/2024', '9/9/2026', '12-15-2026',
                '01-05-2026', '12/15/', '12/'];

// REFLOW is a second population the sweep needs, because on the first one the
// corrected mask mostly agrees with the browser's own splice and therefore
// assigns nothing (out === raw, an early return). A sweep that only ever
// exercised that case would be satisfied by applyEdit rather than by the code
// under test (R76 — a green that had no power). These states are missing a
// delimiter, so the mask MUST rewrite them, and maskActed below counts it.
const REFLOW = ['12152026', '12/152026', '1215/2026', '12-152026',
                '1/152026', '121520', '1215'];

const violations = [];
let digitDeletions = 0, segmentRetypes = 0, changedAtLeastOnce = 0;
let maskActed = 0, validWithLostDigits = 0;
const lostDigitCases = [];

// noDigitLoss is the invariant that makes a corruption LOUD instead of silent:
// after a correction, if the field is one dateValidity ACCEPTS, then it must
// hold exactly the digits the user's own keystroke left in it. A mask that
// dropped, duplicated or re-sliced a digit into an acceptable-looking date
// would put a wrong number in front of the engine; one that leaves the field
// unacceptable is caught by blockInvalidDates before any calculation.
function checkDigits(browserValue, masked, label, start) {
  if (masked !== browserValue) maskActed++;
  if (!dateValidity(masked).valid) return;
  const want = browserValue.replace(/\D/g, '');
  const got = masked.replace(/\D/g, '');
  if (want !== got) {
    validWithLostDigits++;
    lostDigitCases.push({ start: start, label: label, browser: browserValue,
                          masked: masked, want: want, got: got });
  }
}

for (const start of FIELDS) {
  const d = start.indexOf('-') >= 0 ? '-' : '/';
  // (1) every single-DIGIT deletion, by backspace and by forward delete
  for (let i = 0; i < start.length; i++) {
    if (!/\d/.test(start[i])) continue;          // delimiters pinned by name below
    const want = segDigits(start.slice(0, i) + start.slice(i + 1), d);
    for (const kind of ['backspace', 'del']) {
      const caret = kind === 'backspace' ? i + 1 : i;
      const el = mkEl(start, caret, 'amz-loan-date', d);
      const r = applyEdit(el, kind, null);
      if (!r) continue;
      digitDeletions++;
      if (r.value !== start) changedAtLeastOnce++;
      checkDigits(start.slice(0, i) + start.slice(i + 1), r.value, kind + '@' + i, start);
      const got = segDigits(r.value, d);
      if (got !== want) {
        violations.push({ start: start, kind: kind, index: i,
                          want: want, got: got, field: r.value });
      }
    }
  }
  // (2) highlight a two-digit segment and retype it with ONE digit — the
  //     correction the client actually made.
  const segs = start.split(d);
  let off = 0;
  for (let s = 0; s < segs.length; s++) {
    if (segs[s].length === 2) {
      for (const ch of ['1', '3', '7']) {
        const a = off, b = off + 2;
        const want = segDigits(start.slice(0, a) + ch + start.slice(b), d);
        const el = mkEl(start, a, 'amz-loan-date', d);
        const r = applyEdit(el, 'selreplace', { a: a, b: b, ch: ch });
        segmentRetypes++;
        if (r.value !== start) changedAtLeastOnce++;
        checkDigits(start.slice(0, a) + ch + start.slice(b), r.value, 'retype-' + ch, start);
        const got = segDigits(r.value, d);
        if (got !== want) {
          violations.push({ start: start, kind: 'retype-' + ch, index: a,
                            want: want, got: got, field: r.value });
        }
      }
    }
    off += segs[s].length + 1;
  }
}

// (3) MID-STRING INSERTION. The correction the client makes after a deletion:
//     put the caret back and type the digit. Every insertion at every interior
//     position, over the same field states. No invariant on the SHAPE here —
//     the mask may legitimately reflow — but checkDigits forbids any result
//     that is ACCEPTED while holding different digits than the user typed.
let insertions = 0;
for (const start of FIELDS) {
  for (let p = 1; p < start.length; p++) {
    for (const ch of ['0', '5']) {
      const el = mkEl(start, p, 'amz-loan-date', start.indexOf('-') >= 0 ? '-' : '/');
      const r = applyEdit(el, 'type', ch);
      insertions++;
      checkDigits(start.slice(0, p) + ch + start.slice(p), r.value, 'insert-' + ch + '@' + p, start);
    }
  }
}

// (4) THE REFLOW POPULATION — states where the mask MUST rewrite the field.
//     This is what gives the sweep power over the shipped implementation
//     rather than over applyEdit's own splice.
const reflowed = [];
for (const start of REFLOW) {
  const d = start.indexOf('-') >= 0 ? '-' : '/';
  const el = mkEl(start, start.length, 'amz-loan-date', d);
  const r = applyEdit(el, 'backspace', null);
  reflowed.push({ start: start, got: r.value });
  checkDigits(start.slice(0, start.length - 1), r.value, 'reflow', start);
}

// ---- named rows: the client's own keystrokes ------------------------------
function one(v, caret, kind, arg, delim) {
  const el = mkEl(v, caret, 'amz-loan-date', delim || '/');
  const r = applyEdit(el, kind, arg);
  return { value: r.value, caret: r.caret, valid: dateValidity(r.value).valid };
}

const out = {
  power: {
    digitDeletions: digitDeletions,
    segmentRetypes: segmentRetypes,
    insertions: insertions,
    changedAtLeastOnce: changedAtLeastOnce,
    maskActed: maskActed,
  },
  violations: violations,
  lostDigits: { count: validWithLostDigits, cases: lostDigitCases.slice(0, 8) },
  reflowed: reflowed,
  client: {
    // 12/15/2026, caret between the 5 and the /, Backspace
    backspaceDay:  one('12/15/2026', 5, 'backspace', null),
    // highlight the day "15" and type "1"
    retypeDay:     one('12/15/2026', 3, 'selreplace', { a: 3, b: 5, ch: '1' }),
    // backspace a month digit of 11/15/2026
    backspaceMon:  one('11/15/2026', 2, 'backspace', null),
    // forward-delete a month digit
    deleteMon:     one('12/15/2026', 1, 'del', null),
    // and the same on a dash field
    dashDay:       one('12-15-2026', 5, 'backspace', null, '-'),
  },
  // Deleting a DELIMITER deliberately re-flows the remaining digits.
  delimiter: {
    first:  one('12/15/2026', 3, 'backspace', null),
    second: one('12/15/2026', 6, 'backspace', null),
    dash:   one('12-15-2026', 3, 'backspace', null, '-'),
  },
  // The field must recover once the digit count is whole again, with the caret
  // left where the next digit belongs.
  repair: (function () {
    const el = mkEl('12/15/2026', 5, 'amz-loan-date', '/');
    const a = applyEdit(el, 'backspace', null);
    const b = applyEdit(el, 'type', '4');
    return { afterDelete: a.value, caretAfterDelete: a.caret,
             afterRetype: b.value, valid: dateValidity(b.value).valid };
  })(),
  // Clearing the field must unlock the delimiter so a fresh entry can pick
  // either one.
  cleared: (function () {
    const el = mkEl('1', 1, 'amz-loan-date', '-');
    const r = applyEdit(el, 'backspace', null);
    return { value: r.value, delim: el.dataset.delim };
  })(),
  // THE TRAP THE FIRST CUT OF THIS FIX WALKED INTO. Preserving the user's
  // segments means a ONE-DIGIT month or day can now persist in the field. The
  // forward-typing branch must therefore not discard what follows a short
  // segment: before this was pinned, appending a character to 1/15/ produced
  // the single character "1" and the year was gone.
  trap: (function () {
    const el = mkEl('11/15/2026', 2, 'amz-loan-date', '/');
    const afterDelete = applyEdit(el, 'backspace', null).value;
    const yStart = el.value.lastIndexOf('/') + 1;
    el.selectionStart = yStart;
    const afterYearCleared = applyEdit(el, 'seldelete', { a: yStart, b: el.value.length }).value;
    let last = afterYearCleared;
    for (const ch of '2027') { el.selectionStart = el.value.length; last = applyEdit(el, 'type', ch).value; }
    return { afterDelete: afterDelete, afterYearCleared: afterYearCleared, afterRetypedYear: last };
  })(),
  // AN OVER-LONG SEGMENT IS LEFT VISIBLE AND REJECTED, never re-sliced into a
  // different complete-looking date and never truncated. Both of those are
  // silent; this is loud, and blockInvalidDates stops the calculation.
  overlong: {
    dayGrew:  one('1/15/2026', 2, 'type', '0'),
    yearGrew: one('2/15/2026', 6, 'type', '0'),
  },
  // Deleting every digit leaves an EMPTY field, not a stranded delimiter.
  emptied: (function () {
    const el = mkEl('12/', 0, 'amz-loan-date', '/');
    const r = applyEdit(el, 'seldelete', { a: 0, b: 2 });
    const el2 = mkEl('12/15/2026', 0, 'amz-loan-date', '/');
    applyEdit(el2, 'seldelete', { a: 0, b: 2 });
    applyEdit(el2, 'seldelete', { a: 1, b: 3 });
    const el3 = mkEl('1', 1, 'amz-loan-date', '/');
    const r3 = applyEdit(el3, 'backspace', null);
    return { partial: r.value, partialDelim: el.dataset.delim,
             monthAndDayGone: el2.value, lastDigit: r3.value };
  })(),
  // The caret after a rewrite the mask actually performed (out !== raw), so
  // the caret block is exercised rather than skipped by the early return.
  caret: (function () {
    // Caret INTERIOR to the field and a value the mask actually rewrote, so the
    // caret block runs AND its digit count is load-bearing: with the caret at
    // the end, or on a value the mask leaves alone, an off-by-one in that loop
    // lands on the same index and the assertion has no power (R84).
    const el = mkEl('12/152026', 5, 'amz-loan-date', '/');
    const r = applyEdit(el, 'backspace', null);
    // A SECOND row where the digit count before the caret differs between the
    // browser's value and the mask's output. Counting from the wrong one of
    // those two strings is invisible at caret 5 and visible here.
    const el2 = mkEl('12/152026', 7, 'amz-loan-date', '/');
    const r2 = applyEdit(el2, 'backspace', null);
    return { value: r.value, caret: r.caret, value2: r2.value, caret2: r2.caret };
  })(),
  // MULTI-KEYSTROKE JOURNEYS. A single edit is not how a user corrects a date;
  // they delete, retype, fat-finger and keep going. Each journey below ends in
  // a state that an EARLIER cut of this fix left GREEN while holding a
  // different year than the user typed — a wrong date the engine would have
  // accepted. Every one of them must end REJECTED.
  journeys: (function () {
    function run(startValue, steps) {
      const el = mkEl(startValue, 0, 'amz-loan-date', startValue.indexOf('-') >= 0 ? '-' : '/');
      const trail = [];
      for (const st of steps) {
        el.selectionStart = st.at === 'end' ? el.value.length : st.at;
        if (st.del) applyEdit(el, 'backspace', null);
        else applyEdit(el, 'type', st.ch);
        trail.push(el.value);
      }
      return { trail: trail, value: el.value, valid: dateValidity(el.value).valid };
    }
    return {
      // correct the month, fat-finger a digit into the year, keep typing
      shortMonth: run('12/15/2026', [{ at: 2, del: true }, { at: 6, ch: '0' },
                                     { at: 'end', ch: '7' }, { at: 'end', ch: '/' }]),
      // type a digit into a full day, then keep typing at the end
      fullDay:    run('12/15/2026', [{ at: 3, ch: '0' }, { at: 'end', ch: '7' },
                                     { at: 'end', ch: '/' }, { at: 'end', ch: '9' }]),
      // a stray delimiter before the year must not eat the year
      strayDelim: run('12/15/2026', [{ at: 6, ch: '/' }]),
      strayMid:   run('12/15/2026', [{ at: 5, ch: '/' }]),
      // An OVER-LONG DAY whose spill leaves the year exactly one digit too
      // long. Only the raw-segment half of the year-cap gate stops this one:
      // the length half is satisfied, so capping would hand back a green
      // 12/01/5526 for a date the user never typed.
      overlongDay: run('12/15/26', [{ at: 3, ch: '0' }, { at: 4, ch: '5' },
                                    { at: 'end', ch: '7' }]),
      // A one-digit month AND a one-digit day with no year yet: the only
      // shape in which the reassembly's second disjunct is load-bearing.
      shortBoth:  run('12/15/2026', [{ at: 2, del: true }, { at: 10, del: true },
                                     { at: 9, del: true }, { at: 8, del: true },
                                     { at: 7, del: true }, { at: 6, del: true },
                                     { at: 5, del: true }, { at: 4, del: true },
                                     { at: 'end', ch: '5' }]),
    };
  })(),
  // The one-digit DAY half of the trap — the month half alone leaves the
  // second condition of the reassembly untested.
  trapDay: (function () {
    const el = mkEl('12/15/2026', 5, 'amz-loan-date', '/');
    const afterDelete = applyEdit(el, 'backspace', null).value;   // 12/1/2026
    el.selectionStart = el.value.length;
    const afterAppend = applyEdit(el, 'type', '7').value;
    return { afterDelete: afterDelete, afterAppend: afterAppend };
  })(),
  // A field the user locked to "-" that a programmatic write filled with "/".
  // The correction path must still preserve segments rather than re-slicing.
  delimMismatch: (function () {
    const el = mkEl('03/01/2026', 5, 'amz-loan-date', '-');
    return { value: applyEdit(el, 'backspace', null).value };
  })(),
  // THE PERIMETER. The mask is not the only thing that reads these fields.
  // Preserving the user's segments means a field whose year has been deleted
  // now reads "12/15/" and not "12/15", and the amortization smart-year
  // inference — which fills the year from the loan date on blur — matched only
  // the second shape. Deleting a year to retype it is the exact keystroke this
  // round exists to support, so the inference has to accept both.
  smartYear: (function () {
    // END TO END, because the interesting statement is about the SHAPE the
    // mask leaves behind, not about the regex. Deleting a year must leave a
    // bare MM/DD that the inference picks up on blur (it did before this fix
    // and must still). Typing a date and stopping after the day must NOT —
    // the mask leaves a trailing delimiter there, the inference has never
    // fired on it, and reviving it would commit a year the user never typed.
    function shapeAfterDeletingYear(v) {
      const el = mkEl(v, 6, 'amz-firstDate', v.indexOf('-') >= 0 ? '-' : '/');
      return applyEdit(el, 'seldelete', { a: 6, b: v.length }).value;
    }
    function shapeAfterTyping(text) {
      const el = mkEl('', 0, 'amz-firstDate');
      for (const ch of text) { el.selectionStart = el.value.length; applyEdit(el, 'type', ch); }
      return el.value;
    }
    const deleted = shapeAfterDeletingYear('12/15/2026');
    const dashDeleted = shapeAfterDeletingYear('12-15-2026');
    const typed = shapeAfterTyping('1215');
    return {
      deletedShape:   deleted,
      deletedInfers:  inferAmzDateFromLoan(deleted, '2026-06-01'),
      dashShape:      dashDeleted,
      dashInfers:     inferAmzDateFromLoan(dashDeleted, '2026-06-01'),
      typedShape:     typed,
      typedInfers:    inferAmzDateFromLoan(typed, '2026-06-01'),
      partialYear:    inferAmzDateFromLoan('12/15/2', '2026-06-01'),
      complete:       inferAmzDateFromLoan('12/15/2026', '2026-06-01'),
      empties:        inferAmzDateFromLoan('12//', '2026-06-01'),
    };
  })(),
  // A PASTED or restored value can be non-canonical from the first keystroke,
  // which is how the century-strip below can shorten a year into a DIFFERENT
  // complete one. Both the strip and the cap are gated on the same shape.
  pasted: (function () {
    const el = mkEl('1/15/2026', null, 'amz-loanDate', '/');
    el.dataset.yearAuto = '2026';
    el.selectionStart = el.value.length;
    const a = applyEdit(el, 'type', '6').value;
    el.selectionStart = el.value.length;
    const b = applyEdit(el, 'type', '7').value;
    return { afterFirst: a, afterSecond: b, valid: dateValidity(b).valid };
  })(),
  // The century strip must still be CLEARED by a correction, or a later
  // keystroke strips digits the user typed. This is a plain typing journey.
  yearAutoCleared: (function () {
    const el = mkEl('', 0, 'amz-loanDate');
    for (const ch of '121526') { el.selectionStart = el.value.length; applyEdit(el, 'type', ch); }
    const expanded = el.value;
    el.selectionStart = el.value.length;
    applyEdit(el, 'backspace', null);
    for (const ch of '65') { el.selectionStart = el.value.length; applyEdit(el, 'type', ch); }
    return { expanded: expanded, value: el.value };
  })(),
  // A value written programmatically always uses "/", so a "-"-locked field
  // can arrive holding the other delimiter — and a user can type one in.
  mixedDelim: {
    dashIntoSlashField: one('12/15/2026', 0, 'type', '-'),
    slashAtFront:       one('12/15/2026', 0, 'type', '/'),
    extraInsideYear:    one('12/15/2026', 8, 'type', '/'),
  },
  // THE DERIVED WRITE. maybeFillFirstPaymentDefault reads the LOAN DATE and
  // writes a default into a DIFFERENT field. parseDate is looser than
  // dateValidity — it accepts an over-long segment the app paints red — so
  // without a validity gate a red loan date silently paints a plausible and
  // wrong 1st Payment Date. That state is newly reachable now that the mask
  // preserves the user's segments instead of re-slicing them.
  derivedWrite: (function () {
    const fields = {};
    function stub(id, value) { fields[id] = { id: id, value: value, dataset: {}, title: '' }; return fields[id]; }
    globalThis.document = { getElementById: function (id) { return fields[id] || null; } };
    function run(loanValue) {
      stub('amz-loanDate', loanValue);
      const fp = stub('amz-firstDate', '');
      maybeFillFirstPaymentDefault();
      return fp.value;
    }
    const out = {
      good:     run('12/15/2026'),
      overlong: run('121-15-2026'),
      shortMon: run('1/15/2026'),
      empty:    run(''),
    };
    delete globalThis.document;
    return out;
  })(),
  // FORWARD typing is a different branch and must be untouched by any change
  // here: these are the padding / auto-advance / year-expansion behaviours.
  typing: (function () {
    function typeInto(id, text) {
      const el = mkEl('', 0, id);
      for (const ch of text) {
        el.selectionStart = el.value.length;
        applyEdit(el, 'type', ch);
      }
      return el.value;
    }
    return {
      digits:    typeInto('amz-loan-date', '03152026'),
      padMonth:  typeInto('amz-loan-date', '3152026'),
      slashes:   typeInto('amz-loan-date', '03/15/2026'),
      dashes:    typeInto('amz-loan-date', '03-15-2026'),
      yearShort: typeInto('amz-loan-date', '12/15/26'),
      sepCommit: typeInto('amz-loan-date', '1/1/2026'),
      dob50:     typeInto('actu-dob1', '07-20-50'),
      dob10:     typeInto('actu-dob1', '03-15-10'),
    };
  })(),
};
console.log(JSON.stringify(out));
`

	cmd := exec.Command(nodePath, "-")
	cmd.Stdin = strings.NewReader(harness)
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, stdout)
	}

	type edit struct {
		Value string `json:"value"`
		Caret int    `json:"caret"`
		Valid bool   `json:"valid"`
	}
	var r struct {
		Power struct {
			DigitDeletions     int `json:"digitDeletions"`
			SegmentRetypes     int `json:"segmentRetypes"`
			Insertions         int `json:"insertions"`
			ChangedAtLeastOnce int `json:"changedAtLeastOnce"`
			MaskActed          int `json:"maskActed"`
		} `json:"power"`
		LostDigits struct {
			Count int `json:"count"`
			Cases []struct {
				Start   string `json:"start"`
				Label   string `json:"label"`
				Browser string `json:"browser"`
				Masked  string `json:"masked"`
				Want    string `json:"want"`
				Got     string `json:"got"`
			} `json:"cases"`
		} `json:"lostDigits"`
		Reflowed []struct {
			Start string `json:"start"`
			Got   string `json:"got"`
		} `json:"reflowed"`
		Violations []struct {
			Start string `json:"start"`
			Kind  string `json:"kind"`
			Index int    `json:"index"`
			Want  string `json:"want"`
			Got   string `json:"got"`
			Field string `json:"field"`
		} `json:"violations"`
		Client struct {
			BackspaceDay, RetypeDay, BackspaceMon, DeleteMon, DashDay edit
		} `json:"client"`
		Delimiter struct {
			First, Second, Dash edit
		} `json:"delimiter"`
		Repair struct {
			AfterDelete      string `json:"afterDelete"`
			CaretAfterDelete int    `json:"caretAfterDelete"`
			AfterRetype      string `json:"afterRetype"`
			Valid            bool   `json:"valid"`
		} `json:"repair"`
		Cleared struct {
			Value string `json:"value"`
			Delim string `json:"delim"`
		} `json:"cleared"`
		Trap struct {
			AfterDelete      string `json:"afterDelete"`
			AfterYearCleared string `json:"afterYearCleared"`
			AfterRetypedYear string `json:"afterRetypedYear"`
		} `json:"trap"`
		Overlong struct {
			DayGrew, YearGrew edit
		} `json:"overlong"`
		Emptied struct {
			Partial         string `json:"partial"`
			PartialDelim    string `json:"partialDelim"`
			MonthAndDayGone string `json:"monthAndDayGone"`
			LastDigit       string `json:"lastDigit"`
		} `json:"emptied"`
		Caret struct {
			Value  string `json:"value"`
			Caret  int    `json:"caret"`
			Value2 string `json:"value2"`
			Caret2 int    `json:"caret2"`
		} `json:"caret"`
		Journeys struct {
			ShortMonth, FullDay, StrayDelim, StrayMid, OverlongDay, ShortBoth struct {
				Trail []string `json:"trail"`
				Value string   `json:"value"`
				Valid bool     `json:"valid"`
			}
		} `json:"journeys"`
		TrapDay struct {
			AfterDelete string `json:"afterDelete"`
			AfterAppend string `json:"afterAppend"`
		} `json:"trapDay"`
		DelimMismatch struct {
			Value string `json:"value"`
		} `json:"delimMismatch"`
		SmartYear struct {
			DeletedShape  string  `json:"deletedShape"`
			DeletedInfers *string `json:"deletedInfers"`
			DashShape     string  `json:"dashShape"`
			DashInfers    *string `json:"dashInfers"`
			TypedShape    string  `json:"typedShape"`
			TypedInfers   *string `json:"typedInfers"`
			PartialYear   *string `json:"partialYear"`
			Complete      *string `json:"complete"`
			Empties       *string `json:"empties"`
		} `json:"smartYear"`
		Pasted struct {
			AfterFirst  string `json:"afterFirst"`
			AfterSecond string `json:"afterSecond"`
			Valid       bool   `json:"valid"`
		} `json:"pasted"`
		YearAutoCleared struct {
			Expanded string `json:"expanded"`
			Value    string `json:"value"`
		} `json:"yearAutoCleared"`
		MixedDelim struct {
			DashIntoSlashField, SlashAtFront, ExtraInsideYear edit
		} `json:"mixedDelim"`
		DerivedWrite struct {
			Good, Overlong, ShortMon, Empty string
		} `json:"derivedWrite"`
		Typing struct {
			Digits, PadMonth, Slashes, Dashes, YearShort, SepCommit, Dob50, Dob10 string
		} `json:"typing"`
	}
	if err := json.Unmarshal(stdout, &r); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, stdout)
	}

	// POSITIVE CONTROL (R84 — reach is not power). A harness that silently
	// stopped editing would satisfy every invariant below by doing nothing.
	// Assert the sweep actually ran, and that the edits actually changed the
	// field. These counts are structural (they follow from FIELDS), so they are
	// pinned exactly: a change to the population must be deliberate.
	if r.Power.DigitDeletions != 150 {
		t.Errorf("swept digit deletions = %d, want 150 (the sweep did not run over the intended population)", r.Power.DigitDeletions)
	}
	if r.Power.SegmentRetypes != 54 {
		t.Errorf("swept segment retypes = %d, want 54", r.Power.SegmentRetypes)
	}
	if r.Power.Insertions != 170 {
		t.Errorf("swept mid-string insertions = %d, want 170", r.Power.Insertions)
	}
	if r.Power.ChangedAtLeastOnce != r.Power.DigitDeletions+r.Power.SegmentRetypes {
		t.Errorf("edits that changed the field = %d, want all %d (the harness is not editing)",
			r.Power.ChangedAtLeastOnce, r.Power.DigitDeletions+r.Power.SegmentRetypes)
	}
	// R76 — and the harder control. On a correctly-behaving mask most of the
	// swept corrections need no rewrite at all: maskDateInput returns early
	// because out === raw, and the invariant above would then be satisfied by
	// applyEdit's own splice rather than by the code under test. So assert the
	// mask ACTUALLY REWROTE the field somewhere in this run. The REFLOW
	// population exists to guarantee it can.
	//
	// ⚠️ STATED HONESTLY, because an audit pass measured it: on a CORRECT mask
	// most of the swept corrections need no rewrite at all — the browser's own
	// splice is already right and maskDateInput returns early. So the sweep's
	// invariants are, on those cases, satisfied by applyEdit rather than by the
	// function under test, and they would survive a no-op mask. Their power is
	// against a mask that ACTS WRONGLY: on pristine HEAD this same population
	// produces 148 boundary violations and 89 silently-accepted wrong dates.
	// The cases with power over the shipped implementation are the REFLOW rows,
	// the journeys, and the named rows below — and the mutation harness at
	// testplan/harness/r59_date_mask_mutants.sh is what proves it.
	// MaskActed is pinned EXACTLY, not as a floor, so that number moving is a
	// test failure rather than a silent change in what is being exercised.
	if r.Power.MaskActed != 35 {
		t.Errorf("maskDateInput rewrote the field in %d swept cases, want exactly 35 — what the sweep exercises has changed (R76)",
			r.Power.MaskActed)
	}
	if len(r.Reflowed) != 7 {
		t.Errorf("reflow population = %d screens, want 7", len(r.Reflowed))
	}

	// THE INVARIANT.
	if n := len(r.Violations); n != 0 {
		t.Errorf("%d of %d single-character corrections moved a digit across a delimiter",
			n, r.Power.DigitDeletions+r.Power.SegmentRetypes)
		for i, v := range r.Violations {
			if i == 12 {
				t.Errorf("   ... and %d more", n-12)
				break
			}
			t.Errorf("   %s  %s@%d  -> %q  (segments %s, want %s)",
				v.Start, v.Kind, v.Index, v.Field, v.Got, v.Want)
		}
	}

	eq := func(name, got, want string) {
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	// The client's own keystrokes, by name, so a regression names itself.
	eq("12/15/2026 backspace the day digit", r.Client.BackspaceDay.Value, "12/1/2026")
	eq("12/15/2026 highlight the day, type 1", r.Client.RetypeDay.Value, "12/1/2026")
	eq("11/15/2026 backspace a month digit", r.Client.BackspaceMon.Value, "1/15/2026")
	eq("12/15/2026 forward-delete a month digit", r.Client.DeleteMon.Value, "1/15/2026")
	eq("12-15-2026 backspace the day digit (dash field)", r.Client.DashDay.Value, "12-1-2026")

	// Deleting a delimiter deliberately re-flows the digits back into shape.
	eq("backspace the first delimiter", r.Delimiter.First.Value, "12/15/2026")
	eq("backspace the second delimiter", r.Delimiter.Second.Value, "12/15/2026")
	eq("backspace a dash delimiter", r.Delimiter.Dash.Value, "12-15-2026")

	// Recovery: the corrected field is whole and accepted, and the caret sat
	// where the replacement digit belonged.
	eq("after deleting the day digit", r.Repair.AfterDelete, "12/1/2026")
	if r.Repair.CaretAfterDelete != 4 {
		t.Errorf("caret after deleting the day digit = %d, want 4 (just past the surviving digit)", r.Repair.CaretAfterDelete)
	}
	eq("after typing the replacement digit", r.Repair.AfterRetype, "12/14/2026")
	if !r.Repair.Valid {
		t.Errorf("the repaired date is not accepted by dateValidity")
	}

	// Clearing the field unlocks the delimiter.
	eq("cleared field", r.Cleared.Value, "")
	eq("cleared field delimiter lock", r.Cleared.Delim, "")

	// NO SILENTLY-ACCEPTED DIGIT LOSS. A correction that leaves the field in a
	// state dateValidity ACCEPTS must hold exactly the digits the keystroke
	// left there. This is the invariant that keeps a mangled date out of the
	// engine: anything else is rejected, and blockInvalidDates stops the
	// calculation before it starts.
	if r.LostDigits.Count != 0 {
		t.Errorf("%d corrections produced an ACCEPTED date holding different digits than the user's own keystroke", r.LostDigits.Count)
		for _, c := range r.LostDigits.Cases {
			t.Errorf("   %s  %s  browser %q -> masked %q  (digits %s, want %s)",
				c.Start, c.Label, c.Browser, c.Masked, c.Got, c.Want)
		}
	}

	// Removing a delimiter still re-flows the remaining digits into shape.
	wantReflow := map[string]string{
		"12152026":  "12/15/202",
		"12/152026": "12/15/202",
		"1215/2026": "12/15/202",
		"12-152026": "12-15-202",
		"1/152026":  "1/15/202",
		"121520":    "12/15/2",
		"1215":      "12/1",
	}
	for _, rf := range r.Reflowed {
		if w, ok := wantReflow[rf.Start]; !ok {
			t.Errorf("reflow population changed: unexpected start %q", rf.Start)
		} else if rf.Got != w {
			t.Errorf("reflow %q -> %q, want %q", rf.Start, rf.Got, w)
		}
	}

	// THE TRAP. Preserving the user's segments lets a one-digit month or day
	// persist; the forward-typing branch must not discard what follows it.
	eq("trap: after deleting a month digit", r.Trap.AfterDelete, "1/15/2026")
	eq("trap: after clearing the year", r.Trap.AfterYearCleared, "1/15")
	eq("trap: after retyping the year at the end", r.Trap.AfterRetypedYear, "1/15/2027")

	// An over-long segment stays VISIBLE and REJECTED — never re-sliced into a
	// different complete date, never truncated.
	eq("a digit typed into a full day", r.Overlong.DayGrew.Value, "1/015/2026")
	if r.Overlong.DayGrew.Valid {
		t.Errorf("an over-long day segment is being ACCEPTED: %q", r.Overlong.DayGrew.Value)
	}
	eq("a digit typed into a full year", r.Overlong.YearGrew.Value, "2/15/20026")
	if r.Overlong.YearGrew.Valid {
		t.Errorf("an over-long year segment is being ACCEPTED: %q", r.Overlong.YearGrew.Value)
	}

	// No stranded delimiters when every digit is gone.
	eq("delete the digits of 12/", r.Emptied.Partial, "")
	eq("delete the digits of 12/ — delimiter unlocked", r.Emptied.PartialDelim, "")
	eq("delete the last digit", r.Emptied.LastDigit, "")

	// The caret block runs only when the mask actually rewrote the field.
	eq("caret case value", r.Caret.Value, "12/12/026")
	if r.Caret.Caret != 4 {
		t.Errorf("caret after a mask rewrite = %d, want 4 (just past the three digits that preceded it)", r.Caret.Caret)
	}
	eq("caret case 2 value", r.Caret.Value2, "12/15/226")
	if r.Caret.Caret2 != 7 {
		t.Errorf("caret after a mask rewrite that ADDED a delimiter = %d, want 7 — the digit count is taken from the BROWSER's value, not from the mask's output", r.Caret.Caret2)
	}

	// THE JOURNEYS. Each ends in a state an earlier cut of this fix accepted
	// while holding a different year than the user typed.
	journey := func(name string, got struct {
		Trail []string `json:"trail"`
		Value string   `json:"value"`
		Valid bool     `json:"valid"`
	}, wantValue string, wantValid bool) {
		if got.Value != wantValue {
			t.Errorf("journey %s ended at %q, want %q (trail %v)", name, got.Value, wantValue, got.Trail)
		}
		if got.Valid != wantValid {
			t.Errorf("journey %s: dateValidity accepted = %v, want %v — a correction must never end GREEN holding digits the user did not type (value %q)",
				name, got.Valid, wantValid, got.Value)
		}
	}
	journey("corrected month then a digit into the year", r.Journeys.ShortMonth, "1/15/200267", false)
	journey("a digit typed into a full day", r.Journeys.FullDay, "12/01/5202679", false)
	journey("a stray delimiter before the year", r.Journeys.StrayDelim, "12/15/2026", true)
	journey("a stray delimiter inside the day", r.Journeys.StrayMid, "12/15/2026", true)
	journey("an over-long day spilling into the year", r.Journeys.OverlongDay, "12/05/15267", false)
	journey("a one-digit month and day with no year", r.Journeys.ShortBoth, "1/15/", false)

	// The one-digit DAY half of the reassembly condition.
	eq("trapDay: after deleting a day digit", r.TrapDay.AfterDelete, "12/1/2026")
	eq("trapDay: appending behind a one-digit day", r.TrapDay.AfterAppend, "12/1/20267")

	// A "-"-locked field holding slashes still gets segment preservation.
	eq("delimiter-mismatched field", r.DelimMismatch.Value, "03-0-2026")

	// The two-step deletion leaves no stranded digits in the wrong segment.
	eq("month then day deleted", r.Emptied.MonthAndDayGone, "//2026")

	// THE PERIMETER — the smart-year inference must accept the shape a deleted
	// year now leaves behind, and must still refuse everything it refused.
	eqp := func(name string, got *string, want string) {
		if want == "" {
			if got != nil {
				t.Errorf("%s = %q, want null", name, *got)
			}
			return
		}
		if got == nil || *got != want {
			g := "null"
			if got != nil {
				g = *got
			}
			t.Errorf("%s = %q, want %q", name, g, want)
		}
	}
	eq("shape left by deleting a year", r.SmartYear.DeletedShape, "12/15")
	eqp("smart year fires on it", r.SmartYear.DeletedInfers, "2026-12-15")
	eq("shape left by deleting a year (dash field)", r.SmartYear.DashShape, "12-15")
	eqp("smart year fires on it (dash field)", r.SmartYear.DashInfers, "2026-12-15")
	eq("shape left by TYPING a month and day", r.SmartYear.TypedShape, "12/15/")
	eqp("smart year does NOT fire on a typed entry", r.SmartYear.TypedInfers, "")
	eqp("smart year refuses a partial year", r.SmartYear.PartialYear, "")
	eqp("smart year refuses a complete date", r.SmartYear.Complete, "")
	eqp("smart year refuses empty segments", r.SmartYear.Empties, "")

	// A pasted non-canonical value must not have its century silently stripped
	// into a different complete year.
	eq("pasted 1/15/2026 then a digit", r.Pasted.AfterFirst, "1/15/20266")
	eq("pasted 1/15/2026 then two digits", r.Pasted.AfterSecond, "1/15/202667")
	if r.Pasted.Valid {
		t.Errorf("a pasted value whose century was stripped is being ACCEPTED: %q", r.Pasted.AfterSecond)
	}

	// ...and the strip must still be cleared by a correction, or later typing
	// eats digits the user did type.
	eq("year expanded while typing", r.YearAutoCleared.Expanded, "12/15/2026")
	eq("after a backspace and two more digits", r.YearAutoCleared.Value, "12/15/2026")

	// A delimiter typed where the field already holds the other one.
	eq("a dash typed into a slash-locked field", r.MixedDelim.DashIntoSlashField.Value, "12/15/2026")
	eq("a slash typed at the front", r.MixedDelim.SlashAtFront.Value, "/12/152026")
	eq("a slash typed inside the year", r.MixedDelim.ExtraInsideYear.Value, "12/15/2026")

	// The derived write must be driven only by a date the app would accept.
	if r.DerivedWrite.Good == "" {
		t.Errorf("a valid loan date derived no 1st Payment default at all — the stub is not exercising the function")
	}
	eq("1st Payment default from a valid loan date", r.DerivedWrite.Good, "02/01/2027")
	eq("1st Payment default from an over-long loan date", r.DerivedWrite.Overlong, "")
	// A one-digit month is a VALID date (dateValidity accepts \d{1,2}), so the
	// guard must not reject it — only the shapes the app itself paints red.
	eq("1st Payment default from a one-digit month (valid)", r.DerivedWrite.ShortMon, "03/01/2026")
	eq("1st Payment default from an empty loan date", r.DerivedWrite.Empty, "")

	// FORWARD TYPING IS UNCHANGED — the other direction of rule 3. These are the
	// autoComplete behaviours; if a change to the correction branch leaks into
	// them these fail here as well as in frontend_date_delim_test.go.
	eq("type 03152026", r.Typing.Digits, "03/15/2026")
	eq("type 3152026 (month padded and advanced)", r.Typing.PadMonth, "03/15/2026")
	eq("type 03/15/2026", r.Typing.Slashes, "03/15/2026")
	eq("type 03-15-2026", r.Typing.Dashes, "03-15-2026")
	eq("type 12/15/26 (year expanded)", r.Typing.YearShort, "12/15/2026")
	eq("type 1/1/2026 (separator commits a single digit)", r.Typing.SepCommit, "01/01/2026")
	eq("type 07-20-50 into a DOB field", r.Typing.Dob50, "07-20-1950")
	eq("type 03-15-10 into a DOB field", r.Typing.Dob10, "03-15-2010")
}
