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

# Findings -----------------------------------------------------------------------------
# Each finding names the cases that prove it. It is listed as open while any of
# them is not PASS (or INFO), and moves to "Resolved" once they all pass — so
# the report never claims a problem the latest run no longer shows.
VERDICT = {r["id"]: r["verdict"] for r in res}


def ids(spec):
    """'BG-02..BG-17' -> BG-02, BG-03, ...; plain IDs pass through."""
    out = []
    for part in spec.split(","):
        part = part.strip()
        if ".." in part:
            a, b = part.split("..")
            pre = a.rstrip("0123456789")
            width = len(a) - len(pre)
            out += [f"{pre}{i:0{width}d}" for i in range(int(a[len(pre):]), int(b[len(pre):]) + 1)]
        elif part:
            out.append(part)
    return out


def state(spec):
    """Worst current verdict of the cases ("PASS" when all pass or are info)."""
    vs = [VERDICT.get(i, "MISSING") for i in ids(spec)]
    for v in ("FAIL", "MISSING", "GAP", "PARTIAL"):
        if v in vs:
            return "FAIL" if v == "MISSING" else v
    return "PASS"


FINDINGS = [  # title, cases, the problem (while open), what now happens (once resolved)
    ("Orders that DSE would refuse are accepted", "OT-02, OT-095, OT-099, SD-4",
     "A limit order with no price executes as a market order; unknown OrdTypes (5, 9) become limit orders; a Sell-Short (54=5) executes as a buy.",
     "Limit orders need a price; unsupported OrdTypes and Sides are rejected with a reason."),
    ("Fill reports and cancel rejects are valid FIX", "ER-2, CM-3",
     "Fills carry ExecType 150=TRADE (FIX requires F); the Order Cancel Reject (35=9) omits OrderID (37).",
     "Fills carry 150=F; 35=9 carries OrderID (the real one when the order exists)."),
    ("Order Status Request (35=H)", "CM-11, CM-12",
     "No reply of any kind, so the OMS cannot reconcile order state.",
     "Answered with 150=I and the current status; unknown orders get 39=8."),
    ("Amend (35=G) pre-trade checks", "CM-7, CM-8",
     "Amending a resting order to an off-tick price or a price outside the circuit band is accepted.",
     "Amends are checked like new orders (tick, band, lot, session, ClOrdID) and refused with 35=9 and the reason."),
    ("Duplicate ClOrdID", "QT-6",
     "A repeated ClOrdID creates a second order that shadows the first for cancel/replace.",
     "Rejected while the first order is working; ClOrdIDs are tracked per broker."),
    ("Pre-Open auction", "PH-3, PH-4",
     "Pre-Open skips the tick and band checks, and the uncross sends no Execution Report.",
     "Price checks apply in Pre-Open; auction fills are reported to both brokers."),
    ("GTD and Day expiry; post-close session", "TIF-6a, TIF-6b, ST-1, ST-2",
     "GTD without an expiry never expires; expiry sends no report; Day orders never expire; no post-close session.",
     "GTD needs a future expiry; expiry sends 150=C; Day orders expire at the end of the day; post-close trades at the close price only."),
    ("Market-At-Best, Validity S, DisplayQty", "OT-06, TIF-S, QO-1",
     "Market-At-Best (Z) and Validity S are refused or ignored; DisplayQty (1138) has no effect.",
     "Z trades at the best opposite price; S is treated as Day; DisplayQty sets the iceberg peak."),
    ("Validity At-Open / Good-till-Crossing / At-Close", "TIF-2, TIF-5, TIF-7",
     "59=2, 5 and 7 are accepted but treated as Day.", ""),
    ("ExecInst (18): Park / Unpark / Block", "EI-1..EI-6",
     "Tag 18 is not interpreted, and the DSE combinations 'w S' and 'w q' are not in the data dictionary.", ""),
    ("MinQty (110)", "QO-3, QO-4",
     "MinQty is ignored: an order can fill below it, and MinQty > OrderQty is accepted.", ""),
    ("Board groups (762)", "BG-02..BG-17",
     "The 16 non-PUBLIC groups are accepted on a MAIN-board equity (including INDEX, ATBX, SCPX, debt and block groups); tag 762 is not validated.", ""),
    ("Automatic circuit breaker; category and board rules", "ST-3, ST-4",
     "No automatic volatility halt; no category- or board-specific lot, band or session rules.", ""),
]
open_rows, resolved_rows = "", ""
for title, spec, problem, fixed in FINDINGS:
    v = state(spec)
    cases = e(spec.replace("..", " … "))
    if v == "PASS":
        resolved_rows += f'<tr><td><b>{e(title)}</b><br><span class="mut">{cases}</span></td><td>{badge("PASS")}</td><td>{e(fixed)}</td></tr>'
    else:
        open_rows += f'<tr><td><b>{e(title)}</b><br><span class="mut">{cases}</span></td><td>{badge(v)}</td><td>{e(problem)}</td></tr>'
if not open_rows:
    open_rows = '<tr><td colspan="3">No open findings.</td></tr>'

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

# Recommended work: only the items whose cases are still open -------------------------
WORK = [  # priority, change, where, how, cases
    ("High", "Validity At-Open / GTX / At-Close (59=2/5/7)", "fixacceptor.go mapTIF, engine.go",
     "Rest 59=2 for the opening auction only, 59=7 for the close; 59=5 expires at the next uncross.", "TIF-2, TIF-5, TIF-7"),
    ("High", "ExecInst Park / Unpark / Block (18)", "spec/FIX50SP2.xml, fixacceptor.go, engine.go",
     "Add 'w S' and 'w q' to the dictionary; hold parked orders off the book until unparked.", "EI-1..EI-6"),
    ("Medium", "MinQty (110)", "fixacceptor.go, engine.go match",
     "Reject MinQty > OrderQty; only trade when at least MinQty can fill at once.", "QO-3, QO-4"),
    ("Medium", "Validate tag 762 against the instrument's board", "fixacceptor.go validateNew",
     "Map board groups to boards; reject index groups for orders.", "BG-02..BG-17"),
    ("Low", "Automatic circuit breaker; category and board rules", "engine.go, instruments",
     "Halt on band breaches per DSE rules; per-category lots, bands and sessions.", "ST-3, ST-4"),
]
fix_rows = ""
n = 0
for prio, change, where, how, spec in WORK:
    if state(spec) == "PASS":
        continue
    n += 1
    fix_rows += (f'<tr><td>{n}</td><td>{badge("GAP")} <span class="mut">{prio}</span></td>'
                 f'<td><b>{e(change)}</b><br><span class="mut">{e(how)} — {e(spec.replace("..", " … "))}</span></td><td>{e(where)}</td></tr>')
if not fix_rows:
    fix_rows = '<tr><td colspan="4">Nothing open.</td></tr>'

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

<div class="scope"><b>Scope.</b> {total} cases were driven through the engine's real FIX acceptor (FIXT.1.1 / FIX 5.0 SP2, strict data dictionary on) using the exact tag set listed in <i>DSE FIX Order Types · Velexa OMS</i> (11 Oct 2026). Expected DSE behaviour comes from that document, its observed DSE rejects (price band ±7.5% / ±8.75%, minimum quantity step 1000) and the FIX specification. Where DSE behaviour is not specified the case is marked <i>Info</i> and not judged. Findings are classified from this run's results, so a fixed problem appears under <i>Resolved</i>. Reproduce with <span class="mono">go test -run TestConformance</span> then <span class="mono">python3 tools/conformance_report.py</span>.</div>

<div class="kpis">
 <div class="kpi"><b>{total}</b>cases run</div>
 <div class="kpi PASS"><b>{cnt["PASS"]}</b>match DSE</div>
 <div class="kpi FAIL"><b>{cnt["FAIL"]}</b>deviate (blockers)</div>
 <div class="kpi GAP"><b>{cnt["GAP"]}</b>gaps</div>
 <div class="kpi"><b>{cnt["PARTIAL"] + cnt["INFO"]}</b>partial / info</div>
</div>

<h2>Open findings</h2>
<table><thead><tr><th style="width:28%">Finding</th><th style="width:9%">Status</th><th>Detail</th></tr></thead><tbody>{open_rows}</tbody></table>

<h2>Resolved</h2>
<table><thead><tr><th style="width:28%">Finding</th><th style="width:9%">Status</th><th>Now</th></tr></thead><tbody>{resolved_rows}</tbody></table>
<p><b>What works:</b> limit, market and market-at-best matching with price-time priority, partial fills, IOC, FOK and GTD, cancel, amend and status of working orders, the full DSE price-band ladder in every session and on amends, tick-size enforcement, halted and unknown symbols, whole-share quantities, the per-instrument market lot, duplicate-ClOrdID protection, required-field validation, the Pre-Open auction, day-order expiry and the post-close session, and the session messages Logon / Logout / TestRequest / ResendRequest / SequenceReset.</p>

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

<h2>Recommended next work</h2>
<table><thead><tr><th>#</th><th>Priority</th><th>Change</th><th>Where</th></tr></thead><tbody>{fix_rows}</tbody></table>
<p class="mut">Out of scope for this report: OMS-side defects listed in the OMS reference (GTD sent without expiry, modify without Symbol/762, Market-At-Best blocked by the OMS permission check) — they are on the sending side. Items DSE has never been observed to accept or reject (block, buy-in, iceberg, GTD, cancel, modify, status) are judged against the FIX standard and the OMS reference only and should be re-confirmed against DSE.</p>
</body></html>"""

from weasyprint import HTML
HTML(string=doc).write_pdf(OUT)
print("wrote", OUT, f"({total} cases)")
