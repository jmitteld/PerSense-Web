# Bootstrap provenance — which tarball set matches which HEAD

**Note #30 (round 34): the drive's tarball set can be STALE relative to HEAD, and
its own md5 will not tell you.** An md5 matching a document proves the tarball is
INTACT, not that it is CURRENT. This file records provenance so the claim is
checkable; `claude/START_HERE.md` §1 FIX 5 carries the manifest diff that verifies
it independently of anything written here.

## How round 34 found it

The r33 set was built at 00:51 on 2026-08-05, BEFORE round 33's own review commit
`4e8e48d` (which itself came after `47e06fb` and `fddde01`). Extracting it gave a
tree that silently reverted four files:

```
docs/discrepancies.md
docs/history/README.md
docs/history/START_HERE.md
internal/finance/amortization/zzsec70_engine_route_test.go
```

The last is the one that mattered: `4e8e48d` had just closed a coverage gap in it
(the first landing pinned only six of the eight contingency-table reasons, and one
of the missing two was the table's most enriched clause). A round starting from
that tarball would have silently reintroduced the gap it was told had been closed.
`START_HERE.md` asserted the tarballs were "rebuilt after the round-33 commits."
They were not.

## The check — run it every round, it costs three minutes

1. on the drive:
   `git ls-files | while read f; do [ -f "$f" ] && md5sum "$f"; done > _to_delete/drive_manifest.txt`
2. in the container:
   `cd /root/pw && find . -type f | sed 's|^\./||' | sort | while read f; do md5sum "$f"; done > /tmp/container_manifest.txt`
3. `SendUserFile` the container manifest, then `device_commit_files` it to
   `_to_delete/`.
   **⚠️ Note #31 — push, do not pull: a file WRITTEN by `device_bash` may be a
   cloud placeholder that `device_stage_files` refuses.**
4. on the drive, `join -j2` the two and print rows whose hashes differ; `join -v1`
   for tracked files the container lacks.
5. pull every DIFF file from the drive with `device_stage_files` and copy it in.

The ~810 tracked files legitimately absent from the container are images, PDFs,
docx and `legacy/src/win_source/*.pas` — deliberately not in the tarballs.
**Nothing with a `.go`, `.py` or `.sh` extension should be absent.**

## The sets

| set | built at HEAD | date | md5 |
|---|---|---|---|
| `_to_delete/r33src.tar.gz` | ⚠️ `7d4d4e3` — **NOT** `4e8e48d`, despite the docs | 2026-08-05 00:51 | `421e45f1ef68296b7e0a15e4d306ea02` |
| `_to_delete/r33dos.tar.gz` | (unchanged since r27) | | `099986e1791a50ee80fc20438485f048` |
| `_to_delete/r33fix.tar.gz` | (unchanged since r25) | | `808e446d5561eb8d8707c701a12fc498` |
| **`_to_delete/r34src.tar.gz`** | **`605aebb`** | 2026-08-05 20:05 | `05136d0bf570ed346e937d3fd56bd133` |
| **`_to_delete/r34dos.tar.gz`** | **`605aebb`** | 2026-08-05 20:05 | `cb30536419b572da3dd2bb60b0c4d47f` |
| **`_to_delete/r34fix.tar.gz`** | **`605aebb`** | 2026-08-05 20:05 | `ced1d0d7e266171b6de7b40e1d32a5b8` |

The r34 set was verified BY EXTRACTION into a scratch tree before shipping:
**419 `.go`** (r33's 418 plus `zzsec71_walk_terminates_test.go`), 42/42 resolvable
symlinks in `legacy/oracle/units`, 34 `.pas` in `legacy/src/dos_source`, `pkg/`
present, both fixture trees present, and **855 of 855 container files present with
matching content**.

**Whoever builds the next set: record its HEAD here and in the commit message.**

## ⚠️ Snapshot debt

`docs/history/START_HERE.md` is the repo's snapshot of the live project document
`claude/START_HERE.md`, refreshed at commit time by the rule Nate set on
2026-08-01. **It is ONE ROUND STALE as of `605aebb`** — it carries the round-33
state. The round-34 live document is in the claude.ai project and is authoritative;
refresh the snapshot at the next commit. (Round 30 recorded the same debt at
eleven rounds; do not let it grow again.)
