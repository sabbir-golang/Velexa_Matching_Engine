package web

// adminBody is the console's markup: the shell (sidebar, header) and one
// section per page; admin_script.go fills them in.
const adminBody = `<div class="app">
 <aside>
  <div class="brand"><span class="logo">M</span><div><b>Matching Engine</b><small>DSEME · velexa</small></div></div>
  <nav>
   <div class="navgrp">Overview</div>
   <div class="navitem" data-v="dash"><svg viewBox="0 0 24 24"><path d="M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z"/></svg><span>Dashboard</span></div>
   <div class="navgrp">Trading</div>
   <div class="navitem" data-v="trade"><svg viewBox="0 0 24 24"><path d="M4 4v16h16M8 15l3.5-4 3 3L20 8"/></svg><span>Trade Terminal</span></div>
   <div class="navitem" data-v="orders"><svg viewBox="0 0 24 24"><path d="M7 3h8l4 4v14H7zM14 3v5h5M10 12h6M10 16h6"/></svg><span>Order Report</span></div>
   <div class="navitem" data-v="sessions"><svg viewBox="0 0 24 24"><path d="M3 12h4l3-7 4 14 3-7h4"/></svg><span>FIX Sessions</span><span class="badge" id="navSess"></span></div>
   <div class="navitem" data-v="market" data-admin-only><svg viewBox="0 0 24 24"><path d="M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0z"/></svg><span>Market Session</span></div>
   <div class="navitem" data-v="instruments" data-admin-only><svg viewBox="0 0 24 24"><path d="M5 20V11M10 20V5M15 20v-8M20 20v-5M3 20h18"/></svg><span>Instruments</span></div>
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
    <div><h1 id="pageTitle">Dashboard</h1><div class="sub" id="pageSub">Today · whole exchange</div></div></div>
   <div class="tr"><span class="live" title=""><span class="dot"></span><span id="up">&mdash;</span></span><button class="btn" id="themeBtn" type="button" style="min-width:64px">Dark</button></div>
  </div>
  <div class="wrap">

   <section class="view on" id="view-dash">
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
     <div class="tb"><div class="menu"><button class="btn" type="button" onclick="toggleExport(event)">Export</button><div class="menu-pop" id="exp_menu"><button onclick="exportInstr('xml')"><b>XML</b><small>DSE end-of-day ticker file</small></button><button onclick="exportInstr('csv')"><b>CSV</b><small>Spreadsheet</small></button><button onclick="exportInstr('json')"><b>JSON</b><small>Re-importable master</small></button><small style="display:block;padding:6px 10px" class="faint" id="exp_note"></small></div></div><button class="btn pri" type="button" onclick="openInstr()">Add instrument</button></div>
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
