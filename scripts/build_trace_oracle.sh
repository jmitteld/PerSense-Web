#!/usr/bin/env bash
# build_trace_oracle.sh — build an INSTRUMENTED copy of the DOS source-oracle
# that dumps one of AMORTOP's internals to STDERR.
#
# Why: divergence hunts in the fancy solve are basin-selection problems (see
# claude/iterate_small_shadow_newton_basin_2026-07-25.md) and accumulator-state
# problems (see claude/usap_aliasing_reamortize_writeback_2026-07-25.md).
# Without DOS's own internals you have to hand-derive residuals from soft `pay=`
# probes. These builds print what DOS actually did so the Go port can be diffed
# step-for-step.
#
# Two modes:
#
#   -mode itr  (default)  AMORTOP.Iterate's Newton/secant trajectory.
#                         Output: /tmp/oracletrace/<TARGET>_trace
#                         Lines:  ITR0 seedx=.. p=..
#                                 ITR n=.. p=.. delta=.. newx=..
#                                 ITRend bestp=.. bestx=.. count=..
#                         Diff against the port's GITR0/GITR/GITRend, which
#                         dosport_walk.go emits under DPTRACE=1.
#
#   -mode cn              Paymenttype.ComputeNext, one line per computed row.
#                         Output: /tmp/oraclecn/<TARGET>_cn
#                         Lines:  CN d=<y-m-d> bpos=<balloonpos> p=<p_in>
#                                 usap=<usap_in> int=.. pay=.. td=..
#                                 pout=<p_out> uout=<usap_out>
#                         NOTE: years are offset by 1900 (129-12-1 = 2029-12-01).
#                         This is the tracer that exposed Re_Amortize's write-back
#                         through the unit-level `usap` global (AMORTOP.pas:73,
#                         :1508, :1610).
#
#   -mode apr             Amortize.EstimateAndRefineAPRwithPoints's secant
#                         trajectory (Amortize.pas:516-600).
#                         Output: /tmp/oracleapr/<TARGET>_apr
#                         Lines:  APR0 seed=.. oldvalue=.. target=.. ovf=..
#                                 APR n=.. vrate=.. aprvalue=.. denom=..
#                                     delta=.. newvrate=.. ovf=..
#                                 APRend count=.. delta=.. vrate=.. ovf=..
#                         `seed` is DOS's own first guess — note DOS seeds
#                         h^.loanrate ONLY when loanratestatus > defp (an INPUT
#                         rate) and 0.1 otherwise. `ovf` is the global
#                         overflowflag, which exxp(x>70) latches
#                         (INTSUTIL.pas:1145-1152) and which condemns the whole
#                         screen through errorflag. Diff against the port's
#                         GAPR0/GAPR/GAPRend, which backward.go emits under
#                         DPTRACE=1.
#
# In ALL modes stdout stays byte-identical to the frozen oracle; the trace goes
# to stderr only.
#
# The legacy sources are NEVER modified: a patched COPY of the instrumented unit
# (AMORTOP.pas for itr/cn, Amortize.pas for apr) is staged over the symlink,
# exactly the way build_linux.sh already does for VIDEODAT.pas.
#
# Usage:  bash scripts/build_trace_oracle.sh [-mode itr|cn|apr] [TARGET]
#         (TARGET defaults to amort_oracle)
set -euo pipefail

MODE=itr
TARGET=""
while [ $# -gt 0 ]; do
  case "$1" in
    -mode|--mode) MODE="${2:-}"; shift 2;;
    -mode=*|--mode=*) MODE="${1#*=}"; shift;;
    -h|--help) sed -n '2,56p' "$0"; exit 0;;
    *) TARGET="$1"; shift;;
  esac
done
TARGET="${TARGET:-amort_oracle}"
# PATCHUNIT is the legacy unit the instrumentation is injected into. itr/cn live
# in AMORTOP; the APR secant lives in Amortize.
case "$MODE" in
  itr) STAGE=/tmp/oracletracestage; OUT=/tmp/oracletrace; SUFFIX=_trace; PATCHUNIT=AMORTOP.pas;;
  cn)  STAGE=/tmp/oraclecnstage;    OUT=/tmp/oraclecn;    SUFFIX=_cn;    PATCHUNIT=AMORTOP.pas;;
  apr) STAGE=/tmp/oracleaprstage;   OUT=/tmp/oracleapr;   SUFFIX=_apr;   PATCHUNIT=Amortize.pas;;
  aprv) STAGE=/tmp/oracleaprvstage; OUT=/tmp/oracleaprv;  SUFFIX=_aprv;  PATCHUNIT=AMORTOP.pas;;
  ovf) STAGE=/tmp/oracleovfstage;  OUT=/tmp/oracleovf;   SUFFIX=_ovf;   PATCHUNIT=INTSUTIL.pas;;
  ra)  STAGE=/tmp/oracleraStage;    OUT=/tmp/oraclera;    SUFFIX=_ra;    PATCHUNIT=AMORTOP.pas;;
  *)   echo "ERROR: unknown -mode '$MODE' (want itr, cn, apr, aprv, ovf or ra)" >&2; exit 1;;
esac

REPO="$(cd "$(dirname "$0")/.." && pwd)"
FPCROOT=/tmp/fpcroot

PPCBIN="$(find "$FPCROOT" -name 'ppcx64' -o -name 'ppca64' 2>/dev/null | head -1)"
[ -n "$PPCBIN" ] || { echo "ERROR: FPC not staged under $FPCROOT (run legacy/oracle/build_linux.sh first)" >&2; exit 1; }
case "$(uname -m)" in aarch64) FPCARCH=aarch64-linux;; *) FPCARCH=x86_64-linux;; esac
UROOT="$(dirname "$PPCBIN")/units/$FPCARCH"

UNITOUT="$STAGE/_units"
rm -rf "$STAGE"; mkdir -p "$STAGE" "$OUT" "$UNITOUT"
lc(){ printf '%s' "$1" | tr 'A-Z' 'a-z'; }

shopt -s nullglob
for f in "$REPO"/legacy/src/dos_source/*.pas "$REPO"/legacy/src/dos_source/*.PAS; do
  ln -sf "$f" "$STAGE/$(lc "$(basename "$f")")"
done
for f in "$REPO"/legacy/oracle/*.pas; do
  ln -sf "$f" "$STAGE/$(lc "$(basename "$f")")"
done
shopt -u nullglob

# Same 64-bit pointer widening build_linux.sh applies.
rm -f "$STAGE/videodat.pas"
sed -E 's/:[[:space:]]*longint absolute (p|theresult|result|oldresult);/: ptrint absolute \1;/g' \
  "$REPO/legacy/src/dos_source/VIDEODAT.pas" > "$STAGE/videodat.pas"

# --- the instrumentation -------------------------------------------------
PATCHED="$STAGE/$(lc "$PATCHUNIT")"
rm -f "$PATCHED"
python3 - "$REPO/legacy/src/dos_source/$PATCHUNIT" "$PATCHED" "$MODE" <<'PY'
import sys, io
src, dst, mode = sys.argv[1], sys.argv[2], sys.argv[3]
lines = io.open(src, encoding='latin-1').read().split('\n')
out = []

if mode == 'itr':
    i, n, in_iterate = 0, len(lines), False
    while i < n:
        L = lines[i]
        if 'function Iterate (p, usap: real' in L:
            in_iterate = True
        if in_iterate:
            # after the pre-loop terminal evaluation, before `if (abs(p) < halfpenny)`
            if L.strip().startswith('if (abs(p) < halfpenny) then'):
                out.append("    writeln(stderr,'ITR0 seedx=',x:0:10,' p=',p:0:10);  flush(stderr);")
            # inside the repeat, right after `x := x + delta;` and `final := p;`
            if L.strip() == 'final := p;' and 'x := x + delta;' in lines[i-1]:
                out.append(L)
                out.append("      writeln(stderr,'ITR n=',count,' p=',p:0:10,' delta=',delta:0:10,' newx=',x:0:10);  flush(stderr);")
                i += 1
                continue
            if L.strip().startswith('x := bestx;'):
                out.append("    writeln(stderr,'ITRend bestp=',bestp:0:10,' bestx=',bestx:0:10,' count=',count);  flush(stderr);")
                in_iterate = False
        out.append(L)
        i += 1
    t = '\n'.join(out)
    if 'ITRend' not in t:
        sys.exit('ERROR: could not inject Iterate trace points')

elif mode == 'cn':
    in_cn = saw_pin = False
    n = 0
    for L in lines:
        s = L.strip()
        if 'procedure Paymenttype.ComputeNext' in L:
            in_cn = True
        if in_cn:
            # capture p/usap on entry to the arithmetic tail
            if s == 'prevdate := date;':
                out.append("    cn_pin := p; cn_uin := usap;")
                saw_pin = True
            if s == 'principal := p;' and saw_pin:
                out.append("    writeln(stderr,'CN d=',date.y,'-',date.m,'-',date.d,"
                           "' bpos=',balloonpos,' p=',cn_pin:0:6,' usap=',cn_uin:0:6,"
                           "' int=',interest:0:6,' pay=',payamt:0:6,' td=',timedif:0:8,"
                           "' pout=',p:0:6,' uout=',usap:0:6);  flush(stderr);")
                n += 1
                in_cn = False
        out.append(L)
    t = '\n'.join(out)
    # declare the two scratch globals next to the other unit-level reals
    # (AMORTOP.pas:73 — 4-space indent; a 2-space spelling silently no-ops)
    t = t.replace('    f, f_1, p, usap, d, int_to_date, prorate, cumint, cumamt, r78base: real;',
                  '    f, f_1, p, usap, d, int_to_date, prorate, cumint, cumamt, r78base: real;\n    cn_pin, cn_uin: real;', 1)
    if 'cn_pin, cn_uin: real;' not in t:
        sys.exit('ERROR: could not declare cn_pin/cn_uin')
    if n != 1:
        sys.exit('ERROR: injected %d CN trace points (expected 1)' % n)

elif mode == 'apr':
    # EstimateAndRefineAPRwithPoints, Amortize.pas:516-600. The two
    # `oldvalue := aprvalue;` statements are distinguished by indentation:
    # 4 spaces is the pre-loop seed evaluation, 6 spaces is inside the repeat.
    in_fn = False
    n0 = n1 = n2 = 0
    for L in lines:
        s = L.strip()
        if 'function EstimateAndRefineAPRwithPoints' in L:
            in_fn = True
        out.append(L)
        if not in_fn:
            continue
        if s == 'oldvalue := aprvalue;' and L.startswith('    o'):
            out.append("    writeln(stderr,'APR0 seed=',v_rate:0:10,' oldvalue=',oldvalue:0:6,"
                       "' target=',target:0:6,' ovf=',overflowflag);  flush(stderr);")
            n0 += 1
        elif s == 'oldvalue := aprvalue;' and L.startswith('      o'):
            out.append("      writeln(stderr,'APR n=',count,' vrate=',v_rate-delta:0:10,"
                       "' aprvalue=',aprvalue:0:6,' denom=',denom:0:6,' delta=',delta:0:10,"
                       "' newvrate=',v_rate:0:10,' ovf=',overflowflag);  flush(stderr);")
            n1 += 1
        elif s.startswith('until (count = 20)'):
            out.append("    writeln(stderr,'APRend count=',count,' delta=',delta:0:10,"
                       "' vrate=',v_rate:0:10,' ovf=',overflowflag);  flush(stderr);")
            n2 += 1
            in_fn = False
    t = '\n'.join(out)
    if (n0, n1, n2) != (1, 1, 1):
        sys.exit('ERROR: injected APR trace points %s (expected (1,1,1))' % ((n0, n1, n2),))

elif mode == 'aprv':
    # RepayFancyLoan's value_calc accumulator (AMORTOP.pas:1194-1195, :1217-1218,
    # :1224-1225) — one AV line per discounted term, so the port's cashflow
    # stream can be diffed row-for-row against DOS's. Each of the three sites is
    # the single statement of an `if (value_calc) ... then`, so the writeln has
    # to go INSIDE a begin/end or it would fire unconditionally.
    n = 0
    for L in lines:
        s = L.strip()
        if s.startswith('aprvalue := aprvalue + NextPayment.'):
            ind = L[:len(L) - len(L.lstrip())]
            amt = 'NextPayment.payamt' if '.payamt' in s else 'NextPayment.principal'
            tag = 'pay' if '.payamt' in s else 'bal'
            out.append(ind + 'begin')
            out.append(ind + '  ' + s.rstrip('\r'))
            out.append(ind + "  writeln(stderr,'AV " + tag + " d=',NextPayment.date.y,'-',"
                       "NextPayment.date.m,'-',NextPayment.date.d,' amt='," + amt + ":0:6,"
                       "' yd=',YearsDif(NextPayment.date, loandate):0:8,"
                       "' acc=',aprvalue:0:6);  flush(stderr);")
            out.append(ind + 'end;')
            n += 1
            continue
        out.append(L)
    t = '\n'.join(out)
    if n != 3:
        sys.exit('ERROR: injected %d AV trace points (expected 3)' % n)

elif mode == 'ovf':
    # The three INTSUTIL primitives that LATCH the global overflowflag/errorflag
    # pair and thereby condemn the whole screen:
    #
    #   exxp   (x > 70)   INTSUTIL.pas:1145-1152   DO_ExxpOverflow
    #   lnn    (x <= 0)   INTSUTIL.pas:1164-1171   DO_LnnNegative
    #   sqrrt  (x < -teeny) INTSUTIL.pas:1128-1135 DO_SqrrtTiny
    #
    # A refusal that the port does not reproduce is almost always one of these
    # firing somewhere the port either never evaluates or evaluates with a
    # locally-swallowed error. Printing the offending argument plus a call
    # ordinal tells you WHICH evaluation blew up, which is the only thing the
    # stdout `ERR ...` line does not carry.
    n = 0
    for L in lines:
        s = L.strip()
        out.append(L)
        if s.startswith("MessageBox('Overflow error: answer too large"):
            out.append("            writeln(stderr,'OVF exxp x=',x:0:10);  flush(stderr);")
            n += 1
        elif s.startswith("MessageBox('Error: The data you have specified contain an inconsistency.', DO_LnnNegative"):
            out.append("            writeln(stderr,'OVF lnn x=',x:0:10);  flush(stderr);")
            n += 1
        elif s.startswith("MessageBox('Error: The data you have specified contain an inconsistency.', DO_SqrrtTiny"):
            out.append("              writeln(stderr,'OVF sqrrt x=',x:0:10);  flush(stderr);")
            n += 1
    t = '\n'.join(out)
    if n != 3:
        sys.exit('ERROR: injected %d OVF trace points (expected 3)' % n)

elif mode == 'ra':
    # The RE-AMORTIZE TRIGGER. DOS decides to apply adj[next_adj] in exactly two
    # places, and BOTH test the LOOKAHEAD row's date, not the committed row's:
    #
    #   AMORTOP.pas:1075  DecideWhetherToPrintALine  (the Output<>nil path)
    #       if (next_adj <= nadj) and (DateComp(nextt, adj[next_adj]^.date) > 0)
    #   AMORTOP.pas:1215  RepayFancyLoan             (the Output=nil path)
    #       else if ((next_adj<=adjnum) or entire) and (next_adj<=nadj)
    #            and (DateComp(nextpayment.date,adj[next_adj]^.date)>0)
    #
    # `nextt` is whatever ComputeNext most recently produced, which with a
    # prepayment stream running is an EXTRA row, not the next regular payment.
    # DW lines print both the committed row and the lookahead row so the exact
    # row the rate/payment switch lands on can be read off directly; RA lines
    # mark each actual Re_Amortize entry.
    t = '\n'.join(lines)
    dwanchor = ("    if (next_adj <= nadj) and (DateComp(nextt, adj[next_adj]^.date) > 0) then\n"
                "      Re_Amortize(p);\n")
    dwtrace = (
        "    if (next_adj <= nadj) then\n"
        "      writeln(stderr,'DW t=',t.y,'-',t.m,'-',t.d,' nextt=',nextt.y,'-',nextt.m,'-',nextt.d,\n"
        "        ' paynum=',payment.paynum,' pdate=',payment.date.y,'-',payment.date.m,'-',payment.date.d,\n"
        "        ' next_adj=',next_adj,' nadj=',nadj,' adjd=',adj[next_adj]^.date.y,'-',\n"
        "        adj[next_adj]^.date.m,'-',adj[next_adj]^.date.d,\n"
        "        ' cmp=',DateComp(nextt, adj[next_adj]^.date))\n"
        "    else\n"
        "      writeln(stderr,'DW t=',t.y,'-',t.m,'-',t.d,' nextt=',nextt.y,'-',nextt.m,'-',nextt.d,\n"
        "        ' paynum=',payment.paynum,' next_adj=',next_adj,' nadj=',nadj,' (spent)');\n"
        "    flush(stderr);\n")
    if t.count(dwanchor) != 1:
        sys.exit('ERROR: could not find the DecideWhetherToPrintALine trigger')
    t = t.replace(dwanchor, dwtrace + dwanchor, 1)

    raanchor = ("  begin\n"
                "    p := Payment.principal;\n"
                "    usap := Payment.usaprinc;\n")
    ratrace = ("    writeln(stderr,'RA enter next_adj=',next_adj,\n"
               "      ' pdate=',payment.date.y,'-',payment.date.m,'-',payment.date.d,\n"
               "      ' ndate=',nextpayment.date.y,'-',nextpayment.date.m,'-',nextpayment.date.d,\n"
               "      ' p=',p:0:6,' rate=',h^.loanrate:0:10,' d=',d:0:6,\n"
               "      ' npre=',npre,' oldnpre=',old_npre);  flush(stderr);\n")
    if t.count(raanchor) != 1:
        sys.exit('ERROR: could not find the Re_Amortize entry')
    t = t.replace(raanchor, raanchor + ratrace, 1)

    # one line per prepayment cursor as Re_Amortize restores it (AMORTOP.pas:1596)
    pranchor = ("    next_balloon := old_next_balloon;\n"
                "    for i:=1 to old_npre do begin\n"
                "      if( old_pre[i] <> nil ) then\n"
                "        pre[i]^ := old_pre[i]^;\n"
                "    end;\n")
    prtrace = ("    for i:=1 to old_npre do\n"
               "      if( old_pre[i] <> nil ) then\n"
               "        writeln(stderr,'RA pre[',i,'] start=',pre[i]^.startdate.y,'-',\n"
               "          pre[i]^.startdate.m,'-',pre[i]^.startdate.d,' next=',pre[i]^.nextdate.y,'-',\n"
               "          pre[i]^.nextdate.m,'-',pre[i]^.nextdate.d,' stop=',pre[i]^.stopdate.y,'-',\n"
               "          pre[i]^.stopdate.m,'-',pre[i]^.stopdate.d,' peryr=',pre[i]^.peryr,\n"
               "          ' amt=',pre[i]^.payment:0:2);\n"
               "    flush(stderr);\n")
    if t.count(pranchor) == 1:
        t = t.replace(pranchor, pranchor + prtrace, 1)

    # The AMOUNT branch's grid snap (AMORTOP.pas:1573-1575). `t := NextPayment.date`
    # is the LOOKAHEAD row, which NumberOfInstallments then rounds forward onto the
    # h^.firstdate grid via its VAR parameter. Print t on both sides so the port's
    # SEGTERM firstPay/subFirst pair can be diffed against what DOS actually solved.
    amtanchor = ("            t := NextPayment.date;\n"
                 "            n := NumberOfInstallments(h^.firstdate, t, h^.peryr, on_or_after);\n"
                 "            if Iterate(p, usap, Payment.date, t, d, til_adj) then\n")
    amttrace = ("            t := NextPayment.date;\n"
                "            writeln(stderr,'RA amt pre  t=',t.y,'-',t.m,'-',t.d,\n"
                "              ' fd=',h^.firstdate.y,'-',h^.firstdate.m,'-',h^.firstdate.d,\n"
                "              ' ld=',Payment.date.y,'-',Payment.date.m,'-',Payment.date.d,\n"
                "              ' seed=',d:0:6,' p=',p:0:6,' usap=',usap:0:6);  flush(stderr);\n"
                "            n := NumberOfInstallments(h^.firstdate, t, h^.peryr, on_or_after);\n"
                "            writeln(stderr,'RA amt post t=',t.y,'-',t.m,'-',t.d,' n=',n);  flush(stderr);\n"
                "            if Iterate(p, usap, Payment.date, t, d, til_adj) then\n")
    if t.count(amtanchor) == 1:
        t = t.replace(amtanchor, amttrace, 1)
    else:
        sys.stderr.write('WARN: RA amount-branch snap anchor not found\n')
    amtdone = ("                adj[next_adj]^.amount := d;\n"
               "                adj[next_adj]^.amountstatus := outp;\n"
               "                adj[next_adj]^.amtok := true;\n")
    if t.count(amtdone) == 1:
        t = t.replace(amtdone, amtdone +
            "                writeln(stderr,'RA amt solved d=',d:0:6);  flush(stderr);\n", 1)

io.open(dst, 'w', encoding='latin-1').write(t)
PY
echo "injected $(grep -c "writeln(stderr,'" "$PATCHED") $MODE trace point(s) into $(basename "$PATCHED")"

"$PPCBIN" -Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX \
  -Fu"$UROOT/rtl" -Fu"$UROOT/rtl-objpas" -Fu"$UROOT/fcl-base" -Fu"$UROOT/rtl-extra" \
  -Fu"$STAGE" -FU"$UNITOUT" -o"$OUT/${TARGET}${SUFFIX}" "$STAGE/$TARGET.pas" \
  > "$OUT/build.log" 2>&1 || { echo "BUILD FAILED:"; tail -25 "$OUT/build.log"; exit 1; }

echo "built: $OUT/${TARGET}${SUFFIX}"
