package me

import (
	"encoding/json"
	"html"
	"time"
)

const adminFontLink = `<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin><link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500;600&display=swap" rel="stylesheet">`

const adminLoginHTML = `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Matching Engine — Sign in</title>` + adminFontLink + `
<style>
:root{--bg:#080b11;--panel:#121821;--line:#232c39;--ink:#e8eef6;--mut:#7d8a9c;--acc:#4dd0b1}
*{box-sizing:border-box}body{margin:0;height:100vh;display:flex;align-items:center;justify-content:center;
background:radial-gradient(1000px 500px at 50% -5%,#141d29,#080b11);color:var(--ink);
font-family:Inter,system-ui,-apple-system,sans-serif}
.card{width:352px;background:var(--panel);border:1px solid var(--line);border-radius:16px;padding:34px 30px;
box-shadow:0 24px 60px rgba(0,0,0,.55)}
.brand{display:flex;align-items:center;gap:11px;margin-bottom:4px}
.logo{width:30px;height:30px;border-radius:9px;background:linear-gradient(135deg,#4dd0b1,#2f9e86);
display:flex;align-items:center;justify-content:center;color:#04231f;font-weight:800;font-size:15px}
h1{font-size:17px;margin:0;font-weight:700;letter-spacing:-.02em}
.sub{color:var(--mut);font-size:12.5px;margin:5px 0 24px}
label{display:block;font-size:12px;color:var(--mut);margin:15px 0 6px;font-weight:600}
input{width:100%;padding:11px 13px;background:#0b1119;border:1px solid var(--line);border-radius:10px;
color:var(--ink);font-size:14px;outline:none;transition:border .15s}
input:focus{border-color:var(--acc)}
button{width:100%;margin-top:24px;padding:12px;background:var(--acc);color:#04231f;border:0;border-radius:10px;
font-weight:700;font-size:14px;cursor:pointer;transition:filter .15s}
button:hover{filter:brightness(1.07)}
.err{color:#f87171;font-size:12.5px;margin-top:15px;min-height:16px}
.ft{color:var(--mut);font-size:11px;text-align:center;margin-top:20px;font-family:"IBM Plex Mono",monospace;letter-spacing:.05em}
</style></head><body>
<form class="card" method="post" action="/admin/login">
 <div class="brand"><span class="logo">M</span><h1>Matching Engine</h1></div>
 <div class="sub">Administrator console</div>
 <label>Username</label><input name="user" autofocus autocomplete="username">
 <label>Password</label><input name="pass" type="password" autocomplete="current-password">
 <div class="err">__ERR__</div>
 <button type="submit">Sign in</button>
 <div class="ft">VELEXA · DSEME</div>
</form></body></html>`

const adminDashboardHTML = `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Matching Engine — Console</title>` + adminFontLink + `
<style>
:root{--bg:#080b11;--side:#0d131c;--panel:#131a24;--panel2:#0f151e;--line:#222b38;--line2:#2b3644;
--ink:#e8eef6;--mut:#7d8a9c;--mut2:#586675;--acc:#4dd0b1;--amber:#eab04d;--grn:#4ade80;--red:#f87171;--blue:#7aa2f7}
*{box-sizing:border-box}html,body{margin:0}body{background:var(--bg);color:var(--ink);
font-family:Inter,system-ui,-apple-system,sans-serif;font-size:14px;-webkit-font-smoothing:antialiased}
.mono{font-family:"IBM Plex Mono",ui-monospace,monospace}
.app{display:grid;grid-template-columns:238px 1fr;min-height:100vh}
/* sidebar */
aside{background:var(--side);border-right:1px solid var(--line);display:flex;flex-direction:column;
position:sticky;top:0;height:100vh}
.brand{display:flex;align-items:center;gap:11px;padding:18px 20px 16px}
.logo{width:30px;height:30px;border-radius:9px;background:linear-gradient(135deg,#4dd0b1,#2f9e86);
display:flex;align-items:center;justify-content:center;color:#04231f;font-weight:800;font-size:15px}
.brand b{font-size:14.5px;font-weight:700;letter-spacing:-.01em}
.brand small{display:block;color:var(--mut);font-size:11px;font-weight:500;font-family:"IBM Plex Mono",monospace}
nav{flex:1;padding:8px 12px;overflow-y:auto}
.navgrp{color:var(--mut2);font-size:10.5px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;padding:16px 10px 7px}
.navitem{display:flex;align-items:center;gap:11px;padding:9px 11px;border-radius:9px;color:var(--mut);
cursor:pointer;font-weight:500;font-size:13.5px;margin:1px 0;transition:background .12s,color .12s;border-left:2px solid transparent}
.navitem:hover{background:#141c27;color:var(--ink)}
.navitem.on{background:#152232;color:var(--ink);border-left-color:var(--acc)}
.navitem svg{width:17px;height:17px;flex:0 0 auto;stroke:currentColor;fill:none;stroke-width:1.8}
.navitem .badge{margin-left:auto;font-family:"IBM Plex Mono",monospace;font-size:11px;color:var(--acc);font-weight:600}
.sidefoot{padding:14px 16px;border-top:1px solid var(--line)}
.enpill{display:flex;align-items:center;gap:8px;font-size:12px;color:var(--mut);margin-bottom:10px}
.enpill .d{width:8px;height:8px;border-radius:50%;background:var(--grn);box-shadow:0 0 8px var(--grn)}
.outbtn{display:block;text-align:center;color:var(--mut);text-decoration:none;font-size:12.5px;font-weight:600;
border:1px solid var(--line);border-radius:9px;padding:8px}
.outbtn:hover{border-color:var(--line2);color:var(--ink)}
/* main */
main{min-width:0}
.top{display:flex;align-items:center;gap:14px;padding:20px 30px;border-bottom:1px solid var(--line)}
.top h1{font-size:19px;margin:0;font-weight:700;letter-spacing:-.02em}
.top .sub{color:var(--mut);font-size:12.5px;margin-top:2px}
.top .sp{flex:1}
.hamb{display:none;background:none;border:1px solid var(--line);border-radius:9px;padding:8px 9px;color:var(--ink);cursor:pointer}
.hamb:hover{border-color:var(--acc)}
.backdrop{display:none}
.live{display:flex;align-items:center;gap:8px;font-family:"IBM Plex Mono",monospace;font-size:12px;color:var(--mut);
border:1px solid var(--line);border-radius:20px;padding:6px 13px}
.live .d{width:7px;height:7px;border-radius:50%;background:var(--grn);animation:pulse 2s infinite}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.35}}
.wrap{padding:22px 34px 70px;width:100%}
.filterbar{display:flex;align-items:center;gap:9px;flex-wrap:wrap;margin-bottom:14px}
.filterbar .fi{padding:9px 12px;background:#0b1119;border:1px solid var(--line);border-radius:9px;color:var(--ink);font-size:13px;outline:none;min-width:180px}
.filterbar .fi:focus{border-color:var(--acc)}
.filterbar select.fi{min-width:150px;cursor:pointer}
.filterbar .cnt{margin-left:auto;color:var(--mut);font-size:12px;font-family:"IBM Plex Mono",monospace}
.view{display:none}.view.on{display:block}
h2{font-size:12px;text-transform:uppercase;letter-spacing:.09em;color:var(--mut);margin:28px 0 13px;font-weight:700}
h2:first-child{margin-top:4px}
/* kpi */
.kpis{display:grid;grid-template-columns:repeat(auto-fit,minmax(158px,1fr));gap:13px}
.kpi{background:var(--panel);border:1px solid var(--line);border-radius:13px;padding:17px 18px;position:relative;overflow:hidden}
.kpi::after{content:"";position:absolute;left:0;top:0;bottom:0;width:3px;background:var(--acc);opacity:.6}
.kpi.amber::after{background:var(--amber)}.kpi.grn::after{background:var(--grn)}.kpi.blue::after{background:var(--blue)}
.kpi .v{font-size:26px;font-weight:700;font-family:"IBM Plex Mono",monospace;line-height:1;letter-spacing:-.02em}
.kpi .l{color:var(--mut);font-size:12px;margin-top:9px;font-weight:500}
.grn-t{color:var(--grn)}.amber-t{color:var(--amber)}
/* panel + table */
.panel{background:var(--panel);border:1px solid var(--line);border-radius:13px;overflow:hidden}
.panel.pad{padding:20px}
table{width:100%;border-collapse:collapse;font-size:13px}
th,td{text-align:left;padding:11px 16px;border-bottom:1px solid var(--line)}
th{color:var(--mut);font-size:10.5px;text-transform:uppercase;letter-spacing:.06em;background:var(--panel2);font-weight:600}
tr:last-child td{border-bottom:none}
td.right,th.right{text-align:right}
.chip{display:inline-flex;align-items:center;gap:6px;font-size:11px;font-weight:600;padding:3px 9px;border-radius:20px;font-family:"IBM Plex Mono",monospace}
.chip .d{width:6px;height:6px;border-radius:50%;background:currentColor}
.chip.up{background:rgba(74,222,128,.13);color:var(--grn)}
.chip.down{background:rgba(248,113,113,.13);color:var(--red)}
.chip.LOGON{background:rgba(74,222,128,.13);color:var(--grn)}
.chip.LOGOUT{background:rgba(125,138,156,.14);color:var(--mut)}
.chip.REJECT{background:rgba(248,113,113,.13);color:var(--red)}
.grid2{display:grid;grid-template-columns:1fr 1fr;gap:16px}
@media(max-width:820px){
 .grid2{grid-template-columns:1fr}
 .app{grid-template-columns:1fr}
 aside{position:fixed;left:0;top:0;height:100vh;width:250px;z-index:40;transform:translateX(-100%);transition:transform .22s ease}
 aside.open{transform:translateX(0);box-shadow:0 0 50px rgba(0,0,0,.6)}
 .hamb{display:inline-flex;align-items:center}
 .backdrop.on{display:block;position:fixed;inset:0;background:rgba(0,0,0,.55);z-index:35}
 .wrap{padding:20px 18px 60px}.top{padding:16px 18px}
}
.row{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.btn{background:#1a2431;border:1px solid var(--line);color:var(--ink);padding:9px 15px;border-radius:9px;
font-size:13px;font-weight:600;cursor:pointer;transition:border .12s,background .12s;font-family:inherit}
.btn:hover{border-color:var(--acc)}
.btn.acc{background:var(--acc);color:#04231f;border-color:var(--acc)}
.btn.acc:hover{filter:brightness(1.07)}
.btn.sm{padding:5px 11px;font-size:12px}
.btn.ghost{background:transparent}
.btn.danger{color:var(--red)}.btn.danger:hover{border-color:var(--red)}
input.f{padding:10px 12px;background:#0b1119;border:1px solid var(--line);border-radius:9px;color:var(--ink);font-size:13px;outline:none;width:100%;transition:border .15s}
input.f:focus,select.f:focus{border-color:var(--acc)}
select.f{padding:10px 36px 10px 12px;background:#0b1119 url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%237d8a9c' stroke-width='2.5' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E") no-repeat right 12px center;border:1px solid var(--line);border-radius:9px;color:var(--ink);font-size:13px;font-family:inherit;outline:none;width:100%;appearance:none;-webkit-appearance:none;cursor:pointer;color-scheme:dark;transition:border .15s}
select.f option{background:#131a24;color:var(--ink)}
label.fl{display:block;font-size:11px;color:var(--mut);margin:0 0 6px;font-weight:600}
.field{margin-bottom:14px}
.note{color:var(--mut);font-size:12.5px;margin:0 0 16px;line-height:1.6}
.note a{color:var(--acc)}
.reveal{background:#0e2019;border:1px solid #1c4d3f;border-radius:12px;padding:16px 18px;margin-top:16px;display:none}
.reveal.on{display:block}
.reveal .hd{color:var(--grn);font-weight:700;font-size:13px;display:flex;align-items:center;gap:8px}
.reveal .kv{display:grid;grid-template-columns:160px 1fr;gap:7px 14px;font-family:"IBM Plex Mono",monospace;font-size:13px;margin:12px 0 14px}
.reveal .kv .k{color:var(--mut)}.reveal .kv .val{color:var(--amber);font-weight:600}
.muted{color:var(--mut)}.pad16{padding:16px}
/* embedded simulator */
.simbar{margin-bottom:14px}
.benchframe{border:1px solid var(--line);border-radius:13px;overflow:hidden;background:#0b0f16;box-shadow:0 6px 20px rgba(0,0,0,.35)}
.benchframe iframe{width:100%;height:calc(100vh - 142px);min-height:480px;border:0;display:block}
/* iframe views (Simulator / Trade) fill the page — trim the wrap's bottom padding */
body.iframeview .wrap{padding-bottom:12px}
.tt-bar{display:flex;gap:8px;align-items:center;margin-bottom:10px}
.tt-frame{border:0;border-radius:0;box-shadow:none;background:transparent;overflow:visible}
.tt-frame iframe{height:760px;min-height:480px}
.benchframe:fullscreen{border:0;border-radius:0}
.benchframe:fullscreen iframe{height:100vh}
/* instruments page */
.exp{position:relative}
.exp-menu{display:none;position:absolute;right:0;top:calc(100% + 6px);min-width:240px;background:var(--panel);border:1px solid var(--line2);border-radius:11px;box-shadow:0 14px 40px rgba(0,0,0,.5);z-index:30;padding:6px}
.exp-menu.on{display:block}
.exp-menu button{display:flex;flex-direction:column;align-items:flex-start;width:100%;background:none;border:0;color:var(--ink);padding:8px 11px;border-radius:8px;cursor:pointer;text-align:left;font-size:13px}
.exp-menu button:hover{background:#1a2431}
.exp-menu button span{color:var(--mut);font-size:11.5px;margin-top:1px}
.exp-note{border-top:1px solid var(--line);margin-top:4px;padding:8px 11px 4px;color:var(--mut);font-size:11.5px}
.in-grid{display:grid;grid-template-columns:minmax(0,1fr) 290px;gap:16px;align-items:start;margin-top:16px}
@media(max-width:1100px){.in-grid{grid-template-columns:1fr}}
.card-hd{display:flex;align-items:center;gap:12px;padding:15px 18px;flex-wrap:wrap}
.card-hd h3{margin:0;font-size:15px;letter-spacing:-.01em}.card-hd .sp{flex:1}
.in-search{display:flex;align-items:center;gap:8px;background:#0b1119;border:1px solid var(--line);border-radius:9px;padding:0 11px;min-width:230px}
.in-search svg{width:15px;height:15px;stroke:var(--mut);fill:none;stroke-width:2;flex:none}
.in-search input{background:none;border:0;outline:0;color:var(--ink);font-size:13px;padding:9px 0;width:100%}
.in-search:focus-within{border-color:var(--acc)}
input.f::placeholder{text-transform:none}
.in-chips{display:flex;gap:6px;flex-wrap:wrap;padding:12px 18px;border-top:1px solid var(--line);border-bottom:1px solid var(--line)}
.in-chips .btn.sm{padding:4px 11px;font-size:12px;font-weight:600}
.btn svg{width:15px;height:15px;stroke:currentColor;fill:none;stroke-width:2;vertical-align:-3px;margin-right:5px}
.code-tag{font-family:"IBM Plex Mono",monospace;font-size:11.5px;background:#1a2431;border:1px solid var(--line);border-radius:5px;padding:2px 7px}
.pill{display:inline-flex;align-items:center;gap:6px;font-size:11.5px;font-weight:600;padding:3px 10px;border-radius:20px}
.pill .d{width:6px;height:6px;border-radius:50%;background:currentColor}
.pill.active{background:rgba(74,222,128,.13);color:var(--grn)}.pill.halted{background:rgba(248,113,113,.13);color:var(--red)}
.pill.pending{background:rgba(234,176,77,.15);color:var(--amber)}
.ib{background:transparent;border:1px solid var(--line);color:var(--mut);width:30px;height:30px;border-radius:8px;cursor:pointer;display:inline-flex;align-items:center;justify-content:center;padding:0}
.ib svg{width:14px;height:14px;stroke:currentColor;fill:none;stroke-width:2}.ib:hover{border-color:var(--acc);color:var(--ink)}
.ib.warn{color:var(--amber)}.ib.up{color:var(--grn)}.ib.del:hover{border-color:var(--red);color:var(--red)}
.cls-row{display:flex;align-items:center;gap:10px;padding:11px 16px;border-top:1px solid var(--line);font-size:13px}
.cls-row:first-child{border-top:0}.cls-row .d{width:7px;height:7px;border-radius:50%;flex:none}.cls-row .c{margin-left:auto;color:var(--mut);font-size:11.5px;font-family:"IBM Plex Mono",monospace}
.kpi .sub{margin-top:9px;font-size:11.5px;color:var(--mut)}.kpi .sub .pill{padding:2px 9px;font-size:11px}
.mcard .modal-ico{width:38px;height:38px;border-radius:11px;background:rgba(77,208,177,.14);color:var(--acc);display:flex;align-items:center;justify-content:center;flex:none}
.mcard .modal-ico svg{width:19px;height:19px;stroke:currentColor;fill:none;stroke-width:2}
.mcard .mft{display:flex;justify-content:flex-end;gap:9px;margin-top:6px}
/* detail modal */
.modal{display:none;position:fixed;inset:0;background:rgba(4,7,12,.74);z-index:50;padding:30px 16px;overflow-y:auto}
.modal.on{display:flex;align-items:flex-start;justify-content:center}
.mcard{background:var(--panel);border:1px solid var(--line2);border-radius:16px;max-width:660px;width:100%;padding:26px 30px;box-shadow:0 30px 80px rgba(0,0,0,.6)}
.mcard h3{font-size:19px;margin:0 0 2px;letter-spacing:-.01em}
.mcard .mh{display:flex;align-items:flex-start;gap:12px}
.mcard .close{margin-left:auto;background:none;border:1px solid var(--line);color:var(--mut);border-radius:8px;padding:6px 11px;cursor:pointer;font-size:13px}
.mcard .close:hover{border-color:var(--acc);color:var(--ink)}
.mcard .msub{color:var(--mut);font-size:12.5px;margin:0}
.credbox{background:var(--panel2);border:1px solid var(--line);border-radius:11px;margin:16px 0}
.cr{display:grid;grid-template-columns:150px 1fr auto;gap:10px;align-items:center;padding:9px 15px;border-top:1px solid var(--line);font-size:13px}
.cr:first-child{border-top:none}
.cr .ck{color:var(--mut);font-weight:600}
.cr .cv{font-family:"IBM Plex Mono",monospace;color:var(--amber);font-weight:600;word-break:break-all}
.cr .cv.plain{color:var(--ink);font-weight:500}
.cpy{background:none;border:1px solid var(--line);color:var(--mut);border-radius:6px;padding:3px 9px;font-size:11px;cursor:pointer;font-family:"IBM Plex Mono",monospace}
.cpy:hover{border-color:var(--acc);color:var(--acc)}
.guide{font-size:13px;color:#aeb9c8;line-height:1.65}
.guide .st{display:grid;grid-template-columns:24px 1fr;gap:11px;margin:8px 0}
.guide .st .n{width:22px;height:22px;border-radius:50%;background:#1b2a4a;color:#9db4e0;display:flex;align-items:center;justify-content:center;font-family:"IBM Plex Mono",monospace;font-size:11px;font-weight:600}
.guide code{background:var(--panel2);border:1px solid var(--line);border-radius:4px;padding:1px 5px;font-size:12px}
.tagrow{display:grid;grid-template-columns:1fr 1fr;gap:6px 20px;margin-top:8px;font-size:12.5px}
.tagrow code{color:var(--amber)}
.mactions{display:flex;gap:10px;flex-wrap:wrap;margin-top:22px}
.pillrow{display:flex;gap:8px;margin-top:8px}
</style></head><body>
<div class="app">
 <aside>
  <div class="brand"><span class="logo">M</span><div><b>Matching Engine</b><small>DSEME · velexa</small></div></div>
  <nav>
   <div class="navgrp">Overview</div>
   <div class="navitem on" data-v="dash"><svg viewBox="0 0 24 24"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>Dashboard</div>
   <div class="navgrp">Trading</div>
   <div class="navitem" data-v="trade"><svg viewBox="0 0 24 24"><path d="M3 3v18h18"/><path d="M7 14l3-4 3 2 4-6"/></svg>Trade Terminal</div>
   <div class="navitem" data-v="sessions"><svg viewBox="0 0 24 24"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>FIX Sessions<span class="badge" id="navSess"></span></div>
   <div class="navitem" data-v="sim"><svg viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" rx="2"/><path d="M9 9h6v6H9z"/></svg>Simulator</div>
   <div class="navitem" data-v="instruments"><svg viewBox="0 0 24 24"><path d="M4 19V9M10 19V5M16 19v-7M22 19H2"/></svg>Instruments</div>
   <div class="navgrp">Access</div>
   <div class="navitem" data-v="creds"><svg viewBox="0 0 24 24"><circle cx="8" cy="15" r="4"/><path d="M10.85 12.15 19 4M18 5l2 2M15 8l2 2"/></svg>Broker Credentials</div>
   <div class="navitem" data-v="access"><svg viewBox="0 0 24 24"><path d="M12 2 4 6v6c0 5 3.4 7.7 8 10 4.6-2.3 8-5 8-10V6z"/></svg>OMS IP Allowlist</div>
   <div class="navgrp">System</div>
   <div class="navitem" data-v="msglog"><svg viewBox="0 0 24 24"><path d="M4 4h16v16H4z"/><path d="M8 8h8M8 12h8M8 16h5"/></svg>Message Log</div>
   <div class="navitem" data-v="diag"><svg viewBox="0 0 24 24"><path d="M12 9v4M12 17h.01"/><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/></svg>Diagnostics</div>
  </nav>
  <div class="sidefoot">
   <div class="enpill"><span class="d"></span><span id="enText">engine live</span></div>
   <a class="outbtn" href="/admin/logout">Sign out</a>
  </div>
 </aside>
 <div class="backdrop" id="backdrop"></div>
 <main>
  <div class="top">
   <button class="hamb" id="hamb" aria-label="menu"><svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 6h18M3 12h18M3 18h18"/></svg></button>
   <div><h1 id="pageTitle">Dashboard</h1><div class="sub" id="pageSub">Live engine status</div></div>
   <span class="sp"></span>
   <span class="live"><span class="d"></span><span id="up">—</span></span>
  </div>
  <div class="wrap">

   <section class="view on" id="view-dash">
    <div class="kpis" id="kpis"></div>
    <h2>Listeners</h2>
    <div class="panel"><table><tbody id="listeners"></tbody></table></div>
    <h2>Recent activity</h2>
    <div class="filterbar"><input class="fi" id="f_dash" placeholder="Filter by broker or event…" oninput="applyFilters()"></div>
    <div class="panel"><table><thead><tr><th>Time</th><th>Broker</th><th>Event</th></tr></thead><tbody id="miniEvents"></tbody></table></div>
   </section>

   <section class="view" id="view-sessions">
    <h2>Connected brokers</h2>
    <div class="filterbar"><input class="fi" id="f_sess" placeholder="Filter by SenderCompID…" oninput="applyFilters()"><span class="cnt" id="sess_cnt"></span></div>
    <div class="panel"><table><thead><tr><th>SenderCompID</th><th>Connected since</th><th>Seq&nbsp;out&nbsp;/&nbsp;in</th><th class="right">Controls</th></tr></thead><tbody id="sessions"></tbody></table></div>
    <p class="note" style="margin-top:14px"><b class="muted">Seq out / in</b> = last-observed sender / target sequence numbers · <b class="muted">Disconnect</b> force-terminates the session · <b class="muted">Reset seq</b> resets sequence numbers to 1.</p>
   </section>

   <section class="view" id="view-trade">
    <div class="benchframe tt-frame"><iframe id="tradeIframe" title="Trade terminal"></iframe></div>
   </section>

   <section class="view" id="view-sim">
    <div class="simbar">
     <div class="row">
      <span class="muted">Bots</span><span id="botState" class="chip">—</span>
      <button class="btn sm" onclick="sim('bots_pause')">Pause</button>
      <button class="btn sm" onclick="sim('bots_resume')">Resume</button>
      <span style="width:18px"></span>
      <span class="muted">Phase</span><span id="phaseState" class="chip">—</span>
      <button class="btn sm" onclick="sim('preopen')">Pre-Open</button>
      <button class="btn sm acc" onclick="sim('open')">Open</button>
      <span style="flex:1"></span>
      <button class="btn sm acc" onclick="goFull('simFrame')">⛶ Full screen</button>
      <button class="btn sm ghost" onclick="reloadBench()">↻ Reload</button>
      <a class="btn sm ghost" href="/bench" target="_blank">New tab ↗</a>
     </div>
    </div>
    <div class="benchframe" id="simFrame"><iframe id="benchIframe" title="Simulator benchmark"></iframe></div>
   </section>

   <section class="view" id="view-creds">
    <h2>Register a broker</h2>
    <div class="panel pad">
     <p class="note">Register a broker firm with linked FIX (order-entry) and ITCH (market-data) credentials. Leave a password blank to auto-generate. Passwords are shown once at issue — after that, open a broker's detail to reveal and re-send them.</p>
     <div class="grid2">
      <div class="field"><label class="fl">Company / Firm name *</label><input class="f" id="c_company" placeholder="e.g. Alpha Equities Ltd"></div>
      <div class="field"><label class="fl">SenderCompID (FIX 49) *</label><input class="f" id="c_sender" placeholder="e.g. ALPHAEQUITIE"></div>
      <div class="field"><label class="fl">Technical contact</label><input class="f" id="c_contact" placeholder="email / name (optional)"></div>
      <div class="field"><label class="fl">FIX password (tag 554)</label><input class="f" id="c_fixpass" placeholder="blank = auto-generate"></div>
      <div class="field"><label class="fl">ITCH username (6 chars)</label><input class="f" id="c_itchuser" placeholder="blank = from SenderCompID"></div>
      <div class="field"><label class="fl">ITCH password</label><input class="f" id="c_itchpass" placeholder="blank = auto-generate"></div>
     </div>
     <button class="btn acc" onclick="addBroker()">Create credentials</button>
     <div class="reveal" id="reveal">
      <div class="hd">✓ Credentials issued — copy them now</div>
      <div class="kv" id="revealKV"></div>
      <button class="btn sm" onclick="sheetFrom(window._last)">Open onboarding sheet ↗</button>
     </div>
    </div>
    <h2>Registered brokers <span id="brokerCount" class="muted" style="font-weight:400;font-size:11px;text-transform:none;letter-spacing:0"></span></h2>
    <div class="filterbar"><input class="fi" id="f_broker" placeholder="Search company, SenderCompID or ITCH user…" style="min-width:320px" oninput="applyFilters()"></div>
    <div class="panel"><table><thead><tr><th>Company</th><th>SenderCompID</th><th>ITCH user</th><th>Created</th><th class="right">Actions</th></tr></thead><tbody id="brokerList"></tbody></table></div>
   </section>

   <section class="view" id="view-instruments">
    <div class="kpis">
     <div class="kpi"><div class="l" style="margin:0 0 8px">Listed instruments</div><div class="v" id="ik_total">&ndash;</div><div class="sub">across all boards</div></div>
     <div class="kpi grn"><div class="l" style="margin:0 0 8px">Equity &mdash; main board</div><div class="v" id="ik_main">&ndash;</div><div class="sub"><span class="pill active"><span class="d"></span>tradable</span></div></div>
     <div class="kpi blue"><div class="l" style="margin:0 0 8px">Bonds &amp; Sukuk</div><div class="v" id="ik_bonds">&ndash;</div><div class="sub">yield based</div></div>
     <div class="kpi amber"><div class="l" style="margin:0 0 8px">Pending listing</div><div class="v" id="ik_pending">&ndash;</div><div class="sub"><span class="pill pending"><span class="d"></span>awaiting approval</span></div></div>
    </div>
    <div class="in-grid">
     <div class="panel">
      <div class="card-hd">
       <h3>Instruments</h3><span class="muted" style="font-size:12px" id="in_count"></span><span class="sp"></span>
       <label class="in-search"><svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg><input id="f_instr" placeholder="Search symbol or name&hellip;" oninput="renderInstr()"></label>
       <div class="exp"><button class="btn ghost" onclick="toggleExport(event)"><svg viewBox="0 0 24 24"><path d="M12 3v12M7 10l5 5 5-5M5 21h14"/></svg>Export</button>
        <div class="exp-menu" id="exp_menu">
         <button onclick="exportInstr('xml')"><b>XML</b><span>DSE end-of-day ticker file</span></button>
         <button onclick="exportInstr('csv')"><b>CSV</b><span>Spreadsheet</span></button>
         <button onclick="exportInstr('json')"><b>JSON</b><span>Re-importable master</span></button>
         <div class="exp-note" id="exp_note"></div>
        </div></div>
       <button class="btn acc" onclick="openInstr()"><svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>List instrument</button>
      </div>
      <div class="in-chips" id="in_chips"></div>
      <div style="overflow-x:auto"><table><thead><tr><th>Symbol</th><th>Name</th><th>Class</th><th>Board</th><th>Sector</th><th class="right">Lot</th><th class="right">Tick &#2547;</th><th>Status</th><th class="right"></th></tr></thead><tbody id="instrList"></tbody></table></div>
     </div>
     <div class="panel">
      <div class="card-hd" style="padding:14px 16px"><svg viewBox="0 0 24 24" style="width:16px;height:16px;stroke:var(--mut);fill:none;stroke-width:2"><path d="m12 2 10 5-10 5L2 7z"/><path d="m2 17 10 5 10-5M2 12l10 5 10-5"/></svg><h3>Asset Classes</h3></div>
      <div id="in_classes"></div>
      <div class="muted" style="font-size:11.5px;padding:11px 16px;border-top:1px solid var(--line);line-height:1.5">Listed scrips get an order book, ITCH id and market-making bots immediately and show in the Trade Terminal. Changes are saved to the instrument master file.</div>
     </div>
    </div>
    <div class="modal" id="instModal" onclick="if(event.target==this)closeInstr()">
     <div class="mcard" style="max-width:540px">
      <div class="mh">
       <div class="modal-ico"><svg viewBox="0 0 24 24"><path d="m12 2 10 5-10 5L2 7z"/><path d="m2 17 10 5 10-5M2 12l10 5 10-5"/></svg></div>
       <div><h3 id="in_title">List instrument</h3><p class="msub">A listed security on the exchange</p></div>
       <button class="close" onclick="closeInstr()">&#10005;</button>
      </div>
      <div style="height:18px"></div>
      <div class="grid2">
       <div class="field"><label class="fl">Symbol *</label><input class="f mono" id="in_symbol" placeholder="e.g. GP" style="text-transform:uppercase"></div>
       <div class="field"><label class="fl">Status</label><select class="f" id="in_status"><option value="active">Active</option><option value="halted">Halted</option><option value="pending">Pending</option></select></div>
      </div>
      <div class="field"><label class="fl">Name *</label><input class="f" id="in_name" placeholder="e.g. Grameenphone Ltd."></div>
      <div class="grid2">
       <div class="field"><label class="fl">Asset class</label><select class="f" id="in_class"></select></div>
       <div class="field"><label class="fl">Board</label><select class="f" id="in_board"></select></div>
      </div>
      <div class="grid2">
       <div class="field"><label class="fl">Sector</label><select class="f" id="in_sector"></select></div>
       <div class="field"><label class="fl">Market lot</label><input class="f mono" id="in_lot" type="number" min="1" step="1" value="1"></div>
      </div>
      <div class="grid2">
       <div class="field"><label class="fl">Tick size &#2547;</label><input class="f mono" id="in_tick" type="number" min="0" step="0.01" value="0.10"></div>
       <div class="field"><label class="fl">Previous close &#2547; *</label><input class="f mono" id="in_prev" type="number" min="0" step="0.1" placeholder="e.g. 320.50"></div>
      </div>
      <div class="field"><label class="fl">ISIN</label><input class="f mono" id="in_isin" placeholder="optional, e.g. BD0001GP0007" style="text-transform:uppercase"></div>
      <div class="mft"><button class="btn ghost" onclick="closeInstr()">Cancel</button><button class="btn acc" id="in_save" onclick="saveInstr()">Save</button></div>
     </div>
    </div>
   </section>

   <section class="view" id="view-access">
    <h2>Add an allowed OMS IP</h2>
    <div class="panel pad">
     <p class="note">The external plain-FIX gateway (:5443) accepts connections only from these source IPs. Add each OMS partner's public egress IP. Changes take effect immediately — no restart. This is the network gate; every session still authenticates with its SenderCompID + tag-554 password. (An empty list = open to all.)</p>
     <div class="grid2">
      <div class="field"><label class="fl">Source IP *</label><input class="f" id="a_ip" placeholder="e.g. 203.202.247.121"></div>
      <div class="field"><label class="fl">Label (broker / partner)</label><input class="f" id="a_label" placeholder="e.g. BDTASKOMS (old OMS)"></div>
     </div>
     <button class="btn acc" onclick="addAllow()">Add to allowlist</button>
    </div>
    <h2>Allowed IPs <span id="allowCount" class="muted" style="font-weight:400;font-size:11px;text-transform:none;letter-spacing:0"></span></h2>
    <div class="panel"><table><thead><tr><th>Source IP</th><th>Label</th><th class="right">Actions</th></tr></thead><tbody id="allowList"></tbody></table></div>
   </section>

   <section class="view" id="view-msglog">
    <div class="simbar">
     <div class="row">
      <input class="f" id="ml_broker" placeholder="broker (SenderCompID)" style="max-width:220px" onkeydown="if(event.key=='Enter')loadMsgLog()">
      <input class="f" id="ml_clordid" placeholder="ClOrdID contains…" style="max-width:220px" onkeydown="if(event.key=='Enter')loadMsgLog()">
      <button class="btn sm acc" onclick="loadMsgLog()">Search</button>
      <button class="btn sm ghost" onclick="document.getElementById('ml_broker').value='';document.getElementById('ml_clordid').value='';loadMsgLog()">Clear</button>
      <span style="flex:1"></span>
      <span class="muted" id="ml_count"></span>
     </div>
    </div>
    <div class="panel" style="overflow-x:auto"><table><thead><tr><th>Time</th><th>Dir</th><th>Broker</th><th>Type</th><th>ClOrdID</th><th>Message</th></tr></thead><tbody id="msglog"></tbody></table></div>
    <p class="note" style="margin-top:12px">Every FIX message in/out of the engine (heartbeats excluded). Also written to <code>fix_messages.log</code> on the ME. Newest first.</p>
   </section>

   <section class="view" id="view-diag">
    <h2>Authentication</h2>
    <div class="panel"><table><tbody id="authState"></tbody></table></div>
    <h2>Session event log</h2>
    <div class="filterbar"><input class="fi" id="f_ev" placeholder="Filter events (broker, LOGON/LOGOUT/REJECT, detail)…" style="min-width:320px" oninput="applyFilters()"><span class="cnt" id="ev_cnt"></span></div>
    <div class="panel"><table><thead><tr><th>Time</th><th>Broker</th><th>Event</th><th>Detail</th></tr></thead><tbody id="events"></tbody></table></div>
    <h2>Credential audit trail</h2>
    <div class="filterbar"><input class="fi" id="f_audit" placeholder="Filter audit (actor, ISSUE/REVOKE, target)…" style="min-width:320px" oninput="applyFilters()"><span class="cnt" id="audit_cnt"></span></div>
    <div class="panel"><table><thead><tr><th>Time</th><th>Actor (IP)</th><th>Action</th><th>Target</th></tr></thead><tbody id="audit"></tbody></table></div>
   </section>

  </div>
 </main>
</div>
<div class="modal" id="modal"><div class="mcard" id="mcard"></div></div>
<form id="sheetForm" method="post" action="/admin/api/broker/sheet" target="_blank" style="display:none">
 <input type="hidden" name="company"><input type="hidden" name="sender"><input type="hidden" name="fix_pass"><input type="hidden" name="itch_user"><input type="hidden" name="itch_pass">
</form>
<script>
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c]})}
function j(p,o){return fetch("/admin/api/"+p,o).then(function(r){return r.json()})}
var titles={dash:["Dashboard","Live engine status"],trade:["Trade Terminal","Live order book, trade tape &amp; order entry"],sessions:["FIX Sessions","Connected brokers &amp; controls"],sim:["Simulator","Benchmark &amp; live order generation"],creds:["Broker Credentials","Issue &amp; manage FIX/ITCH access"],instruments:["Instruments","List &amp; manage tradeable securities (DSE board, tick, previous close)"],access:["External OMS Access","Source-IP allowlist for the plain-FIX gateway"],msglog:["Message Log","FIX message trace — every order in &amp; out"],diag:["Diagnostics","Auth state &amp; event log"]};
function show(v){
 document.querySelectorAll(".navitem").forEach(function(n){n.classList.toggle("on",n.dataset.v==v)});
 document.querySelectorAll(".view").forEach(function(s){s.classList.remove("on")});
 document.getElementById("view-"+v).classList.add("on");
 document.getElementById("pageTitle").innerHTML=titles[v][0];document.getElementById("pageSub").innerHTML=titles[v][1];
 document.body.classList.toggle("iframeview",v=="sim"||v=="trade");
 if(v=="sim"){var f=document.getElementById("benchIframe");if(!f.getAttribute("src"))f.src="/bench"}
 if(v=="trade"){var t=document.getElementById("tradeIframe");if(!t.getAttribute("src"))t.src="/?embed=1"}
 if(v=="msglog"){loadMsgLog()}
 if(v=="access"){loadAllow()}
 if(v=="instruments"){loadInstr()}
}
function goFull(id){var el=document.getElementById(id);if(el.requestFullscreen)el.requestFullscreen();else if(el.webkitRequestFullscreen)el.webkitRequestFullscreen()}
function msgTypeName(t){var n={D:"NewOrder","8":"ExecReport",F:"Cancel",G:"Replace","3":"Reject","9":"CxlReject",A:"Logon","5":"Logout","2":"ResendReq","4":"SeqReset",j:"BizReject"};return (n[t]||"msg")+" ("+t+")"}
function loadMsgLog(){
 var b=document.getElementById("ml_broker").value.trim(),c=document.getElementById("ml_clordid").value.trim();
 j("fixlog?limit=500&broker="+encodeURIComponent(b)+"&clordid="+encodeURIComponent(c)).then(function(d){
  var rows="",n=0;(d.messages||[]).forEach(function(m){n++;var t=(m.time||"").replace("T"," ").slice(11,23);
   rows+="<tr><td class=mono style='white-space:nowrap'>"+esc(t)+"</td><td><span class='chip "+(m.dir=="IN"?"LOGON":"LOGOUT")+"'>"+esc(m.dir)+"</span></td><td class=mono>"+esc(m.session)+"</td><td class=mono style='white-space:nowrap'>"+esc(msgTypeName(m.msg_type))+"</td><td class=mono>"+esc(m.clordid)+"</td><td class='mono muted' style='font-size:11px;word-break:break-all;min-width:280px'>"+esc(m.raw)+"</td></tr>"});
  document.getElementById("msglog").innerHTML=rows||"<tr><td colspan=6 class='pad16 muted'>No messages.</td></tr>";
  document.getElementById("ml_count").textContent=n+" messages";
 });
}
function reloadBench(){document.getElementById("benchIframe").src="/bench"}
window.addEventListener("message",function(e){
 if(e.origin!==location.origin||!e.data||e.data.type!=="tt-height")return;
 var f=document.getElementById("tradeIframe");if(f)f.style.height=Math.max(480,Math.ceil(e.data.h)+4)+"px"});
function reloadTrade(){document.getElementById("tradeIframe").src="/?embed=1&t="+Date.now()}
var aside=document.querySelector("aside"),bd=document.getElementById("backdrop");
function toggleNav(open){aside.classList.toggle("open",open);bd.classList.toggle("on",open)}
document.getElementById("hamb").onclick=function(){toggleNav(!aside.classList.contains("open"))};
bd.onclick=function(){toggleNav(false)};
document.querySelectorAll(".navitem").forEach(function(n){n.onclick=function(){show(n.dataset.v);toggleNav(false)}});
function flt(id){var e=document.getElementById(id);return e?e.value.trim().toLowerCase():""}
function hit(txt,f){return !f||String(txt).toLowerCase().indexOf(f)>=0}
function setc(id,t){var e=document.getElementById(id);if(e)e.textContent=t}
var _s=null,_b=null;
function refresh(){j("status").then(function(d){_s=d;renderStatus(d)})}
function renderStatus(d){
  if(!d)return;
  document.getElementById("up").textContent="uptime "+d.uptime;
  document.getElementById("enText").textContent=d.running?"engine live":"engine halted";
  function kpi(v,l,cls){return "<div class='kpi "+(cls||"")+"'><div class='v'>"+v+"</div><div class=l>"+l+"</div></div>"}
  var k="";
  k+=kpi("<span class='"+(d.running?"grn-t":"amber-t")+"'>"+(d.running?"LIVE":"HALT")+"</span>","engine","grn");
  k+=kpi(esc(d.phase),"session phase","blue");k+=kpi(d.instruments,"instruments");k+=kpi(d.orders,"orders");
  k+=kpi(d.trades,"trades matched","grn");k+=kpi((d.sessions||[]).length,"live sessions","amber");
  document.getElementById("kpis").innerHTML=k;
  document.getElementById("navSess").textContent=(d.sessions||[]).length||"";
  var L=d.listeners||{},lr="";Object.keys(L).sort().forEach(function(n){
   lr+="<tr><td class=mono>"+esc(n)+"</td><td class=right><span class='chip "+(L[n]?"up":"down")+"'><span class=d></span>"+(L[n]?"UP":"DOWN")+"</span></td></tr>"});
  document.getElementById("listeners").innerHTML=lr;
  var fs=flt("f_sess"),ss="",sc=0;(d.sessions||[]).forEach(function(s){if(!hit(s.broker,fs))return;sc++;
   ss+="<tr><td class=mono>"+esc(s.broker)+"</td><td class='mono muted'>"+esc(s.since)+"</td><td class=mono>"+(s.seq_sender||0)+" / "+(s.seq_target||0)+"</td><td class=right>"+
     "<button class='btn sm ghost' onclick=\"sess('"+esc(s.broker)+"','reset')\">Reset seq</button> <button class='btn sm danger' onclick=\"sess('"+esc(s.broker)+"','disconnect')\">Disconnect</button></td></tr>"});
  document.getElementById("sessions").innerHTML=ss||"<tr><td colspan=4 class='pad16 muted'>No matching sessions.</td></tr>";
  setc("sess_cnt",sc+" of "+(d.sessions||[]).length);
  document.getElementById("botState").className="chip "+(d.bots_paused?"down":"up");
  document.getElementById("botState").innerHTML="<span class=d></span>"+(d.bots_paused?"PAUSED":"RUNNING");
  document.getElementById("phaseState").className="chip up";document.getElementById("phaseState").innerHTML="<span class=d></span>"+esc(d.phase);
  document.getElementById("authState").innerHTML=
    "<tr><td>FIX logon authentication</td><td class=right><span class='chip "+(d.fix_auth?"up":"down")+"'><span class=d></span>"+(d.fix_auth?"ENFORCED":"OPEN")+"</span></td></tr>"+
    "<tr><td>ITCH feed authentication</td><td class=right><span class='chip "+(d.itch_auth?"up":"down")+"'><span class=d></span>"+(d.itch_auth?"ENFORCED":"OPEN")+"</span></td></tr>";
  var fe=flt("f_ev"),fd=flt("f_dash"),ev="",ec=0,mini="",mc=0;(d.events||[]).forEach(function(e){var t=(e.time||"").replace("T"," ").slice(0,19);
   var row="<td class=mono>"+esc(t)+"</td><td class=mono>"+esc(e.broker)+"</td><td><span class='chip "+esc(e.kind)+"'>"+esc(e.kind)+"</span></td>";
   if(hit(e.broker+" "+e.kind+" "+e.detail,fe)){ev+="<tr>"+row+"<td class=muted>"+esc(e.detail)+"</td></tr>";ec++}
   if(hit(e.broker+" "+e.kind,fd)&&mc<14){mini+="<tr>"+row+"</tr>";mc++}});
  document.getElementById("events").innerHTML=ev||"<tr><td colspan=4 class='pad16 muted'>No matching events.</td></tr>";
  document.getElementById("miniEvents").innerHTML=mini||"<tr><td colspan=3 class='pad16 muted'>No activity.</td></tr>";
  setc("ev_cnt",ec+" events");
  var fa=flt("f_audit"),ad="",ac=0;(d.audit||[]).forEach(function(a){if(!hit(a.actor+" "+a.action+" "+a.target,fa))return;ac++;var t=(a.time||"").replace("T"," ").slice(0,19);
   var cls=a.action.indexOf("ISSUE")==0?"LOGON":(a.action.indexOf("REVOKE")==0?"REJECT":"LOGOUT");
   ad+="<tr><td class=mono>"+esc(t)+"</td><td class=mono>"+esc(a.actor)+"</td><td><span class='chip "+cls+"'>"+esc(a.action)+"</span></td><td class=muted>"+esc(a.target)+"</td></tr>"});
  document.getElementById("audit").innerHTML=ad||"<tr><td colspan=4 class='pad16 muted'>No matching records.</td></tr>";
  setc("audit_cnt",ac+" records");
}
function toast(msg,err,warn){
 var el=document.getElementById("toast");
 if(!el){el=document.createElement("div");el.id="toast";el.style.cssText="position:fixed;right:20px;bottom:20px;max-width:420px;padding:12px 16px;border-radius:8px;font-size:13px;line-height:1.4;z-index:9999;box-shadow:0 8px 24px rgba(0,0,0,.4);color:#fff;transition:opacity .2s";document.body.appendChild(el)}
 el.style.background=err?"#b4323c":(warn?"#a8741a":"#1f8f6e");
 el.textContent=msg+(warn?" — Warning: "+warn:"");el.style.opacity=1;
 clearTimeout(el._t);el._t=setTimeout(function(){el.style.opacity=0},warn||err?8000:3500)}
var _in=[],_cls=[],_board="",_class="";
var ASSET=[["EQ","Equity"],["MF","Mutual Fund"],["CB","Corporate Bond"],["DB","Debenture"]];
var ASSET_LABEL={};ASSET.forEach(function(x){ASSET_LABEL[x[0]]=x[1]});
var IB_GEAR='<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>';
var IB_POWER='<svg viewBox="0 0 24 24"><path d="M18.4 6.6a9 9 0 1 1-12.8 0M12 2v10"/></svg>';
var IB_X='<svg viewBox="0 0 24 24"><path d="M18 6 6 18M6 6l12 12"/></svg>';
var SECTORS=[["Bank","Bank"],["Cement","Cement"],["Ceramic","Ceramic"],["CorpBond","Corporate Bond"],["Debenture","Debenture"],["Engineering","Engineering"],["Financial In","Financial Institutions"],["FoodAllied","Food & Allied"],["FuelPower","Fuel & Power"],["Insurance","Insurance"],["IT","IT"],["Jute","Jute"],["Misc","Miscellaneous"],["MutFund","Mutual Fund"],["PaperPrint","Paper & Printing"],["PharmaChem","Pharmaceuticals & Chemicals"],["ServRealEst","Services & Real Estate"],["Tannery","Tannery"],["Telecom","Telecom"],["Textile","Textile"],["TravelLeisur","Travel & Leisure"]];
var SECTOR_LABEL={};SECTORS.forEach(function(x){SECTOR_LABEL[x[0]]=x[1]});
var BOARDS=["Main (A)","Main (B)","Main (N)","Main (Z)","Bond","Mutual Fund","SME / ATB"];
function loadInstr(){j("instruments").then(function(d){
 _in=d.rows||[];_cls=d.classes||[];var s=d.summary||{};
 setc("ik_total",(s.total||0).toLocaleString());setc("ik_main",(s.main_equity||0).toLocaleString());
 setc("ik_bonds",(s.bonds||0).toLocaleString());setc("ik_pending",(s.pending||0).toLocaleString());
 document.getElementById("in_classes").innerHTML=_cls.map(function(c){return "<div class=cls-row style='cursor:pointer"+(_class==c.code?";background:rgba(77,208,177,.08)":"")+"' onclick=\"setClass('"+esc(c.code)+"')\" title='Filter by "+esc(c.label)+"'><span class=d style='background:"+esc(c.color)+"'></span><span style='font-weight:500'>"+esc(c.label)+"</span><span class=c>"+c.count+" listed</span></div>"}).join("");
 renderInstr()})}
function renderInstr(){
 var seen={},boards=[];_in.forEach(function(r){if(!seen[r.board]){seen[r.board]=1;boards.push(r.board)}});
 boards.sort();if(_board&&!seen[_board])_board="";
 document.getElementById("in_chips").innerHTML=["All"].concat(boards).map(function(b){
  var on=(b=="All"&&!_board)||b==_board;
  return "<button class='btn sm "+(on?"acc":"ghost")+"' onclick=\"setBoard('"+(b=="All"?"":esc(b))+"')\">"+esc(b)+"</button>"}).join("");
 var f=flt("f_instr"),rows="",n=0;
 _in.forEach(function(r){
  if(_board&&r.board!=_board)return;
  if(_class&&r.asset_class!=_class)return;
  if(!hit(r.symbol+" "+r.name,f))return;n++;
  var st=r.status||"active",lbl=st.charAt(0).toUpperCase()+st.slice(1);
  var pw=st=="active"?"<button class='ib warn' title='Halt' onclick=\"haltInstr('"+esc(r.symbol)+"',true)\">"+IB_POWER+"</button>":"<button class='ib up' title='Activate' onclick=\"haltInstr('"+esc(r.symbol)+"',false)\">"+IB_POWER+"</button>";
  rows+="<tr><td><span class=code-tag>"+esc(r.symbol)+"</span></td><td style='font-weight:600'>"+esc(r.name)+"</td><td class=muted>"+esc(ASSET_LABEL[r.asset_class]||r.asset_class||"")+"</td><td class=muted>"+esc(r.board)+"</td><td class=muted>"+(r.sector?esc(SECTOR_LABEL[r.sector]||r.sector):"&mdash;")+"</td><td class='right mono'>"+r.lot+"</td><td class='right mono'>"+Number(r.tick||0).toFixed(2)+"</td><td><span class='pill "+st+"'><span class=d></span>"+lbl+"</span></td><td class=right style='white-space:nowrap'><button class=ib title='Edit' onclick=\"openInstr('"+esc(r.symbol)+"')\">"+IB_GEAR+"</button> "+pw+" <button class='ib del' title='Delist' onclick=\"delistInstr('"+esc(r.symbol)+"')\">"+IB_X+"</button></td></tr>"});
 document.getElementById("instrList").innerHTML=rows||"<tr><td colspan=9 style='text-align:center;padding:34px' class=muted>No instruments"+(f||_board||_class?" match the filter":"")+".</td></tr>";
 setc("in_count",n+(f||_board||_class?" match":" shown"));
}
function setBoard(b){_board=b;renderInstr()}
function toggleExport(e){e.stopPropagation();var m=document.getElementById("exp_menu");m.classList.toggle("on");
 if(m.classList.contains("on")){var n=_in.filter(function(r){return(!_board||r.board==_board)&&(!_class||r.asset_class==_class)&&hit(r.symbol+" "+r.name,flt("f_instr"))}).length;
  setc("exp_note","Exports the "+n+" instrument"+(n==1?"":"s")+" currently shown"+(_board||_class||flt("f_instr")?" (filters applied).":" (all)."))}}
function exportInstr(fmt){document.getElementById("exp_menu").classList.remove("on");
 var u="/admin/api/instruments/export?format="+fmt+"&board="+encodeURIComponent(_board)+"&class="+encodeURIComponent(_class)+"&q="+encodeURIComponent(document.getElementById("f_instr").value.trim());
 toast("Preparing "+fmt.toUpperCase()+" export…",false);window.location=u}
document.addEventListener("click",function(){var m=document.getElementById("exp_menu");if(m)m.classList.remove("on")});
function setClass(c){_class=_class==c?"":c;loadInstr()}
var _editing="";
function openInstr(sym){
 var r=sym?_in.filter(function(x){return x.symbol==sym})[0]:null;_editing=r?r.symbol:"";
 var bs=BOARDS.slice();if(r&&bs.indexOf(r.board)<0)bs.push(r.board);
 document.getElementById("in_board").innerHTML=bs.map(function(b){return "<option>"+esc(b)+"</option>"}).join("");
 var secs=SECTORS.slice();if(r&&r.sector&&!SECTOR_LABEL[r.sector])secs.push([r.sector,r.sector]);
 document.getElementById("in_sector").innerHTML="<option value=''>&mdash; Select sector &mdash;</option>"+secs.map(function(x){return "<option value=\""+esc(x[0])+"\">"+esc(x[1])+"</option>"}).join("");
 document.getElementById("in_class").innerHTML=ASSET.map(function(x){return "<option value=\""+x[0]+"\">"+esc(x[1])+"</option>"}).join("");
 var v=function(i,x){document.getElementById(i).value=x==null?"":x};
 v("in_symbol",r?r.symbol:"");document.getElementById("in_symbol").disabled=!!r;
 v("in_name",r?r.name:"");v("in_class",r?r.asset_class:"EQ");v("in_board",r?r.board:"Main (A)");v("in_sector",r?r.sector:"");
 v("in_lot",r?r.lot:1);v("in_tick",r?Number(r.tick).toFixed(2):"0.10");v("in_prev",r?r.prev:"");v("in_isin",r?r.isin:"");
 v("in_status",r?r.status:"active");
 setc("in_title",r?"Edit "+r.symbol:"List instrument");
 document.getElementById("instModal").classList.add("on");
 setTimeout(function(){document.getElementById(r?"in_name":"in_symbol").focus()},30)}
function closeInstr(){document.getElementById("instModal").classList.remove("on")}
function saveInstr(){
 var g=function(i){return document.getElementById(i).value.trim()};
 var body={Symbol:g("in_symbol"),Name:g("in_name"),AssetClass:g("in_class"),Board:g("in_board"),Sector:g("in_sector"),Status:g("in_status"),ISIN:g("in_isin"),
  Lot:parseInt(g("in_lot"),10)||1,Tick:parseFloat(g("in_tick"))||0,Prev:parseFloat(g("in_prev"))||0};
 if(!body.Symbol){toast("Symbol is required",true);return}
 if(!body.Name){toast("Name is required",true);return}
 var edit=!!_editing;
 j(edit?"instrument/update":"instrument/add",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)}).then(function(d){
  if(!d.ok){toast(d.error||"Failed",true);return}
  closeInstr();
  toast(edit?"Saved "+body.Symbol:"Listed "+body.Symbol+" — now live in the Trade Terminal",false,d.warning);
  loadInstr()})}
function haltInstr(sym,h){if(!confirm((h?"Halt trading on ":"Activate trading on ")+sym+"?"))return;
 j("instrument/halt",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({Symbol:sym,Halted:h})}).then(function(d){
  if(!d.ok)toast(d.error||"Failed",true);else toast(sym+(h?" halted":" activated"),false,d.warning);loadInstr()})}
function delistInstr(sym){if(!confirm("Delist "+sym+"? It stops trading and is removed from the directory."))return;
 j("instrument/delist",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({Symbol:sym})}).then(function(d){
  if(!d.ok)toast(d.error||"Failed",true);else toast(sym+" delisted",false,d.warning);loadInstr()})}
document.addEventListener("keydown",function(e){if(e.key=="Escape")closeInstr()});
function loadAllow(){j("allowlist").then(function(d){
 var rows="";(d.list||[]).forEach(function(r){
  rows+="<tr><td class=mono>"+esc(r.ip)+"</td><td>"+(r.label?esc(r.label):"<span class=muted>&mdash;</span>")+"</td><td class=right><button class='btn sm danger' onclick=\"delAllow('"+esc(r.ip)+"')\">Remove</button></td></tr>"});
 document.getElementById("allowList").innerHTML=rows||"<tr><td colspan=3 class='pad16 muted'>No IPs yet &mdash; the gateway is open to all. Add partner IPs to lock it down.</td></tr>";
 document.getElementById("allowCount").textContent="&middot; "+(d.count||0)+" allowed";
})}
function addAllow(){var ip=document.getElementById("a_ip").value.trim(),label=document.getElementById("a_label").value.trim();
 if(!ip){alert("Source IP is required");return}
 j("allowlist/add",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({IP:ip,Label:label})}).then(function(r){
  if(!r.ok){alert(r.error||"failed");return}
  document.getElementById("a_ip").value="";document.getElementById("a_label").value="";loadAllow()})}
function delAllow(ip){if(!confirm("Remove "+ip+" from the allowlist?"))return;
 j("allowlist/del",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({IP:ip})}).then(loadAllow)}
function loadBrokers(){j("brokers").then(function(b){_b=b;renderBrokers(b)})}
function renderBrokers(b){
  if(!b)return;
  var fb=flt("f_broker"),rows="",n=0;(b.brokers||[]).forEach(function(r){
   if(!hit((r.company||"")+" "+r.sender+" "+(r.itch_user||""),fb))return;n++;
   rows+="<tr><td>"+(r.company?esc(r.company):"<span class=muted>—</span>")+"</td><td class=mono>"+esc(r.sender)+"</td><td class='mono muted'>"+esc(r.itch_user)+"</td><td class='mono muted'>"+esc(r.created)+"</td><td class=right>"+
     "<button class='btn sm ghost' onclick=\"openDetail('"+esc(r.sender)+"')\">View</button> <button class='btn sm danger' onclick=\"del('"+esc(r.sender)+"')\">Revoke</button></td></tr>"});
  document.getElementById("brokerList").innerHTML=rows||"<tr><td colspan=5 class='pad16 muted'>No matching brokers.</td></tr>";
  document.getElementById("brokerCount").textContent="· "+n+" of "+(b.brokers||[]).length+" managed · "+b.fix_total+" FIX · "+b.itch_total+" ITCH";
}
function applyFilters(){renderStatus(_s);renderBrokers(_b)}
function sim(a){j("sim?action="+a,{method:"POST"}).then(refresh)}
function sess(b,a){if(a=="disconnect"&&!confirm("Disconnect "+b+"?"))return;
 j("session?broker="+encodeURIComponent(b)+"&action="+a,{method:"POST"}).then(function(r){if(!r.ok)alert(r.error||"failed");refresh()})}
function addBroker(){
 var body={Company:c_company.value,Sender:c_sender.value,Contact:c_contact.value,FixPass:c_fixpass.value,ItchUser:c_itchuser.value,ItchPass:c_itchpass.value};
 if(!body.Sender.trim()){alert("SenderCompID is required");return}
 j("broker/add",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)}).then(function(r){
  if(!r.ok){alert(r.error||"failed");return}
  window._last={company:r.company,sender:r.sender,fix_pass:r.fix_password,itch_user:r.itch_user,itch_pass:r.itch_password};
  function kv(k,v){return "<div class=k>"+k+"</div><div class=val>"+esc(v)+"</div>"}
  var ep=r.endpoints||{};
  document.getElementById("revealKV").innerHTML=(r.company?kv("Company",r.company):"")
    +kv("FIXS endpoint (external)",ep.pub_ip+" : "+ep.fixs_port+"  (TLS)")
    +kv("Host / SNI",ep.pub_host)
    +kv("Internal endpoint (on-platform)",ep.int_ip+" : "+ep.int_port)
    +kv("TargetCompID",ep.target)+kv("SenderCompID",r.sender)+kv("FIX password",r.fix_password)
    +kv("ITCH feed",ep.itch_ip+" : "+ep.itch_port)+kv("ITCH username",r.itch_user)+kv("ITCH password",r.itch_password);
  document.getElementById("reveal").className="reveal on";
  ["c_company","c_sender","c_contact","c_fixpass","c_itchuser","c_itchpass"].forEach(function(id){document.getElementById(id).value=""});
  loadBrokers();
 })}
function sheetFrom(o){if(!o)return;var f=document.getElementById("sheetForm");
 f.company.value=o.company||"";f.sender.value=o.sender||"";f.fix_pass.value=o.fix_pass||"";f.itch_user.value=o.itch_user||"";f.itch_pass.value=o.itch_pass||"";f.submit()}
function copyCell(btn){var cv=btn.parentNode.querySelector(".cv");if(cv&&navigator.clipboard){navigator.clipboard.writeText(cv.textContent);btn.textContent="copied";setTimeout(function(){btn.textContent="copy"},900)}}
function closeModal(){document.getElementById("modal").className="modal"}
function openDetail(sender){j("broker/detail?sender="+encodeURIComponent(sender)).then(function(d){
  window._detail={company:d.company,sender:d.sender,fix_pass:d.fix_password,itch_user:d.itch_user,itch_pass:d.itch_password};
  function cr(k,v,plain){return "<div class=cr><div class=ck>"+k+"</div><div class='cv"+(plain?" plain":"")+"'>"+esc(v)+"</div><button class=cpy onclick='copyCell(this)'>copy</button></div>"}
  var stat=(d.fix_active?"<span class='chip up'><span class=d></span>FIX ACTIVE</span>":"<span class='chip down'><span class=d></span>FIX OFF</span>")+" "+(d.itch_active?"<span class='chip up'><span class=d></span>ITCH ACTIVE</span>":"<span class='chip down'><span class=d></span>ITCH OFF</span>");
  var h="<div class=mh><div><h3>"+(d.company?esc(d.company):esc(d.sender))+"</h3><p class=msub>SenderCompID "+esc(d.sender)+(d.created?" · issued "+esc(d.created):"")+(d.contact?" · "+esc(d.contact):"")+"</p></div><button class=close onclick='closeModal()'>Close</button></div>";
  h+="<div class=pillrow>"+stat+"</div>";
  var ep=d.endpoints||{};
  h+="<div class=credbox>"+cr("FIXS endpoint (external)",ep.pub_ip+" : "+ep.fixs_port+"  (TLS)",1)+cr("Host / SNI",ep.pub_host,1)+cr("Internal endpoint (on-platform)",ep.int_ip+" : "+ep.int_port,1)+cr("TargetCompID",ep.target,1)+cr("SenderCompID",d.sender)+cr("FIX password",d.fix_password||"(not set)")+cr("ITCH feed",ep.itch_ip+" : "+ep.itch_port,1)+cr("ITCH username",d.itch_user||"—")+cr("ITCH password",d.itch_password||"(not set)")+"</div>";
  h+="<h2 style='margin-top:18px'>How to connect</h2><div class=guide>";
  h+="<div class=st><span class=n>1</span><div>Open a <b>TLS</b> connection to <code>"+esc(ep.pub_ip+":"+ep.fixs_port)+"</code> (set <code>SocketUseSSL=Y</code>). Connect to the <b>origin IP</b> — the <code>"+esc(ep.pub_host)+"</code> hostname is Cloudflare-proxied and does not pass this port. On-platform brokers instead point their bm_itch to the internal endpoint <code>"+esc(ep.int_ip+":"+ep.int_port)+"</code>.</div></div>";
  h+="<div class=st><span class=n>2</span><div>Send <b>Logon (35=A)</b> with your SenderCompID, <code>TargetCompID=DSEME</code>, Username (553) + Password (554).</div></div>";
  h+="<div class=st><span class=n>3</span><div>Submit <b>NewOrderSingle (35=D)</b>; receive <b>ExecutionReport (35=8)</b> per state change.</div></div>";
  h+="<div class=st><span class=n>4</span><div>Subscribe to market data on the <b>ITCH</b> feed using the username/password above.</div></div>";
  h+="<div class=tagrow style='margin-top:12px'><div>OrdType (40): <code>1</code> Mkt <code>2</code> Lmt <code>3</code> Stop <code>4</code> StopLmt</div><div>Iceberg: <code>111</code> MaxFloor</div><div>TIF (59): <code>0</code> Day <code>1</code> GTC <code>3</code> IOC <code>4</code> FOK <code>6</code> GTD</div><div>Cancel / Amend: <code>35=F</code> / <code>35=G</code></div></div></div>";
  h+="<div class=mactions><button class='btn acc' onclick='sheetFrom(window._detail)'>Open onboarding sheet ↗</button><button class='btn danger' onclick=\"closeModal();del('"+esc(d.sender)+"')\">Revoke broker</button></div>";
  document.getElementById("mcard").innerHTML=h;document.getElementById("modal").className="modal on";
 })}
function del(sender){if(!confirm("Revoke ALL credentials for "+sender+"?"))return;
 j("broker/del",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({Sender:sender})}).then(function(){closeModal();loadBrokers()})}
document.getElementById("modal").onclick=function(e){if(e.target.id=="modal")closeModal()};
refresh();loadBrokers();setInterval(refresh,3000);
</script></body></html>`

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
	txt := "VELEXA MATCHING ENGINE — FIX / FIXS + ITCH CONNECTIVITY\n" +
		"Broker: " + title + "  (SenderCompID " + sender + ")\n" +
		"Issued: " + issued + "   ·   Confidential to the receiving broker\n" +
		"TargetCompID: " + ep["target"] + "   BeginString: FIXT.1.1   ApplVerID: 9 (FIX.5.0 SP2)\n\n" +
		"SESSION ENDPOINT\n" +
		"  " + pad("FIXS external (TLS):") + ep["pub_ip"] + ":" + ep["fixs_port"] + "\n" +
		"  " + pad("Host / SNI:") + ep["pub_host"] + "\n" +
		"  " + pad("Internal (on-platform):") + ep["int_ip"] + ":" + ep["int_port"] + "\n\n" +
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
<title>Connectivity — ` + e(title) + `</title>` + adminFontLink + `
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
      <span class="pill">FIXT.1.1 · FIX 5.0 SP2</span>
    </div>
  </div>

  <div class="body">
    <h2>Session endpoint</h2>
    <div class="card">` +
		crow("FIXS — external (TLS)", ep["pub_ip"]+" : "+ep["fixs_port"], false) +
		crow("Host / SNI", ep["pub_host"], false) +
		crow("Internal — on-platform", ep["int_ip"]+" : "+ep["int_port"], false) +
		crow("BeginString (8)", "FIXT.1.1", false) +
		crow("ApplVerID (1128)", "9  (FIX.5.0 SP2)", false) +
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
		step("1", `Open a <b>TLS</b> connection to <code>`+e(ep["pub_ip"]+":"+ep["fixs_port"])+`</code> (set <code>SocketUseSSL=Y</code>). Use the <b>origin IP</b> — <code>`+e(ep["pub_host"])+`</code> is Cloudflare-proxied and will not pass this port. On-platform brokers use the internal endpoint <code>`+e(ep["int_ip"]+":"+ep["int_port"])+`</code> instead.`) +
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
