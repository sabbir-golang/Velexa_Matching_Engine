package web

// adminCSS is the console's stylesheet: the design's tokens (dark; light in
// theme.go) and its components — shell, controls, cards, tables, dialogs —
// plus the dashboard, terminal and message-log pieces.
const adminCSS = `/* Design tokens (dark; light in theme.go). Names: panel = surface, panel2 = sunk, line = border, ink = text, mut = muted, mut2 = faint. */
:root{--bg:#0d1210;--side:#101614;--panel:#141b18;--panel2:#111714;--line:#24302a;--line2:#2f3c35;
--ink:#e5ece8;--mut:#9eaea5;--mut2:#6f7e76;--acc:#3dbd84;--onacc:#0b120f;--accSoft:#173328;--amber:#e0a648;--amberSoft:#33291a;
--grn:#3dbd84;--red:#ee6a60;--sellSoft:#3a1d1b;--blue:#6ea4e6;--blueSoft:#1a2738;--track:#1f2924;--flash:#3a3216;--scrim:rgba(0,0,0,.6);color-scheme:dark}
*{scrollbar-color:var(--line2) transparent;scrollbar-width:thin}
*{box-sizing:border-box}html,body{margin:0}
body{background:var(--bg);color:var(--ink);font-family:"IBM Plex Sans",system-ui,-apple-system,sans-serif;font-size:14px;line-height:1.4;-webkit-font-smoothing:antialiased}
button,input,select,textarea{font-family:inherit}
a{color:var(--acc);text-decoration:none}a:hover{text-decoration:underline}
.mono{font-family:"IBM Plex Mono",ui-monospace,monospace}.muted{color:var(--mut)}.faint{color:var(--mut2)}
/* shell */
.app{display:grid;grid-template-columns:232px minmax(0,1fr);min-height:100vh}
aside{background:var(--side);border-right:1px solid var(--line);display:flex;flex-direction:column;position:sticky;top:0;height:100vh}
.brand{display:flex;align-items:center;gap:10px;padding:18px 16px 14px}
.logo{width:32px;height:32px;border-radius:8px;background:var(--acc);color:var(--onacc);display:grid;place-items:center;font-weight:700;font-size:15px;flex:none}
.brand b{display:block;font-size:14px;font-weight:600}.brand small{font-family:"IBM Plex Mono",monospace;font-size:11px;color:var(--mut2)}
nav{display:flex;flex-direction:column;padding:0 10px 10px;flex:1;overflow:auto}
.navgrp{font-size:11px;font-weight:600;letter-spacing:.08em;color:var(--mut2);padding:14px 8px 6px;text-transform:uppercase}
.navitem{display:flex;align-items:center;gap:10px;font-size:14px;border-radius:6px;padding:7px 10px;margin-bottom:2px;cursor:pointer;color:var(--mut);user-select:none}
.navitem:hover{color:var(--ink);background:var(--track)}
.navitem.on{background:var(--accSoft);color:var(--acc);font-weight:600}
.navitem svg{width:16px;height:16px;flex:none;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}
.navitem .badge{margin-left:auto;font-family:"IBM Plex Mono",monospace;font-size:11px;color:var(--acc);font-weight:600}
.sidefoot{border-top:1px solid var(--line);padding:14px 16px;display:flex;flex-direction:column;gap:8px;font-size:12px;color:var(--mut)}
.sidefoot .row{display:flex;align-items:center;gap:6px;flex-wrap:wrap}.sidefoot b{color:var(--ink)}
.dot{width:7px;height:7px;border-radius:50%;background:var(--acc);display:inline-block;flex:none}
.rolebadge{font-family:"IBM Plex Mono",monospace;font-size:10px;border:1px solid var(--acc);color:var(--acc);border-radius:10px;padding:0 6px;text-transform:uppercase}
.rolebadge.user{border-color:var(--blue);color:var(--blue)}
.sidefoot .btns{display:flex;gap:6px}.sidefoot .btns>*{flex:1;margin:0}
.sidefoot .btns button{width:100%;font-size:12px;color:var(--ink);background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:6px 4px;cursor:pointer;white-space:nowrap}
.sidefoot .btns button:hover{border-color:var(--mut2)}
main{display:flex;flex-direction:column;min-width:0}
.top{position:sticky;top:0;z-index:20;background:var(--bg);border-bottom:1px solid var(--line);padding:16px 32px;display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:12px}
.top .tl{display:flex;align-items:center;gap:12px;min-width:0}
.top h1{margin:0;font-size:22px;font-weight:600;letter-spacing:-.01em}
.top .sub{color:var(--mut);font-size:13px;margin-top:2px}
.top .tr{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.live{display:flex;align-items:center;gap:6px;font-family:"IBM Plex Mono",monospace;font-size:12px;color:var(--mut);border:1px solid var(--line);background:var(--panel);border-radius:20px;padding:5px 12px;max-width:440px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.live .dot{animation:pulse 2s infinite}@keyframes pulse{0%,100%{opacity:1}50%{opacity:.35}}
.hamb{display:none;background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:6px 8px;color:var(--ink);cursor:pointer}
.backdrop{display:none}
.wrap{padding:24px 32px 48px;min-width:0}
.view{display:none}.view.on{display:flex;flex-direction:column;gap:14px}
/* controls */
.btn{display:inline-flex;align-items:center;justify-content:center;gap:6px;font-size:13px;font-weight:500;color:var(--ink);background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:7px 12px;cursor:pointer;white-space:nowrap;text-decoration:none}
.btn:hover{border-color:var(--mut2);text-decoration:none}
.btn.pri{background:var(--acc);border-color:var(--acc);color:var(--onacc);font-weight:600}.btn.pri:hover{filter:brightness(1.06)}
.btn.dng{background:var(--red);border-color:var(--red);color:var(--onacc);font-weight:600}
.btn.sm{font-size:12px;padding:4px 9px;border-radius:5px}
.btn.red{color:var(--red);background:transparent}
.btn.lnk{background:transparent;border-color:transparent;color:var(--acc);padding-left:6px;padding-right:6px}
.btn:disabled{opacity:.45;cursor:default;filter:none}
.in{font-size:13px;color:var(--ink);background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:7px 10px;outline:none;min-width:0}
.in:focus{border-color:var(--acc)}.in.bad{border-color:var(--red)}.in::placeholder{text-transform:none;color:var(--mut2)}select.in{padding:7px 8px;cursor:pointer}
input[type=date].in,input[type=time].in{font-family:"IBM Plex Mono",monospace;color-scheme:dark}
[data-theme="light"] input[type=date].in,[data-theme="light"] input[type=time].in{color-scheme:light}
select.in option{background:var(--panel);color:var(--ink)}
.in:disabled{opacity:.5}
.fld{display:flex;flex-direction:column;gap:5px;font-size:12px;color:var(--mut);min-width:0}.fld .in{font-size:14px;padding:8px;width:100%}
.chk{display:flex;align-items:center;gap:8px;font-size:13px;color:var(--ink)}
.tbar{display:flex;flex-wrap:wrap;gap:8px;align-items:center;justify-content:space-between}
.tb{display:flex;flex-wrap:wrap;gap:8px;align-items:center}
.seg{display:flex;gap:2px;background:var(--track);border-radius:7px;padding:2px}
.seg button{font:500 12px "IBM Plex Sans",sans-serif;border:0;border-radius:5px;padding:5px 12px;cursor:pointer;background:transparent;color:var(--ink)}.seg button.on{background:var(--panel)}
.seg.sm button{padding:4px 9px}.seg.md button{font-size:13px}
/* cards, tables */
.card{background:var(--panel);border:1px solid var(--line);border-radius:10px;min-width:0}
.card.p{padding:18px 20px}.card.t{padding:16px 0 6px}.card.o{overflow:hidden}
.card .hd{display:flex;justify-content:space-between;align-items:center;gap:8px;padding:0 18px 10px;flex-wrap:wrap}
.cap{font-size:11px;font-weight:600;letter-spacing:.08em;color:var(--mut);text-transform:uppercase}
.row2{display:flex;flex-wrap:wrap;gap:12px;align-items:flex-start}.row2>.w3{flex:3 1 600px;min-width:0}.row2>.w2{flex:2 1 460px;min-width:0}.row2>.w1{flex:1 1 280px;min-width:0}
.kpis{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}
.kc{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:14px 16px;display:flex;flex-direction:column;gap:4px;min-width:0}
.kc .l{font-size:13px;color:var(--mut)}.kc .v{font-family:"IBM Plex Mono",monospace;font-size:26px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.kc .s{font-size:12px;color:var(--mut2)}
.tw{overflow-x:auto}
table.tbl{width:100%;border-collapse:collapse;font-size:13px}
table.tbl th{text-align:left;font-size:11px;font-weight:600;letter-spacing:.06em;color:var(--mut2);text-transform:uppercase;background:var(--panel2);padding:10px 16px;border-bottom:1px solid var(--line);white-space:nowrap}
table.tbl td{padding:9px 16px;border-bottom:1px solid var(--line);vertical-align:middle;white-space:nowrap}
table.tbl td.wrap{white-space:normal}
table.tbl.tight th,table.tbl.tight td{padding-left:10px;padding-right:10px}table.tbl.tight td.ell{max-width:190px}
table.tbl tbody tr:last-child td{border-bottom:0}
table.tbl .r{text-align:right}table.tbl td.m,table.tbl .m{font-family:"IBM Plex Mono",monospace}
table.tbl td.ell{max-width:260px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
table.tbl tbody tr.ck{cursor:pointer}table.tbl tbody tr.ck:hover td{background:var(--panel2)}
table.tbl tbody tr.sel td{background:var(--accSoft)}
table.tbl td .acts{display:flex;gap:6px;justify-content:flex-end;flex-wrap:nowrap}
.empty{padding:24px 16px;color:var(--mut2)}
.t-acc{color:var(--acc)!important;background:var(--accSoft)!important}.t-blue{color:var(--blue)!important;background:var(--blueSoft)!important}
.t-amber{color:var(--amber)!important;background:var(--amberSoft)!important}.t-red{color:var(--red)!important;background:var(--sellSoft)!important}
.t-mut{color:var(--mut2)!important;background:var(--track)!important}
.sdot{display:inline-flex;align-items:center;gap:6px;font-size:11px;font-weight:600;letter-spacing:.04em;white-space:nowrap}.sdot i{width:7px;height:7px;border-radius:50%;background:currentColor;display:inline-block}
.pchip{display:inline-flex;align-items:baseline;gap:8px;border-radius:20px;padding:5px 12px;cursor:pointer;border:1px solid var(--line);background:var(--panel);color:var(--ink);font-size:13px}
.pchip.on{border-color:var(--acc)}.pchip i{width:7px;height:7px;border-radius:50%;display:inline-block}.pchip b{font-family:"IBM Plex Mono",monospace;font-weight:500}
.avatar{width:28px;height:28px;border-radius:50%;background:var(--track);display:inline-grid;place-items:center;font-size:12px;font-weight:600;color:var(--mut);flex:none}
.note{color:var(--mut2);font-size:12px;margin:0;line-height:1.5}
.progress{height:6px;background:var(--track);border-radius:3px;overflow:hidden}.progress i{display:block;height:100%;background:var(--acc)}
.kv{display:grid;grid-template-columns:auto 1fr;gap:6px 14px;font-size:13px}.kv>span:nth-child(odd){color:var(--mut)}
.tline{display:grid;grid-template-columns:14px 1fr;gap:10px}.tline .ax{display:flex;flex-direction:column;align-items:center}
.tline .ax i{width:9px;height:9px;border-radius:50%;margin-top:4px;flex:none}.tline .ax b{flex:1;width:1px;background:var(--line)}
.tline .tx{display:flex;flex-direction:column;padding-bottom:12px}.tline .tx span{font-size:13px;font-weight:500}.tline .tx small{font-family:"IBM Plex Mono",monospace;font-size:12px;color:var(--mut2)}
.sticky{position:sticky;top:96px}
/* dialogs */
.dlg-bg{position:fixed;inset:0;background:var(--scrim);display:none;place-items:center;z-index:100;padding:24px;overflow:auto}.dlg-bg.on{display:grid}
.dlg{width:100%;max-width:420px;background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:24px;display:flex;flex-direction:column;gap:14px;margin:auto}
.dlg.wide{max-width:540px}.dlg.xwide{max-width:660px}
.dlg h3{margin:0;font-size:17px;font-weight:600}.dlg>p{margin:0;color:var(--mut);font-size:14px}
.dlg .acts{display:flex;justify-content:flex-end;gap:8px;margin-top:4px;flex-wrap:wrap}
.g2{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:12px}.g2>*{min-width:0}.g2 .span{grid-column:span 2}
.err{color:var(--red);font-size:13px}.ok-t{color:var(--acc);font-size:13px}
.secret{display:flex;gap:8px;align-items:center;background:var(--panel2);border:1px solid var(--line);border-radius:6px;padding:10px 12px}
.secret span{flex:1;font-family:"IBM Plex Mono",monospace;font-size:14px;word-break:break-all}
.creds{background:var(--panel2);border:1px solid var(--line);border-radius:8px}
.cr{display:grid;grid-template-columns:150px 1fr auto;gap:10px;align-items:center;padding:8px 12px;border-top:1px solid var(--line);font-size:13px}.cr:first-child{border-top:0}
.cr .ck{color:var(--mut)}.cr .cv{font-family:"IBM Plex Mono",monospace;word-break:break-all}.cr .cv.sec{color:var(--amber);font-weight:600}
.guide{font-size:13px;color:var(--mut);line-height:1.6}.guide ol{margin:0;padding-left:18px}.guide code{font-family:"IBM Plex Mono",monospace;background:var(--panel2);border:1px solid var(--line);border-radius:4px;padding:0 4px;font-size:12px;color:var(--ink)}
.menu{position:relative}.menu-pop{display:none;position:absolute;right:0;top:calc(100% + 6px);min-width:230px;background:var(--panel);border:1px solid var(--line);border-radius:8px;box-shadow:0 12px 32px rgba(0,0,0,.3);z-index:30;padding:4px}
.menu-pop.on{display:block}.menu-pop button{display:flex;flex-direction:column;align-items:flex-start;width:100%;background:none;border:0;color:var(--ink);padding:8px 10px;border-radius:6px;cursor:pointer;text-align:left;font-size:13px}
.menu-pop button:hover{background:var(--track)}.menu-pop small{color:var(--mut2);font-size:11.5px}
/* trade terminal */
.symstrip{display:flex;gap:6px;overflow-x:auto;padding-bottom:4px}
.sym{flex:0 0 auto;display:flex;flex-direction:column;align-items:flex-start;gap:1px;text-align:left;border-radius:8px;padding:8px 12px;cursor:pointer;border:1px solid var(--line);background:var(--panel);color:var(--ink)}
.sym.on{background:var(--accSoft);border-color:var(--acc)}.sym b{font-size:12px;font-weight:600}.sym span{font-family:"IBM Plex Mono",monospace;font-size:12px}
.book .hrow,.book .lv{display:grid;grid-template-columns:1fr 1fr 60px;gap:8px;padding:3px 16px}
.book .hrow,.tape .hrow{font-size:11px;font-weight:600;letter-spacing:.06em;color:var(--mut2);padding-top:6px;padding-bottom:6px;border-bottom:1px solid var(--line)}
.book .lv{position:relative;font-family:"IBM Plex Mono",monospace;font-size:13px}.book .lv i{position:absolute;top:1px;bottom:1px;right:0}.book .lv span{position:relative}
.book .mid{display:flex;justify-content:space-between;padding:6px 16px;border-top:1px solid var(--line);border-bottom:1px solid var(--line);background:var(--panel2);font-size:12px;color:var(--mut)}
.tape .hrow,.tape .tr3{display:grid;grid-template-columns:70px 1fr 1fr;gap:8px;padding:3px 16px}.tape .tr3{font-family:"IBM Plex Mono",monospace;font-size:13px}
.sidesw{display:grid;grid-template-columns:1fr 1fr;gap:4px;background:var(--track);border-radius:8px;padding:3px}
.sidesw button{font-size:14px;font-weight:600;border:0;border-radius:6px;padding:8px;cursor:pointer;background:transparent;color:var(--mut)}
.sidesw .b.on{background:var(--acc);color:var(--onacc)}.sidesw .s.on{background:var(--red);color:var(--onacc)}
/* combobox */
.cbx{position:relative}.cbx .in{width:100%;padding-right:30px}
.cbx-btn{position:absolute;right:4px;top:50%;transform:translateY(-50%);border:0;background:none;color:var(--mut);cursor:pointer;font-size:12px;padding:6px}
.cbx-pop{display:none;position:absolute;left:0;right:0;top:calc(100% + 4px);max-height:248px;overflow:auto;background:var(--panel);border:1px solid var(--line);border-radius:8px;box-shadow:0 12px 32px rgba(0,0,0,.25);z-index:130;padding:4px}
.cbx-pop.on{display:block}
.cbx-opt{display:flex;justify-content:space-between;align-items:baseline;gap:10px;padding:7px 10px;border-radius:6px;cursor:pointer;font-size:13px;color:var(--ink)}
.cbx-opt small{font-family:"IBM Plex Mono",monospace;font-size:11px;color:var(--mut2)}
.cbx-opt.act{background:var(--track)}.cbx-opt.sel{color:var(--acc);font-weight:600}.cbx-opt.new{color:var(--acc);border-top:1px solid var(--line);margin-top:2px}
.cbx-empty{padding:8px 10px;font-size:13px;color:var(--mut2)}
/* market watch */
table.mw td.bid{background:color-mix(in srgb,var(--accSoft) 70%,transparent)}table.mw td.ask{background:color-mix(in srgb,var(--sellSoft) 70%,transparent)}
table.mw th.bid{background:color-mix(in srgb,var(--accSoft) 70%,var(--panel2))}table.mw th.ask{background:color-mix(in srgb,var(--sellSoft) 70%,var(--panel2))}
table.mw th.so{cursor:pointer;user-select:none}table.mw th.so:hover{color:var(--ink)}
table.mw tbody tr{cursor:pointer}table.mw tbody tr:hover td{filter:brightness(.97)}[data-theme="dark"] table.mw tbody tr:hover td,:root:not([data-theme]) table.mw tbody tr:hover td{filter:brightness(1.18)}
table.mw tbody tr.sel td:first-child{box-shadow:inset 3px 0 0 var(--acc)}table.mw tbody tr.sel td{font-weight:600}
table.mw tbody tr.fl td{background:var(--flash)!important;transition:background .6s}
/* terminal side: a column beside the market watch on wide screens, a row of three below it otherwise */
.ttside{flex:1 1 340px;min-width:0;display:flex;flex-wrap:wrap;gap:12px;align-content:flex-start;align-items:flex-start}.ttside>.card{flex:1 1 300px;min-width:0}
table.mw td,table.mw th{padding-left:9px;padding-right:9px}table.mw td{font-size:12.5px}
/* message log */
.mtype{font-size:12px;border-radius:14px;padding:4px 10px;cursor:pointer;border:1px solid var(--line);background:var(--panel);color:var(--ink)}.mtype.on{border-color:var(--acc);background:var(--accSoft)}.mtype b{font-family:"IBM Plex Mono",monospace;font-weight:600}
.raw{font-family:"IBM Plex Mono",monospace;font-size:11px;line-height:1.6;color:var(--mut);background:var(--panel2);border:1px solid var(--line);border-radius:6px;padding:10px;word-break:break-all}
.ftag{display:grid;grid-template-columns:44px 130px minmax(0,1fr);gap:8px;padding:4px 0;border-top:1px solid var(--line);font-size:12px}
.ftag span:first-child{font-family:"IBM Plex Mono",monospace;color:var(--mut2);text-align:right}.ftag span:nth-child(2){color:var(--mut)}.ftag span:last-child{font-family:"IBM Plex Mono",monospace;word-break:break-all}
/* dashboard (overview) */
.dbtools{display:none;align-items:center;gap:8px;flex-wrap:wrap}body.v-dash .dbtools{display:flex}
.dbbtn{font:500 13px "IBM Plex Sans",sans-serif;color:var(--ink);background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:7px 12px;cursor:pointer}
.dbbtn:hover{border-color:var(--mut2)}input.dbbtn{padding:6px 10px;color-scheme:dark}[data-theme="light"] input.dbbtn{color-scheme:light}
.db{display:flex;flex-direction:column;gap:20px}
.dbc{background:var(--panel);border:1px solid var(--line);border-radius:10px;min-width:0}
.dbc.p{padding:18px 20px}.dbc.t{padding:18px 0 6px}
.cap{font-size:11px;font-weight:600;letter-spacing:.08em;color:var(--mut);text-transform:uppercase}
.dbm{font-family:"IBM Plex Mono",monospace}
.faint{color:var(--mut2)}
.dbrow{display:flex;flex-wrap:wrap;gap:12px}.dbrow>.w2{flex:2 1 560px}.dbrow>.w1{flex:1 1 320px}
.sess{display:flex;flex-direction:column;gap:14px}
.sess .hd{display:flex;flex-wrap:wrap;justify-content:space-between;align-items:baseline;gap:12px}
.sess .ph{font-size:18px;font-weight:600}
.tl{position:relative;display:flex;height:12px;gap:2px}.tl .sg{opacity:.35}.tl .sg.cur{opacity:.6}
.tl .mk{position:absolute;top:-4px;bottom:-4px;width:3px;margin-left:-1px;border-radius:2px;background:var(--ink);transition:left .8s}
.tlx{position:relative;height:14px;font-family:"IBM Plex Mono",monospace;font-size:11px;color:var(--mut2)}.tlx span{position:absolute;transform:translateX(-50%);white-space:nowrap}
.tlx span:first-child{transform:none}.tlx span:last-child{transform:translateX(-100%)}
.legend{display:flex;flex-wrap:wrap;gap:6px 18px;font-size:12px;color:var(--mut)}.legend span{display:flex;align-items:center;gap:6px}.legend i{width:10px;height:10px;border-radius:2px;display:inline-block}
.dbk{display:grid;grid-template-columns:repeat(auto-fit,minmax(210px,1fr));gap:12px}
.dbk>div{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:16px 18px;display:flex;flex-direction:column;gap:6px;min-width:0}
.dbk .l{color:var(--mut);font-size:13px}.dbk .v{font-family:"IBM Plex Mono",monospace;font-size:30px;font-weight:500;letter-spacing:-.02em;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.dbk .s{display:flex;justify-content:space-between;gap:8px;font-size:12px;color:var(--mut2)}.dbk .s b{font-weight:500}
.seg{display:flex;gap:2px;background:var(--track);border-radius:7px;padding:2px}
.seg button{font:500 12px "IBM Plex Sans",sans-serif;border:0;border-radius:5px;padding:5px 12px;cursor:pointer;background:transparent;color:var(--ink)}.seg button.on{background:var(--panel)}
.seg.sm button{padding:4px 9px}
.chart{display:flex;align-items:flex-end;gap:3px;height:190px;border-bottom:1px solid var(--line)}
.chart>div{flex:1;min-width:0;height:100%;display:flex;align-items:flex-end}.chart>div>i{display:block;width:100%;border-radius:3px 3px 0 0;background:var(--acc)}
.chart>div.fut>i{background:var(--track)}.chart>div.cur>i{opacity:.55}
.chx{position:relative;height:14px;font-family:"IBM Plex Mono",monospace;font-size:11px;color:var(--mut2);margin-top:-8px}.chx span{position:absolute;transform:translateX(-50%);white-space:nowrap}
.chx span:first-child{transform:none}.chx span:last-child{transform:translateX(-100%)}
.oc .big{font-family:"IBM Plex Mono",monospace;font-size:34px;font-weight:500;letter-spacing:-.02em}
.stack{display:flex;height:10px;border-radius:5px;overflow:hidden;gap:2px;background:var(--track)}
.ocl{display:grid;grid-template-columns:12px 1fr auto 52px;align-items:center;gap:8px;font-size:13px}.ocl i{width:8px;height:8px;border-radius:2px;display:inline-block}
.bs{display:flex;height:6px;border-radius:3px;overflow:hidden;gap:2px}
.it{overflow-x:auto}.it .g{display:grid;grid-template-columns:1.3fr 1fr 1fr 1fr 1fr 1fr .6fr 1fr 1fr;gap:6px;align-items:center;padding:9px 20px;border-bottom:1px solid var(--line);min-width:640px}
.it .h{letter-spacing:.03em}.it .r{font-size:12.5px}
.it .r small{display:block;font-size:11px;opacity:.85}
.it .h{padding:8px 20px;font-size:11px;font-weight:600;letter-spacing:.06em;color:var(--mut2)}.it .h span{cursor:pointer;user-select:none}.it .h span:hover{color:var(--ink)}
.it .r{font-family:"IBM Plex Mono",monospace;font-size:13px;transition:background .6s}.it .r.fl{background:var(--flash)}
.it .r b{font-family:"IBM Plex Sans",sans-serif;font-weight:600}.it .rt{text-align:right}
.feedr{display:grid;grid-template-columns:62px minmax(0,1fr) auto;gap:10px;align-items:baseline;padding:8px 20px;border-top:1px solid var(--line);font-size:13px}
.feedr .x{font-family:"IBM Plex Mono",monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.feedr .y{font-size:12px;color:var(--mut2);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.tag{font-size:10px;font-weight:600;letter-spacing:.06em;padding:2px 6px;border-radius:4px;background:var(--track);color:var(--mut)}
.tag.TRADE,.tag.FILLED{color:var(--acc);background:var(--accSoft)}.tag.NEW,.tag.PARTIAL,.tag.STOP{color:var(--blue)}.tag.CANCEL,.tag.EXPIRED{color:var(--amber)}
.tag.REJECT{color:var(--red);background:var(--sellSoft)}
.brk{display:grid;grid-template-columns:minmax(0,1fr) 120px 96px;gap:14px;align-items:center;padding:9px 20px;border-top:1px solid var(--line);font-size:13px}
.brk .n{font-weight:500;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.brk .bb{height:6px;background:var(--track);border-radius:3px;overflow:hidden}.brk .bb i{display:block;height:100%;background:var(--blue)}
.lst{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:16px;align-items:center;padding:12px 20px;border-top:1px solid var(--line)}
.upc{display:flex;align-items:center;gap:5px;font-size:11px;font-weight:600;color:var(--acc);background:var(--accSoft);padding:3px 8px;border-radius:10px}.upc i{width:6px;height:6px;border-radius:50%;background:currentColor}
.upc.dn{color:var(--red);background:var(--sellSoft)}
.dbempty{padding:16px 20px;color:var(--mut2);border-top:1px solid var(--line)}
.dbfoot{margin:0;font-size:12px;color:var(--mut2)}
.dbmore{background:none;border:0;color:var(--acc);font:500 12px "IBM Plex Sans",sans-serif;cursor:pointer;padding:10px 20px}
@media(max-width:820px){.dbrow>.w2,.dbrow>.w1{flex-basis:100%}.brk{grid-template-columns:minmax(0,1fr) 70px 86px}}

body.role-user [data-admin-only]{display:none!important}
@media(max-width:820px){
 .app{grid-template-columns:1fr}
 aside{position:fixed;left:0;top:0;height:100vh;width:250px;z-index:40;transform:translateX(-100%);transition:transform .2s ease}
 aside.open{transform:translateX(0);box-shadow:0 0 50px rgba(0,0,0,.5)}
 .hamb{display:inline-flex}
 .backdrop.on{display:block;position:fixed;inset:0;background:var(--scrim);z-index:35}
 .wrap{padding:20px 16px 48px}.top{padding:12px 16px}.top h1{font-size:19px}
 .g2{grid-template-columns:1fr}.g2 .span{grid-column:auto}.sticky{position:static}
 .cr{grid-template-columns:1fr auto}.cr .ck{grid-column:span 2}
 table.mw th:nth-child(2),table.mw td:nth-child(2),table.mw th:nth-child(3),table.mw td:nth-child(3){display:none}
}
@media print{aside,.top,.noprint,.backdrop{display:none!important}.app{display:block}.wrap{padding:0}body{background:#fff;color:#000}.card{border-color:#ccc}}
`
