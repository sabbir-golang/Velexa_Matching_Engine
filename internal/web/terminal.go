// Package web holds the HTML pages the engine serves: the trade terminal, the admin console and the benchmark page.
package web

const IndexHTML = `<!doctype html><html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Velexa · Simulator Matching Engine</title>
<script>try{if(new URLSearchParams(location.search).has('embed'))document.documentElement.classList.add('embed')}catch(e){}</script>
<link rel="preconnect" href="https://fonts.googleapis.com"><link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500;600&display=swap" rel="stylesheet">
<style>
:root{--bg:#0a1310;--surf:#111f1a;--surf2:#0e1a16;--line:rgba(255,255,255,.08);--ink:#e9f1ec;--ink2:#9db0a8;--ink3:#6b7d75;--brand:#0aa376;--up:#16c784;--down:#ea3943;--mono:ui-monospace,"SF Mono",Menlo,monospace}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;font-size:14px}
.top{display:flex;align-items:center;gap:18px;padding:12px 20px;border-bottom:1px solid var(--line);background:var(--surf2);position:sticky;top:0;z-index:5}
.logo{width:28px;height:28px;border-radius:8px;background:linear-gradient(135deg,var(--brand),#0f8);display:grid;place-items:center;color:#052;font-weight:800}
.brand{font-weight:700;letter-spacing:-.02em}.brand small{display:block;font-weight:500;color:var(--ink3);font-size:11px;letter-spacing:.04em}
.phase{margin-left:auto;display:flex;align-items:center;gap:8px;font-family:var(--mono);font-size:12px;color:var(--ink2)}
.dot{width:8px;height:8px;border-radius:50%;background:var(--up);box-shadow:0 0 0 3px rgba(22,199,132,.2);animation:p 2s infinite}@keyframes p{50%{opacity:.4}}
.idx{font-family:var(--mono);font-weight:700}.idx .v{font-size:16px}
.wrap{display:grid;grid-template-columns:minmax(0,1.9fr) minmax(0,1fr);gap:14px;padding:14px 20px;max-width:1800px;margin:0 auto}
.card{background:var(--surf);border:1px solid var(--line);border-radius:12px;overflow:hidden}
.hd{display:flex;align-items:center;gap:10px;padding:11px 15px;border-bottom:1px solid var(--line);font-weight:600}
.hd .mut{margin-left:auto;font-weight:500;color:var(--ink3);font-size:12px;font-family:var(--mono)}
table{width:100%;border-collapse:collapse;font-size:13px}
th{text-align:right;padding:8px 14px;color:var(--ink3);font-weight:600;font-size:11px;text-transform:uppercase;letter-spacing:.05em;position:sticky;top:0;background:var(--surf2)}
th.l,td.l{text-align:left}td{padding:8px 14px;text-align:right;font-family:var(--mono);border-top:1px solid var(--line)}
tbody tr{cursor:pointer}tbody tr:hover{background:rgba(255,255,255,.03)}tr.sel{background:rgba(10,163,118,.12)}
.up{color:var(--up)}.down{color:var(--down)}.sym{font-family:system-ui;font-weight:600;color:var(--ink)}.nm{color:var(--ink3);font-size:11px}
.mw{overflow-x:auto}
.fbar{display:flex;flex-wrap:wrap;align-items:center;gap:8px;padding:10px 15px;border-bottom:1px solid var(--line);background:var(--surf2)}
.fbar select,.fbar input{font-family:system-ui,sans-serif;font-size:12.5px;padding:6px 9px;border-radius:7px;min-width:0}
.fbar input{width:170px}
.tabs{display:flex;gap:4px}.tab{border:1px solid var(--line);background:transparent;color:var(--ink2);border-radius:7px;padding:5px 12px;font-size:12.5px;font-weight:600;cursor:pointer}
.tab:hover{border-color:var(--brand)}.tab.on{background:var(--brand);border-color:var(--brand);color:#04231a}
.mwt th{white-space:nowrap;padding:8px 11px;border-bottom:1px solid var(--line);z-index:2}.mwt td{padding:5px 11px;white-space:nowrap}
.mwt tbody tr:nth-child(even){background:rgba(255,255,255,.025)}.mwt tbody tr.sel{background:rgba(10,163,118,.22)}
.mwt td.bid{background:rgba(22,199,132,.09)}.mwt td.ask{background:rgba(234,57,67,.09)}
.mwt tr.off td{opacity:.55}.mwt .sym{font-weight:700}.mwt td.c{text-align:left;color:var(--ink2);font-family:system-ui,sans-serif;font-size:12.5px}
.tag{display:inline-block;margin-left:6px;padding:1px 6px;border-radius:9px;font-family:system-ui;font-size:10px;font-weight:700;vertical-align:1px}
.tag.halted{background:rgba(234,57,67,.18);color:var(--down)}.tag.pending{background:rgba(234,176,77,.18);color:#eab04d}
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
.stat{display:flex;gap:18px;color:var(--ink3);font-family:var(--mono);font-size:12px;margin-left:18px;white-space:nowrap}
.stat b{color:var(--ink)}
html{color-scheme:dark}
*{scrollbar-width:thin;scrollbar-color:#2b3644 transparent}
select{appearance:none;-webkit-appearance:none;padding-right:28px;cursor:pointer;background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%237d8a9c' stroke-width='2.5' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E");background-repeat:no-repeat;background-position:right 9px center}
select:focus,input:focus{outline:0;border-color:var(--brand)}
select option{background:#131a24;color:var(--ink)}
/* embedded in the admin console: same palette and chrome as the admin, no duplicate header */
.pager{display:flex;align-items:center;gap:10px;padding:10px 16px;border-top:1px solid var(--line);background:var(--surf2);color:var(--ink3);font-size:12.5px}
.pager label{display:flex;flex-direction:row;align-items:center;gap:6px;font-size:12.5px;color:var(--ink3)}
.pager select{padding:4px 26px 4px 8px;font-family:inherit;font-size:12.5px}
.pager .tbtn:disabled{opacity:.4;cursor:default}
@media(max-width:1250px){.wrap{grid-template-columns:minmax(0,1fr)}}
.top{flex-wrap:wrap}
html.embed,html.embed body{overflow:hidden}
.tools{display:none;gap:8px;margin-left:14px}
.tbtn{background:transparent;border:1px solid var(--line);color:var(--ink);border-radius:8px;padding:5px 11px;font:600 12px Inter,system-ui,sans-serif;cursor:pointer;text-decoration:none}
.tbtn:hover{border-color:var(--brand)}
html.embed .tools{display:flex}
html.embed .mwt th,html.embed .mwt td{padding-left:9px;padding-right:9px}
html.embed{--bg:#080b11;--surf:#131a24;--surf2:#0f151e;--line:#222b38;--ink:#e8eef6;--ink2:#9aa7b8;--ink3:#7d8a9c;--brand:#4dd0b1;--up:#4ade80;--down:#f87171}
html.embed body{font-family:Inter,system-ui,-apple-system,sans-serif}
html.embed .logo,html.embed .brand{display:none}
html.embed .top{position:static;background:transparent;border:0;padding:2px 0 14px;gap:16px}
html.embed .phase{margin-left:0}html.embed .stat{margin-left:auto}
html.embed .wrap{padding:0;max-width:none}
html.embed .card{border-radius:13px}
html.embed .hd{font-size:14px;padding:13px 16px}
html.embed body{padding:0 2px 12px}
html.embed .tab.on,html.embed .btn.buy{color:#04231f}
</style></head><body>
<div class="top">
  <div class="logo">V</div>
  <div class="brand">Velexa · Simulator Matching Engine<small>DSE OMS — technical demonstration</small></div>
  <div class="phase"><span class="dot"></span><span id="phase">Continuous Trading</span></div>
  <div class="idx">DSEX <span class="v" id="idx">—</span> <span id="idxchg"></span></div>
  <div class="stat"><span>Instruments <b id="sInstr">—</b></span><span>Orders <b id="sOrd">—</b></span><span>Trades <b id="sTrd">—</b></span><span>Engine <b style="color:var(--up)">RUNNING</b></span></div>
  <div class="tools"><button type="button" class="tbtn" onclick="location.reload()">&#8635; Reload</button><a class="tbtn" href="/" target="_blank">New tab &#8599;</a></div>
</div>
<div class="wrap">
  <div class="card">
    <div class="hd">Market Watch <span class="mut" id="mwmut">live</span></div>
    <div class="fbar">
      <select id="fBoard"><option value="">All Boards</option></select>
      <div class="tabs" id="fTabs"><button type="button" class="tab on" data-c="">All</button><button type="button" class="tab" data-c="EQ">Equity</button><button type="button" class="tab" data-c="MF">Mutual Fund</button><button type="button" class="tab" data-c="BOND">Bonds</button></div>
      <select id="fSector"><option value="">All Sectors</option></select>
      <select id="fCat"><option value="">All Category</option></select>
      <select id="fStatus"><option value="">All Status</option><option value="active">Active</option><option value="halted">Halted</option><option value="pending">Pending</option></select>
      <input id="fSearch" placeholder="Search symbol or name…">
    </div>
    <div class="mw"><table class="mwt">
      <thead><tr><th class="l">Short Name</th><th class="l">Board</th><th class="l">Class</th><th class="l">Cat</th><th>Last</th><th>Chg%</th><th>Volume</th><th>Bid</th><th>Bid Qty</th><th>Ask</th><th>Ask Qty</th></tr></thead>
      <tbody id="mkt"></tbody>
    </table></div>
    <div class="pager">
      <span id="pgInfo">—</span><span style="flex:1"></span>
      <label>Rows <select id="pgSize"><option>25</option><option>50</option><option>100</option></select></label>
      <button type="button" class="tbtn" id="pgPrev">&#8249; Prev</button>
      <span id="pgNum">1 / 1</span>
      <button type="button" class="tbtn" id="pgNext">Next &#8250;</button>
    </div>
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
<script>
let sel=null,mdrows=[];
const $=id=>document.getElementById(id);
const fmt=(n,d=2)=>Number(n||0).toLocaleString('en-US',{minimumFractionDigits:d,maximumFractionDigits:d});
const SECT={Bank:'Bank',Cement:'Cement',Ceramic:'Ceramic',CorpBond:'Corporate Bond',Debenture:'Debenture',Engineering:'Engineering','Financial In':'Financial Institutions',FoodAllied:'Food & Allied',FuelPower:'Fuel & Power',Insurance:'Insurance',IT:'IT',Jute:'Jute',Misc:'Miscellaneous',MutFund:'Mutual Fund',PaperPrint:'Paper & Printing',PharmaChem:'Pharmaceuticals & Chemicals',ServRealEst:'Services & Real Estate',Tannery:'Tannery',Telecom:'Telecom',Textile:'Textile',TravelLeisur:'Travel & Leisure'};
const CLS={EQ:'Equity',MF:'Mutual Fund',CB:'Corporate Bond',DB:'Debenture'};
const esc=x=>String(x==null?'':x).replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));
let fClass='',optSig={},page=0,pageSize=25;
function fillSelect(id,first,vals,label){
  const sig=vals.join('|');if(optSig[id]===sig)return;optSig[id]=sig;
  const el=$(id),cur=el.value;
  el.innerHTML='<option value="">'+first+'</option>'+vals.map(v=>'<option value="'+esc(v)+'">'+esc(label?label(v):v)+'</option>').join('');
  el.value=vals.includes(cur)?cur:'';
}
function uniq(rows,k){return [...new Set(rows.map(r=>r[k]).filter(Boolean))].sort();}
function inClass(r){return !fClass||(fClass==='BOND'?(r.asset_class==='CB'||r.asset_class==='DB'):r.asset_class===fClass);}
function renderMarket(rows,index){
  mdrows=rows;
  $('idx').textContent=fmt(index);
  fillSelect('fBoard','All Boards',uniq(rows,'board'));
  fillSelect('fSector','All Sectors',uniq(rows,'sector'),v=>SECT[v]||v);
  fillSelect('fCat','All Category',uniq(rows,'category'));
  const b=$('fBoard').value,sc=$('fSector').value,ct=$('fCat').value,st=$('fStatus').value,q=$('fSearch').value.trim().toLowerCase();
  const vis=rows.filter(r=>(!b||r.board===b)&&(!sc||r.sector===sc)&&(!ct||r.category===ct)&&(!st||r.status===st)&&inClass(r)&&
    (!q||r.symbol.toLowerCase().includes(q)||String(r.name).toLowerCase().includes(q)));
  $('mwmut').textContent=vis.length+' / '+rows.length+' · live';
  const pages=Math.max(1,Math.ceil(vis.length/pageSize));
  if(page>=pages)page=pages-1;
  const from=page*pageSize,shown=vis.slice(from,from+pageSize);
  $('pgInfo').textContent=vis.length?((from+1)+'–'+(from+shown.length)+' of '+vis.length):'0 results';
  $('pgNum').textContent=(page+1)+' / '+pages;
  $('pgPrev').disabled=page<=0;$('pgNext').disabled=page>=pages-1;
  const tb=$('mkt');
  tb.innerHTML=shown.map(r=>{
    const cls=r.change_pct>0?'up':(r.change_pct<0?'down':'');
    const arrow=r.change_pct>0?'▲':(r.change_pct<0?'▼':'');
    const tag=r.status&&r.status!=='active'?'<span class="tag '+r.status+'">'+r.status.toUpperCase()+'</span>':'';
    return '<tr data-s="'+esc(r.symbol)+'" class="'+(r.symbol===sel?'sel':'')+(r.status!=='active'?' off':'')+'">'+
      '<td class="l" title="'+esc(r.name)+'"><span class="sym">'+esc(r.symbol)+'</span>'+tag+'</td>'+
      '<td class="c">'+esc(r.board)+'</td><td class="c">'+esc(CLS[r.asset_class]||r.asset_class)+'</td><td class="c">'+esc(r.category||'—')+'</td>'+
      '<td class="'+cls+'">'+fmt(r.last)+'</td>'+
      '<td class="'+cls+'">'+arrow+' '+fmt(r.change_pct)+'%</td>'+
      '<td>'+r.volume.toLocaleString()+'</td>'+
      '<td class="bid up">'+(r.bid?fmt(r.bid):'—')+'</td><td class="bid">'+(r.bid_qty||0).toLocaleString()+'</td>'+
      '<td class="ask down">'+(r.ask?fmt(r.ask):'—')+'</td><td class="ask">'+(r.ask_qty||0).toLocaleString()+'</td>'+
      '</tr>';
  }).join('')||'<tr><td colspan="11" style="text-align:center;padding:28px;color:var(--ink3);font-family:system-ui">No instruments match the filters.</td></tr>';
  [...tb.querySelectorAll('tr[data-s]')].forEach(tr=>tr.onclick=()=>select(tr.dataset.s));
  if(!sel&&rows.length){select(rows[0].symbol);}
}
const redraw=()=>renderMarket(mdrows,parseFloat($('idx').textContent.replace(/,/g,''))||0);
['fBoard','fSector','fCat','fStatus'].forEach(id=>$(id).onchange=()=>{page=0;redraw();});
$('fSearch').oninput=()=>{page=0;redraw();};
$('pgSize').onchange=()=>{pageSize=parseInt($('pgSize').value,10)||25;page=0;redraw();};
$('pgPrev').onclick=()=>{page=Math.max(0,page-1);redraw();};
$('pgNext').onclick=()=>{page++;redraw();};
$('fTabs').onclick=e=>{const t=e.target.closest('.tab');if(!t)return;fClass=t.dataset.c;page=0;
  document.querySelectorAll('#fTabs .tab').forEach(x=>x.classList.toggle('on',x===t));
  renderMarket(mdrows,parseFloat($('idx').textContent.replace(/,/g,''))||0);};
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
if(document.documentElement.classList.contains('embed')){
  const report=()=>parent.postMessage({type:'tt-height',h:Math.ceil(document.body.getBoundingClientRect().height)},location.origin);
  new ResizeObserver(report).observe(document.body);report();
}
fetch('/api/market').then(r=>r.json()).then(d=>renderMarket(d.rows,d.index));
</script></body></html>`
