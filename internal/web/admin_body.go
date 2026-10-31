package web

// adminBody is the console's markup: the shell (sidebar, header) and one
// section per page; admin_script.go fills them in.
const adminBody = `<div class="app">
 <aside>
  <div class="brand"><span class="logo">M</span><div><b>Matching Engine</b><small>DSEME · velexa</small></div></div>
  <nav>
   <div class="navgrp">Overview</div>
   <div class="navitem" data-v="home"><svg viewBox="0 0 24 24"><path d="M4 11l8-7 8 7M6 9.5V20h12V9.5M10 20v-6h4v6"/></svg><span>Welcome</span></div>
   <div class="navitem" data-v="dash"><svg viewBox="0 0 24 24"><path d="M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z"/></svg><span>Dashboard</span></div>
   <div class="navgrp">Trading</div>
   <div class="navitem" data-v="trade"><svg viewBox="0 0 24 24"><path d="M4 4v16h16M8 15l3.5-4 3 3L20 8"/></svg><span>Trade Terminal</span></div>
   <div class="navitem" data-v="orders"><svg viewBox="0 0 24 24"><path d="M7 3h8l4 4v14H7zM14 3v5h5M10 12h6M10 16h6"/></svg><span>Order Report</span></div>
   <div class="navitem" data-v="sessions"><svg viewBox="0 0 24 24"><path d="M3 12h4l3-7 4 14 3-7h4"/></svg><span>FIX Sessions</span><span class="badge" id="navSess"></span></div>
   <div class="navitem" data-v="market" data-admin-only><svg viewBox="0 0 24 24"><path d="M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0z"/></svg><span>Market Session</span></div>
   <div class="navitem" data-v="instruments" data-admin-only><svg viewBox="0 0 24 24"><path d="M5 20V11M10 20V5M15 20v-8M20 20v-5M3 20h18"/></svg><span>Instruments</span></div>
   <div class="navitem" data-v="sim" data-admin-only><svg viewBox="0 0 24 24"><path d="M9 3h6M10 3v6l-5 9a2 2 0 0 0 1.7 3h10.6a2 2 0 0 0 1.7-3l-5-9V3M7.5 14h9"/></svg><span>Simulator</span></div>
   <div class="navgrp">Access</div>
   <div class="navitem" data-v="creds"><svg viewBox="0 0 24 24"><path d="M14.5 9.5a4.5 4.5 0 1 0-1.3 3.2L20 19.5M17 16.5l2-2M15 14.5l1.5-1.5"/></svg><span>Broker Credentials</span></div>
   <div class="navitem" data-v="access" data-admin-only><svg viewBox="0 0 24 24"><path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/></svg><span>OMS IP Allowlist</span></div>
   <div class="navgrp" data-admin-only>Administration</div>
   <div class="navitem" data-v="users" data-admin-only><svg viewBox="0 0 24 24"><path d="M16 20v-1a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v1M9.5 11a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7zM21 20v-1a4 4 0 0 0-3-3.8M15.5 4.2a3.5 3.5 0 0 1 0 6.6"/></svg><span>Users</span></div>
   <div class="navgrp">System</div>
   <div class="navitem" data-v="msglog"><svg viewBox="0 0 24 24"><path d="M5 4h14v16H5zM9 9h6M9 13h6M9 17h3"/></svg><span>Message Log</span></div>
   <div class="navitem" data-v="diag" data-admin-only><svg viewBox="0 0 24 24"><path d="M12 4l9 16H3zM12 10v4M12 17v.5"/></svg><span>Diagnostics</span></div>
  </nav>
  <div class="sidefoot">
   <div class="row"><span class="dot" id="enDot"></span><span id="enText">engine live</span></div>
   <div class="row">Signed in as <b id="meName">&ndash;</b><span class="rolebadge" id="meRole"></span></div>
   <div class="btns"><button type="button" onclick="pwDlg()">Change password</button><form method="post" action="/admin/logout"><button type="submit">Sign out</button></form></div>
  </div>
 </aside>
 <div class="backdrop" id="backdrop"></div>
 <main>
  <div class="top">
   <div class="tl"><button class="hamb" id="hamb" aria-label="Menu"><svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 6h18M3 12h18M3 18h18"/></svg></button>
    <div><h1 id="pageTitle">Welcome</h1><div class="sub" id="pageSub">Your engine at a glance</div></div></div>
   <div class="tr"><span class="live" title=""><span class="dot"></span><span id="up">&mdash;</span></span><button class="btn" id="themeBtn" type="button" style="min-width:64px">Dark</button></div>
  </div>
  <div class="wrap">

   <section class="view on" id="view-home">
    <div class="wl">
     <div class="wl-hero">
      <div class="wl-hi"><span class="wl-date" id="wl_date"></span><h2 id="wl_greet">Welcome</h2><span class="wl-sum" id="wl_sum"></span></div>
      <div class="wl-phase"><i id="wl_pdot"></i><div><b id="wl_phase">&mdash;</b><span id="wl_clock"></span></div></div>
     </div>
     <div class="wl-cards" id="wl_cards"></div>
     <div class="wl-row">
      <div class="wl-attn"><div class="wl-hd"><span class="wl-cap">Needs attention</span><span class="wl-cnt" id="wl_acnt"></span></div><div id="wl_attn"></div></div>
      <div class="wl-side">
       <div class="wl-box"><span class="wl-cap">Quick actions</span><div class="wl-acts" id="wl_acts"></div></div>
       <div class="wl-box wl-recent"><div class="wl-hd"><span class="wl-cap">Recent activity</span><button class="wl-link" id="wl_audit" type="button" onclick="show('diag')">Audit log</button></div><div id="wl_recent"></div></div>
      </div>
     </div>
    </div>
   </section>

   <section class="view" id="view-dash">
    <div class="db">
     <div class="tbar"><span class="cap" id="ov_cap">Today · whole exchange · live</span>
      <div class="tb noprint"><input type="date" class="in" id="ov_date" onchange="ovLoad()" aria-label="Day" title="Show another day"><button class="btn" id="ov_today" onclick="ovToday()" style="display:none">Today</button><button class="btn" id="ov_pause" onclick="ovPause()">Pause live</button><button class="btn" onclick="repCSV()">Download CSV</button><button class="btn" onclick="repPrint()">Print</button></div>
     </div>
     <section class="dbc p sess">
      <div class="hd">
       <div style="display:flex;align-items:baseline;gap:12px;flex-wrap:wrap"><span class="cap">Market session</span><span class="ph" id="ov_phase">&mdash;</span><span class="dbm muted" id="ov_clock"></span></div>
       <span class="muted" id="ov_note"></span>
      </div>
      <div style="display:flex;flex-direction:column;gap:6px">
       <div class="tl" id="ov_tl"></div>
       <div class="tlx" id="ov_tlx"></div>
      </div>
      <div class="legend" id="ov_legend"></div>
     </section>

     <section class="dbk">
      <div><span class="l">Turnover</span><span class="v" id="k_to">&mdash;</span><div class="s"><span class="dbm" id="k_to_s"></span><b id="k_to_d"></b></div></div>
      <div><span class="l">Volume (shares)</span><span class="v" id="k_vol">&mdash;</span><div class="s"><span id="k_vol_s"></span><b id="k_vol_d"></b></div></div>
      <div><span class="l">Trades executed</span><span class="v" id="k_tr">&mdash;</span><div class="s"><span id="k_tr_s"></span><b id="k_tr_d"></b></div></div>
      <div><span class="l">Orders received</span><span class="v" id="k_or">&mdash;</span><div class="s"><span id="k_or_s"></span><b id="k_or_d"></b></div></div>
     </section>

     <section class="dbrow">
      <div class="dbc p w2" style="display:flex;flex-direction:column;gap:16px">
       <div style="display:flex;flex-wrap:wrap;justify-content:space-between;align-items:center;gap:10px">
        <div style="display:flex;flex-direction:column;gap:2px"><span class="cap" id="ch_cap">Intraday</span><span class="faint" style="font-size:13px" id="ch_peak"></span></div>
        <div class="seg" id="ch_seg"><button data-m="turnover" class="on">Turnover</button><button data-m="trades">Trades</button><button data-m="volume">Volume</button></div>
       </div>
       <div class="chart" id="ch_bars"></div>
       <div class="chx" id="ch_x"></div>
      </div>
      <div class="dbc p w1 oc" style="display:flex;flex-direction:column;gap:16px">
       <span class="cap">Order outcomes</span>
       <div style="display:flex;align-items:baseline;gap:10px"><span class="big" id="oc_rate">&mdash;</span><span class="muted">of orders filled</span></div>
       <div class="stack" id="oc_stack"></div>
       <div style="display:flex;flex-direction:column;gap:8px" id="oc_list"></div>
       <div style="border-top:1px solid var(--line);padding-top:14px;display:flex;flex-direction:column;gap:8px">
        <div style="display:flex;justify-content:space-between;font-size:13px"><span style="color:var(--acc);font-weight:500" id="oc_buy">Buy</span><span class="faint" id="oc_amend"></span><span style="color:var(--red);font-weight:500" id="oc_sell">Sell</span></div>
        <div class="bs"><div id="oc_buybar" style="background:var(--acc);width:50%"></div><div style="flex:1;background:var(--red)"></div></div>
       </div>
      </div>
     </section>

     <section class="dbrow">
      <div class="dbc t w2" style="display:flex;flex-direction:column;gap:10px">
       <div style="display:flex;justify-content:space-between;align-items:baseline;padding:0 20px"><span class="cap">Instruments</span><span class="faint" style="font-size:12px">As on the DSE share price page · change = LTP − YCP · click a column to sort</span></div>
       <div class="it"><div class="g h" id="it_h"></div><div id="it_rows"></div></div>
       <button class="dbmore" id="it_more" onclick="_ovAll=!_ovAll;ovRender()" style="display:none"></button>
      </div>
      <div class="dbc t w1" style="display:flex;flex-direction:column;gap:12px;padding-bottom:8px">
       <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;padding:0 20px;flex-wrap:wrap">
        <span class="cap">Recent activity</span>
        <div class="seg sm" id="fd_seg"><button data-f="all" class="on">All</button><button data-f="trade">Trades</button><button data-f="order">Orders</button><button data-f="session">Sessions</button></div>
       </div>
       <div id="fd_rows"></div>
      </div>
     </section>

     <section style="display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,420px),1fr));gap:12px">
      <div class="dbc t" style="display:flex;flex-direction:column;gap:10px"><span class="cap" style="padding:0 20px" id="br_cap">Brokers by turnover</span><div id="br_rows"></div></div>
      <div class="dbc t" style="display:flex;flex-direction:column;gap:10px">
       <div style="display:flex;justify-content:space-between;align-items:baseline;padding:0 20px"><span class="cap">Connectivity</span><span class="muted" style="font-size:13px"><b class="dbm" style="color:var(--ink)" id="cn_sess">0</b> live FIX sessions</span></div>
       <div id="cn_rows"></div>
      </div>
     </section>
     <p class="dbfoot" id="db_foot"></p>
    </div>
   </section>

   <section class="view" id="view-trade">
    <div class="row2">
     <div class="card o" style="flex:3 1 880px;min-width:0">
      <div class="tbar" style="padding:16px 18px 12px"><span class="cap">Market watch</span><span class="faint mono" style="font-size:12px" id="mw_count"></span></div>
      <div class="tb" style="padding:0 18px 12px">
       <select class="in" id="mw_board" onchange="mwRender(true)" aria-label="Board"><option value="">All boards</option></select>
       <div class="seg md" id="mw_class"><button type="button" data-c="" class="on">All</button><button type="button" data-c="EQ">Equity</button><button type="button" data-c="MF">Mutual fund</button><button type="button" data-c="BOND">Bonds</button></div>
       <select class="in" id="mw_sector" onchange="mwRender(true)" aria-label="Sector"><option value="">All sectors</option></select>
       <select class="in" id="mw_cat" onchange="mwRender(true)" aria-label="Category"><option value="">All categories</option></select>
       <select class="in" id="mw_status" onchange="mwRender(true)" aria-label="Status"><option value="">All statuses</option></select>
       <input class="in" id="mw_q" placeholder="Search symbol or name&hellip;" oninput="mwRender(true)" style="flex:1 1 170px">
      </div>
      <div class="tw"><table class="tbl tight mw"><thead><tr id="mw_head"></tr></thead><tbody id="mw_rows"></tbody></table></div>
      <div class="tbar" style="padding:10px 18px;font-size:13px;color:var(--mut)"><span id="mw_range"></span>
       <div class="tb"><span>Rows</span><select class="in" id="mw_per" onchange="mwRender(true)" style="padding:4px 6px"><option>25</option><option>50</option><option>100</option></select>
        <button class="btn sm" id="mw_prev" onclick="mwPage(-1)">&lsaquo; Prev</button><span class="mono" style="font-size:12px" id="mw_pg"></span><button class="btn sm" id="mw_next" onclick="mwPage(1)">Next &rsaquo;</button></div></div>
     </div>
     <div class="ttside">
      <div class="card book" style="padding:16px 0 8px">
       <div class="tbar" style="padding:0 16px 8px"><div style="display:flex;align-items:baseline;gap:10px"><span style="font-size:18px;font-weight:600" id="tt_sym">&mdash;</span><span class="mono" style="font-size:18px" id="tt_last"></span><span class="mono" style="font-size:13px" id="tt_chg"></span></div><span class="faint" style="font-size:12px" id="tt_spread"></span></div>
       <div class="hrow"><span>PRICE</span><span style="text-align:right">QTY</span><span style="text-align:right">ORDERS</span></div>
       <div id="tt_asks"></div>
       <div class="mid"><span>Last <b class="mono" style="color:var(--ink)" id="tt_last2">&mdash;</b></span><span id="tt_hl"></span><span id="tt_mid"></span></div>
       <div id="tt_bids"></div>
      </div>
      <div class="card ticket" style="display:flex;flex-direction:column;gap:14px;padding:16px">
       <span class="cap">New order &middot; <span id="tt_sym3"></span></span>
       <div class="sidesw"><button class="b on" id="tt_buy" type="button" onclick="ttSide('BUY')">Buy</button><button class="s" id="tt_sell" type="button" onclick="ttSide('SELL')">Sell</button></div>
       <div class="g2">
        <label class="fld">Order type<select class="in" id="tt_type" onchange="ttForm()"><option value="LIMIT">Limit</option><option value="MARKET">Market</option></select></label>
        <label class="fld">Time in force<select class="in" id="tt_tif"><option value="DAY">Day</option><option value="IOC">IOC</option><option value="FOK">FOK</option></select></label>
        <label class="fld">Quantity<input class="in mono" id="tt_qty" value="100" inputmode="numeric" oninput="ttForm()"></label>
        <label class="fld">Limit price<input class="in mono" id="tt_px" inputmode="decimal" oninput="ttForm()"></label>
       </div>
       <label class="fld">Placed as<input class="in" id="tt_as" disabled></label>
       <div style="display:flex;justify-content:space-between;font-size:13px;color:var(--mut);border-top:1px solid var(--line);padding-top:12px"><span>Estimated value</span><span class="mono" style="color:var(--ink)" id="tt_est">&mdash;</span></div>
       <button class="btn pri" id="tt_go" type="button" onclick="ttSubmit()" style="padding:11px;font-size:14px">Buy</button>
       <span id="tt_msg" style="font-size:13px"></span>
      </div>
      <div class="card tape" style="padding:16px 0 8px">
       <span class="cap" style="padding:0 16px">Time &amp; sales &middot; <span id="tt_sym2"></span></span>
       <div class="hrow" style="margin-top:8px"><span>TIME</span><span style="text-align:right">PRICE</span><span style="text-align:right">QTY</span></div>
       <div id="tt_tape"></div>
      </div>
     </div>
    </div>
    <div class="card t">
     <div class="hd"><span class="cap">Orders placed from this console</span><span class="faint" style="font-size:12px" id="tt_mine_n"></span></div>
     <div class="tw"><table class="tbl"><thead><tr><th>Order</th><th>Time</th><th>Side</th><th>Symbol</th><th class="r">Qty</th><th class="r">Filled</th><th class="r">Price</th><th>TIF</th><th>Status</th><th></th></tr></thead><tbody id="tt_mine"></tbody></table></div>
    </div>
   </section>

   <section class="view" id="view-orders">
    <div class="tbar">
     <div class="tb">
      <input type="date" class="in" id="or_date" onchange="loadOrders()" aria-label="Date">
      <input class="in" id="or_q" placeholder="Search OrderID, ClOrdID, symbol, client&hellip;" style="width:260px" oninput="orRender(true)">
      <select class="in" id="or_side" onchange="orRender(true)" aria-label="Side"><option value="">Buy &amp; sell</option><option>Buy</option><option>Sell</option></select>
      <select class="in" id="or_broker" onchange="orRender(true)" aria-label="Broker"><option value="">All brokers</option></select>
      <button class="btn lnk" id="or_clear" type="button" onclick="orClear()" style="display:none">Clear filters</button>
     </div>
     <div class="tb noprint"><button class="btn" onclick="orExport('xlsx')">Excel</button><button class="btn" onclick="orExport('csv')">Download CSV</button><button class="btn" onclick="window.print()">Print</button></div>
    </div>
    <div class="tb" id="or_chips"></div>
    <div class="row2">
     <div class="card o w3">
      <div class="tw"><table class="tbl"><thead><tr><th>Order</th><th>Time</th><th>Broker</th><th>Symbol</th><th>Side</th><th class="r">Qty</th><th class="r">Filled</th><th class="r">Price</th><th>Status</th></tr></thead><tbody id="orders"></tbody></table></div>
      <div class="tbar" style="padding:10px 16px;font-size:13px;color:var(--mut)"><span id="or_range"></span><div class="tb"><button class="btn sm" id="or_prev" onclick="orPage(-1)">Previous</button><span class="mono" style="font-size:12px" id="or_pg"></span><button class="btn sm" id="or_next" onclick="orPage(1)">Next</button></div></div>
     </div>
     <div class="card p w1 sticky" id="or_detail" style="display:flex;flex-direction:column;gap:14px;padding:18px"><span class="faint" style="font-size:13px">Select an order to see its details and execution history.</span></div>
    </div>
   </section>

   <section class="view" id="view-sessions">
    <div class="kpis">
     <div class="kc"><span class="l">Logged on</span><span class="v" style="color:var(--acc)" id="fx_on">0</span></div>
     <div class="kc"><span class="l">Disconnected</span><span class="v" id="fx_off">0</span></div>
     <div class="kc"><span class="l">Inbound messages</span><span class="v" id="fx_in">0</span><span class="s">since each logon</span></div>
     <div class="kc"><span class="l">Registered brokers</span><span class="v" id="fx_reg">0</span></div>
    </div>
    <div class="tbar"><div class="seg md" id="fx_tabs"></div><span class="faint" style="font-size:12px" id="fx_acc"></span></div>
    <div class="card o"><div class="tw"><table class="tbl"><thead><tr><th>Session</th><th>Broker</th><th>Status</th><th class="r">In seq</th><th class="r">Out seq</th><th>Connected</th><th>Last logon</th><th></th></tr></thead><tbody id="sessions"></tbody></table></div></div>
    <p class="note">Log out sends a Logout (35=5) and closes the connection; resting orders stay in the book. Reset seq sets both sequence numbers back to 1. Sequence numbers also reset at every logon.</p>
   </section>

   <section class="view" id="view-market" data-admin-only>
    <div class="row2" style="align-items:stretch">
     <div class="card w2" style="display:flex;flex-direction:column;gap:18px;padding:20px">
      <div class="tbar" style="align-items:flex-start">
       <div style="display:flex;flex-direction:column;gap:4px"><span class="cap">Current phase</span><span style="font-size:28px;font-weight:600" id="ms_phase">&mdash;</span><span class="muted" id="ms_note"></span></div>
       <div style="display:flex;flex-direction:column;align-items:flex-end;gap:4px"><span class="cap">Engine clock</span><span class="mono" style="font-size:28px" id="ms_clock">&mdash;</span><span class="faint" style="font-size:13px" id="ms_date"></span></div>
      </div>
      <div style="display:flex;flex-direction:column;gap:6px"><div class="tl" id="ms_tl"></div><div class="tbar mono faint" style="font-size:11px"><span id="ms_tl0"></span><span id="ms_tl1"></span></div></div>
      <div class="tb" style="border-top:1px solid var(--line);padding-top:16px">
       <button class="btn dng" id="ms_end" type="button" onclick="msAsk('next')">End phase now</button>
       <button class="btn" id="ms_clockbtn" type="button" onclick="msAsk('clock')">Switch to manual</button>
       <span class="faint" style="font-size:12px;margin-left:6px">Move to</span>
       <button class="btn sm" onclick="msAsk('preopen')">Pre-Open</button><button class="btn sm" onclick="msAsk('open')">Continuous</button><button class="btn sm" onclick="msAsk('close')">Close</button><button class="btn sm" onclick="msAsk('postclose')">Post-Close</button><button class="btn sm" onclick="msAsk('endofday')">End of day</button>
      </div>
     </div>
     <div class="card t w1"><span class="cap" style="padding:0 18px">Today&rsquo;s session actions</span><div id="ms_hist" style="margin-top:8px"></div></div>
    </div>
    <div class="card t">
     <div class="hd"><span class="cap">Daily schedule</span><span class="faint" style="font-size:12px">Asia/Dhaka &middot; Sunday to Thursday &middot; Fridays and Saturdays are closed</span></div>
     <div id="ms_sched"></div>
     <p class="note" style="padding:10px 18px 8px">The schedule comes from ME_SESSION_SCHEDULE in the engine&rsquo;s settings (.env) and applies after a restart. While the clock is on, the phases follow it; moving a phase by hand switches the clock to manual.</p>
    </div>
   </section>

   <section class="view" id="view-instruments" data-admin-only>
    <div class="tbar">
     <div class="tb"><input class="in" id="f_instr" placeholder="Search symbol, name or ISIN&hellip;" style="width:240px" oninput="renderInstr()"><div class="seg md" id="in_tabs"></div></div>
     <div class="tb"><button class="btn red" type="button" onclick="deleteAllInstr()">Delete all</button><button class="btn" type="button" onclick="importDlg()">Import</button><div class="menu"><button class="btn" type="button" onclick="toggleExport(event)">Export</button><div class="menu-pop" id="exp_menu"><button onclick="exportInstr('xml')"><b>XML</b><small>DSE end-of-day ticker file</small></button><button onclick="exportInstr('csv')"><b>CSV</b><small>Spreadsheet</small></button><button onclick="exportInstr('json')"><b>JSON</b><small>Re-importable master</small></button><small style="display:block;padding:6px 10px" class="faint" id="exp_note"></small></div></div><button class="btn pri" type="button" onclick="openInstr()">Add instrument</button></div>
    </div>
    <div class="card o"><div class="tw"><table class="tbl tight"><thead><tr><th>Symbol</th><th>Name</th><th>ISIN</th><th>Board</th><th class="r">Lot</th><th class="r">Tick</th><th class="r">Ref price</th><th class="r">Circuit range</th><th>Status</th><th></th></tr></thead><tbody id="instrList"></tbody></table></div></div>
    <p class="note" id="in_count"></p>
   </section>

   <section class="view" id="view-creds">
    <div class="tbar"><input class="in" id="f_broker" placeholder="Search broker, SenderCompID or ITCH user&hellip;" style="width:280px" oninput="renderBrokers(_b)"><button class="btn pri" type="button" onclick="openBroker()">Add broker</button></div>
    <div class="card o"><div class="tw"><table class="tbl"><thead><tr><th>SenderCompID</th><th>Broker</th><th>ITCH user</th><th data-admin-only>Owner</th><th>Session</th><th>Last logon</th><th>Created</th><th></th></tr></thead><tbody id="brokerList"></tbody></table></div></div>
    <p class="note" id="brokerCount"></p>
   </section>

   <section class="view" id="view-access" data-admin-only>
    <div class="card" style="display:flex;flex-direction:column;gap:12px;padding:16px">
     <span class="cap">Add network</span>
     <div class="tb" style="align-items:flex-end"><label class="fld" style="flex:1 1 180px">IP address<input class="in mono" id="a_ip" placeholder="203.0.113.25"></label><label class="fld" style="flex:2 1 220px">Label (broker / partner)<input class="in" id="a_label" placeholder="e.g. BDTASK OMS"></label><button class="btn pri" type="button" onclick="addAllow()" style="padding:9px 16px">Add to allowlist</button></div>
     <span class="err" id="a_err"></span>
    </div>
    <div class="tbar"><input class="in" id="f_allow" placeholder="Filter by IP or label&hellip;" style="width:240px" oninput="renderAllow()"><span class="faint" style="font-size:12px" id="allowCount"></span></div>
    <div class="card o"><div class="tw"><table class="tbl"><thead><tr><th>Network</th><th>Label</th><th></th></tr></thead><tbody id="allowList"></tbody></table></div></div>
    <p class="note">The external FIX gateway (:5443) accepts connections only from these addresses, and changes apply at once. Every session still logs on with its SenderCompID and password. An empty list lets every address in.</p>
   </section>

   <section class="view" id="view-users" data-admin-only>
    <div class="tbar"><input class="in" id="f_user" placeholder="Search username or name&hellip;" style="width:240px" oninput="renderUsers()"><button class="btn pri" type="button" onclick="userDlg('new')">Add user</button></div>
    <div class="row2">
     <div class="card o w3"><div class="tw"><table class="tbl"><thead><tr><th>Username</th><th>Name &amp; contact</th><th>Role</th><th>Created</th><th>Last sign-in</th><th>Status</th><th></th></tr></thead><tbody id="userList"></tbody></table></div></div>
     <div class="card w1" style="display:flex;flex-direction:column;gap:14px;padding:18px">
      <span class="cap">Roles</span>
      <div style="display:flex;flex-direction:column;gap:4px"><span class="mono" style="font-size:12px;color:var(--acc)">ADMIN</span><span class="muted" style="font-size:13px">Full console: market session, instruments, allowlist, users, and every broker.</span></div>
      <div style="display:flex;flex-direction:column;gap:4px"><span class="mono" style="font-size:12px;color:var(--blue)">USER</span><span class="muted" style="font-size:13px">Sees and manages only the brokers they registered, their sessions, message log and reports. Can place orders from the Trade Terminal.</span></div>
      <span class="note" style="border-top:1px solid var(--line);padding-top:12px">Changing a password, disabling or deleting an account signs it out everywhere.</span>
     </div>
    </div>
    <p class="note" id="userCount"></p>
   </section>

   <section class="view" id="view-msglog">
    <div class="tbar">
     <div class="tb"><input class="in mono" id="ml_q" placeholder="Search tag=value, e.g. 55=GP or text&hellip;" style="width:280px;font-size:12px" oninput="mlRender()"><select class="in" id="ml_sess" onchange="mlRender()" aria-label="Session"><option value="">All sessions</option></select><div class="seg sm" id="ml_dir"><button data-d="" class="on">All</button><button data-d="IN">In</button><button data-d="OUT">Out</button></div></div>
     <button class="btn" id="ml_pause" type="button" onclick="mlPause()">Pause live</button>
    </div>
    <div class="tb" id="ml_types"></div>
    <div class="row2">
     <div class="card o w3"><div class="tw"><table class="tbl" style="font-size:12px"><thead><tr><th>Time</th><th>Dir</th><th>Session</th><th>Type</th><th class="r">Seq</th><th>Summary</th></tr></thead><tbody id="msglog"></tbody></table></div><div class="note" style="padding:8px 16px" id="ml_count"></div></div>
     <div class="card w2 sticky" id="ml_detail" style="display:flex;flex-direction:column;gap:12px;padding:18px;flex-basis:340px"><span class="faint" style="font-size:13px">Select a message to decode it.</span></div>
    </div>
   </section>

   <section class="view" id="view-sim" data-admin-only>
    <div class="card p sm-head">
     <div class="tbar">
      <div class="tb"><span class="cap">Market</span><span class="tag" id="sm_phase">&mdash;</span><span class="note" id="sm_phnote"></span></div>
      <div class="tb"><button class="btn" type="button" onclick="smPhase('preopen')">Pre-Open</button><button class="btn" type="button" onclick="smPhase('open')">Open market</button></div>
     </div>
     <p class="note">The <b>market bots</b> trade <b>through your OMS</b>: they sign in as the dealer you give and place, amend and cancel orders for its clients on the OMS's own order channel, so the orders pass the OMS risk checks, reach this engine through the broker's FIX session and show in the OMS Blotter, Execution, Positions and Purchase Power. The <b>benchmark</b> logs on to this engine's FIX gateway as <span class="mono">SIMBENCH1&ndash;2</span>. <b>Clean test data</b> takes the simulator's orders out of the book and the benchmark's out of the order report.</p>
    </div>
    <div class="row2">
     <div class="card p w2 sm-card">
      <div class="tbar"><div class="tb"><span class="cap">Market bots</span><span class="tag" id="sm_bstate">STOPPED</span><span class="note" id="sm_bhead"></span></div>
       <div class="tb"><button class="btn pri" id="sm_bstart" type="button" onclick="smBots('start')">Start</button><button class="btn" id="sm_bpause" type="button" onclick="smBots(SM.paused?'resume':'pause')" disabled>Pause</button><button class="btn" id="sm_bstop" type="button" onclick="smBots('stop')" disabled>Stop</button><button class="btn red" id="sm_bclean" type="button" onclick="smBotsClean()" disabled title="Cancel the bots' orders through the OMS and sign out">Clean Data</button></div></div>
      <div class="sm-f">
       <label class="fld">OMS address<input class="in" id="sm_burl" placeholder="https://jbl.bdtaskoms.com" autocomplete="off" spellcheck="false"></label>
       <label class="fld">Dealer login<input class="in" id="sm_buser" placeholder="OMS username" autocomplete="off" spellcheck="false"></label>
       <label class="fld">Password<span class="sm-rate"><input class="in" id="sm_bpass" type="password" autocomplete="new-password"><label class="sm-max" title="Save the login on the engine, so the next Start works with the password left blank"><input type="checkbox" id="sm_bremember" checked> Remember</label></span></label>
      </div>
      <div class="note" id="sm_bsaved" hidden></div>
      <div class="sm-f">
       <label class="fld">Speed<select class="in" id="sm_brate"><option value="30">30 orders/min</option><option value="60" selected>60 orders/min</option><option value="120">120 orders/min</option><option value="300">300 orders/min</option><option value="600">600 orders/min</option></select></label>
       <label class="fld">Clients<select class="in" id="sm_bcl" onchange="smSet()"><option value="4">4 clients</option><option value="8" selected>8 clients</option><option value="12">12 clients</option><option value="20">20 clients</option><option value="list">My list&hellip;</option></select></label>
       <label class="fld">Shares<select class="in" id="sm_bset" onchange="smSet()"><option value="10">10 shares</option><option value="20" selected>20 shares</option><option value="50">50 shares</option><option value="list">My list&hellip;</option></select></label>
       <label class="fld">Largest buy (Tk)<input class="in" id="sm_bmax" type="number" min="1000" step="1000" value="20000" inputmode="numeric"></label>
       <label class="fld" id="sm_bclistf" hidden>Client codes, comma separated<input class="in" id="sm_bclist" placeholder="00018, 02301" title="A client that is not in the dealer's list: CODE/BO-ID" autocomplete="off" spellcheck="false"></label>
       <label class="fld" id="sm_blistf" hidden>Symbols, comma separated<input class="in" id="sm_blist" placeholder="GP, BATBC, SQURPHARMA" autocomplete="off" spellcheck="false"></label>
      </div>
      <div class="sm-k" id="sm_bk"></div>
      <div class="note" id="sm_bnote"></div>
      <div class="tb" id="sm_bsess"></div>
     </div>
     <div class="card p w2 sm-card">
      <div class="tbar"><div class="tb"><span class="cap">Benchmark</span><span class="tag" id="sm_qstate">IDLE</span></div>
       <div class="tb"><button class="btn pri" id="sm_qstart" type="button" onclick="smBench('start')">Run</button><button class="btn" id="sm_qstop" type="button" onclick="smBench('stop')" disabled>Stop</button></div></div>
      <div class="sm-f">
       <div class="fld">What to send<div class="seg md" id="sm_qmode"><button type="button" class="on" data-m="trade" title="SIMBENCH1 sells, SIMBENCH2 buys it with an IOC order: every pair trades">Trades</button><button type="button" data-m="orders" title="IOC buys at the bottom of the band: accepted and cancelled, no trades">Orders only</button></div></div>
       <label class="fld">Rate<span class="sm-rate"><input class="in" id="sm_qrate" type="number" min="1" max="100000" value="500" inputmode="numeric"><span class="faint">/s</span><label class="sm-max"><input type="checkbox" id="sm_qmax" onchange="document.getElementById('sm_qrate').disabled=this.checked"> MAX</label></span></label>
       <label class="fld">Duration<select class="in" id="sm_qsec"><option value="5">5 s</option><option value="10" selected>10 s</option><option value="30">30 s</option><option value="60">1 min</option><option value="120">2 min</option></select></label>
       <label class="fld">Share<input class="in" id="sm_qsym" value="GP" autocomplete="off" spellcheck="false"></label>
      </div>
      <div class="sm-tp"><div><span class="l">Throughput</span><span class="v" id="sm_qnow">0</span><span class="faint">acknowledged orders/s</span></div><svg id="sm_qspark" viewBox="0 0 300 60" preserveAspectRatio="none" aria-label="Orders acknowledged per second"></svg></div>
      <div class="sm-k" id="sm_qk"></div>
      <div class="tw"><table class="tbl tight sm-lat"><thead><tr><th>Latency (ms)</th><th class="r">avg</th><th class="r">p50</th><th class="r">p95</th><th class="r">p99</th><th class="r">max</th><th class="r">samples</th></tr></thead><tbody id="sm_qlat"></tbody></table></div>
      <p class="note"><b>Trades</b>: SIMBENCH1 sells and SIMBENCH2 buys it with an IOC order, so every pair trades; use a share the market bots are not trading. <b>Orders only</b>: IOC buys at the bottom of the band, accepted and cancelled. Latency runs from sending an order over FIX to its acknowledgement (150=0 or 8), and for the buys to their first fill. At MAX up to 1,000 orders wait in the gateway at once, so latency there includes that queue; a set rate shows the engine's own latency.</p>
     </div>
    </div>
    <div class="row2">
     <div class="card t w3"><div class="hd"><span class="cap">Live order tape</span><span class="note">latest bot orders and fills; the benchmark is sampled</span></div>
      <div class="tw"><table class="tbl tight"><thead><tr><th>Time</th><th>Bot</th><th>Share</th><th>Side</th><th class="r">Qty</th><th class="r">Price</th><th>Status</th><th>Note</th></tr></thead><tbody id="sm_tape"></tbody></table></div></div>
     <div class="w1 sm-side">
      <div class="card p sm-card"><span class="cap">Reject reasons</span><div id="sm_rej"></div></div>
      <div class="card p sm-card sm-danger"><span class="cap">Clean test data</span>
       <p class="note">Stops the market bots and the benchmark, cancels every simulator order still in the book (the market bots' through the OMS, signing them out) and removes the benchmark's rows from the order report. The market bots' orders are the broker's own, so they stay in the report. Trades stay in the market: prices, volume and the trade log follow from them.</p>
       <button class="btn dng" type="button" onclick="smClean()">Clean test data</button></div>
     </div>
    </div>
   </section>

   <section class="view" id="view-diag" data-admin-only>
    <div class="kpis" id="dg_kpis"></div>
    <div class="row2">
     <div class="card t w2"><span class="cap" style="padding:0 18px">Configuration</span><div id="dg_cfg" style="margin-top:8px"></div></div>
     <div class="card t w1"><span class="cap" style="padding:0 18px">Listeners</span><div id="dg_lst" style="margin-top:8px"></div></div>
    </div>
    <div class="card" style="display:flex;flex-direction:column;gap:12px;padding:18px">
     <div class="tbar"><span class="cap">Self-test</span><button class="btn pri" id="dg_run" type="button" onclick="dgTest()">Run self-test</button></div>
     <div id="dg_checks" style="display:flex;flex-direction:column;gap:8px"></div>
     <span id="dg_done" style="font-size:13px"></span>
     <p class="note">Read-only checks against the running engine; nothing is changed.</p>
    </div>
    <div class="card t"><div class="hd"><span class="cap">Audit log</span><div class="seg sm" id="dg_tabs"></div></div><div class="tw"><table class="tbl"><tbody id="audit"></tbody></table></div></div>
   </section>

  </div>
 </main>
</div>
<div class="dlg-bg" id="dlg" onclick="if(event.target==this)dlgClose()"><div class="dlg" id="dlgc"></div></div>
<form id="sheetForm" method="post" action="/admin/api/broker/sheet" target="_blank" style="display:none">
 <input type="hidden" name="company"><input type="hidden" name="sender"><input type="hidden" name="fix_pass"><input type="hidden" name="itch_user"><input type="hidden" name="itch_pass">
</form>
`
