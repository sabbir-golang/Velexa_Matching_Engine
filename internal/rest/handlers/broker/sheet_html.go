package broker

import (
	"encoding/json"
	"html"
	"time"

	"velexa-sim-me/internal/web"
)

// brokerSheetHTML renders a printable, self-contained FIX/FIXS + ITCH
// connectivity guide for one broker, filled with its credentials.
func brokerSheetHTML(company, sender, fixPass, itchUser, itchPass string) string {
	e := html.EscapeString
	ep := meEndpoints()
	title := sender
	if company != "" {
		title = company
	}
	issued := time.Now().UTC().Format("02 Jan 2006")

	// crow renders a copyable credential/connection row. hot=true marks a secret
	// (amber). The value carries a data-copy attribute for the per-row Copy button.
	crow := func(k, v string, hot bool) string {
		cls := "cv"
		if hot {
			cls += " hot"
		}
		return `<div class="crow"><div class="ck">` + e(k) + `</div>` +
			`<div class="` + cls + `" data-copy="` + e(v) + `">` + e(v) + `</div>` +
			`<button class="cp no-print" type="button" onclick="cpy(this)">Copy</button></div>`
	}
	step := func(n, t string) string {
		return `<div class="step"><span class="n">` + n + `</span><div>` + t + `</div></div>`
	}

	// Plain-text version for Copy-all / Download / Share.
	pad := func(k string) string {
		for len(k) < 22 {
			k += " "
		}
		return k
	}
	extLabel, tlsNote := "FIX (plain TCP)", ""
	if ep["ext_tls"] != "" {
		extLabel, tlsNote = "FIXS external (TLS)", " (set <code>SocketUseSSL=Y</code>). Use the <b>origin IP</b> — <code>"+e(ep["pub_host"])+"</code> may sit behind a proxy that does not pass this port"
	}
	versionText := "BeginString: " + ep["begin"]
	if ep["applver"] != "" {
		versionText += "   ApplVerID: " + ep["applver"] + " (" + ep["applver_name"] + ")"
	}
	intText, intRow, intStep := "", "", ""
	if ep["show_int"] != "" {
		intText = "  " + pad("Internal (on-platform):") + ep["int_ip"] + ":" + ep["int_port"] + "\n"
		intRow = crow("Internal — on-platform", ep["int_ip"]+" : "+ep["int_port"], false)
		intStep = ` On-platform brokers use the internal endpoint <code>` + e(ep["int_ip"]+":"+ep["int_port"]) + `</code> instead.`
	}
	applRow := ""
	if ep["applver"] != "" {
		applRow = crow("ApplVerID (1128)", ep["applver"]+"  ("+ep["applver_name"]+")", false)
	}
	txt := "VELEXA MATCHING ENGINE — FIX / FIXS + ITCH CONNECTIVITY\n" +
		"Broker: " + title + "  (SenderCompID " + sender + ")\n" +
		"Issued: " + issued + "   ·   Confidential to the receiving broker\n" +
		"TargetCompID: " + ep["target"] + "   " + versionText + "\n\n" +
		"SESSION ENDPOINT\n" +
		"  " + pad(extLabel+":") + ep["ext_addr"] + "\n" +
		"  " + pad("Host / SNI:") + ep["pub_host"] + "\n" +
		intText + "\n" +
		"ORDER ENTRY — FIX\n" +
		"  " + pad("SenderCompID (49):") + sender + "\n" +
		"  " + pad("Username (553):") + sender + "\n" +
		"  " + pad("Password (554):") + fixPass + "\n\n" +
		"MARKET DATA — ITCH\n" +
		"  " + pad("Feed (IP:port):") + ep["itch_ip"] + ":" + ep["itch_port"] + "\n" +
		"  " + pad("Username:") + itchUser + "\n" +
		"  " + pad("Password:") + itchPass + "\n\n" +
		"Confidential — these credentials authorise order entry under your firm."
	jsTxt, _ := json.Marshal(txt)
	jsName, _ := json.Marshal("Velexa-" + sender + "-connectivity.txt")
	jsTitle, _ := json.Marshal("Velexa Connectivity — " + title)

	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Connectivity — ` + e(title) + `</title>` + web.AdminFontLink + `
<style>
:root{--ink:#0f1729;--mut:#5b6577;--faint:#8a93a3;--line:#e4e8ef;--brand:#16233f;--brand2:#24345c;
      --acc:#0e9f8a;--acc-soft:#e6f6f2;--hot:#b45309;--hot-bg:#fff8f0;--hot-line:#f0dcc0;--page:#eef1f6;--card:#fff;}
*{box-sizing:border-box}
body{margin:0;background:var(--page);color:var(--ink);line-height:1.6;
     font-family:Inter,system-ui,-apple-system,sans-serif;-webkit-font-smoothing:antialiased;
     padding:34px 18px 70px;font-feature-settings:"tnum" 1,"cv05" 1;}
.mono{font-family:"IBM Plex Mono",ui-monospace,Menlo,monospace}

/* ── action toolbar (screen only) ── */
.toolbar{position:fixed;top:16px;right:18px;display:flex;gap:8px;z-index:20}
.tbtn{display:inline-flex;align-items:center;gap:7px;border:1px solid var(--line);background:var(--card);
      color:var(--ink);font:600 12.5px Inter,sans-serif;padding:9px 13px;border-radius:9px;cursor:pointer;
      box-shadow:0 1px 2px rgba(16,23,41,.06);transition:.15s}
.tbtn:hover{border-color:var(--acc);color:var(--acc);transform:translateY(-1px)}
.tbtn.primary{background:var(--brand);color:#fff;border-color:var(--brand)}
.tbtn.primary:hover{background:var(--brand2);color:#fff}
.tbtn svg{width:15px;height:15px;stroke:currentColor;fill:none;stroke-width:1.9;stroke-linecap:round;stroke-linejoin:round}

/* ── sheet ── */
.sheet{max-width:800px;margin:0 auto;background:var(--card);border:1px solid var(--line);border-radius:18px;
       overflow:hidden;box-shadow:0 24px 60px -30px rgba(16,23,41,.45),0 2px 8px rgba(16,23,41,.05)}
.band{background:linear-gradient(135deg,var(--brand) 0%,#0c1730 100%);color:#fff;padding:30px 44px 26px;position:relative}
.band:after{content:"";position:absolute;left:0;bottom:0;height:4px;width:100%;
            background:linear-gradient(90deg,var(--acc),#5fd0bd 60%,transparent)}
.brandline{display:flex;align-items:center;gap:12px;margin-bottom:20px}
.mark{width:38px;height:38px;border-radius:10px;background:rgba(255,255,255,.10);border:1px solid rgba(255,255,255,.22);
      display:flex;align-items:center;justify-content:center;font-weight:700;font-size:20px;letter-spacing:-.02em;color:#fff}
.brandtxt b{font-size:14px;letter-spacing:.02em;display:block;line-height:1.2}
.brandtxt span{font-size:11px;color:#9fb0d0;letter-spacing:.14em;text-transform:uppercase}
.eyebrow{font-family:"IBM Plex Mono",monospace;font-size:10.5px;letter-spacing:.22em;text-transform:uppercase;
         color:#7fe6d3;font-weight:600;margin:0 0 6px}
.band h1{font-size:27px;line-height:1.15;margin:0 0 8px;letter-spacing:-.025em;font-weight:700;text-wrap:balance}
.metaline{display:flex;flex-wrap:wrap;gap:6px 18px;font-size:12px;color:#b9c6de}
.metaline .mono{color:#dbe6f7}
.pill{display:inline-flex;align-items:center;gap:6px;background:rgba(127,230,211,.14);border:1px solid rgba(127,230,211,.3);
      color:#9ff0df;font-size:11px;font-weight:600;padding:3px 9px;border-radius:999px}

.body{padding:30px 44px 40px}
h2{font-family:"IBM Plex Mono",monospace;font-size:11px;text-transform:uppercase;letter-spacing:.13em;
   color:var(--acc);margin:30px 0 12px;font-weight:600;display:flex;align-items:center;gap:9px}
h2:first-of-type{margin-top:4px}
h2:before{content:"";width:16px;height:1.5px;background:var(--acc);display:inline-block}

.card{border:1px solid var(--line);border-radius:12px;overflow:hidden}
.card.secret{background:var(--hot-bg);border-color:var(--hot-line)}
.crow{display:grid;grid-template-columns:200px 1fr auto;align-items:center;gap:14px;padding:11px 16px;border-bottom:1px solid var(--line)}
.card.secret .crow{border-bottom-color:var(--hot-line)}
.crow:last-child{border-bottom:none}
.ck{color:var(--mut);font-size:12.5px;font-weight:600}
.cv{font-family:"IBM Plex Mono",monospace;font-size:13.5px;color:var(--ink);word-break:break-all}
.cv.hot{color:var(--hot);font-weight:600}
.cp{border:1px solid var(--line);background:var(--card);color:var(--faint);font:600 11px Inter,sans-serif;
    padding:5px 10px;border-radius:7px;cursor:pointer;transition:.12s;white-space:nowrap}
.cp:hover{border-color:var(--acc);color:var(--acc)}
.cp.ok{border-color:var(--acc);color:var(--acc);background:var(--acc-soft)}

.steps{margin-top:4px}
.step{display:grid;grid-template-columns:28px 1fr;gap:13px;margin:11px 0;font-size:13.5px;color:#3a4457}
.step .n{width:26px;height:26px;border-radius:50%;background:var(--acc-soft);color:var(--acc);
         display:flex;align-items:center;justify-content:center;font-family:"IBM Plex Mono",monospace;font-weight:600;font-size:12px}
.step b{color:var(--ink)}
code{background:#f1f4f9;border:1px solid var(--line);border-radius:5px;padding:1px 6px;font-family:"IBM Plex Mono",monospace;font-size:12px;color:var(--brand2)}

.ref{display:grid;grid-template-columns:1fr 1fr;gap:9px 26px;font-size:12.5px;color:#3a4457;
     border:1px solid var(--line);border-radius:12px;padding:16px 18px;background:#fafbfd}
@media(max-width:620px){.ref{grid-template-columns:1fr}}
.ref code{background:#eef1f7}

.foot{margin-top:26px;border-top:1px solid var(--line);padding-top:16px;color:var(--mut);font-size:12px;line-height:1.65}
.foot .warn{display:inline-flex;align-items:center;gap:7px;color:var(--hot);font-weight:600;margin-bottom:5px}
.foot .warn svg{width:14px;height:14px;stroke:currentColor;fill:none;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}

/* ── print ── */
@media print{
  @page{margin:14mm}
  body{background:#fff;padding:0}
  .toolbar,.cp{display:none!important}
  .sheet{box-shadow:none;border:none;border-radius:0;max-width:none}
  .band{-webkit-print-color-adjust:exact;print-color-adjust:exact;padding:22px 8px 18px}
  .band h1{font-size:23px}
  .body{padding:22px 8px}
  .card,.ref{break-inside:avoid}
  .crow{grid-template-columns:200px 1fr}
}
</style></head><body>

<div class="toolbar">
  <button class="tbtn primary" type="button" onclick="window.print()" title="Print or save as PDF">
    <svg viewBox="0 0 24 24"><path d="M6 9V2h12v7"/><path d="M6 18H4a2 2 0 0 1-2-2v-4a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2h-2"/><path d="M6 14h12v8H6z"/></svg>Print / PDF</button>
  <button class="tbtn" type="button" onclick="copyAll(this)" title="Copy all credentials as text">
    <svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>Copy all</button>
  <button class="tbtn" type="button" onclick="dl(this)" title="Download as a text file">
    <svg viewBox="0 0 24 24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M7 10l5 5 5-5"/><path d="M12 15V3"/></svg>Download</button>
  <button class="tbtn" type="button" onclick="share(this)" title="Share">
    <svg viewBox="0 0 24 24"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><path d="M8.6 13.5l6.8 4M15.4 6.5l-6.8 4"/></svg>Share</button>
</div>

<div class="sheet">
  <div class="band">
    <div class="brandline">
      <div class="mark">V</div>
      <div class="brandtxt"><b>Velexa Matching Engine</b><span>DSE · Order &amp; Market Data</span></div>
    </div>
    <p class="eyebrow">FIX / FIXS + ITCH Connectivity Certificate</p>
    <h1>` + e(title) + `</h1>
    <div class="metaline">
      <span>SenderCompID <b class="mono">` + e(sender) + `</b></span>
      <span>Target <b class="mono">` + e(ep["target"]) + `</b></span>
      <span>Issued <b class="mono">` + e(issued) + `</b></span>
      <span class="pill">` + e(fixVersion(ep)) + `</span>
    </div>
  </div>

  <div class="body">
    <h2>Session endpoint</h2>
    <div class="card">` +
		crow(extLabel, ep["ext_addr"], false) +
		crow("Host / SNI", ep["pub_host"], false) +
		intRow +
		crow("BeginString (8)", ep["begin"], false) +
		applRow +
		crow("TargetCompID (56)", ep["target"], false) + `</div>

    <h2>Order entry — FIX credentials</h2>
    <div class="card secret">` +
		crow("SenderCompID (49)", sender, true) +
		crow("Username (553)", sender, true) +
		crow("Password (554)", fixPass, true) + `</div>

    <h2>Market data — ITCH feed</h2>
    <div class="card secret">` +
		crow("Feed (IP : port)", ep["itch_ip"]+" : "+ep["itch_port"], true) +
		crow("Username", itchUser, true) +
		crow("Password", itchPass, true) + `</div>

    <h2>How to connect</h2>
    <div class="steps">` +
		step("1", `Open a `+map[bool]string{true: "<b>TLS</b>", false: "TCP"}[ep["ext_tls"] != ""]+` connection to <code>`+e(ep["ext_addr"])+`</code>`+tlsNote+`.`+intStep) +
		step("2", `Send a <b>Logon (35=A)</b> with your SenderCompID, <code>TargetCompID=`+e(ep["target"])+`</code>, and Username (553) + Password (554).`) +
		step("3", `On accept, submit orders with <b>NewOrderSingle (35=D)</b>; receive an <b>ExecutionReport (35=8)</b> for each state change.`) +
		step("4", `Subscribe to market data on the <b>ITCH</b> feed with the username / password above.`) + `</div>

    <h2>Supported orders</h2>
    <div class="ref">
      <div>Order types (40): <code>1</code> Market · <code>2</code> Limit · <code>3</code> Stop · <code>4</code> Stop-Limit</div>
      <div>Iceberg: <code>111</code> MaxFloor (displayed qty)</div>
      <div>Time-in-force (59): <code>0</code> Day · <code>1</code> GTC · <code>3</code> IOC · <code>4</code> FOK · <code>6</code> GTD</div>
      <div>Cancel / Amend: <code>35=F</code> / <code>35=G</code></div>
    </div>

    <div class="foot">
      <div class="warn"><svg viewBox="0 0 24 24"><path d="M12 2 2 21h20L12 2z"/><path d="M12 9v5"/><path d="M12 17h.01"/></svg>Confidential</div>
      These credentials authorise order entry under your firm — share only over a secure channel and store them safely.
      Sequence numbers reset on logon (ResetSeqNumFlag); on a gap send a ResendRequest (35=2).
    </div>
  </div>
</div>

<script>
var SHEET_TEXT=` + string(jsTxt) + `,SHEET_NAME=` + string(jsName) + `,SHEET_TITLE=` + string(jsTitle) + `;
function flash(btn,t){if(!btn)return;var o=btn.dataset.o||btn.textContent;btn.dataset.o=o;btn.textContent=t;btn.classList.add('ok');setTimeout(function(){btn.textContent=o;btn.classList.remove('ok')},1150)}
function put(v,btn,label){if(navigator.clipboard&&navigator.clipboard.writeText){navigator.clipboard.writeText(v).then(function(){flash(btn,label)},function(){flash(btn,'Ctrl+C')})}else{flash(btn,'—')}}
function cpy(btn){var el=btn.previousElementSibling;put(el.getAttribute('data-copy')||el.textContent,btn,'Copied')}
function copyAll(btn){put(SHEET_TEXT,btn,'Copied all')}
function dl(btn){try{var b=new Blob([SHEET_TEXT],{type:'text/plain'}),u=URL.createObjectURL(b),a=document.createElement('a');a.href=u;a.download=SHEET_NAME;document.body.appendChild(a);a.click();a.remove();URL.revokeObjectURL(u);flash(btn,'Saved')}catch(e){copyAll(btn)}}
function share(btn){if(navigator.share){navigator.share({title:SHEET_TITLE,text:SHEET_TEXT}).then(function(){flash(btn,'Shared')},function(){});}else{copyAll(btn)}}
</script>
</body></html>`
}
