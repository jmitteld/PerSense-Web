# Actuarial — the missing files to send (replaces the earlier question list)

*This supersedes `actuarial_questions_for_client.md`. That earlier list asked
about conventions (interpolation, age basis, death timing) as if we'd
reconstruct the math. That was the wrong framing — the right approach is to
**translate the existing, tested code**, not rewrite it. We only need the files
that aren't in the source snapshot we received.*

## What we confirmed is already in the source you sent

You were right that the table names are in the code. We found and verified:

- **Default table names** `MALE` and `FEMALE` — `PEDATA.pas:66`
  (`actuarialfilename:('MALE','FEMALE')`).
- **The config field** that stores the two table filenames —
  `PETYPES.pas:434` (`actuarialfilename: array[1..2] of str8`), and
  `PETYPES.dcu` still carries it, confirming it shipped compiled.
- **The contingency type system** (`N L D 1 2 E B`) and per-row contingency
  fields — `PETYPES.pas:169-171`, `act0`/`actn`.
- **The dispatch + integration you described** — the per-screen "hit Enter,
  search the fields, decide what to compute" logic and its 36 `{$ifdef ACTU}`
  hooks that call `LifeProb`, `PODValue`, and `XPODValue` — all present across
  `PRESVALU.pas`, `PVLXSCRN.pas`, `pvltable.pas`.

So the configuration and the calling code are here. We translate those directly.

## What is *not* in the snapshot we received

Two things are referenced by the code above but their actual contents aren't in
the files we got — so they appear to have been left out when the source was
packaged, not that they don't exist:

1. **The two mortality data files: `MALE` and `FEMALE`** (the 1988 HHS tables).
   No file by either name is anywhere in the tree. Whatever format they're in
   (death-probability-by-age, or survivors-out-of-100,000) is fine — the code
   that reads them auto-detects, per your note.

2. **The source that *defines* `LifeProb`, `PODValue`, `XPODValue`, and the
   table reader** (the file open + the "common sense" format detection + the
   interpolation). In our copy, `actuarialfilename` is declared and defaulted
   but never *read* anywhere, and those three functions are *called* but never
   *defined* — the file holding their bodies isn't in the snapshot. (The `uses`
   clauses even point at it: `//{$ifdef ACTU} ,ACTUARY {$endif}`, commented
   out.) Wherever that code actually lives in your archive — a unit, an include
   file, or inline in a version of these screens with `ACTU` enabled — that's
   the piece we need.

## The ask

Please send, from your archive:

- the **`MALE` and `FEMALE` table files**, and
- the **source file(s) that define `LifeProb` / `PODValue` / `XPODValue` and
  read the table** (whatever and wherever they are — a `.pas`, an include, or
  the `ACTU`-enabled version of the present-value screens).

That's it. With those, we translate your tested code line-for-line rather than
infer anything.

## How we'll prove the translation matches your original

We won't ask you to trust new code on faith — you're right that it shouldn't be
trusted until tested to the same standard. We already compile your **real** DOS
units and run them headlessly as the source of truth, then diff our translation
against them across thousands of randomized cases. We've done exactly this for
the other five engines, all matching to ~9 significant figures with zero
divergences:

| Engine | Cases | Result |
|---|---:|---|
| Amortization (incl. balloons) | ~2,400 | bit-identical to DOS |
| Present value (incl. COLA) | 1,000 | bit-identical |
| Variable-rate schedule | 500 | bit-identical |
| PV backward solves | 1,200 | bit-identical |
| Mortgage | 800 | bit-identical |

Once we have the actuarial files, we compile them into the same harness and hold
the translation to the same bar — a per-case numeric match against your original
code, not our interpretation of it.

## Your answers we've recorded (for the eventual cross-check, not for rewriting)

Noted for verification once we can run your code: payment on death is made at
the time of death; the two lives' death probabilities are independent;
probabilities integrated by month (to be confirmed against the code); first /
second / only-death benefits all handled; section-C contingency meanings all
confirmed. We'll check the translation reproduces each of these from your actual
source.
