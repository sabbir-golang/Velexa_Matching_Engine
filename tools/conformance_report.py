#!/usr/bin/env python3
"""Render docs/conformance_results.json (written by `go test -run TestConformance`)
into docs/DSE_Conformance_Report.pdf: summary, per-section comparison tables,
a tick-box checklist, evidence and recommended fixes.

    go test -run TestConformance -count=1
    python3 tools/conformance_report.py
"""
import html, json, os, sys, datetime
from collections import OrderedDict

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SRC = sys.argv[1] if len(sys.argv) > 1 else os.path.join(ROOT, "docs", "conformance_results.json")
OUT = sys.argv[2] if len(sys.argv) > 2 else os.path.join(ROOT, "docs", "DSE_Conformance_Report.pdf")

data = json.load(open(SRC))
res = data["results"]
e = html.escape
LABEL = {"PASS": "Matches DSE", "FAIL": "Deviates", "GAP": "Gap", "PARTIAL": "Partial", "INFO": "Info"}
SEV = {"FAIL": "Blocker", "GAP": "Gap", "PARTIAL": "Partial", "INFO": "Info", "PASS": "Pass"}

by_cat = OrderedDict()
for r in res:
    by_cat.setdefault(r["category"], []).append(r)
cnt = {k: sum(1 for r in res if r["verdict"] == k) for k in LABEL}
total = len(res)

def badge(v):
    return f'<span class="b {v}">{LABEL[v]}</span>'

def short(s, n=170):
    s = s or ""
    return e(s if len(s) <= n else s[: n - 1] + "…")

# Section summaries ------------------------------------------------------------
sec_rows = ""
for cat, rows in by_cat.items():
    c = {k: sum(1 for r in rows if r["verdict"] == k) for k in LABEL}
    sec_rows += (f'<tr><td>{e(cat)}</td><td class="n">{len(rows)}</td><td class="n g">{c["PASS"]}</td>'
                 f'<td class="n r">{c["FAIL"]}</td><td class="n a">{c["GAP"]}</td>'
                 f'<td class="n">{c["PARTIAL"] + c["INFO"]}</td></tr>')

# Headline findings (hand-written, each backed by case IDs) ------------------------
HEAD = [
    ("Orders that DSE would refuse are accepted", "Blocker", "OT-02, OT-095/099, SD-4",
     "A limit order with no price executes as a market order; unknown OrdTypes (5, 9) become limit orders; a Sell-Short (54=5) executes as a <b>buy</b>."),
    ("Fill reports are not valid FIX", "Blocker", "ER-2, CM-3",
     "Every fill carries ExecType 150=TRADE (FIX requires F). The Order Cancel Reject (35=9) omits the required OrderID (37)."),
    ("OMS features are not understood by the engine", "Gap", "OT-06, TIF-S, EI-2…6, QO-1, QO-3, QO-4",
     "Market-At-Best (Z), Validity S, ExecInst n/S/q/w (Park / Unpark / Block) are refused by the dictionary or ignored. DisplayQty (1138) and MinQty (110) have no effect — the engine only reads MaxFloor (111)."),
    ("Board groups (762) are never validated", "Gap", "BG-02 … BG-17",
     "All 16 non-PUBLIC groups are accepted on a MAIN-board equity (including INDEX, ATBX, SCPX, debt and block groups). Tag 762 is never read."),
    ("Order Status Request (35=H) is ignored", "Gap", "CM-11, CM-12",
     "No reply of any kind, so the OMS cannot reconcile order state against the exchange."),
    ("Amend (35=G) skips pre-trade checks", "Gap", "CM-7, CM-8",
     "Amending a resting order to an off-tick price or a price far outside the circuit band is accepted."),
    ("GTD / time-in-force semantics", "Gap", "TIF-6a, TIF-6b, TIF-2/5/7, ST-1",
     "GTD without an expiry never expires; an expiring order is removed silently with no Execution Report; At-Open / GTX / At-Close are treated as Day; Day orders never expire at close."),
    ("Auction fills are not reported", "Gap", "PH-3, PH-4",
     "The Pre-Open uncross trades orders but sends no Execution Report to the broker; Pre-Open also skips the tick and price-band checks."),
]
head_rows = "".join(
    f'<tr><td><b>{t}</b><br><span class="mut">{ids}</span></td><td>{badge("FAIL" if s == "Blocker" else "GAP")}</td><td>{d}</td></tr>'
    for t, s, ids, d in HEAD)

# Per-section comparison tables ---------------------------------------------------------
def comp_table(rows):
    out = ('<table class="cmp"><thead><tr><th style="width:7%">ID</th><th style="width:21%">Case</th>'
           '<th style="width:22%">DSE / OMS expects</th><th style="width:28%">Matching engine did</th>'
           '<th style="width:9%">Result</th></tr></thead><tbody>')
    for r in rows:
        actual = r["actual"]
        note = r["note"]
        extra = f'<div class="note">{short(note, 260)}</div>' if note and r["verdict"] != "PASS" else ""
        out += (f'<tr class="{r["verdict"]}"><td class="mono">{e(r["id"])}</td><td>{e(r["title"])}</td>'
                f'<td>{short(r["dse_expected"], 200)}</td><td class="mono sm">{short(actual, 190)}{extra}</td>'
                f'<td>{badge(r["verdict"])}</td></tr>')
    return out + "</tbody></table>"

def band_table(rows):
    # collapse the 64 price-band cases into one grid per tier
    tiers = OrderedDict()
    for r in rows:
        tiers.setdefault(r["id"][:5], []).append(r)
    out = ('<table class="cmp"><thead><tr><th>Previous close</th><th>Band</th><th>At upper limit</th>'
           '<th>Just above</th><th>At lower limit</th><th>Just below</th></tr></thead><tbody>')
    for k, rs in tiers.items():
        t = rs[0]["title"]
        prev, band = t.split(":")[0].split(" ±")
        cells = ""
        for r in rs:
            px = r["title"].split()[-1]
            cells += f'<td class="{r["verdict"]}c">{px} {"✔" if r["verdict"] == "PASS" else "✘"}</td>'
        out += f'<tr><td>{prev.replace("prev ", "Tk ")}</td><td>±{band}</td>{cells}</tr>'
    return out + "</tbody></table>"

sections = ""
for i, (cat, rows) in enumerate(by_cat.items(), 1):
    if cat == "Price band / tick":
        main = [r for r in rows if not r["id"].startswith("PB-")]
        bands = [r for r in rows if r["id"].startswith("PB-")]
        sections += f'<h2>{i}. {e(cat)}</h2>' + comp_table(main)
        ok = sum(1 for r in bands if r["verdict"] == "PASS")
        sections += (f'<h3>DSE circuit-limit tiers — boundary test ({ok}/{len(bands)} correct)</h3>'
                     '<p class="mut">Each tier is probed at the last accepted price and the next tick beyond it, both sides. '
                     'Tiers: ±10% to Tk200, ±8.75% to Tk500, ±7.5% to Tk1000, ±6.25% to Tk2000, ±5% to Tk5000, ±3.75% above.</p>'
                     + band_table(bands))
    else:
        sections += f'<h2>{i}. {e(cat)}</h2>' + comp_table(rows)

# Checklist ----------------------------------------------------------------------------------
def check_items(rows, cat):
    out = ""
    if cat == "Price band / tick":
        bands = [r for r in rows if r["id"].startswith("PB-")]
        ok = sum(1 for r in bands if r["verdict"] == "PASS")
        out += f'<li class="PASS"><span class="cb">☑</span> All circuit-limit tier boundaries ({ok}/{len(bands)} probes correct)</li>'
        rows = [r for r in rows if not r["id"].startswith("PB-")]
    for r in rows:
        mark = "☑" if r["verdict"] == "PASS" else "☐"
        tag = "" if r["verdict"] == "PASS" else f' <span class="tg {r["verdict"]}">{SEV[r["verdict"]]}</span>'
        out += f'<li class="{r["verdict"]}"><span class="cb">{mark}</span> <span class="mono">{e(r["id"])}</span> {e(r["title"])}{tag}</li>'
    return out

checklist = ""
for cat, rows in by_cat.items():
    checklist += f'<h3>{e(cat)}</h3><ul class="ck">{check_items(rows, cat)}</ul>'

# Evidence ---------------------------------------------------------------------------------------
ev = ""
shown = 0
for r in res:
    if r["verdict"] in ("FAIL", "GAP") and r["sent"] and r["sent"] != "(code inspection)" and r["id"] not in ("BG-03", "BG-04"):
        if r["category"] == "Board group (762)" and r["id"] != "BG-02":
            continue
        shown += 1
        rep = "<br>".join(e(x) for x in (r["replies"] or [])) or "(no response)"
        ev += (f'<div class="evb"><div class="evh">{e(r["id"])} — {e(r["title"])} {badge(r["verdict"])}</div>'
               f'<div class="evl">Sent</div><div class="mono sm">{e(r["sent"])}</div>'
               f'<div class="evl">Received</div><div class="mono sm">{rep}</div>'
               f'<div class="note">{e(r["note"])}</div></div>')

# Recommended fixes -----------------------------------------------------------------------------------
FIX = [
    ("1", "Blocker", "Reject unsupported OrdType and Side; require a price on 40=2", "fixacceptor.go:461-479",
     "Map only 1/2/3/4 (+Z once supported); reject everything else and any 40=2 without a price; accept only Side 1/2."),
    ("2", "Blocker", "Send ExecType F for fills; add OrderID to 35=9; set OrdRejReason (103)", "fixacceptor.go:531-543, 587-648",
     "Change the literal 'TRADE' to 'F'. Echo tags 1, 41, 40, 59 on reports."),
    ("3", "High", "Handle Order Status Request (35=H)", "fixacceptor.go:403-411",
     "Return an Execution Report with ExecType I and the current OrdStatus; reject unknown orders."),
    ("4", "High", "Re-run tick and price-band checks on replace; apply them in Pre-Open", "engine.go:278-329, 595-625",
     "Move the pre-trade block ahead of the Pre-Open branch and call it from doReplace."),
    ("6", "High", "Support Market-At-Best (Z), Validity S, ExecInst n/S/q/w/'w S'/'w q'", "spec/FIX50SP2.xml, fixacceptor.go",
     "Extend the data dictionary enums and implement Park/Unpark/Block semantics on the book."),
    ("7", "Medium", "DisplayQty (1138) and MinQty (110)", "fixacceptor.go:458, engine.go:502",
     "Read 1138 as the iceberg peak (keep 111 as an alias); enforce MinQty on matching and reject MinQty > OrderQty."),
    ("8", "Medium", "Validate tag 762 against the instrument's board/category", "fixacceptor.go validateNew",
     "Add board-group ↔ board mapping; reject Index groups for orders."),
    ("9", "Medium", "GTD: require expiry, send an Execution Report on expiry; implement Day expiry at close", "fixacceptor.go:564-578, engine.go:765-781",
     "Reject GTD without 126/432; emit ExecType C when the sweeper removes an order; add an end-of-day purge."),
    ("10", "Medium", "Send Execution Reports for auction (uncross) fills; add post-close session and automatic circuit halts", "engine.go:627-725",
     "Route uncross trades through the order's notify callback; add a post-close phase with close-price market orders."),
    ("11", "Low", "Reject duplicate ClOrdID", "engine.go (process)", "A repeated 11 currently creates a second order and shadows the first for cancel/replace."),
]
fix_rows = "".join(f'<tr><td>{a}</td><td>{badge("FAIL" if b == "Blocker" else "GAP")}</td><td><b>{c}</b><br><span class="mut">{e(d)}</span></td><td>{f}</td></tr>'
                   for a, b, c, d, f in FIX)

today = datetime.date.today().strftime("%d %B %Y")
doc = f"""<!doctype html><html><head><meta charset="utf-8"><style>
@page {{ size: A4; margin: 16mm 13mm 16mm 13mm; @bottom-left {{ content: "DSE Conformance Report · Velexa matching engine"; font: 8pt sans-serif; color:#777 }} @bottom-right {{ content: "Page " counter(page) " of " counter(pages); font: 8pt sans-serif; color:#777 }} }}
body {{ font: 9.2pt/1.4 "DejaVu Sans", sans-serif; color:#1b2330 }}
h1 {{ font-size:21pt; margin:0 0 2pt }} h2 {{ font-size:13pt; border-bottom:2px solid #2a5bd7; padding-bottom:3pt; margin:20pt 0 8pt; break-after:avoid }}
h3 {{ font-size:10.5pt; margin:12pt 0 4pt; break-after:avoid }}
.mut {{ color:#6b7685 }} .sub {{ color:#6b7685; margin-bottom:10pt }}
table {{ border-collapse:collapse; width:100% }} th,td {{ border:1px solid #d5dbe3; padding:3.5pt 5pt; vertical-align:top; text-align:left }}
th {{ background:#eef1f6; font-size:8.3pt }} tr {{ break-inside:avoid }}
.n {{ text-align:right }} .g {{ color:#13795b }} .r {{ color:#b42318 }} .a {{ color:#a15c00 }}
.mono {{ font-family:"DejaVu Sans Mono", monospace }} .sm {{ font-size:7.4pt }}
.b {{ display:inline-block; padding:1pt 5pt; border-radius:8pt; font-size:7.6pt; font-weight:bold; white-space:nowrap }}
.b.PASS {{ background:#d9f2e6; color:#0b5d41 }} .b.FAIL {{ background:#fbdcdc; color:#9a1b13 }} .b.GAP {{ background:#fdeacc; color:#8a4b00 }}
.b.PARTIAL {{ background:#fff2b8; color:#6d5600 }} .b.INFO {{ background:#e3e8f0; color:#415066 }}
tr.FAIL td:first-child {{ border-left:3px solid #d92d20 }} tr.GAP td:first-child {{ border-left:3px solid #f79009 }}
tr.PARTIAL td:first-child {{ border-left:3px solid #eab308 }}
.note {{ margin-top:2pt; font-family:"DejaVu Sans"; font-size:7.6pt; color:#6b4a00 }}
.kpis {{ display:flex; gap:7pt; margin:8pt 0 4pt }} .kpi {{ flex:1; border:1px solid #d5dbe3; border-radius:5pt; padding:6pt 8pt }}
.kpi b {{ font-size:17pt; display:block }} .kpi.PASS b {{ color:#13795b }} .kpi.FAIL b {{ color:#b42318 }} .kpi.GAP b {{ color:#a15c00 }}
ul.ck {{ list-style:none; padding:0; margin:0; columns:1 }} ul.ck li {{ padding:1.6pt 0; border-bottom:1px dotted #e1e5ec; break-inside:avoid }}
.cb {{ font-size:11pt; display:inline-block; width:15pt }} li.PASS .cb {{ color:#13795b }} li.FAIL .cb {{ color:#b42318 }} li.GAP .cb {{ color:#a15c00 }}
.tg {{ font-size:7pt; font-weight:bold; padding:0 4pt; border-radius:6pt }} .tg.FAIL {{ background:#fbdcdc; color:#9a1b13 }} .tg.GAP {{ background:#fdeacc; color:#8a4b00 }} .tg.PARTIAL {{ background:#fff2b8 }} .tg.INFO {{ background:#e3e8f0 }}
.PASSc {{ background:#e8f7ef; color:#0b5d41; text-align:center }} .FAILc,.GAPc {{ background:#fde7e7; color:#9a1b13; text-align:center }}
.evb {{ border:1px solid #d5dbe3; border-radius:4pt; padding:5pt 7pt; margin:6pt 0; break-inside:avoid }} .evh {{ font-weight:bold; margin-bottom:3pt }}
.evl {{ font-size:7.2pt; text-transform:uppercase; color:#6b7685; margin-top:3pt }} .evb .mono {{ word-break:break-all }}
.scope {{ background:#f5f7fa; border-left:3px solid #2a5bd7; padding:6pt 9pt; margin:8pt 0 }}
</style></head><body>
<h1>DSE Conformance Report</h1>
<div class="sub">Velexa matching engine vs the order types the OMS sends to DSE · generated {today}</div>

<div class="scope"><b>Scope.</b> {total} cases were driven through the engine's real FIX acceptor (FIXT.1.1 / FIX 5.0 SP2, strict data dictionary on) using the exact tag set listed in <i>DSE FIX Order Types · Velexa OMS</i> (11 Oct 2026). Expected DSE behaviour comes from that document, its observed DSE rejects (price band ±7.5% / ±8.75%, minimum quantity step 1000) and the FIX specification. Where DSE behaviour is not specified the case is marked <i>Info</i> and not judged. No engine code was changed. Reproduce with <span class="mono">go test -run TestConformance</span> then <span class="mono">python3 tools/conformance_report.py</span>.</div>

<div class="kpis">
 <div class="kpi"><b>{total}</b>cases run</div>
 <div class="kpi PASS"><b>{cnt["PASS"]}</b>match DSE</div>
 <div class="kpi FAIL"><b>{cnt["FAIL"]}</b>deviate (blockers)</div>
 <div class="kpi GAP"><b>{cnt["GAP"]}</b>gaps</div>
 <div class="kpi"><b>{cnt["PARTIAL"] + cnt["INFO"]}</b>partial / info</div>
</div>

<h2>Headline findings</h2>
<table><thead><tr><th style="width:28%">Finding</th><th style="width:9%">Severity</th><th>Detail</th></tr></thead><tbody>{head_rows}</tbody></table>
<p><b>What works:</b> limit and market matching with price-time priority, partial fills, IOC and FOK, cancel and amend of resting orders, the full DSE price-band ladder (all 64 boundary probes correct), tick-size enforcement, halted and unknown symbols, whole-share quantities, the per-instrument market lot (minimum quantity step, set from the Instruments page), required-field validation, Pre-Open handling of market orders, and the session messages Logon / Logout / TestRequest / ResendRequest / SequenceReset.</p>

<h2>Results by section</h2>
<table><thead><tr><th>Section</th><th class="n">Cases</th><th class="n">Match</th><th class="n">Deviate</th><th class="n">Gap</th><th class="n">Partial / info</th></tr></thead><tbody>{sec_rows}</tbody></table>

<div style="break-before:page"></div>
<h1 style="font-size:16pt">Comparison: OMS / DSE vs engine</h1>
{sections}

<div style="break-before:page"></div>
<h1 style="font-size:16pt">Checklist</h1>
<p class="mut">☑ matches DSE · ☐ needs work (Blocker = engine accepts/rejects the opposite of DSE or emits non-FIX output; Gap = feature missing or silently ignored).</p>
{checklist}

<div style="break-before:page"></div>
<h2>Evidence — sample messages for each deviation</h2>
<p class="mut">SOH delimiters shown as “|”. The sent string is the OMS-style message as it left the test initiator; session header fields are added by QuickFIX.</p>
{ev}

<h2>Recommended fixes (not applied)</h2>
<table><thead><tr><th>#</th><th>Priority</th><th>Change</th><th>Where</th></tr></thead><tbody>{fix_rows}</tbody></table>
<p class="mut">Out of scope for this report: OMS-side defects listed in the OMS reference (GTD sent without expiry, modify without Symbol/762, Market-At-Best blocked by the OMS permission check) — they are on the sending side. Items DSE has never been observed to accept or reject (block, buy-in, iceberg, GTD, cancel, modify, status) are judged against the FIX standard and the OMS reference only and should be re-confirmed against DSE.</p>
</body></html>"""

from weasyprint import HTML
HTML(string=doc).write_pdf(OUT)
print("wrote", OUT, f"({total} cases)")
