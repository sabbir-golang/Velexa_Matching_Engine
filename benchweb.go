package main

import "net/http"

const benchHTML = `<!doctype html><html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Velexa ME — Pipeline Benchmark</title>
<style>
:root{--bg:#0a0e14;--panel:#111823;--panel2:#0d141d;--bd:#1d2836;--tx:#e6edf3;--mut:#7d8896;--grn:#00d47e;--blu:#4c8dff;--amb:#f5a623;--red:#ff5c5c;--vio:#9d7bff}
*{box-sizing:border-box;margin:0}body{background:var(--bg);color:var(--tx);font:14px/1.5 -apple-system,Segoe UI,Roboto,sans-serif}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-variant-numeric:tabular-nums}
.wrap{max-width:1280px;margin:0 auto;padding:20px}
header{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap;margin-bottom:18px}
.brand{display:flex;align-items:center;gap:12px}.dot{width:12px;height:12px;border-radius:3px;background:var(--grn);box-shadow:0 0 14px var(--grn)}
h1{font-size:18px;font-weight:700;letter-spacing:.2px}.sub{color:var(--mut);font-size:12px}
.req{background:#101a13;border:1px solid #1c3a2a;color:var(--grn);padding:6px 12px;border-radius:8px;font-size:12px;font-weight:600}
.ctl{display:flex;align-items:center;gap:8px;flex-wrap:wrap;background:var(--panel);border:1px solid var(--bd);border-radius:12px;padding:12px 14px;margin-bottom:18px}
.ctl .lbl{color:var(--mut);font-size:12px;margin-right:2px}
button{background:#182231;color:var(--tx);border:1px solid var(--bd);border-radius:8px;padding:8px 14px;font-size:13px;cursor:pointer;transition:.15s}
button:hover{border-color:#2f4a6b}button.on{background:var(--grn);color:#04130c;border-color:var(--grn);font-weight:700}
button.rate.sel{background:#12233a;border-color:var(--blu);color:#cfe2ff}
.start{background:var(--grn);color:#04130c;font-weight:700;border-color:var(--grn)}.stop{background:var(--red);color:#fff;border-color:var(--red)}
.clean{background:#2a1e0a;color:var(--amb);border-color:#5a4020;font-weight:600}.clean:hover{border-color:var(--amb)}
.tog{display:inline-flex;align-items:center;gap:6px;padding:6px 10px;border:1px solid var(--bd);border-radius:8px;cursor:pointer;font-size:12px;user-select:none}
.tog.on{border-color:var(--grn);color:var(--grn)}
.hero{display:grid;grid-template-columns:1.3fr 1fr;gap:16px;margin-bottom:16px}
.card{background:var(--panel);border:1px solid var(--bd);border-radius:12px;padding:16px}
.big{font-size:56px;font-weight:800;line-height:1;letter-spacing:-1px}.big.g{color:var(--grn)}
.k{color:var(--mut);font-size:11px;text-transform:uppercase;letter-spacing:.6px}
.mult{display:inline-block;margin-top:8px;background:#101a13;border:1px solid #1c3a2a;color:var(--grn);padding:4px 10px;border-radius:20px;font-size:12px;font-weight:700}
.mult.miss{background:#1a1010;border-color:#3a1c1c;color:var(--red)}
.row2{display:flex;gap:20px;margin-top:10px}.row2 .k{margin-bottom:2px}.row2 .v{font-size:22px;font-weight:700}
canvas{width:100%;height:96px;display:block}
.pipe{display:grid;grid-template-columns:repeat(5,1fr);gap:8px;align-items:stretch;margin-bottom:16px}
.stage{background:var(--panel);border:1px solid var(--bd);border-radius:12px;padding:14px;position:relative}
.stage .nm{font-size:12px;color:var(--mut);text-transform:uppercase;letter-spacing:.5px;margin-bottom:8px}
.stage .lat{font-size:30px;font-weight:800}.stage .u{font-size:14px;color:var(--mut);font-weight:600}
.stage .p9{font-size:11px;color:var(--mut);margin-top:4px}
.stage .arrow{position:absolute;right:-14px;top:50%;transform:translateY(-50%);color:#2b3a4d;font-size:20px;z-index:2}
.s-gen{border-top:2px solid var(--vio)}.s-rms{border-top:2px solid var(--amb)}.s-mat{border-top:2px solid var(--grn)}.s-db{border-top:2px solid var(--blu)}.s-ex{border-top:2px solid #cfe2ff}
.grid3{display:grid;grid-template-columns:1fr 1fr;gap:16px}
.bar{height:8px;background:#0c141d;border-radius:6px;overflow:hidden;margin:6px 0 10px}.bar>i{display:block;height:100%}
.rsn{display:flex;justify-content:space-between;font-size:12px;color:var(--mut);margin-top:6px}
table{width:100%;border-collapse:collapse;font-size:12px}th{text-align:left;color:var(--mut);font-weight:600;padding:6px 8px;border-bottom:1px solid var(--bd);font-size:11px;text-transform:uppercase}
td{padding:5px 8px;border-bottom:1px solid #131c27}.tape{max-height:280px;overflow:auto}
.pill{padding:2px 7px;border-radius:5px;font-size:11px;font-weight:600}.f{background:#0f2419;color:var(--grn)}.n{background:#1a2436;color:#9db4d6}.r{background:#2a1414;color:var(--red)}
.buy{color:var(--grn)}.sell{color:var(--red)}
.foot{color:var(--mut);font-size:11px;margin-top:14px;text-align:center}
.hwgrid{display:grid;grid-template-columns:repeat(5,1fr);gap:10px}
.hw{background:var(--panel2);border:1px solid var(--bd);border-radius:10px;padding:12px}
.hw .hn{width:22px;height:22px;border-radius:6px;background:#182231;display:flex;align-items:center;justify-content:center;font-weight:700;font-size:12px;margin-bottom:8px}
.hw .ht{font-weight:700;font-size:13px;margin-bottom:6px}
.hw .hd{color:var(--mut);font-size:11.5px;line-height:1.45}
.hwnote{margin-top:12px;color:var(--mut);font-size:12px;background:var(--panel2);border:1px solid var(--bd);border-radius:8px;padding:10px 12px;line-height:1.5}
.hwnote b{color:var(--tx)}
@media(max-width:900px){.hwgrid{grid-template-columns:1fr 1fr}.pipe{grid-template-columns:1fr 1fr}.hero{grid-template-columns:1fr}}
.sel{background:#182231;color:var(--tx);border:1px solid var(--bd);border-radius:8px;padding:7px 10px;font-size:12px}
.lpwrap{overflow-x:auto}.lpstages{display:flex;gap:0;align-items:stretch;min-width:max-content}.lpstage{background:#0e141d;border:1px solid #22303f;border-radius:8px;padding:9px 12px;min-width:98px;text-align:center}.lpstage.lppend{opacity:.4}.lpnm{font-size:10.5px;color:#7fa;text-transform:uppercase;letter-spacing:.03em;white-space:nowrap}.lpp95{font-family:monospace;font-size:16px;font-weight:600;color:#e8eef6;margin-top:5px}.lpsub2{font-size:10px;color:#7d8896;margin-top:2px;white-space:nowrap}.lparrow{align-self:center;color:#3a4757;padding:0 5px;font-size:13px}.pipe8{display:flex;gap:8px;overflow-x:auto;align-items:stretch;margin-bottom:16px;padding-bottom:6px}.pipe8 .stage{flex:1 1 120px;min-width:112px}.pipe8 .stage .lat{font-size:23px}.st-api{border-top:2px solid var(--blu)}.st-rms{border-top:2px solid var(--amb)}.st-kafka{border-top:2px solid #a78bfa}.st-fix{border-top:2px solid #f5a623}.st-me{border-top:2px solid var(--grn)}.st-db{border-top:2px solid var(--blu)}.st-blotter{border-top:2px solid #22d3ee}.st-total{border-top:2px solid #cfe2ff}.st-pend{opacity:.4}</style></head><body><div class="wrap">
<header>
  <div class="brand"><div class="dot"></div><div><h1>Velexa Matching Engine — Live Pipeline Benchmark</h1>
  <div class="sub">Order generation → RMS pre-trade checks → price-time matching → durable DB store → execution report</div></div></div>
</header>

<div class="ctl">
  <span class="lbl">Mode</span>
  <select id="mode" class="sel"><option value="synth">Benchmark (synthetic)</option><option value="live" selected>Live — real OMS orders</option></select>
  <span class="lbl liveonly" style="display:none">Broker</span>
  <select id="broker" class="sel liveonly" style="display:none"></select>
  <span style="width:10px"></span>
  <span class="lbl">Side</span>
  <select id="side" class="sel"><option value="BOTH">Buy + Sell (Trade)</option><option value="BUY">Buy only</option><option value="SELL">Sell only</option></select>
  <span style="width:10px"></span>
  <span class="lbl">Rate</span>
  <input id="rateIn" class="sel mono" type="number" min="0" step="100" value="8000" style="width:96px;text-align:right" title="Orders per second to generate — 0 = MAX (unlimited)">
  <span class="lbl">/sec</span>
  <button class="rate" id="btnMax" title="Run flat-out — unlimited rate">MAX</button>
  <span style="width:14px"></span>
  <span class="tog on" id="tRms">● RMS check</span>
  <span class="tog on" id="tDb">● DB check</span>
  <span style="flex:1"></span>
  <span class="tog on" id="tBots" title="Simulator background market bots">● Market bots</span>
  <span class="tog" id="stat" style="color:#7d8896">● STOPPED</span>
  <button class="start" id="btnStart">▶ Start</button>
  <button class="stop" id="btnStop">■ Stop</button>
  <button class="clean liveonly" id="btnClean" style="display:none" title="Delete every simulator-generated (LIVE-*) order/exec across all brokers + master routes + audit, and reset cash/positions. Seeded fixtures are kept.">🧹 Clean test data</button>
</div>

<div id="synthWrap" style="display:none"><div class="hero">
  <div class="card">
    <div class="k">Trades / second — full pipeline with RMS</div>
    <div class="big g mono" id="tps">0</div>
    <div id="mult" class="mult">—</div>
    <div class="row2">
      <div><div class="k">Orders / sec</div><div class="v mono" id="ops">0</div></div>
      <div><div class="k">Generated</div><div class="v mono" id="gen">0</div></div>
      <div><div class="k">Persisted</div><div class="v mono" id="pst">0</div></div>
      <div><div class="k">Elapsed</div><div class="v mono" id="el">0s</div></div>
    </div>
  </div>
  <div class="card">
    <div class="k">Throughput (trades/sec)</div>
    <canvas id="chart"></canvas>
    <div class="k" style="margin-top:6px">End-to-end latency (gen → matched)</div>
    <div style="display:flex;gap:24px;margin-top:4px">
      <div><span class="k">avg</span> <b class="mono" id="e2e">0</b><span class="u"> µs</span></div>
      <div><span class="k">p99</span> <b class="mono" id="e2e9">0</b><span class="u"> µs</span></div>
    </div>
  </div>
</div>

<div class="pipe">
  <div class="stage s-gen"><div class="nm">1 · Generate</div><div><span class="lat mono" id="l_gen">0</span></div><div class="p9" id="g_cnt">0 orders</div><span class="arrow">→</span></div>
  <div class="stage s-rms"><div class="nm">2 · RMS Check</div><div><span class="lat mono" id="l_rms">0</span><span class="u"> µs</span></div><div class="p9">p99 <span id="p_rms">0</span> µs</div><span class="arrow">→</span></div>
  <div class="stage s-mat"><div class="nm">3 · Matching</div><div><span class="lat mono" id="l_mat">0</span><span class="u"> µs</span></div><div class="p9">p99 <span id="p_mat">0</span> µs</div><span class="arrow">→</span></div>
  <div class="stage s-db"><div class="nm">4 · DB Store</div><div><span class="lat mono" id="l_db">0</span><span class="u"> µs</span></div><div class="p9">p99 <span id="p_db">0</span> µs</div><span class="arrow">→</span></div>
  <div class="stage s-ex"><div class="nm">5 · Exec Report</div><div><span class="lat mono" id="l_ex">0</span></div><div class="p9" id="ex_cnt">0 execs</div></div>
</div>

<div class="grid3">
  <div class="card">
    <div class="k" style="margin-bottom:10px">RMS Pre-Trade Risk — pass / reject</div>
    <div style="display:flex;gap:24px;margin-bottom:10px">
      <div><div class="k">Passed</div><div class="v mono" style="color:var(--grn)" id="rp">0</div></div>
      <div><div class="k">Rejected</div><div class="v mono" style="color:var(--red)" id="rr">0</div></div>
      <div><div class="k">Reject %</div><div class="v mono" id="rpc">0%</div></div>
    </div>
    <div id="reasons"></div>
  </div>
  <div class="card">
    <div class="k" style="margin-bottom:8px">Execution Report tape (sampled)</div>
    <div class="tape"><table><thead><tr><th>Time</th><th>Symbol</th><th>Side</th><th>Status</th><th>Last</th><th>Cum</th></tr></thead><tbody id="tape"></tbody></table></div>
  </div>
</div>
</div>
<div id="liveWrap">
  <div class="hero">
    <div class="card">
      <div class="k">Live throughput (orders/sec) &mdash; REAL orders into <span id="lbBroker" style="color:var(--grn)">broker</span> OMS</div>
      <div class="big g mono" id="lvAcc">0</div>
      <div class="row2">
        <div><div class="k">Sent</div><div class="v mono" id="lvSent">0</div></div>
        <div><div class="k">Accepted</div><div class="v mono" style="color:var(--grn)" id="lvAccT">0</div></div>
        <div><div class="k">Rejected</div><div class="v mono" style="color:var(--red)" id="lvRej">0</div></div>
        <div><div class="k">Max orders/sec</div><div class="v mono" style="color:var(--grn)" id="lvMax">0</div></div>
        <div><div class="k">Total time (s)</div><div class="v mono" id="lvTime">0</div></div>
      </div>
    </div>
    <div class="card">
      <div class="k">Round-trip latency (order &rarr; first exec)</div>
      <div style="display:flex;gap:26px;margin-top:6px">
        <div><span class="k">avg</span> <b class="mono" id="lvLatA">0</b> <span class="u">ms</span></div>
        <div><span class="k">p95</span> <b class="mono" id="lvLat95">0</b> <span class="u">ms</span></div>
        <div><span class="k">p99</span> <b class="mono" id="lvLat99">0</b> <span class="u">ms</span></div>
      </div>
    </div>
  </div>

  <div class="k" style="margin:2px 0 8px">Order Pipeline &mdash; P95 latency per stage &middot; real path (p99 &amp; sample count below each)</div>
  <div class="pipe8" id="lpStages"><div class="sub">waiting for orders&hellip;</div></div>

  <div class="grid3">
    <div class="card">
      <div class="k" style="margin-bottom:10px">RMS Pre-Trade Risk &mdash; accept / reject</div>
      <div style="display:flex;gap:24px;margin-bottom:10px">
        <div><div class="k">Accepted</div><div class="v mono" style="color:var(--grn)" id="lvPass">0</div></div>
        <div><div class="k">Rejected</div><div class="v mono" style="color:var(--red)" id="lvRej2">0</div></div>
        <div><div class="k">Reject %</div><div class="v mono" id="lvRejPct">0%</div></div>
      </div>
      <div id="lvReasons"></div>
    </div>
    <div class="card">
      <div class="k" style="margin-bottom:8px">Live order tape (sampled) &mdash; real orders &rarr; RMS &rarr; FIX &rarr; Matching Engine</div>
      <div class="tape"><table><thead><tr><th>Time</th><th>Sym</th><th>Side</th><th>Qty</th><th>Price</th><th>Status</th></tr></thead><tbody id="lvTape"></tbody></table></div>
    </div>
  </div>
</div>

<div class="card" style="margin-top:16px">
  <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px">
    <div class="k" style="font-size:13px">How it works — the order pipeline</div>
    <button id="hwToggle" style="padding:4px 12px;font-size:12px">Hide</button>
  </div>
  <div id="hwBody">
    <p style="color:var(--mut);font-size:13px;margin:0 0 14px;max-width:960px">A self-contained synthetic market runs through the exact stages of the live OMS. Each instrument&#39;s order book is pre-seeded with 300 price levels of resting liquidity; synthetic orders then flow through risk checks, matching, and durable storage — every stage timed live on this server.</p>
    <div class="hwgrid">
      <div class="hw stage s-gen"><div class="hn">1</div><div class="ht">Generate</div><div class="hd">Synthetic orders — random instrument, side &amp; size. ~30% market (always trade), ~66% limit (cross or rest), ~4% priced outside the band to exercise RMS.</div></div>
      <div class="hw stage s-rms"><div class="hn">2</div><div class="ht">RMS Check</div><div class="hd">Six pre-trade risk factors: trading halt, max quantity, price band &plusmn;10%, max order value, buying power, position limit. Failures are rejected before matching.</div></div>
      <div class="hw stage s-mat"><div class="hn">3</div><div class="ht">Matching</div><div class="hd">Price-time priority against the seeded book. Each fill is a trade; a crossing order can sweep several levels. Unfilled limits rest as new liquidity.</div></div>
      <div class="hw stage s-db"><div class="hn">4</div><div class="ht">DB Store</div><div class="hd">Every order &amp; execution is batched and bulk-inserted (COPY) into PostgreSQL (sim_orders / sim_execs) — durable persistence, not just in-memory.</div></div>
      <div class="hw stage s-ex"><div class="hn">5</div><div class="ht">Exec Report</div><div class="hd">The execution result (fill price, quantity, status) is produced and streamed to the tape — the confirmation a trader receives.</div></div>
    </div>
    <div class="hwnote"><b>Rate:</b> type any orders/sec and the generator paces to that exact rate — queues stay empty, so end-to-end latency is true microseconds. <b>MAX</b> (rate 0) runs flat-out to show peak capacity (~185k/sec), where orders begin to queue. &nbsp;<b>Side:</b> run Buy-only, Sell-only, or a Buy+Sell mix (Trade). &nbsp;<b>RMS check / DB check</b> toggle the pre-trade risk stage and the durable Postgres write independently, so you can isolate each stage's cost. &nbsp;<b>Liquidity is self-sustaining:</b> crossing orders consume it, resting orders replenish it, so a run continues indefinitely. &nbsp;<b>Trades/sec</b> counts fills through the full pipeline — the figure compared against the DSE requirement.</div>
  </div>
</div>
<div class="foot">Velexa OMS · isolated simulator · UNLOGGED sim_orders / sim_execs on PostgreSQL 16 · figures measured live on this host</div>
</div>
<script>
var rate=8000, rms=true, db=true, hist=[], running=false, TARGET=8000;
function $(id){return document.getElementById(id)}
function fmt(n){return (n||0).toLocaleString('en-US')}
function f1(n){return (n||0).toFixed(1)}
function readRate(){var v=parseInt($('rateIn').value);if(isNaN(v)||v<0)v=0;rate=v;$('btnMax').classList.toggle('sel',v===0);return v}
$('rateIn').oninput=readRate;
$('btnMax').onclick=function(){$('rateIn').value=0;readRate()};
$('tRms').onclick=function(){rms=!rms;this.classList.toggle('on',rms)};
$('tDb').onclick=function(){db=!db;this.classList.toggle('on',db)};
var starting=false;
var mode='live', broker='ALL', side='BOTH';
$('side').onchange=function(){side=this.value};
fetch('/api/live/stats').then(function(r){return r.json()}).then(function(d){var sel=$('broker');var oa=document.createElement('option');oa.value='ALL';oa.textContent='All brokers (equal split)';sel.appendChild(oa);(d.brokers||[]).forEach(function(b){var o=document.createElement('option');o.value=b;o.textContent=b;sel.appendChild(o)});broker='ALL';sel.value='ALL'}).catch(function(){});
$('mode').onchange=function(){mode=this.value;var live=mode==='live';document.querySelectorAll('.liveonly').forEach(function(e){e.style.display=live?'':'none'});$('synthWrap').style.display=live?'none':'';$('liveWrap').style.display=live?'':'none';if(live&&rate>2000){$('rateIn').value=500;readRate()}lpPoll();};
$('broker').onchange=function(){broker=this.value};
function livePoll(){fetch('/api/live/stats').then(function(r){return r.json()}).then(function(d){
  if(!starting)setStat(d.running?'running':'stopped');
  $('lbBroker').textContent=(d.targets>1)?(d.targets+' brokers (equal split)'):((d.cfg&&d.cfg.broker)||broker);
  var now=Date.now();var ir=0;if(window._lpA!=null&&window._lpT){var dt=(now-window._lpT)/1000;if(dt>0)ir=Math.max(0,Math.round((d.accepted-window._lpA)/dt))}window._lpA=d.accepted;window._lpT=now;if(!d.running)ir=0;$('lvAcc').textContent=fmt(ir);if(d.running&&ir>(window._lpMax||0))window._lpMax=ir;$('lvMax').textContent=fmt(window._lpMax||0);$('lvTime').textContent=(d.elapsed_s||0).toFixed(1)+'s';
  $('lvSent').textContent=fmt(d.sent);$('lvAccT').textContent=fmt(d.accepted);$('lvRej').textContent=fmt(d.rejected);$('lvPass').textContent=fmt(d.accepted);$('lvRej2').textContent=fmt(d.rejected);var _st=d.sent||0;$('lvRejPct').textContent=(_st>0?(d.rejected/_st*100).toFixed(1):0)+'%';
  $('lvLatA').textContent=f1(d.lat_avg_ms);$('lvLat95').textContent=f1(d.lat_p95_ms);$('lvLat99').textContent=f1(d.lat_p99_ms);
  var rs=d.reject_reasons||{},keys=Object.keys(rs);
  $('lvReasons').innerHTML=keys.length?keys.map(function(k){return '<div class="rsn"><span style="max-width:72%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+k+'</span><span class="mono">'+fmt(rs[k])+'</span></div>'}).join(''):'<div class="sub">none</div>';
  var tp=d.tape||[];$('lvTape').innerHTML=tp.map(function(t){var sc=t.status==='ACCEPTED'?'f':'r';return '<tr><td class="mono">'+t.t+'</td><td>'+t.sym+'</td><td class="'+(t.side==='BUY'?'buy':'sell')+'">'+t.side+'</td><td class="mono">'+t.qty+'</td><td class="mono">'+(t.price?t.price.toFixed(2):'')+'</td><td><span class="pill '+sc+'">'+t.status+'</span></td></tr>'}).join('');
}).catch(function(){})}
function sleep(ms){return new Promise(function(r){setTimeout(r,ms)})}
function setStat(s){var e=$('stat');if(s==='running'){e.textContent='● RUNNING';e.style.color='#00d47e'}else if(s==='starting'){e.textContent='● starting…';e.style.color='#f5a623'}else{e.textContent='● STOPPED';e.style.color='#7d8896'}}
$('btnStart').onclick=async function(){
  readRate();
  if(mode==='live'){starting=true;setStat('starting');window._lpMax=0;window._lpA=null;window._lpT=0;try{await fetch('/api/live/stop',{method:'POST'})}catch(e){}for(var _w=0;_w<25;_w++){try{var _s=await (await fetch('/api/live/stats')).json();if(!_s.running)break;}catch(e){}await new Promise(function(r){setTimeout(r,200)});}try{await fetch('/api/live/start',{method:'POST',body:JSON.stringify({broker:broker,rate:rate,side:side,sells:side!=='BUY',total:0})})}catch(e){}starting=false;setStat('running');return;}
  starting=true;$('btnStart').disabled=true;setStat('starting');
  try{await fetch('/api/bench/stop',{method:'POST'})}catch(e){}
  for(var i=0;i<25;i++){try{var d=await (await fetch('/api/bench/stats')).json();if(!d.running)break}catch(e){}await sleep(80)}
  try{await fetch('/api/bench/start',{method:'POST',body:JSON.stringify({rms:rms,persist:db,rate:rate,side:side,total:0})})}catch(e){}
  hist=[];running=true;starting=false;$('btnStart').disabled=false;setStat('running');
};
$('btnStop').onclick=async function(){if(mode==='live'){try{await fetch('/api/live/stop',{method:'POST'})}catch(e){}setStat('stopped');return;}try{await fetch('/api/bench/stop',{method:'POST'})}catch(e){}running=false;setStat('stopped')};
$('btnClean').onclick=async function(){
  if(!confirm('Delete ALL simulator-generated (LIVE-*) orders, executions, master routes and audit events across every broker, and reset cash/positions to baseline?\n\nSeeded fixtures (investors, traders, holdings, limits) are kept.'))return;
  var b=this,old=b.textContent;b.disabled=true;b.textContent='Cleaning…';
  try{var r=await (await fetch('/api/live/cleanup',{method:'POST'})).json();
    var nb=(r.brokers&&r.brokers.length)||0;
    alert('Test data cleared.\n\n'+fmt(r.total_orders)+' orders + '+fmt(r.total_execs)+' executions across '+nb+' brokers\n'+fmt(r.routes)+' master routes\n'+fmt(r.audit_events)+' audit events\ncash + positions reset to baseline');
  }catch(e){alert('Cleanup failed: '+e)}
  b.disabled=false;b.textContent=old;
};
function drawChart(){var c=$('chart'),x=c.getContext('2d'),W=c.width=c.clientWidth*2,H=c.height=192;x.clearRect(0,0,W,H);
  if(hist.length<2)return;var mx=Math.max.apply(null,hist)*1.15||1;x.beginPath();x.moveTo(0,H);
  for(var i=0;i<hist.length;i++){var px=i/(hist.length-1)*W,py=H-hist[i]/mx*H;x.lineTo(px,py)}
  x.lineTo(W,H);x.closePath();var g=x.createLinearGradient(0,0,0,H);g.addColorStop(0,'rgba(0,212,126,.35)');g.addColorStop(1,'rgba(0,212,126,0)');x.fillStyle=g;x.fill();
  x.beginPath();for(var j=0;j<hist.length;j++){var qx=j/(hist.length-1)*W,qy=H-hist[j]/mx*H;j?x.lineTo(qx,qy):x.moveTo(qx,qy)}x.strokeStyle='#00d47e';x.lineWidth=3;x.stroke();
  var tgt=H-TARGET/mx*H;if(tgt>0&&tgt<H){x.beginPath();x.moveTo(0,tgt);x.lineTo(W,tgt);x.strokeStyle='rgba(245,166,35,.6)';x.setLineDash([6,6]);x.lineWidth=2;x.stroke();x.setLineDash([])}}
function poll(){fetch('/api/bench/stats').then(function(r){return r.json()}).then(function(d){
  var tps=d.trades_per_sec||0;TARGET=d.target_tps||8000;$('tps').textContent=fmt(tps);if(!starting)setStat(d.running?'running':'stopped');
  $('ops').textContent=fmt(d.orders_per_sec);$('gen').textContent=fmt(d.generated);$('pst').textContent=fmt(d.persisted);$('el').textContent=f1(d.elapsed_s)+'s';
  var m=tps/TARGET;var el=$('mult');if(tps>=TARGET){el.className='mult';el.textContent='✓ '+m.toFixed(1)+'× the DSE requirement ('+TARGET.toLocaleString()+'/s)'}else if(tps>0){el.className='mult miss';el.textContent=m.toFixed(2)+'× of '+TARGET.toLocaleString()+' target'}else{el.textContent='idle'}
  $('e2e').textContent=fmt(Math.round(d.stage_e2e.avg_us));$('e2e9').textContent=fmt(Math.round(d.stage_e2e.p99_us));
  $('l_rms').textContent=f1(d.stage_rms.avg_us);$('p_rms').textContent=f1(d.stage_rms.p99_us);
  $('l_mat').textContent=f1(d.stage_match.avg_us);$('p_mat').textContent=f1(d.stage_match.p99_us);
  $('l_db').textContent=f1(d.stage_persist.avg_us);$('p_db').textContent=f1(d.stage_persist.p99_us);
  $('g_cnt').textContent=fmt(d.generated)+' orders';$('ex_cnt').textContent=fmt(d.stage_persist.count)+' rows';
  $('l_gen').textContent=fmt(d.orders_per_sec);$('l_ex').textContent=fmt(d.trades);
  $('rp').textContent=fmt(d.rms_pass);$('rr').textContent=fmt(d.rms_rej);
  var tot=d.rms_pass+d.rms_rej;$('rpc').textContent=(tot?(d.rms_rej/tot*100).toFixed(1):0)+'%';
  var rs=d.reject_reasons||{},keys=Object.keys(rs),mxr=Math.max.apply(null,keys.map(function(k){return rs[k]}).concat([1]));
  var cols={PRICE_BAND:'#f5a623',QTY_LIMIT:'#ff5c5c',VALUE_LIMIT:'#9d7bff',BUYING_POWER:'#4c8dff',TRADING_HALTED:'#ff5c5c',POSITION_LIMIT:'#4c8dff'};
  $('reasons').innerHTML=keys.map(function(k){return '<div class="rsn"><span>'+k+'</span><span class="mono">'+fmt(rs[k])+'</span></div><div class="bar"><i style="width:'+(rs[k]/mxr*100)+'%;background:'+(cols[k]||'#7d8896')+'"></i></div>'}).join('')||'<div class="sub">no rejects yet</div>';
  var tp=d.tape||[];$('tape').innerHTML=tp.map(function(t){var sc=t.status==='FILLED'?'f':(t.status==='REJECTED'?'r':'n');return '<tr><td class="mono">'+t.t+'</td><td>'+t.sym+'</td><td class="'+(t.side==='BUY'?'buy':'sell')+'">'+t.side+'</td><td><span class="pill '+sc+'">'+t.status+'</span></td><td class="mono">'+(t.last_px?t.last_px.toFixed(2):'—')+'</td><td class="mono">'+fmt(t.cum)+'</td></tr>'}).join('');
  if(d.running){hist.push(tps);if(hist.length>120)hist.shift()}drawChart();
}).catch(function(){})}
$('hwToggle').onclick=function(){var b=$('hwBody');var h=b.style.display==='none';b.style.display=h?'block':'none';this.textContent=h?'Hide':'Show'};
fetch("/api/bots").then(function(r){return r.json()}).then(function(d){var e=$("tBots");e.classList.toggle("on",!d.paused);e.textContent=d.paused?"\u25cf Market bots OFF":"\u25cf Market bots"}).catch(function(){});$("tBots").onclick=function(){var on=this.classList.contains("on");fetch(on?"/api/bots/pause":"/api/bots/resume",{method:"POST"}).catch(function(){});this.classList.toggle("on",!on);this.textContent=on?"\u25cf Market bots OFF":"\u25cf Market bots"};
setInterval(function(){mode==='live'?livePoll():poll()},500);$('mode').dispatchEvent(new Event('change'));mode==='live'?livePoll():poll();
function lpFmt(us){if(us==null)return '\u2014';if(us>=1000)return (us/1000).toFixed(us>=100000?0:us>=10000?1:2)+' ms';return Math.round(us)+' \u00b5s';}
var _stC={api:'st-api',rms:'st-rms',kafka:'st-kafka',fix:'st-fix',me:'st-me',db:'st-db',blotter:'st-blotter',total:'st-total'};
function lpPoll(){if(mode!=='live')return;fetch('/api/live/latency').then(function(r){return r.json()}).then(function(d){var rows=(d&&d.stages)||[];if(!rows.length)return;$('lpStages').innerHTML=rows.map(function(s,i){var has=s.p95_us!=null;var arr=i<rows.length-1?'<span class="arrow">\u2192</span>':'';return '<div class="stage '+(_stC[s.key]||'')+(has?'':' st-pend')+'"><div class="nm">'+(i+1)+' \u00b7 '+s.label+'</div><div><span class="lat mono">'+lpFmt(s.p95_us)+'</span></div><div class="p9">p99 '+lpFmt(s.p99_us)+' \u00b7 '+(has?fmt(s.count)+' obs':'pending')+'</div>'+arr+'</div>';}).join('');var t=rows[rows.length-1],e;if(t){if(e=$('lvTotP95'))e.textContent=lpFmt(t.p95_us);if(e=$('lvTotP99'))e.textContent=lpFmt(t.p99_us);if(e=$('lvTotN'))e.textContent=fmt(t.count||0);}}).catch(function(){});}
setInterval(lpPoll,1500);lpPoll();
</script></body></html>`

func serveBenchUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(benchHTML))
}
