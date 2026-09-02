package main

const indexHTML = `<!doctype html><html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Velexa · Simulator Matching Engine</title>
<style>
:root{--bg:#0a1310;--surf:#111f1a;--surf2:#0e1a16;--line:rgba(255,255,255,.08);--ink:#e9f1ec;--ink2:#9db0a8;--ink3:#6b7d75;--brand:#0aa376;--up:#16c784;--down:#ea3943;--mono:ui-monospace,"SF Mono",Menlo,monospace}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;font-size:14px}
.top{display:flex;align-items:center;gap:18px;padding:12px 20px;border-bottom:1px solid var(--line);background:var(--surf2);position:sticky;top:0;z-index:5}
.logo{width:28px;height:28px;border-radius:8px;background:linear-gradient(135deg,var(--brand),#0f8);display:grid;place-items:center;color:#052;font-weight:800}
.brand{font-weight:700;letter-spacing:-.02em}.brand small{display:block;font-weight:500;color:var(--ink3);font-size:11px;letter-spacing:.04em}
.phase{margin-left:auto;display:flex;align-items:center;gap:8px;font-family:var(--mono);font-size:12px;color:var(--ink2)}
.dot{width:8px;height:8px;border-radius:50%;background:var(--up);box-shadow:0 0 0 3px rgba(22,199,132,.2);animation:p 2s infinite}@keyframes p{50%{opacity:.4}}
.idx{font-family:var(--mono);font-weight:700}.idx .v{font-size:16px}
.wrap{display:grid;grid-template-columns:minmax(0,1.4fr) minmax(0,1fr);gap:14px;padding:14px 20px;max-width:1500px;margin:0 auto}
.card{background:var(--surf);border:1px solid var(--line);border-radius:12px;overflow:hidden}
.hd{display:flex;align-items:center;gap:10px;padding:11px 15px;border-bottom:1px solid var(--line);font-weight:600}
.hd .mut{margin-left:auto;font-weight:500;color:var(--ink3);font-size:12px;font-family:var(--mono)}
table{width:100%;border-collapse:collapse;font-size:13px}
th{text-align:right;padding:8px 14px;color:var(--ink3);font-weight:600;font-size:11px;text-transform:uppercase;letter-spacing:.05em;position:sticky;top:0;background:var(--surf2)}
th.l,td.l{text-align:left}td{padding:8px 14px;text-align:right;font-family:var(--mono);border-top:1px solid var(--line)}
tbody tr{cursor:pointer}tbody tr:hover{background:rgba(255,255,255,.03)}tr.sel{background:rgba(10,163,118,.12)}
.up{color:var(--up)}.down{color:var(--down)}.sym{font-family:system-ui;font-weight:600;color:var(--ink)}.nm{color:var(--ink3);font-size:11px}
.mw{max-height:62vh;overflow:auto}
.book{display:grid;grid-template-columns:1fr 1fr;gap:0}
.book .side{padding:6px 0}.book .row{display:flex;justify-content:space-between;padding:3px 14px;font-family:var(--mono);font-size:12px;position:relative}
.book .bar{position:absolute;top:0;bottom:0;opacity:.12}.book .ask .bar{right:0;background:var(--down)}.book .bid .bar{right:0;background:var(--up)}
.book .px{position:relative;z-index:1}.book .ask .px{color:var(--down)}.book .bid .px{color:var(--up)}
.spread{text-align:center;padding:7px;font-family:var(--mono);font-size:12px;color:var(--ink2);border-top:1px solid var(--line);border-bottom:1px solid var(--line);background:var(--surf2)}
.tape .row{display:flex;justify-content:space-between;padding:4px 14px;font-family:var(--mono);font-size:12px;border-top:1px solid var(--line)}
.tape .t{color:var(--ink3)}
form{display:flex;flex-wrap:wrap;gap:8px;padding:12px 15px;align-items:end}
label{display:flex;flex-direction:column;gap:3px;font-size:11px;color:var(--ink3)}
input,select{background:var(--surf2);border:1px solid var(--line);color:var(--ink);border-radius:7px;padding:7px 9px;font-family:var(--mono);font-size:13px}
.btn{border:none;border-radius:7px;padding:8px 16px;font-weight:600;cursor:pointer;color:#052}
.btn.buy{background:var(--up)}.btn.sell{background:var(--down);color:#fff}
.flash{animation:fl .6s}@keyframes fl{0%{background:rgba(10,163,118,.25)}100%{background:transparent}}
.stat{display:flex;gap:20px;padding:10px 20px;border-top:1px solid var(--line);color:var(--ink3);font-family:var(--mono);font-size:12px;justify-content:center}
.stat b{color:var(--ink)}
</style></head><body>
<div class="top">
  <div class="logo">V</div>
  <div class="brand">Velexa · Simulator Matching Engine<small>DSE OMS — technical demonstration</small></div>
  <div class="phase"><span class="dot"></span><span id="phase">Continuous Trading</span></div>
  <div class="idx">DSEX <span class="v" id="idx">—</span> <span id="idxchg"></span></div>
</div>
<div class="wrap">
  <div class="card">
    <div class="hd">Market Watch <span class="mut" id="mwmut">live</span></div>
    <div class="mw"><table>
      <thead><tr><th class="l">Instrument</th><th>LTP</th><th>Chg%</th><th>Bid</th><th>Ask</th><th>Volume</th></tr></thead>
      <tbody id="mkt"></tbody>
    </table></div>
  </div>
  <div style="display:grid;gap:14px;align-content:start">
    <div class="card">
      <div class="hd" id="bkhd">Order Book <span class="mut" id="bksym">—</span></div>
      <div class="book"><div class="side ask" id="asks"></div><div class="side bid" id="bids"></div></div>
      <div class="spread" id="spread"></div>
    </div>
    <div class="card">
      <div class="hd">Manual Order <span class="mut">→ matching engine</span></div>
      <form id="ofrm">
        <label>Symbol<input id="osym" style="width:110px" readonly></label>
        <label>Side<select id="oside"><option>BUY</option><option>SELL</option></select></label>
        <label>Type<select id="otype"><option>LIMIT</option><option>MARKET</option></select></label>
        <label>Price<input id="oprice" type="number" step="0.01" style="width:90px"></label>
        <label>Qty<input id="oqty" type="number" value="500" style="width:80px"></label>
        <button class="btn buy" type="submit" id="obtn">Submit</button>
      </form>
      <div id="ores" style="padding:0 15px 12px;font-family:var(--mono);font-size:12.5px;min-height:18px"></div>
    </div>
    <div class="card tape">
      <div class="hd">Trade Tape <span class="mut" id="tpsym">—</span></div>
      <div id="tape"></div>
    </div>
  </div>
</div>
<div class="stat"><span>Instruments <b id="sInstr">—</b></span><span>Orders <b id="sOrd">—</b></span><span>Trades <b id="sTrd">—</b></span><span>Engine <b style="color:var(--up)">RUNNING</b></span></div>
<script>
let sel=null,mdrows=[];
const $=id=>document.getElementById(id);
const fmt=(n,d=2)=>Number(n||0).toLocaleString('en-US',{minimumFractionDigits:d,maximumFractionDigits:d});
function renderMarket(rows,index){
  mdrows=rows;
  $('idx').textContent=fmt(index);
  const tb=$('mkt');
  tb.innerHTML=rows.map(r=>{
    const cls=r.change_pct>0?'up':(r.change_pct<0?'down':'');
    const arrow=r.change_pct>0?'▲':(r.change_pct<0?'▼':'');
    return '<tr data-s="'+r.symbol+'" class="'+(r.symbol===sel?'sel':'')+'">'+
      '<td class="l"><span class="sym">'+r.symbol+'</span><div class="nm">'+r.name+'</div></td>'+
      '<td class="'+cls+'">'+fmt(r.last)+'</td>'+
      '<td class="'+cls+'">'+arrow+' '+fmt(r.change_pct)+'%</td>'+
      '<td class="up">'+(r.bid?fmt(r.bid):'—')+'</td>'+
      '<td class="down">'+(r.ask?fmt(r.ask):'—')+'</td>'+
      '<td>'+r.volume.toLocaleString()+'</td></tr>';
  }).join('');
  [...tb.querySelectorAll('tr')].forEach(tr=>tr.onclick=()=>select(tr.dataset.s));
  if(!sel&&rows.length){select(rows[0].symbol);}
}
function select(s){sel=s;$('bksym').textContent=s;$('tpsym').textContent=s;$('osym').value=s;
  const r=mdrows.find(x=>x.symbol===s);if(r&&!$('oprice').value)$('oprice').value=fmt(r.last);
  [...document.querySelectorAll('#mkt tr')].forEach(tr=>tr.classList.toggle('sel',tr.dataset.s===s));loadBook();}
async function loadBook(){if(!sel)return;const d=await (await fetch('/api/book?symbol='+sel)).json();if(d.error)return;
  const maxq=Math.max(1,...d.asks.map(l=>l.qty),...d.bids.map(l=>l.qty));
  $('asks').innerHTML=d.asks.slice().reverse().map(l=>row('ask',l,maxq)).join('')||'<div class="row" style="color:#6b7d75;justify-content:center">no asks</div>';
  $('bids').innerHTML=d.bids.map(l=>row('bid',l,maxq)).join('')||'<div class="row" style="color:#6b7d75;justify-content:center">no bids</div>';
  const sp=(d.md.ask&&d.md.bid)?(d.md.ask-d.md.bid):0;
  $('spread').innerHTML='Spread <b>'+fmt(sp)+'</b> &nbsp;·&nbsp; Last <b>'+fmt(d.md.last)+'</b> &nbsp;·&nbsp; H '+fmt(d.md.high)+' L '+fmt(d.md.low);
  $('tape').innerHTML=(d.trades||[]).map(t=>'<div class="row"><span class="'+(t.aggressor==='BUY'?'up':'down')+'">'+fmt(t.price)+'</span><span>'+t.qty.toLocaleString()+'</span><span class="t">'+new Date(t.time).toLocaleTimeString()+'</span></div>').join('');
}
function row(side,l,maxq){return '<div class="row '+side+'"><span class="bar" style="width:'+(l.qty/maxq*100)+'%"></span><span class="px">'+fmt(l.price)+'</span><span>'+l.qty.toLocaleString()+'</span></div>';}
$('ofrm').onsubmit=async e=>{e.preventDefault();
  const body={symbol:$('osym').value,side:$('oside').value,type:$('otype').value,price:parseFloat($('oprice').value)||0,qty:parseInt($('oqty').value)||0,actor:'MANUAL'};
  const rep=await (await fetch('/api/order',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})).json();
  const c=rep.status==='FILLED'?'up':(rep.status==='REJECTED'?'down':'');
  $('ores').innerHTML='<span class="'+c+'">'+rep.status+'</span> '+(rep.filled_qty?('· '+rep.filled_qty.toLocaleString()+' @ '+fmt(rep.avg_px)+(rep.leaves_qty?(' · '+rep.leaves_qty+' resting'):'')):'· resting in book');
  $('obtn').classList.add('flash');setTimeout(()=>$('obtn').classList.remove('flash'),600);loadBook();};
$('oside').onchange=()=>{$('obtn').className='btn '+($('oside').value==='BUY'?'buy':'sell');};
// SSE
const es=new EventSource('/events');
es.onmessage=ev=>{const m=JSON.parse(ev.data);if(m.type==='market'){renderMarket(m.rows,m.index);$('phase').textContent=m.phase;}};
async function poll(){const s=await (await fetch('/api/status')).json();$('sInstr').textContent=s.instruments;$('sOrd').textContent=s.orders.toLocaleString();$('sTrd').textContent=s.trades.toLocaleString();}
setInterval(loadBook,800);setInterval(poll,1500);poll();
fetch('/api/market').then(r=>r.json()).then(d=>renderMarket(d.rows,d.index));
</script></body></html>`
