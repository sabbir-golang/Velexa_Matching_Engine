package web

// adminJS drives the console: navigation, dialogs, the status poll and one
// block per page (dashboard, terminal, order report, FIX sessions, market
// session, instruments, brokers, allowlist, users, message log, diagnostics).
const adminJS = `function esc(s){return String(s==null?"":s).replace(/[&<>"'\x60]/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;","\x60":"&#96;"}[c]})}
// jsa makes a value safe as a JavaScript string argument inside an HTML attribute: onclick="f("+jsa(x)+")".
function jsa(s){return esc(JSON.stringify(String(s==null?"":s)))}
function j(p,o){return fetch("/admin/api/"+p,o).then(function(r){return r.json()})}
function jpost(p,b){return j(p,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(b)})}
// jr calls a route outside /admin/api; an error page (plain text) becomes {ok:false,error}.
function jr(u,o){return fetch(u,o).then(function(r){return r.text().then(function(t){try{var d=JSON.parse(t);if(!r.ok&&d&&typeof d=="object")d.ok=false;return d}catch(e){return {ok:false,error:t.trim()||("HTTP "+r.status)}}})})}
function setc(id,t){var e=document.getElementById(id);if(e)e.textContent=t}
function seth(id,h){var e=document.getElementById(id);if(e)e.innerHTML=h}
function val(id){var e=document.getElementById(id);return e?e.value.trim():""}
function flt(id){return val(id).toLowerCase()}
function hit(txt,f){return !f||String(txt).toLowerCase().indexOf(f)>=0}
function fmtN(v,d){return Number(v||0).toLocaleString("en-US",d?{minimumFractionDigits:d,maximumFractionDigits:d}:{})}
function hms(t){return String(t||"").replace("T"," ").slice(11,19)}
function ymdhm(t){return String(t||"").replace("T"," ").slice(0,16)}
function toast(msg,err,warn){
 var el=document.getElementById("toast");
 if(!el){el=document.createElement("div");el.id="toast";el.style.cssText="position:fixed;right:20px;bottom:20px;max-width:420px;padding:12px 16px;border-radius:8px;font-size:13px;line-height:1.4;z-index:9999;box-shadow:0 8px 24px rgba(0,0,0,.3);color:#fff;transition:opacity .2s";document.body.appendChild(el)}
 el.style.background=err?"#b4323c":(warn?"#a8741a":"#1d7a50");
 el.textContent=msg+(warn?" — Warning: "+warn:"");el.style.opacity=1;
 clearTimeout(el._t);el._t=setTimeout(function(){el.style.opacity=0},warn||err?8000:3500)}
// ── dialogs ──
function dlg(html,cls){var c=document.getElementById("dlgc");c.className="dlg"+(cls?" "+cls:"");c.innerHTML=html;document.getElementById("dlg").classList.add("on");
 var f=c.querySelector("input:not([disabled]),textarea");if(f)setTimeout(function(){f.focus()},30)}
function dlgClose(){document.getElementById("dlg").classList.remove("on")}
function dlgErr(m){var e=document.getElementById("dlg_err");if(e)e.textContent=m||""}
// ── combobox: a text box with a filtered list — type to search, arrows + Enter to pick,
// Esc to close; with allowNew, text that matches nothing can be added as a new value.
// The chosen code is in the hidden input <id>; the text box is <id>_in.
var CBX={};
function comboHTML(id,ph){return "<div class=cbx id="+id+"_wrap><input class=in id="+id+"_in autocomplete=off spellcheck=false role=combobox aria-autocomplete=list aria-expanded=false aria-controls="+id+"_pop placeholder=\""+esc(ph||"Type to search…")+"\"><button type=button class=cbx-btn tabindex=-1 aria-label='Show all' id="+id+"_btn>▾</button><input type=hidden id="+id+"><div class=cbx-pop role=listbox id="+id+"_pop></div></div>"}
function comboInit(id,items,value,allowNew,onPick){var c=CBX[id]={items:items,allowNew:allowNew,onPick:onPick,act:0,list:[]},inp=document.getElementById(id+"_in");
 comboSet(id,value);
 inp.addEventListener("focus",function(){comboOpen(id,false)});
 inp.addEventListener("click",function(){comboOpen(id,false)});
 inp.addEventListener("input",function(){document.getElementById(id).value="";comboOpen(id,true);inp.classList.remove("bad");dlgErr("")});
 inp.addEventListener("keydown",function(e){var open=document.getElementById(id+"_pop").classList.contains("on");
  if(e.key=="ArrowDown"||e.key=="ArrowUp"){e.preventDefault();if(!open)return comboOpen(id,false);c.act=Math.max(0,Math.min(c.list.length-1,c.act+(e.key=="ArrowDown"?1:-1)));comboDraw(id)}
  else if(e.key=="Enter"){if(open&&c.list.length){e.preventDefault();comboPick(id,c.act)}}
  else if(e.key=="Escape"&&open){e.preventDefault();e.stopPropagation();comboClose(id)} // close the list, not the dialog
  else if(e.key=="Tab"&&open&&inp.value.trim()&&c.list.length&&!document.getElementById(id).value)comboPick(id,c.act)});
 inp.addEventListener("blur",function(){setTimeout(function(){comboClose(id);comboCommit(id)},120)});
 document.getElementById(id+"_btn").addEventListener("mousedown",function(e){e.preventDefault();var p=document.getElementById(id+"_pop");if(p.classList.contains("on"))comboClose(id);else{inp.focus();comboOpen(id,false)}});
 document.getElementById(id+"_pop").addEventListener("mousedown",function(e){var o=e.target.closest(".cbx-opt");if(!o)return;e.preventDefault();comboPick(id,+o.dataset.i)})}
function comboLabel(id,code){var x=CBX[id].items.filter(function(i){return i[0]==code})[0];return x?x[1]:code}
function comboSet(id,code){document.getElementById(id).value=code||"";document.getElementById(id+"_in").value=code?comboLabel(id,code):""}
function comboOpen(id,filter){var c=CBX[id],q=filter?document.getElementById(id+"_in").value.trim().toLowerCase():"",cur=document.getElementById(id).value;
 c.list=c.items.filter(function(i){return !q||i[1].toLowerCase().indexOf(q)>=0||i[0].toLowerCase().indexOf(q)>=0}).map(function(i){return {code:i[0],label:i[1]}});
 var typed=document.getElementById(id+"_in").value.trim();
 if(c.allowNew&&typed&&!c.items.some(function(i){return i[1].toLowerCase()==typed.toLowerCase()||i[0].toLowerCase()==typed.toLowerCase()}))c.list.push({code:typed,label:typed,isNew:1});
 c.act=Math.max(0,c.list.findIndex(function(o){return o.code==cur}));comboDraw(id)}
function comboDraw(id){var c=CBX[id],p=document.getElementById(id+"_pop"),cur=document.getElementById(id).value;
 p.innerHTML=c.list.map(function(o,i){return "<div class='cbx-opt"+(i==c.act?" act":"")+(o.code==cur&&!o.isNew?" sel":"")+(o.isNew?" new":"")+"' role=option data-i="+i+" aria-selected="+(o.code==cur)+">"+(o.isNew?"＋ Add “"+esc(o.label)+"” as a new sector":"<span>"+esc(o.label)+"</span>"+(o.code!=o.label?"<small>"+esc(o.code)+"</small>":""))+"</div>"}).join("")||"<div class=cbx-empty>No match — keep typing or clear the box.</div>";
 p.classList.add("on");document.getElementById(id+"_in").setAttribute("aria-expanded","true");
 var a=p.querySelector(".act");if(a)a.scrollIntoView({block:"nearest"})}
function comboClose(id){var p=document.getElementById(id+"_pop");if(p)p.classList.remove("on");var i=document.getElementById(id+"_in");if(i)i.setAttribute("aria-expanded","false")}
function comboPick(id,i){var o=CBX[id].list[i];if(!o)return;if(o.isNew)CBX[id].items.push([o.code,o.label]);comboSet(id,o.code);comboClose(id);document.getElementById(id+"_in").classList.remove("bad");dlgErr("");if(CBX[id].onPick)CBX[id].onPick(o.code)}
// leaving the box: an exact name or code picks it; other text stays as typed but picks nothing
// (a new value is only added by choosing "Add …" in the list)
function comboCommit(id){var c=CBX[id];if(!c)return;var inp=document.getElementById(id+"_in"),t=inp.value.trim(),h=document.getElementById(id);if(!t){h.value="";return}
 var x=c.items.filter(function(i){return i[1].toLowerCase()==t.toLowerCase()||i[0].toLowerCase()==t.toLowerCase()})[0];
 if(x)comboSet(id,x[0])}
document.addEventListener("keydown",function(e){if(e.key=="Escape")dlgClose()});
var _confirmFn=null;
// confirmDlg asks before an action; with a reason label the button waits for text.
function confirmDlg(title,body,action,danger,fn,reasonLabel){_confirmFn=fn;
 dlg("<h3>"+esc(title)+"</h3><p>"+esc(body)+"</p>"+(reasonLabel?"<label class=fld>"+esc(reasonLabel)+"<textarea class=in id=dlg_reason rows=2 oninput=\"document.getElementById('dlg_ok').disabled=!this.value.trim()\"></textarea></label>":"")+
  "<span class=err id=dlg_err></span><div class=acts><button class=btn type=button onclick='dlgClose()'>Cancel</button><button class='btn "+(danger?"dng":"pri")+"' id=dlg_ok type=button onclick='confirmGo()'"+(reasonLabel?" disabled":"")+">"+esc(action)+"</button></div>")}
function confirmGo(){var r=val("dlg_reason");if(_confirmFn)_confirmFn(r)}
// ── navigation ──
var TITLES={home:["Welcome","Your engine at a glance"],dash:["Dashboard","Today · whole exchange"],trade:["Trade Terminal","Place and monitor orders against the live book"],orders:["Order Report","Every order received, with its execution history"],
 sessions:["FIX Sessions","Broker connections on the FIX acceptor"],market:["Market Session","Trading phases, schedule and the DSE clock"],instruments:["Instruments","Listed securities and their trading parameters"],
 creds:["Broker Credentials","FIX and ITCH logins issued to member brokers"],access:["OMS IP Allowlist","Networks allowed to reach the order gateway"],users:["Users","Console accounts and roles"],
 sim:["Simulator","Market bots through your OMS, and the FIX benchmark"],msglog:["Message Log","FIX traffic in and out of the engine"],diag:["Diagnostics","Engine health, configuration and self-tests"]};
var VIEW="home";
function show(v){
 if(!TITLES[v])v="home";
 if(ME_ROLE!="admin"&&document.querySelector('.navitem[data-v="'+v+'"][data-admin-only]'))v="home";
 VIEW=v;
 document.querySelectorAll(".navitem").forEach(function(n){n.classList.toggle("on",n.dataset.v==v)});
 document.querySelectorAll(".view").forEach(function(s){s.classList.toggle("on",s.id=="view-"+v)});
 setc("pageTitle",TITLES[v][0]);setc("pageSub",TITLES[v][1]);
 try{history.replaceState(null,"","#"+v)}catch(e){}
 window.scrollTo(0,0);
 if(v=="home")wlLoad();
 if(v=="dash"){ovSub();ovLoad()}
 if(v=="trade")ttOpen();
 if(v=="orders")loadOrders();
 if(v=="sessions"){loadBrokers();renderFix()}
 if(v=="market")renderMarket();
 if(v=="instruments")loadInstr();
 if(v=="creds")loadBrokers();
 if(v=="access")loadAllow();
 if(v=="users")loadUsers();
 if(v=="msglog")mlLoad();
 if(v=="diag")dgLoad();
 if(v=="sim")smLoad();
}
var aside=document.querySelector("aside"),bd=document.getElementById("backdrop");
function toggleNav(open){aside.classList.toggle("open",open);bd.classList.toggle("on",open)}
document.getElementById("hamb").onclick=function(){toggleNav(!aside.classList.contains("open"))};
bd.onclick=function(){toggleNav(false)};
document.querySelectorAll(".navitem").forEach(function(n){n.onclick=function(){show(n.dataset.v);toggleNav(false)}});
window.addEventListener("hashchange",function(){var v=location.hash.slice(1);if(v&&v!=VIEW)show(v)});
// the theme button says which theme it switches to (ThemeHead does the switching)
function themeLabel(){setc("themeBtn",document.documentElement.getAttribute("data-theme")=="light"?"Dark":"Light")}
new MutationObserver(themeLabel).observe(document.documentElement,{attributes:true,attributeFilter:["data-theme"]});themeLabel();
// ── who is signed in ──
var ME_USER="",ME_ROLE="user";
function loadMe(){return j("me").then(function(m){
 ME_USER=m.user||"";ME_ROLE=m.role||"user";
 document.body.classList.toggle("role-user",ME_ROLE!="admin");
 setc("meName",ME_USER);var r=document.getElementById("meRole");r.textContent=ME_ROLE;r.className="rolebadge "+ME_ROLE;
 setc("tt_as","Console ("+ME_USER+")");var a=document.getElementById("tt_as");if(a)a.value="Console ("+ME_USER+")";
 if(ME_ROLE!="admin"&&document.querySelector('.navitem[data-v="'+VIEW+'"][data-admin-only]'))show("home")})}
function pwDlg(){dlg("<h3>Change password</h3><label class=fld>Current password<input class=in type=password id=pw_cur autocomplete=current-password></label>"+
 "<label class=fld>New password<input class=in type=password id=pw_new autocomplete=new-password></label><label class=fld>Confirm new password<input class=in type=password id=pw_cf autocomplete=new-password></label>"+
 "<span class=note>10 to 128 characters, with at least 3 of: lower-case, upper-case, digits, symbols. You stay signed in on this device.</span><span class=err id=dlg_err></span>"+
 "<div class=acts><button class=btn type=button onclick='dlgClose()'>Cancel</button><button class='btn pri' type=button onclick='pwSave()'>Update password</button></div>")}
function pwSave(){var c=val("pw_cur"),n=document.getElementById("pw_new").value,f=document.getElementById("pw_cf").value;
 if(!c)return dlgErr("Enter your current password.");if(n.length<10)return dlgErr("The new password must be at least 10 characters.");if(n!=f)return dlgErr("The two new passwords do not match.");
 jpost("me/password",{Old:c,New:n}).then(function(x){if(!x.ok)return dlgErr(x.error||"The password was not changed.");dlgClose();toast("Password updated")})}
// ── status (every 3 s): header pill, sidebar, FIX sessions, market session, diagnostics ──
var _s=null;
function refresh(){return j("status").then(function(d){_s=d;renderStatus(d);if(VIEW=="home")wlRender()})}
function renderStatus(d){
 if(!d)return;
 var sv="";if(d.state_saved=="off")sv=" · market not saved";else if(d.state_age>=0)sv=d.state_age>10?" · ! market last saved "+Math.round(d.state_age)+" s ago":" · market saved";
 var q=0;for(var qb in (d.fix_queued||{}))q+=d.fix_queued[qb];if(q)sv+=" · "+q+" report"+(q>1?"s":"")+" waiting";
 setc("up","uptime "+d.uptime+sv);
 document.querySelector(".top .live").title=d.state_saved&&d.state_saved!="off"?"Order books, prices and trades last saved "+d.state_saved:"";
 setc("enText",d.running?"engine live":"engine halted");document.getElementById("enDot").style.background=d.running?"var(--acc)":"var(--red)";
 setc("navSess",(d.sessions||[]).length||"");
 TITLES.sessions[1]="Broker connections on FIX :"+(d.fix_port||"5001");if(VIEW=="sessions")setc("pageSub",TITLES.sessions[1]);
 if(VIEW=="sessions")renderFix();
 if(VIEW=="creds")renderBrokers(_b);
 if(VIEW=="market")renderMarket();
 if(VIEW=="diag")renderDiag();
}

// Dashboard: /admin/api/overview — the session clock, the day's totals, the
// intraday chart, order outcomes, instruments, activity, brokers, listeners.
var OV=null,_ovPaused=false,_ovMetric="turnover",_ovSort=["turnover",-1],_ovFilter="all",_ovAll=false,_ovLast={},_ovFlash={};
function ovDate(){return document.getElementById("ov_date").value||""}
function repQ(){var d=ovDate();return d?"date="+encodeURIComponent(d):""}
function repCSV(){window.location.href="/admin/api/report/daily?format=csv&"+repQ()}
function repPrint(){window.open("/admin/report/daily?"+repQ(),"_blank")}
function ovToday(){document.getElementById("ov_date").value="";ovLoad()}
function ovPause(){_ovPaused=!_ovPaused;setc("ov_pause",_ovPaused?"Resume live":"Pause live");ovSub();if(!_ovPaused)ovLoad()}
function ovLoad(){j("overview?"+repQ()).then(function(d){
 if(!d.ok){toast(d.error||"The dashboard could not be loaded",true);return}
 var el=document.getElementById("ov_date");
 if(d.today){el.max=d.date;if(!el.value)el.value=d.date}
 document.getElementById("ov_today").style.display=d.today?"none":"";
 OV=d;ovRender()})}
function nf(v,dg){return Number(v||0).toLocaleString("en-US",{minimumFractionDigits:dg||0,maximumFractionDigits:dg||0})}
// Taka in the Bangladeshi way: crore (10 million) and lakh (100 thousand).
function tk(v){v=Number(v||0);if(v>=1e7)return (v/1e7).toFixed(2)+" Cr";if(v>=1e5)return (v/1e5).toFixed(2)+" Lakh";return nf(v)}
function mins(hm){var p=String(hm||"0:0").split(":");return (+p[0])*60+(+p[1])}
function hm(m){m=Math.round(m);return ("0"+Math.floor(m/60)).slice(-2)+":"+("0"+(m%60)).slice(-2)}
function dhakaNow(){var t=new Date().toLocaleTimeString("en-GB",{timeZone:"Asia/Dhaka",hour12:false});return t.length>=8?t.slice(0,8):(OV&&OV.now)||""}
function ovSub(){if(!OV)return;var who=OV.scope=="own"?"Your brokers ("+(OV.my_brokers||0)+")":"Whole exchange";
 setc("ov_cap",(OV.today?"Today":OV.weekday)+" · "+who+" · "+(OV.today?(_ovPaused?"paused":"live"):"final"));
 TITLES.dash[1]=(OV.today?"Today":OV.weekday)+" · "+who.toLowerCase();if(VIEW=="dash")setc("pageSub",TITLES.dash[1])}
function ovSession(){if(!OV)return;var d=OV,sc=d.schedule,pre=mins(sc.pre_open),op=mins(sc.open),cl=mins(sc.close),po=mins(sc.post_close);
 var t=dhakaNow(),m=mins(t)+(+(t.split(":")[2]||0))/60,note="";
 if(!d.today){setc("ov_phase","Day's totals");setc("ov_clock","");note="Trading day "+d.date+(d.has_data?"":" · nothing recorded")}
 else{setc("ov_phase",d.phase);setc("ov_clock",t);
  if(!d.trading_day)note="No DSE trading today (weekend)";
  else if(!d.auto_clock)note="Phases changed by hand (clock off)";
  else if(m<pre)note="Pre-open at "+sc.pre_open;
  else if(m<op)note="Opens in "+Math.ceil(op-m)+"m";
  else if(m<cl){var l=cl-m;note="Closes in "+Math.floor(l/60)+"h "+Math.floor(l%60)+"m"}
  else if(m<po)note="Post-close until "+sc.post_close;
  else note="Trading ended at "+sc.close}
 setc("ov_note",note);
 var cur=!d.today?"":(m>=pre&&m<op?"pre":m>=op&&m<cl?"cont":m>=cl&&m<po?"post":"");
 var seg=function(k,w,c,r){return "<div class='sg"+(cur==k?" cur":"")+"' style='flex:"+w+";background:"+c+";border-radius:"+r+"'></div>"};
 var pos=Math.max(0,Math.min(100,(m-pre)/(po-pre)*100));
 document.getElementById("ov_tl").innerHTML=seg("pre",op-pre,"var(--mut2)","4px 0 0 4px")+seg("cont",cl-op,"var(--acc)","0")+seg("post",po-cl,"var(--mut2)","0 4px 4px 0")+
  (d.today&&d.trading_day?"<div class=mk style='left:"+pos.toFixed(2)+"%'></div>":"");
 var tw=document.getElementById("ov_tl").clientWidth||800,tg=Math.max(20,(po-pre)*46/tw); // keep labels about 46px apart
 var ticks=[[pre,sc.pre_open]];for(var h=Math.floor(pre/60)+1;h*60<po;h++){if(h*60-ticks[ticks.length-1][0]>=tg&&po-h*60>=tg)ticks.push([h*60,hm(h*60)])}ticks.push([po,sc.post_close]);
 document.getElementById("ov_tlx").innerHTML=ticks.map(function(x){return "<span style='left:"+((x[0]-pre)/(po-pre)*100).toFixed(2)+"%'>"+esc(x[1])+"</span>"}).join("");
 document.getElementById("ov_legend").innerHTML="<span><i style='background:var(--mut2);opacity:.5'></i>Pre-open "+esc(sc.pre_open)+"–"+esc(sc.open)+"</span><span><i style='background:var(--acc);opacity:.5'></i>Continuous trading "+esc(sc.open)+"–"+esc(sc.close)+"</span><span><i style='background:var(--mut2);opacity:.5'></i>Post-close "+esc(sc.close)+"–"+esc(sc.post_close)+"</span>"}
function ovDelta(id,cur,prev,key){var e=document.getElementById(id);if(!prev||!prev.totals){e.textContent="";return}
 var pv=key=="orders"?(prev.totals.buy_orders+prev.totals.sell_orders):prev.totals[key];if(!pv){e.textContent="";return}
 var dd=cur/pv-1,lbl=prev.date.slice(8)+" "+["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"][+prev.date.slice(5,7)-1];
 e.textContent=(dd>=0?"▲ ":"▼ ")+Math.abs(dd*100).toFixed(1)+"% vs "+lbl;e.style.color=dd>=0?"var(--acc)":"var(--red)";e.title=lbl+": "+nf(pv,key=="turnover"?2:0)+" (whole day)"}
function ovRender(){if(!OV)return;var d=OV,t=d.totals||{},orders=(t.buy_orders||0)+(t.sell_orders||0);
 ovSub();ovSession();
 // KPIs
 setc("k_to","Tk "+tk(t.turnover));setc("k_to_s","Tk "+nf(t.turnover,2));ovDelta("k_to_d",t.turnover,d.prev,"turnover");
 setc("k_vol",nf(t.volume));setc("k_vol_s",t.trades?"avg "+nf(t.volume/t.trades)+" shares / trade":"no trades yet");ovDelta("k_vol_d",t.volume,d.prev,"volume");
 // the rate over the time trading actually happened: first trade to the last (or to now while the market is open)
 var span=0,live=d.today&&/Pre-Open|Continuous/.test(d.phase);
 if(d.first_trade){span=(live?mins(dhakaNow())+(+dhakaNow().slice(6,8)||0)/60:mins(d.last_trade)+(+d.last_trade.slice(6,8))/60)-(mins(d.first_trade)+(+d.first_trade.slice(6,8))/60);span=Math.max(1,span)}
 var rate=t.trades&&span?t.trades/span:0;
 setc("k_tr",nf(t.trades));setc("k_tr_s",!rate?"no trades yet":rate>=1?rate.toFixed(1)+" per minute":(rate*60).toFixed(1)+" per hour");
 document.getElementById("k_tr_s").title=d.first_trade?"from "+d.first_trade+" to "+(live?"now":d.last_trade):"";ovDelta("k_tr_d",t.trades,d.prev,"trades");
 setc("k_or",nf(orders));setc("k_or_s","across "+nf(d.instruments_listed)+" instruments");ovDelta("k_or_d",orders,d.prev,"orders");
 // intraday chart
 var B=d.buckets||[],mk=_ovMetric,max=1;B.forEach(function(b){if(b[mk]>max)max=b[mk]});
 setc("ch_cap","Intraday · "+d.bucket_minutes+"-min buckets");
 var fmtv=function(v){return mk=="turnover"?"Tk "+tk(v):nf(v)+(mk=="trades"?" trades":" shares")};
 var peak=0;B.forEach(function(b){if(b[mk]>peak)peak=b[mk]});setc("ch_peak",peak?"Peak "+fmtv(peak):"No trades yet");
 document.getElementById("ch_bars").innerHTML=B.map(function(b,i){var fut=d.today&&i>d.now_bucket,cur=d.today&&i==d.now_bucket;
  var h=fut?2:Math.max(b[mk]?3:1,b[mk]/max*100);
  return "<div class='"+(fut?"fut":cur?"cur":"")+"' title='"+esc(b.start+" · "+fmtv(b[mk]))+"'><i style='height:"+h.toFixed(1)+"%'></i></div>"}).join("");
 var xs=[],n=B.length,end=mins(d.buckets_end),beg=n?mins(B[0].start):0,span=Math.max(1,end-beg);
 var gap=Math.max(span*0.07,span*46/(document.getElementById("ch_bars").clientWidth||600)); // labels closer than this would overlap
 B.forEach(function(b){var m=mins(b.start);if((m%60==0||m==beg)&&(!xs.length||m-xs[xs.length-1][0]>=gap))xs.push([m,b.start])});
 while(xs.length&&end-xs[xs.length-1][0]<gap)xs.pop();xs.push([end,d.buckets_end]);
 document.getElementById("ch_x").innerHTML=xs.map(function(x){return "<span style='left:"+((x[0]-beg)/span*100).toFixed(2)+"%'>"+esc(x[1])+"</span>"}).join("");
 // order outcomes
 var o=d.outcomes||{},R=0,OC=[["Filled","filled","var(--acc)"],["Working","working","var(--blue)"],["Cancelled","cancelled","var(--amber)"],["Rejected","rejected","var(--red)"],["Expired","expired","var(--mut2)"]];
 OC.forEach(function(x){R+=o[x[1]]||0});var pc=function(v){return R?(v/R*100):0};
 setc("oc_rate",R?pc(o.filled).toFixed(1)+"%":"—");
 document.getElementById("oc_stack").innerHTML=OC.map(function(x){return o[x[1]]?"<div style='width:"+pc(o[x[1]]).toFixed(2)+"%;background:"+x[2]+"'></div>":""}).join("");
 document.getElementById("oc_list").innerHTML=OC.map(function(x){return "<div class=ocl><i style='background:"+x[2]+"'></i><span class=muted>"+x[0]+"</span><span class=dbm>"+nf(o[x[1]])+"</span><span class='dbm faint' style='text-align:right'>"+pc(o[x[1]]).toFixed(1)+"%</span></div>"}).join("");
 var bp=orders?Math.round(t.buy_orders/orders*100):50;
 setc("oc_buy","Buy "+(orders?bp+"%":"—"));setc("oc_sell","Sell "+(orders?(100-bp)+"%":"—"));setc("oc_amend",nf(t.amended)+" amended");
 document.getElementById("oc_buybar").style.width=bp+"%";
 // instruments
 // the DSE "Latest Share Price" columns; CHANGE = LTP - YCP in taka, with the percentage under it
 var cols=[["symbol","Trading code",""],["last","LTP","rt"],["high","High","rt"],["low","Low","rt"],["ycp","YCP","rt"],["change","Change","rt"],["trades","Trade","rt"],["turnover","Value (mn)","rt"],["volume","Volume","rt"]];
 document.getElementById("it_h").innerHTML=cols.map(function(c){return "<span class='"+c[2]+"' onclick=\"ovSortBy('"+c[0]+"')\">"+c[1].toUpperCase()+(_ovSort[0]==c[0]?(_ovSort[1]>0?" ↑":" ↓"):"")+"</span>"}).join("");
 var rows=(d.instruments||[]).slice(),k=_ovSort[0],dir=_ovSort[1];
 rows.sort(function(a,b){var x=a[k],y=b[k];if(k=="symbol")return String(x).localeCompare(String(y))*dir;
  if(x==null&&y==null)return a.symbol.localeCompare(b.symbol);if(x==null)return 1;if(y==null)return -1; // values a day lacks go last
  return (x-y)*dir||a.symbol.localeCompare(b.symbol)});
 var stamp=Date.now();rows.forEach(function(r){var key=r.last+"/"+r.trades,prev=_ovLast[r.symbol];if(prev!==undefined&&prev!==key)_ovFlash[r.symbol]=stamp+900;_ovLast[r.symbol]=key});
 var lim=_ovAll?rows.length:12,shown=rows.slice(0,lim),px=function(v){return v==null?"<span class=faint>—</span>":nf(v,2)};
 document.getElementById("it_rows").innerHTML=shown.map(function(r){var c=r.change,col=c==null||c==0?"var(--mut2)":c>0?"var(--acc)":"var(--red)";
  var chg=c==null?"—":(c>0?"+":"")+nf(c,2)+"<small>"+(r.change_pct>0?"+":"")+Number(r.change_pct||0).toFixed(2)+"%</small>";
  return "<div class='g r"+(_ovFlash[r.symbol]>stamp?" fl":"")+"'><b>"+esc(r.symbol)+"</b><span class=rt>"+nf(r.last,2)+"</span><span class=rt>"+px(r.high)+"</span><span class=rt>"+px(r.low)+"</span><span class='rt muted'>"+px(r.ycp)+"</span><span class=rt style='color:"+col+"'>"+chg+"</span><span class='rt muted'>"+nf(r.trades)+"</span><span class=rt title='Tk "+nf(r.turnover,2)+"'>"+(r.turnover/1e6).toFixed(3)+"</span><span class='rt muted'>"+nf(r.volume)+"</span></div>"}).join("")||"<div class=dbempty>No instruments"+(d.today?" listed.":" traded on this day.")+"</div>";
 setTimeout(function(){document.querySelectorAll("#it_rows .r.fl").forEach(function(e){e.classList.remove("fl")})},900);
 var mb=document.getElementById("it_more");mb.style.display=rows.length>12?"":"none";mb.textContent=_ovAll?"Show the top 12":"Show all "+rows.length+" instruments";
 // activity
 var F=(d.feed||[]).filter(function(e){return _ovFilter=="all"||e.kind==_ovFilter}).slice(0,10);
 document.getElementById("fd_rows").innerHTML=F.map(function(e){return "<div class=feedr><span class='dbm faint' style='font-size:12px'>"+esc(e.time)+"</span><div style='display:flex;flex-direction:column;min-width:0'><span class=x title='"+esc(e.text)+"'>"+esc(e.text)+"</span><span class=y>"+esc(e.broker)+"</span></div><span class='tag "+esc(e.tag)+"'>"+esc(e.tag)+"</span></div>"}).join("")||"<div class=dbempty>No activity yet.</div>";
 // brokers
 var BR=(d.brokers||[]).slice(0,8),maxB=0;BR.forEach(function(b){if(b.turnover>maxB)maxB=b.turnover});
 setc("br_cap",d.scope=="own"?"Your brokers by turnover":"Brokers by turnover");
 document.getElementById("br_rows").innerHTML=BR.map(function(b){var meta=nf(b.orders)+" order"+(b.orders==1?"":"s")+" · "+nf(b.trades)+" fill"+(b.trades==1?"":"s")+(b.fix&&d.today?" · "+(b.online?"<span style='color:var(--acc)'>online</span>":"offline"):"");
  return "<div class=brk><div style='display:flex;flex-direction:column;min-width:0'><span class=n title='"+esc(b.id)+"'>"+esc(b.name)+"</span><span class=faint style='font-size:12px'>"+meta+"</span></div><div class=bb><i style='width:"+(maxB?b.turnover/maxB*100:0).toFixed(1)+"%'></i></div><span class=dbm style='text-align:right'>"+tk(b.turnover)+"</span></div>"}).join("")||"<div class=dbempty>No broker activity"+(d.today?" yet.":" on this day.")+"</div>";
 // connectivity
 setc("cn_sess",d.sessions||0);
 document.getElementById("cn_rows").innerHTML=(d.listeners||[]).map(function(l){return "<div class=lst><div style='display:flex;flex-direction:column'><span class=dbm style='font-size:13px'>"+esc(l.name)+"</span><span class=faint style='font-size:12px'>"+esc(l.desc)+"</span></div><span class='upc"+(l.up?"":" dn")+"'><i></i>"+(l.up?"UP":"DOWN")+"</span></div>"}).join("");
 var saved=_s&&_s.state_saved&&_s.state_saved!="off"&&_s.state_saved!="not yet"?" · Market saved "+_s.state_saved.slice(11):"";
 document.getElementById("db_foot").textContent="Turnover in taka (Cr = crore, Lakh = 100,000); the instrument table's value is in million taka, as on the DSE. A broker's turnover counts its buys and sells."+(d.today?" Updates every 3 s"+(_ovPaused?" (paused)":"")+saved+".":"")}
function ovSortBy(k){_ovSort=[k,_ovSort[0]==k?-_ovSort[1]:(k=="symbol"?1:-1)];ovRender()}
document.getElementById("ch_seg").onclick=function(e){var b=e.target.closest("button");if(!b)return;_ovMetric=b.dataset.m;this.querySelectorAll("button").forEach(function(x){x.classList.toggle("on",x==b)});ovRender()};
var _ovRT;window.addEventListener("resize",function(){clearTimeout(_ovRT);_ovRT=setTimeout(ovRender,150)}); // label spacing follows the width
document.getElementById("fd_seg").onclick=function(e){var b=e.target.closest("button");if(!b)return;_ovFilter=b.dataset.f;this.querySelectorAll("button").forEach(function(x){x.classList.toggle("on",x==b)});ovRender()};

// ── Trade Terminal: symbols (/api/market), book and tape (/api/book), order entry (/api/order), own orders (order report) ──
var TT={sym:"",side:"BUY",rows:[],book:null,mine:[]};
function ttOpen(){var a=document.getElementById("tt_as");if(a)a.value="Console ("+ME_USER+")";ttMarket();ttMine()}
function ttMarket(){return jr("/api/market").then(function(d){TT.rows=(d&&d.rows)||[];
 if(!TT.sym||!TT.rows.some(function(r){return r.symbol==TT.sym})){var t=TT.rows.filter(function(r){return r.trades>0})[0]||TT.rows[0];TT.sym=t?t.symbol:""}
 ttStrip();ttBook()})}
function ttChg(r){return r.prev>0?(r.last-r.prev)/r.prev*100:0}
// ── Market watch: every instrument with its price, volume and best bid / ask ──
var MW={cls:"",sort:["",1],page:0,last:{},flash:{}},MWCLS={EQ:"Equity",MF:"Mutual fund",CB:"Corporate bond",DB:"Debenture"};
var MWCOLS=[["symbol","Symbol",""],["board","Board",""],["asset_class","Class",""],["category","Cat",""],["last","Last","r"],["change_pct","Chg%","r"],["volume","Volume","r"],["bid","Bid","r bid"],["bid_qty","Bid qty","r bid"],["ask","Ask","r ask"],["ask_qty","Ask qty","r ask"]];
function secLabel(c){var x=SECTORS.filter(function(s){return s[0]==c})[0];return x?x[1]:c}
function mwFill(id,vals,label,all){var e=document.getElementById(id),cur=e.value;e.innerHTML="<option value=''>"+all+"</option>"+vals.map(function(v){return "<option value=\""+esc(v)+"\">"+esc(label(v))+"</option>"}).join("");e.value=vals.indexOf(cur)>=0?cur:""}
function mwDistinct(k){var m={};TT.rows.forEach(function(r){if(r[k])m[r[k]]=1});return Object.keys(m).sort()}
function mwRender(reset){if(reset)MW.page=0;
 mwFill("mw_board",mwDistinct("board"),function(v){return v},"All boards");
 mwFill("mw_sector",mwDistinct("sector"),secLabel,"All sectors");
 mwFill("mw_cat",mwDistinct("category"),function(v){return "Category "+v},"All categories");
 mwFill("mw_status",mwDistinct("status"),function(v){return v.charAt(0).toUpperCase()+v.slice(1)},"All statuses");
 var q=flt("mw_q"),bd=val("mw_board"),sec=val("mw_sector"),cat=val("mw_cat"),st=val("mw_status");
 var rows=TT.rows.filter(function(r){return (!bd||r.board==bd)&&(!sec||r.sector==sec)&&(!cat||r.category==cat)&&(!st||r.status==st)&&
  (!MW.cls||(MW.cls=="BOND"?(r.asset_class=="CB"||r.asset_class=="DB"):r.asset_class==MW.cls))&&hit(r.symbol+" "+(r.name||""),q)});
 var k=MW.sort[0],dir=MW.sort[1];
 if(k)rows.sort(function(a,b){var x=a[k],y=b[k];return (typeof x=="string"?String(x).localeCompare(String(y)):(x||0)-(y||0))*dir||a.symbol.localeCompare(b.symbol)});
 else rows.sort(function(a,b){return (b.trades>0)-(a.trades>0)||a.symbol.localeCompare(b.symbol)}); // traded first
 var per=+val("mw_per")||25,pages=Math.max(1,Math.ceil(rows.length/per));if(MW.page>pages-1)MW.page=pages-1;
 seth("mw_head",MWCOLS.map(function(c){return "<th class='so "+c[2]+"' onclick=\"mwSort('"+c[0]+"')\">"+c[1]+(k==c[0]?(dir>0?" ↑":" ↓"):"")+"</th>"}).join(""));
 var now=Date.now(),px=function(v,col){return v>0?"<span style='color:"+col+"'>"+fmtN(v,2)+"</span>":"<span class=faint>—</span>"};
 TT.rows.forEach(function(r){var key=r.last+"/"+r.volume+"/"+r.bid+"/"+r.ask,p=MW.last[r.symbol];if(p!==undefined&&p!==key)MW.flash[r.symbol]=now+900;MW.last[r.symbol]=key});
 seth("mw_rows",rows.slice(MW.page*per,MW.page*per+per).map(function(r){var c=r.change_pct||0,cc=c>0?"var(--acc)":c<0?"var(--red)":"var(--mut2)";
  return "<tr class='"+(r.symbol==TT.sym?"sel ":"")+(MW.flash[r.symbol]>now?"fl":"")+"' onclick=\"ttPick("+jsa(r.symbol)+")\" title=\""+esc(r.name||r.symbol)+(r.sector?" · "+esc(secLabel(r.sector)):"")+"\">"+
   "<td><b>"+esc(r.symbol)+"</b>"+(r.status&&r.status!="active"?" <span class='tag "+(r.status=="halted"?"t-red":"t-amber")+"'>"+esc(r.status.toUpperCase())+"</span>":"")+"</td><td class=muted>"+esc(r.board)+"</td><td class=muted>"+esc(MWCLS[r.asset_class]||r.asset_class||"")+"</td><td class=muted>"+esc(r.category||"—")+"</td>"+
   "<td class='r m' style='color:"+(c?cc:"var(--ink)")+"'>"+fmtN(r.last,2)+"</td><td class='r m' style='color:"+cc+"'>"+(c>0?"▲ ":c<0?"▼ ":"")+Math.abs(c).toFixed(2)+"%</td><td class='r m'>"+fmtN(r.volume)+"</td>"+
   "<td class='r m bid'>"+px(r.bid,"var(--acc)")+"</td><td class='r m bid'>"+fmtN(r.bid_qty)+"</td><td class='r m ask'>"+px(r.ask,"var(--red)")+"</td><td class='r m ask'>"+fmtN(r.ask_qty)+"</td></tr>"}).join("")||"<tr><td colspan=11 class=empty>No instruments match these filters.</td></tr>");
 setTimeout(function(){document.querySelectorAll("#mw_rows tr.fl").forEach(function(e){e.classList.remove("fl")})},900);
 setc("mw_count",rows.length+" / "+TT.rows.length+" · live");
 setc("mw_range",rows.length?(MW.page*per+1)+"–"+Math.min(rows.length,MW.page*per+per)+" of "+rows.length:"0 of 0");
 setc("mw_pg",(MW.page+1)+" / "+pages);document.getElementById("mw_prev").disabled=MW.page==0;document.getElementById("mw_next").disabled=MW.page>=pages-1}
function mwSort(k){MW.sort=[k,MW.sort[0]==k?-MW.sort[1]:(k=="symbol"||k=="board"||k=="asset_class"||k=="category"?1:-1)];mwRender(false)}
function mwPage(d){MW.page+=d;mwRender(false)}
function ttStrip(){mwRender(false)}
document.getElementById("mw_class").onclick=function(e){var b=e.target.closest("button");if(!b)return;MW.cls=b.dataset.c;this.querySelectorAll("button").forEach(function(x){x.classList.toggle("on",x==b)});mwRender(true)};
function ttPick(s){TT.sym=s;document.getElementById("tt_px").value="";setc("tt_msg","");mwRender(false);ttBook()}
function ttBook(){if(!TT.sym)return;var sym=TT.sym;
 jr("/api/book?symbol="+encodeURIComponent(sym)).then(function(b){if(sym!=TT.sym||!b||b.error)return;TT.book=b;var md=b.md||{};
  ["tt_sym","tt_sym2","tt_sym3"].forEach(function(i){setc(i,sym)});
  var c=md.prev>0?(md.last-md.prev)/md.prev*100:0;
  setc("tt_last",fmtN(md.last,2));setc("tt_last2",fmtN(md.last,2));
  var ce=document.getElementById("tt_chg");ce.textContent=(c>0?"+":"")+c.toFixed(2)+"%";ce.style.color=c>0?"var(--acc)":c<0?"var(--red)":"var(--mut2)";
  var A=b.asks||[],B=b.bids||[],mx=1;A.concat(B).forEach(function(l){if(l.qty>mx)mx=l.qty});
  var lv=function(l,col,bg){return "<div class=lv><i style='width:"+(l.qty/mx*100).toFixed(1)+"%;background:"+bg+"'></i><span style='color:"+col+"'>"+fmtN(l.price,2)+"</span><span style='text-align:right'>"+fmtN(l.qty)+"</span><span style='text-align:right;color:var(--mut2)'>"+(l.orders||"")+"</span></div>"};
  seth("tt_asks",A.slice(0,8).reverse().map(function(l){return lv(l,"var(--red)","var(--sellSoft)")}).join("")||"<div class=lv><span class=faint>No offers</span></div>");
  seth("tt_bids",B.slice(0,8).map(function(l){return lv(l,"var(--acc)","var(--accSoft)")}).join("")||"<div class=lv><span class=faint>No bids</span></div>");
  setc("tt_spread",A[0]&&B[0]?"Spread "+(A[0].price-B[0].price).toFixed(2):"");
  setc("tt_mid",A[0]&&B[0]?"Mid "+((A[0].price+B[0].price)/2).toFixed(2):"");
  setc("tt_hl",md.trades?"H "+fmtN(md.high,2)+" · L "+fmtN(md.low,2):"");
  seth("tt_tape",(b.trades||[]).slice(0,14).map(function(t){return "<div class=tr3><span style='color:var(--mut2);font-size:12px'>"+esc(hms(t.time))+"</span><span style='text-align:right;color:"+(t.aggressor=="SELL"?"var(--red)":"var(--acc)")+"'>"+fmtN(t.price,2)+"</span><span style='text-align:right'>"+fmtN(t.qty)+"</span></div>"}).join("")||"<div style='padding:8px 16px' class=faint>No trades yet in "+esc(sym)+".</div>");
  ttForm()})}
function ttSide(s){TT.side=s;document.getElementById("tt_buy").classList.toggle("on",s=="BUY");document.getElementById("tt_sell").classList.toggle("on",s=="SELL");ttForm()}
function ttBest(){var b=TT.book||{};var l=TT.side=="BUY"?(b.asks||[])[0]:(b.bids||[])[0];return l?l.price:((b.md||{}).last||0)}
function ttForm(){var mkt=val("tt_type")=="MARKET",px=document.getElementById("tt_px"),qty=parseInt(val("tt_qty"),10)||0;
 px.disabled=mkt;if(mkt)px.value="";var best=ttBest();px.placeholder=mkt?"Best available":(best?best.toFixed(2):"");
 var p=mkt?best:(parseFloat(px.value)||0);setc("tt_est",qty&&p?"Tk "+fmtN(qty*p,2):"—");
 var go=document.getElementById("tt_go");go.textContent=(TT.side=="BUY"?"Buy ":"Sell ")+(qty?fmtN(qty)+" ":"")+(TT.sym||"");go.className="btn "+(TT.side=="BUY"?"pri":"dng")}
function ttSay(m,bad){var e=document.getElementById("tt_msg");e.textContent=m;e.style.color=bad?"var(--red)":"var(--mut)"}
function ttSubmit(){var mkt=val("tt_type")=="MARKET",qty=parseInt(val("tt_qty"),10),px=parseFloat(val("tt_px"));
 if(!TT.sym)return ttSay("Pick a symbol first.",true);
 if(!(qty>0))return ttSay("Enter a quantity greater than zero.",true);
 if(!mkt&&!(px>0))return ttSay("Enter a limit price.",true);
 var cl="T"+Date.now().toString(36).toUpperCase(),btn=document.getElementById("tt_go");btn.disabled=true;
 jr("/api/order",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({symbol:TT.sym,side:TT.side,type:mkt?"MARKET":"LIMIT",price:mkt?0:px,qty:qty,tif:val("tt_tif"),cl_ord_id:cl})}).then(function(r){
  btn.disabled=false;
  if(r.ok===false||r.status=="REJECTED")return ttSay("Rejected: "+(r.reason||r.error||"refused by the engine"),true);
  var f=r.filled_qty||0,lv=r.leaves_qty||0,id=r.order_id?"#"+r.order_id:"Order";
  ttSay(r.status=="FILLED"?id+" filled "+fmtN(f)+" @ "+fmtN(r.avg_px,2)+".":r.status=="CANCELED"?id+": "+fmtN(f)+" filled, the rest cancelled ("+val("tt_tif")+").":f>0?id+": "+fmtN(f)+" filled, "+fmtN(lv)+" resting.":id+" accepted, resting at "+fmtN(px,2)+".");
  ttBook();ttMine();ttMarket()})}
function ttMine(){if(!ME_USER)return;j("report/orders?broker="+encodeURIComponent("USER:"+ME_USER)).then(function(r){TT.mine=(r&&r.rows)||[];
 setc("tt_mine_n",TT.mine.length+" order"+(TT.mine.length==1?"":"s")+" today");
 seth("tt_mine",TT.mine.slice(0,50).map(function(o){var st=ORST[o.status]||[o.status,"t-mut"],work=(o.status=="New"||o.status=="Partial")&&o.cl_ord_id;
  return "<tr><td class=m>"+esc(o.order_id||"—")+"</td><td class='m faint'>"+esc(hms(o.time))+"</td><td style='font-weight:600;color:"+(o.side=="Buy"?"var(--acc)":"var(--red)")+"'>"+esc(o.side)+"</td><td><b>"+esc(o.symbol)+"</b></td><td class='r m'>"+fmtN(o.qty)+"</td><td class='r m'>"+fmtN(o.filled)+"</td><td class='r m'>"+(o.price?fmtN(o.price,2):"MKT")+"</td><td class=muted>"+esc(o.tif||"")+"</td><td><span class='tag "+st[1]+"'>"+esc(st[0])+"</span></td><td class=r>"+(work?"<button class='btn sm red' onclick=\"ttCancel("+jsa(o.cl_ord_id)+")\">Cancel</button>":"")+"</td></tr>"}).join("")||"<tr><td colspan=10 class=empty>No orders yet. Orders you submit above appear here with their fills.</td></tr>")})}
function ttCancel(cl){jr("/api/order/cancel",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({cl_ord_id:cl})}).then(function(r){
 if(!r.ok)return toast(r.error||"Not cancelled",true);toast("Order cancelled");ttMine();ttBook()})}

// ── Order Report: the day's orders, filtered here; details and fills on the right ──
var ORST={Filled:["Filled","t-acc","var(--acc)"],Partial:["Partially filled","t-blue","var(--blue)"],New:["Working","t-blue","var(--blue)"],Pending:["Stop (waiting)","t-blue","var(--blue)"],
 Cancelled:["Cancelled","t-amber","var(--amber)"],Rejected:["Rejected","t-red","var(--red)"],Expired:["Expired","t-mut","var(--mut2)"]};
var ORCHIPS=[["","All","var(--ink)"],["Filled","Filled","var(--acc)"],["Partial","Partially filled","var(--blue)"],["Working","Working","var(--blue)"],["Cancelled","Cancelled","var(--amber)"],["Rejected","Rejected","var(--red)"],["Expired","Expired","var(--mut2)"]];
var OR={rows:[],chip:"",page:0,sel:"",date:"",today:true},ORPER=15;
function orKey(o){return o.broker_id+"|"+(o.order_id||o.cl_ord_id)+"|"+o.time}
function loadOrders(){var d=val("or_date");j("report/orders"+(d?"?date="+encodeURIComponent(d):"")).then(function(r){
 if(!r.ok){seth("orders","<tr><td colspan=9 class=empty>"+esc(r.error||"Not available")+"</td></tr>");OR.today=false;return}
 var de=document.getElementById("or_date");if(r.today){de.max=r.date;if(!de.value)de.value=r.date}
 OR.rows=r.rows||[];OR.date=r.date;OR.today=!!r.today;
 var bs=document.getElementById("or_broker"),cur=bs.value;
 bs.innerHTML="<option value=''>All brokers</option>"+(r.brokers||[]).map(function(b){return "<option value='"+esc(b.id)+"'>"+esc(b.label)+"</option>"}).join("");bs.value=cur;
 orRender(false)})}
function orBase(){var q=flt("or_q"),side=val("or_side"),br=val("or_broker");
 return OR.rows.filter(function(o){return (!side||o.side==side)&&(!br||o.broker_id==br)&&hit([o.order_id,o.cl_ord_id,o.symbol,o.client,o.account].join(" "),q)})}
function orChipHit(o,c){return !c||(c=="Working"?(o.status=="New"||o.status=="Pending"):o.status==c)}
function orRender(reset){if(reset)OR.page=0;
 var base=orBase(),list=base.filter(function(o){return orChipHit(o,OR.chip)}),pages=Math.max(1,Math.ceil(list.length/ORPER));
 if(OR.page>pages-1)OR.page=pages-1;
 seth("or_chips",ORCHIPS.map(function(c){var n=base.filter(function(o){return orChipHit(o,c[0])}).length;
  return "<button class='pchip"+(OR.chip==c[0]?" on":"")+"' type=button onclick=\"OR.chip="+jsa(c[0])+";orRender(true)\"><i style='background:"+c[2]+"'></i><span class=muted>"+c[1]+"</span><b>"+fmtN(n)+"</b></button>"}).join(""));
 var page=list.slice(OR.page*ORPER,OR.page*ORPER+ORPER);
 seth("orders",page.map(function(o){var st=ORST[o.status]||[o.status,"t-mut"];
  return "<tr class='ck"+(orKey(o)==OR.sel?" sel":"")+"' onclick=\"orOpen("+jsa(orKey(o))+")\"><td class='m muted'>"+esc(o.order_id||"—")+"</td><td class='m faint' style='font-size:12px'>"+esc(hms(o.time))+"</td><td class=ell>"+esc(o.broker)+"</td><td><b>"+esc(o.symbol)+"</b></td><td style='font-weight:600;color:"+(o.side=="Buy"?"var(--acc)":"var(--red)")+"'>"+esc(o.side)+"</td><td class='r m'>"+fmtN(o.qty)+"</td><td class='r m'>"+fmtN(o.filled)+"</td><td class='r m'>"+(o.price?fmtN(o.price,2):"MKT")+"</td><td><span class='tag "+st[1]+"'>"+esc(st[0])+"</span></td></tr>"}).join("")||"<tr><td colspan=9 class=empty>"+(OR.rows.length?"No orders match these filters.":"No orders on "+esc(OR.date)+".")+"</td></tr>");
 setc("or_range",list.length?(OR.page*ORPER+1)+"–"+Math.min(list.length,OR.page*ORPER+ORPER)+" of "+fmtN(list.length)+" orders":"0 orders");
 setc("or_pg",(OR.page+1)+" / "+pages);document.getElementById("or_prev").disabled=OR.page==0;document.getElementById("or_next").disabled=OR.page>=pages-1;
 document.getElementById("or_clear").style.display=(val("or_q")||val("or_side")||val("or_broker")||OR.chip)?"":"none";
 if(OR.sel){var s=OR.rows.filter(function(o){return orKey(o)==OR.sel})[0];if(s)orDetail(s,false)}}
function orPage(d){OR.page+=d;orRender(false)}
function orClear(){["or_q","or_side","or_broker"].forEach(function(i){document.getElementById(i).value=""});OR.chip="";orRender(true)}
function orExport(f){var p=[["format",f],["date",val("or_date")],["broker",val("or_broker")],["side",val("or_side").toLowerCase()],["status",OR.chip=="Working"?"new":OR.chip.toLowerCase()]];
 window.location.href="/admin/api/report/orders?"+p.filter(function(x){return x[1]}).map(function(x){return x[0]+"="+encodeURIComponent(x[1])}).join("&")}
function orOpen(k){OR.sel=k;orRender(false);var s=OR.rows.filter(function(o){return orKey(o)==k})[0];if(s)orDetail(s,true)}
var _orFills={};
function orDetail(o,fetchFills){var st=ORST[o.status]||[o.status,"t-mut"],k=orKey(o);
 var pct=o.qty?Math.min(100,o.filled/o.qty*100):0,val2=o.filled?o.filled*o.avg_px:0;
 var kv=function(a,b,m){return "<span>"+a+"</span><span"+(m?" class=mono":"")+">"+b+"</span>"};
 var h="<div class=tbar style='align-items:flex-start'><div style='display:flex;flex-direction:column;gap:2px'><span class='mono faint' style='font-size:12px'>"+esc(o.order_id||"no OrderID")+"</span><span style='font-size:17px;font-weight:600'><span style='color:"+(o.side=="Buy"?"var(--acc)":"var(--red)")+"'>"+esc(o.side)+"</span> "+fmtN(o.qty)+" "+esc(o.symbol)+"</span></div><span class='tag "+st[1]+"' style='font-size:10px;padding:3px 8px'>"+esc(st[0])+"</span></div>"+
  "<div class=kv>"+kv("Broker",esc(o.broker))+kv("ClOrdID",esc(o.cl_ord_id||"—"),1)+kv("Client",esc(o.client||"—"),1)+kv("BO account",esc(o.account||"—"),1)+kv("Board",esc(o.board||"—"))+kv("Trader",esc(o.trader||"—"))+
  kv("Type · TIF",esc(o.type)+" · "+esc(o.tif||"—"))+kv("Limit price",o.price?fmtN(o.price,2):"Market",1)+kv("Filled",fmtN(o.filled)+" of "+fmtN(o.qty),1)+kv("Avg fill price",o.filled?fmtN(o.avg_px,2):"—",1)+kv("Value",o.filled?"Tk "+fmtN(val2,2):"—",1)+"</div>"+
  "<div class=progress><i style='width:"+pct.toFixed(1)+"%'></i></div>"+
  "<span class=cap style='border-top:1px solid var(--line);padding-top:14px'>Execution history</span><div id=or_tl>"+orTimeline(o,_orFills[k])+"</div>";
 seth("or_detail",h);
 if(fetchFills&&o.order_id)j("report/order-history?date="+encodeURIComponent(OR.date)+"&order_id="+encodeURIComponent(o.order_id)+"&broker_id="+encodeURIComponent(o.broker_id)).then(function(r){
  if(!r.ok)return;_orFills[k]=r.fills||[];if(OR.sel==k)seth("or_tl",orTimeline(o,_orFills[k]))})}
function orTimeline(o,fills){var ev=[],t=hms(o.time),cum=0;
 var add=function(label,time,detail,col){ev.push("<div class=tline><div class=ax><i style='background:"+col+"'></i><b></b></div><div class=tx><span>"+esc(label)+"</span><small>"+esc(time)+(detail?" · "+esc(detail):"")+"</small></div></div>")};
 add("Received",t,"from "+o.broker,"var(--mut2)");
 if(o.status=="Rejected"){add("Rejected",t,o.reason||"refused","var(--red)");return ev.join("")}
 add(o.status=="Pending"?"Accepted, waiting for its trigger":"Accepted",t,"entered the book","var(--blue)");
 if(fills===undefined&&o.filled)ev.push("<small class=faint>Loading fills…</small>");
 (fills||[]).forEach(function(f){cum+=f.qty;add(cum>=o.qty?"Filled":"Partial fill",f.time,fmtN(f.qty)+" @ "+fmtN(f.price,2)+" · trade #"+f.trade_id+" with "+f.against,"var(--acc)")});
 if(fills&&!fills.length&&o.filled)add("Filled "+fmtN(o.filled),hms(o.updated),"fills are not in the trade log (before it was kept)","var(--acc)");
 if(o.status=="Cancelled")add("Cancelled",hms(o.updated),o.reason||"cancelled by the broker","var(--amber)");
 if(o.status=="Expired")add("Expired",hms(o.updated),o.reason||"end of day","var(--mut2)");
 return ev.join("")}

// ── FIX Sessions: registered brokers and live sessions ──
var FX={tab:"all"};
function dur(sec){sec=Math.max(0,Math.floor(sec));return sec<60?sec+"s":sec<3600?Math.floor(sec/60)+"m":Math.floor(sec/3600)+"h "+Math.floor(sec%3600/60)+"m"}
function lastLogon(broker){var ev=((_s&&_s.events)||[]).filter(function(e){return e.kind=="LOGON"&&String(e.broker).toUpperCase()==String(broker).toUpperCase()})[0];return ev?ev.time:""}
function renderFix(){var d=_s||{},sess={},rows=[];
 (d.sessions||[]).forEach(function(s){sess[String(s.broker).toUpperCase()]=s});
 var seen={};((_b&&_b.brokers)||[]).forEach(function(b){var k=String(b.sender).toUpperCase();seen[k]=1;rows.push({id:b.sender,company:b.company||"",s:sess[k]})});
 Object.keys(sess).forEach(function(k){if(!seen[k])rows.push({id:sess[k].broker,company:"(not registered)",s:sess[k]})});
 var on=rows.filter(function(r){return r.s}).length,inn=0;rows.forEach(function(r){if(r.s)inn+=r.s.seq_target||0});
 setc("fx_on",on);setc("fx_off",rows.length-on);setc("fx_in",fmtN(inn));setc("fx_reg",((_b&&_b.brokers)||[]).length);
 seth("fx_tabs",[["all","All ("+rows.length+")"],["on","Logged on ("+on+")"],["off","Disconnected ("+(rows.length-on)+")"]].map(function(t){return "<button type=button class='"+(FX.tab==t[0]?"on":"")+"' onclick=\"FX.tab='"+t[0]+"';renderFix()\">"+t[1]+"</button>"}).join(""));
 setc("fx_acc","Acceptor "+(d.fix_begin||"FIXT.1.1")+" on :"+(d.fix_port||"5001")+" · TargetCompID "+(d.fix_sender||"DSEME"));
 var list=rows.filter(function(r){return FX.tab=="all"||(FX.tab=="on"?!!r.s:!r.s)});
 seth("sessions",list.map(function(r){var s=r.s,ll=lastLogon(r.id);
  var since=s?(Date.now()-new Date(String(s.since).replace(" ","T")+"+06:00").getTime())/1000:0;
  return "<tr><td class=m>"+esc(r.id)+" → "+esc(d.fix_sender||"DSEME")+"</td><td class=ell>"+esc(r.company)+"</td><td><span class=sdot style='color:"+(s?"var(--acc)":"var(--mut2)")+"'><i></i>"+(s?"LOGGED ON":"DISCONNECTED")+"</span></td>"+
   "<td class='r m'>"+(s?fmtN(s.seq_target):"—")+"</td><td class='r m'>"+(s?fmtN(s.seq_sender):"—")+"</td><td class='m muted'>"+(s?dur(since):"—")+"</td><td class='m muted' style='font-size:12px'>"+(ll?esc(ymdhm(ll).slice(5)):"—")+"</td>"+
   "<td class=r>"+(s?"<div class=acts><button class='btn sm' onclick=\"fxAsk("+jsa(r.id)+",'disconnect')\">Log out</button><button class='btn sm' onclick=\"fxAsk("+jsa(r.id)+",'reset')\">Reset seq</button></div>":"")+"</td></tr>"}).join("")||"<tr><td colspan=8 class=empty>"+(FX.tab=="on"?"No broker is logged on.":FX.tab=="off"?"Every registered broker is logged on.":"No brokers registered yet.")+"</td></tr>")}
function fxAsk(b,a){
 if(a=="disconnect")confirmDlg("Log out "+b+"?","The engine sends a Logout (35=5) and closes the connection. Resting orders stay in the book; the broker can log on again.","Log out",true,function(){fxDo(b,a)});
 else confirmDlg("Reset sequence numbers for "+b+"?","Both inbound and outbound sequence numbers go back to 1. The broker must reset on its side too, or its next message is rejected.","Reset",true,function(){fxDo(b,a)})}
function fxDo(b,a){j("session?broker="+encodeURIComponent(b)+"&action="+a,{method:"POST"}).then(function(r){if(!r.ok)return dlgErr(r.error||"Failed");dlgClose();toast(a=="disconnect"?b+" logged out":"Sequence numbers reset for "+b);refresh()})}
// ── Market Session ──
var MSPH=[["Pre-Open","pre_open","open","var(--blue)","Orders can be entered, amended and cancelled. No matching; the opening auction runs when trading opens."],
 ["Continuous Trading","open","close","var(--acc)","Orders match at once by price and time priority, within the circuit limits."],
 ["Close","close","post_close","var(--amber)","Closing prices are published. No orders are accepted."],
 ["Post-Close","post_close","end","var(--amber)","Orders are accepted at the closing price only."],
 ["Closed","end","pre_open","var(--mut2)","No orders are accepted. Day orders expire; the next Pre-Open starts a new trading day."]];
var MSNEXT={"Pre-Open":["open","Continuous Trading"],"Continuous Trading":["close","Close"],"Close":["postclose","Post-Close"],"Post-Close":["endofday","Closed (end of day)"],"Closed":["preopen","Pre-Open"]};
function msSched(){var p=String((_s&&_s.schedule)||"09:30,10:00,14:30,14:40,15:00").split(",");return {pre_open:p[0],open:p[1],close:p[2],post_close:p[3],end:p[4]}}
function renderMarket(){var d=_s;if(!d)return;var sc=msSched(),ph=d.phase,cur=MSPH.filter(function(p){return p[0]==ph})[0];
 var pe=document.getElementById("ms_phase");pe.textContent=ph;pe.style.color=cur?cur[3]:"var(--ink)";
 var t=dhakaNow(),m=mins(t)+(+t.slice(6,8)||0)/60;
 setc("ms_note",d.auto_clock?(cur&&cur[0]!="Closed"?"Follows the DSE schedule · ends at "+sc[cur[2]]:"Follows the DSE schedule"):"Manual: phases change only by hand");
 setc("ms_clock",t);setc("ms_date","Asia/Dhaka · "+new Date().toLocaleDateString("en-GB",{timeZone:"Asia/Dhaka",weekday:"short",day:"numeric",month:"short",year:"numeric"}));
 var a=mins(sc.pre_open),z=mins(sc.end),pos=Math.max(0,Math.min(100,(m-a)/(z-a)*100));
 seth("ms_tl",MSPH.slice(0,4).map(function(p,i){return "<div class='sg"+(p[0]==ph?" cur":"")+"' style='flex:"+Math.max(1,mins(sc[p[2]])-mins(sc[p[1]]))+";background:"+p[3]+";border-radius:"+(i==0?"4px 0 0 4px":i==3?"0 4px 4px 0":"0")+"'></div>"}).join("")+"<div class=mk style='left:"+pos.toFixed(2)+"%'></div>");
 setc("ms_tl0",sc.pre_open);setc("ms_tl1",sc.end);
 var nx=MSNEXT[ph];var eb=document.getElementById("ms_end");eb.textContent=nx?(ph=="Closed"?"Start Pre-Open now":"End "+ph+" now"):"End phase now";eb.disabled=!nx;
 setc("ms_clockbtn",d.auto_clock?"Switch to manual":"Follow the schedule");
 seth("ms_sched",MSPH.map(function(p){var now=p[0]==ph;return "<div style='display:grid;grid-template-columns:minmax(0,1fr) 160px;gap:12px;align-items:center;padding:12px 18px;border-top:1px solid var(--line)'><div style='display:flex;flex-direction:column;gap:2px'><span style='display:flex;align-items:center;gap:8px;font-weight:600'><i style='width:9px;height:9px;border-radius:2px;background:"+p[3]+";display:inline-block'></i>"+p[0]+(now?" <span class='tag t-acc'>NOW</span>":"")+"</span><span class=muted style='font-size:13px'>"+p[4]+"</span></div><span class=mono style='text-align:right'>"+sc[p[1]]+" – "+sc[p[2]]+"</span></div>"}).join(""));
 var today=new Date().toLocaleDateString("en-CA",{timeZone:"Asia/Dhaka"});
 var hist=(d.audit||[]).filter(function(e){return /^SESSION (PREOPEN|OPEN|CLOSE|POSTCLOSE|ENDOFDAY|AUTO_ON|AUTO_OFF)$/.test(e.action)&&String(e.time).slice(0,10)==today});
 var lbl={PREOPEN:"Moved to Pre-Open",OPEN:"Opened continuous trading",CLOSE:"Moved to Close",POSTCLOSE:"Moved to Post-Close",ENDOFDAY:"Ended the trading day",AUTO_ON:"Clock follows the schedule",AUTO_OFF:"Clock switched to manual"};
 seth("ms_hist",hist.map(function(e){var a=e.action.slice(8),n=String(e.target||"").split(" · ").slice(1).join(" · ");
  return "<div style='display:grid;grid-template-columns:70px 1fr;gap:10px;padding:8px 18px;border-top:1px solid var(--line);font-size:13px'><span class='mono faint' style='font-size:12px'>"+esc(hms(e.time))+"</span><div style='display:flex;flex-direction:column'><span>"+esc(lbl[a]||a)+(n?": "+esc(n):"")+"</span><span class=faint style='font-size:12px'>"+esc(String(e.actor).split(" (")[0])+"</span></div></div>"}).join("")||"<div style='padding:8px 18px;border-top:1px solid var(--line);font-size:13px' class=faint>No session changes by hand today. Changes by the schedule are not listed.</div>")}
function msAsk(k){var d=_s||{},ph=d.phase;
 if(k=="clock"){var on=!d.auto_clock;return confirmDlg(on?"Follow the DSE schedule?":"Switch the clock to manual?",on?"The engine moves to the phase the schedule says now, running each step's housekeeping on the way.":"Phases stay as they are until you move them by hand.",on?"Follow schedule":"Switch to manual",false,function(){msDo(on?"auto_on":"auto_off","")})}
 var act=k,name={preopen:"Pre-Open",open:"Continuous Trading",close:"Close",postclose:"Post-Close",endofday:"Closed (end of day)"}[k];
 if(k=="next"){var nx=MSNEXT[ph];if(!nx)return;act=nx[0];name=nx[1]}
 var body="The market moves to "+name+" now, ahead of the schedule, and the clock switches to manual."+(act=="endofday"?" Day orders expire and closing prices are published.":act=="open"&&ph=="Pre-Open"?" The opening auction runs.":"");
 confirmDlg((k=="next"?"End "+ph+" now?":"Move to "+name+"?"),body,k=="next"?"End phase":"Move",true,function(r){msDo(act,r)},"Reason for the audit log")}
function msDo(a,note){j("trading-session?action="+a+"&note="+encodeURIComponent(note||""),{method:"POST"}).then(function(r){if(!r.ok)return dlgErr(r.error||"Failed");dlgClose();toast("Phase: "+r.phase);refresh()})}

// ── Instruments ──
var _in=[],_inTab="";
var ASSET=[["EQ","Equity"],["MF","Mutual Fund"],["CB","Corporate Bond"],["DB","Debenture"]];
// DSE sectors: the code is what the DSE end-of-day ticker file and the instrument master use; the label is the DSE's own name.
var SECTORS=[["Bank","Bank"],["Cement","Cement"],["Ceramic","Ceramics Sector"],["CorpBond","Corporate Bond"],["Debenture","Debenture"],["Engineering","Engineering"],["Financial In","Financial Institutions"],["FoodAllied","Food & Allied"],["FuelPower","Fuel & Power"],["Insurance","Insurance"],["IT","IT Sector"],["Jute","Jute"],["Misc","Miscellaneous"],["MutFund","Mutual Funds"],["PaperPrint","Paper & Printing"],["PharmaChem","Pharmaceuticals & Chemicals"],["ServRealEst","Services & Real Estate"],["Tannery","Tannery Industries"],["Telecom","Telecommunication"],["Textile","Textile"],["TravelLeisur","Travel & Leisure"]];
// the sector that goes with a non-equity asset class
var CLASS_SECTOR={MF:"MutFund",CB:"CorpBond",DB:"Debenture"};
var BOARDS=["Main (A)","Main (B)","Main (N)","Main (Z)","Bond","Mutual Fund","SME / ATB"];
// DSE daily price limits, tiered by the reference price (as the engine applies them to broker orders)
function bandPct(p){return p<=200?10:p<=500?8.75:p<=1000?7.5:p<=2000?6.25:p<=5000?5:3.75}
function bandText(p){if(!(p>0))return "—";var b=bandPct(p);return (p*(1-b/100)).toFixed(2)+" – "+(p*(1+b/100)).toFixed(2)}
function loadInstr(){j("instruments").then(function(d){_in=d.rows||[];renderInstr()})}
function renderInstr(){var f=flt("f_instr"),cnt={"":_in.length,active:0,halted:0,pending:0};
 _in.forEach(function(r){var s=r.status||"active";if(cnt[s]!=null)cnt[s]++});
 seth("in_tabs",[["","All"],["active","Active"],["halted","Halted"],["pending","Pending"]].map(function(t){return "<button type=button class='"+(_inTab==t[0]?"on":"")+"' onclick=\"_inTab='"+t[0]+"';renderInstr()\">"+t[1]+" ("+cnt[t[0]]+")</button>"}).join(""));
 var rows=_in.filter(function(r){return (!_inTab||(r.status||"active")==_inTab)&&hit(r.symbol+" "+r.name+" "+(r.isin||""),f)});
 seth("instrList",rows.map(function(r){var st=r.status||"active",cls=st=="active"?"t-acc":st=="halted"?"t-red":"t-amber";
  return "<tr><td><b>"+esc(r.symbol)+"</b></td><td class='ell muted'>"+esc(r.name)+"</td><td class='m muted' style='font-size:12px'>"+esc(r.isin||"—")+"</td><td style='font-size:12px'>"+esc(r.board)+"</td><td class='r m'>"+r.lot+"</td><td class='r m'>"+Number(r.tick||0).toFixed(2)+"</td><td class='r m'>"+fmtN(r.prev,2)+"</td><td class='r m muted' title='±"+bandPct(r.prev)+"% of the reference price'>"+bandText(r.prev)+"</td><td><span class='tag "+cls+"'>"+st.toUpperCase()+"</span></td>"+
   "<td class=r><div class=acts>"+(st=="pending"?"":"<button class='btn sm' onclick=\"haltInstr("+jsa(r.symbol)+","+(st=="active")+")\">"+(st=="active"?"Halt":"Activate")+"</button>")+"<button class='btn sm' onclick=\"openInstr("+jsa(r.symbol)+")\">Edit</button></div></td></tr>"}).join("")||"<tr><td colspan=10 class=empty>No instruments"+(f||_inTab?" match.":" listed yet — add one.")+"</td></tr>");
 setc("in_count",rows.length+" of "+_in.length+" instruments. Circuit range = the DSE daily price limit around the reference price (previous close).")}
function toggleExport(e){e.stopPropagation();var m=document.getElementById("exp_menu");m.classList.toggle("on");setc("exp_note","Exports the instruments currently shown"+(_inTab||flt("f_instr")?" (filters applied).":"."))}
function exportInstr(fmt){document.getElementById("exp_menu").classList.remove("on");
 window.location="/admin/api/instruments/export?format="+fmt+"&q="+encodeURIComponent(val("f_instr"))}
document.addEventListener("click",function(){var m=document.getElementById("exp_menu");if(m)m.classList.remove("on")});
// ── Instruments: import a security master (DSE end-of-day XML, CSV/TSV, JSON) ──
// The file is read in the browser, previewed with dry_run=1, then imported.
var IMP={};
var IMP_MODES=[["merge","Add new + update listed","Adds the instruments that are not listed and updates the listed ones."],
 ["new","Add new only","Adds only instruments that are not listed; the listed ones stay as they are."],
 ["update","Update listed only","Updates only listed instruments, for example closing prices; nothing new is listed."]];
function importDlg(){IMP={text:"",name:"",mode:"merge",tab:"new",d:null};
 dlg("<h3>Import instruments</h3>"+
  "<label class=imp-drop id=imp_drop><input type=file id=imp_file accept='.xml,.csv,.tsv,.txt,.json' hidden onchange='impFile(this.files[0])'><b id=imp_fname>Choose a file or drop it here</b>"+
  "<span class=faint>DSE end-of-day ticker file (XML), CSV or TSV with a header row, or a JSON master as exported. Up to 32 MB.</span></label>"+
  "<div class=fld>What to do<div class='seg md' id=imp_mode></div><span class=note id=imp_mnote></span></div>"+
  "<div id=imp_prev style='display:flex;flex-direction:column;gap:10px'></div>"+
  "<p class=note>A blank field keeps the listed value (a DSE ticker file has no names, lots or ticks). Instruments that are not in the file stay listed. The result is saved, so it survives a restart. <a href='#' onclick='impTemplate();return false'>Download a CSV template</a></p>"+
  "<span class=err id=dlg_err></span><div class=acts><button class=btn type=button onclick='dlgClose()'>Cancel</button><button class='btn pri' id=imp_go type=button disabled onclick='impGo()'>Import</button></div>","xwide");
 var d=document.getElementById("imp_drop");
 ["dragenter","dragover"].forEach(function(t){d.addEventListener(t,function(e){e.preventDefault();d.classList.add("on")})});
 ["dragleave","drop"].forEach(function(t){d.addEventListener(t,function(e){e.preventDefault();d.classList.remove("on")})});
 d.addEventListener("drop",function(e){if(e.dataTransfer&&e.dataTransfer.files.length)impFile(e.dataTransfer.files[0])});
 impModes()}
function impModes(){seth("imp_mode",IMP_MODES.map(function(m){return "<button type=button class='"+(IMP.mode==m[0]?"on":"")+"' onclick=\"IMP.mode='"+m[0]+"';impModes();impPreview()\">"+m[1]+"</button>"}).join(""));
 setc("imp_mnote",IMP_MODES.filter(function(m){return m[0]==IMP.mode})[0][2])}
function impFile(f){if(!f)return;dlgErr("");
 if(f.size>32*1024*1024){dlgErr(f.name+" is larger than 32 MB.");return}
 var r=new FileReader();r.onload=function(){IMP.text=String(r.result||"");IMP.name=f.name;setc("imp_fname",f.name+" · "+(f.size<1024?f.size+" bytes":fmtN(f.size/1024,1)+" KB"));impPreview()};
 r.onerror=function(){dlgErr("The file could not be read.")};r.readAsText(f)}
function impPost(dry){return j("instruments/import?mode="+IMP.mode+(dry?"&dry_run=1":""),{method:"POST",headers:{"Content-Type":"text/plain"},body:IMP.text})}
function impPreview(){var go=document.getElementById("imp_go");if(!IMP.text){go.disabled=true;return}
 go.disabled=true;seth("imp_prev","<span class=note>Checking the file…</span>");
 impPost(true).then(function(d){if(!d.ok){IMP.d=null;seth("imp_prev","");dlgErr(d.error||"The file could not be read.");return}
  dlgErr("");IMP.d=d;impRender()}).catch(function(){seth("imp_prev","");dlgErr("The preview failed; check the connection and try again.")})}
function impBig(c){if(c.field!="Ref price")return false;var a=parseFloat(c.from),b=parseFloat(c.to);return a>0&&Math.abs(b/a-1)>0.2}
function impRender(){var d=IMP.d;if(!d)return;var p=d.plan||{},nw=p.new||[],up=p.updated||[],sk=p.skipped||[];
 var fmt={xml:"DSE end-of-day ticker file",csv:"CSV / TSV",json:"JSON master"}[d.format]||d.format;
 var big=up.filter(function(u){return u.changes.some(impBig)}).length;
 if(!nw.length&&IMP.tab=="new")IMP.tab=up.length?"updated":"skipped";
 var tabs=[["new","New",nw.length],["updated","Updated",up.length],["skipped","Skipped",sk.length]];
 var rows=IMP.tab=="new"?nw.map(function(n){return "<tr><td><b>"+esc(n.symbol)+"</b>"+(n.relisted?" <span class='tag t-amber'>RE-LIST</span>":"")+"</td><td class=ell>"+esc(n.name)+"</td><td>"+esc(n.board||"—")+"</td><td class=ell>"+esc(secLabel(n.sector)||"—")+"</td><td class='r m'>"+fmtN(n.prev,2)+"</td></tr>"})
  :IMP.tab=="updated"?up.map(function(u){return "<tr><td><b>"+esc(u.symbol)+"</b></td><td class=wrap>"+u.changes.map(function(c){return "<span"+(impBig(c)?" class=imp-warn":"")+">"+esc(c.field)+": <span class=faint>"+esc(c.from||"—")+"</span> → "+esc(c.to||"—")+"</span>"}).join("<br>")+"</td></tr>"})
  :sk.map(function(s){return "<tr><td><b>"+esc(s.symbol||"(blank)")+"</b></td><td class='wrap muted'>"+esc(s.reason)+"</td></tr>"});
 var head=IMP.tab=="new"?"<tr><th>Symbol</th><th>Name</th><th>Board</th><th>Sector</th><th class=r>Ref price</th></tr>":IMP.tab=="updated"?"<tr><th>Symbol</th><th>What changes</th></tr>":"<tr><th>Symbol</th><th>Why it is skipped</th></tr>";
 var empty={new:"No new instruments in this file"+(IMP.mode=="update"?" (this import only updates listed ones).":"."),updated:"No listed instrument changes"+(IMP.mode=="new"?" (this import only adds new ones).":"."),skipped:"Every row can be imported."}[IMP.tab];
 seth("imp_prev","<div class=imp-sum><span class=faint>"+esc(fmt)+" · "+fmtN(d.rows)+" rows</span><span><b>"+fmtN(nw.length)+"</b> new</span><span><b>"+fmtN(up.length)+"</b> updated</span><span><b>"+fmtN(p.unchanged)+"</b> unchanged</span><span><b>"+fmtN(sk.length)+"</b> skipped</span>"+(p.indices?"<span class=faint>"+p.indices+" index rows ignored</span>":"")+"</div>"+
  (big?"<span class='note imp-warn'>"+big+" listed instrument"+(big>1?"s change their":" changes its")+" reference price by more than 20%. The daily price band moves with it.</span>":"")+
  "<div class='seg sm'>"+tabs.map(function(t){return "<button type=button class='"+(IMP.tab==t[0]?"on":"")+"' onclick=\"IMP.tab='"+t[0]+"';impRender()\">"+t[1]+" ("+fmtN(t[2])+")</button>"}).join("")+"</div>"+
  "<div class=imp-box><table class='tbl tight'><thead>"+head+"</thead><tbody>"+(rows.join("")||"<tr><td colspan=5 class=empty>"+empty+"</td></tr>")+"</tbody></table></div>");
 var n=nw.length+up.length,go=document.getElementById("imp_go");go.disabled=!n;go.textContent=n?"Import "+fmtN(n)+" instrument"+(n>1?"s":""):"Nothing to import"}
function impGo(){var go=document.getElementById("imp_go");go.disabled=true;go.textContent="Importing…";
 impPost(false).then(function(d){if(!d.ok){dlgErr(d.error||"The import failed.");impRender();return}var p=d.plan||{};
  dlgClose();toast("Imported "+IMP.name+": "+fmtN((p.new||[]).length)+" new, "+fmtN((p.updated||[]).length)+" updated"+((p.skipped||[]).length?", "+fmtN(p.skipped.length)+" skipped":""),false,d.warning);loadInstr()})
 .catch(function(){dlgErr("The import failed; check the connection and try again.");impRender()})}
function impTemplate(){var csv="symbol,name,isin,board,sector,asset_class,lot,tick,prev_close,status\n"+
  "GP,Grameenphone Ltd.,BD0001GP0004,Main (A),Telecom,EQ,1,0.10,286.60,active\n"+
  "1JANATAMF,1JANATAMF,BD0178JBMF19,Mutual Fund,MutFund,MF,1,0.10,6.10,active\n";
 var a=document.createElement("a");a.href=URL.createObjectURL(new Blob([csv],{type:"text/csv"}));a.download="instruments-template.csv";document.body.appendChild(a);a.click();
 setTimeout(function(){URL.revokeObjectURL(a.href);a.remove()},1000)}
var _editing="";
function opts(list,cur){return list.map(function(x){var v=Array.isArray(x)?x[0]:x,l=Array.isArray(x)?x[1]:x;return "<option value=\""+esc(v)+"\""+(v==cur?" selected":"")+">"+esc(l)+"</option>"}).join("")}
function openInstr(sym){var r=sym?_in.filter(function(x){return x.symbol==sym})[0]:null;_editing=r?r.symbol:"";
 var bs=BOARDS.slice();if(r&&bs.indexOf(r.board)<0)bs.push(r.board);
 var secs=SECTORS.slice();_in.concat(r?[r]:[]).forEach(function(x){if(x.sector&&!secs.some(function(s){return s[0]==x.sector}))secs.push([x.sector,x.sector])}); // sectors already in use, too
 secs.sort(function(a,b){return a[1].localeCompare(b[1])});
 var fld=function(lbl,html,span){return "<label class='fld"+(span?" span":"")+"'>"+lbl+html+"</label>"};
 dlg("<h3>"+(r?"Edit "+esc(r.symbol):"Add instrument")+"</h3><div class=g2>"+
  fld("Symbol","<input class='in mono' id=in_symbol style='text-transform:uppercase' value=\""+esc(r?r.symbol:"")+"\""+(r?" disabled":"")+">")+
  fld("ISIN","<input class='in mono' id=in_isin style='text-transform:uppercase' value=\""+esc(r?r.isin:"")+"\" placeholder=optional>")+
  fld("Company name","<input class=in id=in_name value=\""+esc(r?r.name:"")+"\">",1)+
  fld("Board","<select class=in id=in_board>"+opts(bs,r?r.board:"Main (A)")+"</select>")+
  fld("Asset class","<select class=in id=in_class onchange='inClass()'>"+opts(ASSET,r?r.asset_class:"EQ")+"</select>")+
  "<div class=fld>Sector"+comboHTML("in_sector","Type or pick a sector…")+"</div>"+
  fld("Status","<select class=in id=in_status>"+opts([["active","Active"],["halted","Halted"],["pending","Pending"]],r?r.status:"active")+"</select>")+
  fld("Reference price (Tk)","<input class='in mono' id=in_prev type=number min=0 step=0.1 value=\""+esc(r?r.prev:"")+"\" oninput=\"setc('in_band',bandText(parseFloat(this.value)))\">")+
  fld("Lot size","<input class='in mono' id=in_lot type=number min=1 step=1 value=\""+esc(r?r.lot:1)+"\">")+
  fld("Tick size","<input class='in mono' id=in_tick type=number min=0 step=0.01 value=\""+(r?Number(r.tick).toFixed(2):"0.10")+"\">")+
  "<div class=fld>Circuit range (DSE tiers)<span class=mono id=in_band style='font-size:14px;color:var(--ink);padding:8px 0'>"+bandText(r?r.prev:0)+"</span></div></div>"+
  "<span class=err id=dlg_err></span><div class=acts>"+(r?"<button class='btn red' type=button style='margin-right:auto' onclick=\"delistInstr("+jsa(r.symbol)+")\">Delist</button>":"")+
  "<button class=btn type=button onclick='dlgClose()'>Cancel</button><button class='btn pri' type=button onclick='saveInstr()'>Save</button></div>","wide");
 comboInit("in_sector",secs,r?r.sector:"",true)}
function inClass(){var c=CLASS_SECTOR[val("in_class")];if(c){comboSet("in_sector",c);document.getElementById("in_sector_in").classList.remove("bad")}}
function saveInstr(){var body={Symbol:val("in_symbol").toUpperCase(),Name:val("in_name"),AssetClass:val("in_class"),Board:val("in_board"),Sector:val("in_sector"),Status:val("in_status"),ISIN:val("in_isin").toUpperCase(),
  Lot:parseInt(val("in_lot"),10)||1,Tick:parseFloat(val("in_tick"))||0,Prev:parseFloat(val("in_prev"))||0};
 comboCommit("in_sector");body.Sector=val("in_sector");
 var need=function(id,msg){var e=document.getElementById(id=="in_sector"?"in_sector_in":id);e.classList.add("bad");e.focus();dlgErr(msg)};
 if(!body.Symbol)return need("in_symbol","Symbol is required.");if(!body.Name)return need("in_name","Company name is required.");
 if(!body.Sector)return need("in_sector",val("in_sector_in")?"Pick “"+val("in_sector_in")+"” from the list, or choose “Add … as a new sector”.":"Choose a sector, or type a new one and choose “Add”.");
 var edit=!!_editing;jpost(edit?"instrument/update":"instrument/add",body).then(function(d){if(!d.ok)return dlgErr(d.error||"Not saved");
  dlgClose();toast(edit?"Saved "+body.Symbol:"Listed "+body.Symbol+" — it trades now",false,d.warning);loadInstr()})}
function haltInstr(sym,h){confirmDlg((h?"Halt ":"Activate ")+sym+"?",h?"New orders in "+sym+" are rejected until you activate it again. Resting orders stay in the book.":"Trading in "+sym+" resumes.",h?"Halt":"Activate",h,function(){
 jpost("instrument/halt",{Symbol:sym,Halted:h}).then(function(d){if(!d.ok)return dlgErr(d.error||"Failed");dlgClose();toast(sym+(h?" halted":" active"),false,d.warning);loadInstr()})})}
function delistInstr(sym){confirmDlg("Delist "+sym+"?",sym+" stops trading and leaves the directory and the saved master.","Delist",true,function(){
 jpost("instrument/delist",{Symbol:sym}).then(function(d){if(!d.ok)return dlgErr(d.error||"Failed");dlgClose();toast(sym+" delisted",false,d.warning);loadInstr()})})}
// delete all: the engine counts what goes, then the button waits for DELETE to be typed
function deleteAllInstr(){jpost("instruments/delete-all?dry_run=1",{}).then(function(d){if(!d.ok)return toast(d.error||"Failed",true);if(!d.instruments)return toast("There are no instruments to delete",true);
 var n=d.instruments,o=d.orders;
 dlg("<h3>Delete all "+n+" instruments?</h3><p>Every instrument is delisted: it stops trading and leaves the directory and the saved master, so the engine starts with none after a restart. "+
  (o?o+" open order"+(o==1?" is":"s are")+" cancelled and "+(o==1?"its broker is":"their brokers are")+" told.":"No orders are open.")+" Export a JSON copy first if you may want them back: Import lists them again.</p>"+
  "<label class=fld>Type DELETE to confirm<input class=in id=del_conf autocomplete=off spellcheck=false oninput=\"document.getElementById('dlg_ok').disabled=this.value!='DELETE'\" onkeydown=\"if(event.key=='Enter'&&this.value=='DELETE')deleteAllGo()\"></label>"+
  "<span class=err id=dlg_err></span><div class=acts><button class=btn type=button onclick='dlgClose()'>Cancel</button><button class='btn dng' id=dlg_ok type=button disabled onclick='deleteAllGo()'>Delete all</button></div>");
 document.getElementById("del_conf").focus()})}
function deleteAllGo(){var b=document.getElementById("dlg_ok");if(b)b.disabled=true;
 jpost("instruments/delete-all",{Confirm:val("del_conf")}).then(function(d){if(!d.ok){if(b)b.disabled=false;return dlgErr(d.error||"Failed")}dlgClose();
  toast(d.deleted+" instruments deleted"+(d.cancelled?", "+d.cancelled+" orders cancelled":""),false,d.warning);loadInstr()})}
// ── Broker credentials ──
var _b=null;
function loadBrokers(){return j("brokers").then(function(b){_b=b;renderBrokers(b);if(VIEW=="sessions")renderFix()})}
function renderBrokers(b){if(!b)return;var f=flt("f_broker"),on={};((_s&&_s.sessions)||[]).forEach(function(s){on[String(s.broker).toUpperCase()]=s});
 var rows=(b.brokers||[]).filter(function(r){return hit((r.company||"")+" "+r.sender+" "+(r.itch_user||""),f)});
 seth("brokerList",rows.map(function(r){var s=on[String(r.sender).toUpperCase()],ll=lastLogon(r.sender);
  return "<tr><td class=m><b>"+esc(r.sender)+"</b></td><td>"+(r.company?esc(r.company):"<span class=faint>—</span>")+"</td><td class='m muted'>"+esc(r.itch_user||"—")+"</td><td class=muted data-admin-only>"+(r.owner?esc(r.owner):"exchange")+"</td>"+
   "<td><span class=sdot style='color:"+(s?"var(--acc)":"var(--mut2)")+"'><i></i>"+(s?"ONLINE":"OFFLINE")+"</span></td><td class='m muted' style='font-size:12px'>"+(ll?esc(ymdhm(ll).slice(5)):"—")+"</td><td class='m muted' style='font-size:12px'>"+esc(String(r.created||"").slice(0,16))+"</td>"+
   "<td class=r><div class=acts><button class='btn sm' onclick=\"openDetail("+jsa(r.sender)+")\">View</button><button class='btn sm red' onclick=\"del("+jsa(r.sender)+")\">Revoke</button></div></td></tr>"}).join("")||"<tr><td colspan=8 class=empty>"+(b.scope=="own"&&!(b.brokers||[]).length?"You have no brokers yet — add one.":"No brokers match.")+"</td></tr>");
 setc("brokerCount",b.scope=="own"?rows.length+" of your "+(b.brokers||[]).length+" brokers. Revoking removes a broker's FIX and ITCH logins at once.":rows.length+" of "+(b.brokers||[]).length+" brokers · "+b.fix_total+" FIX logins · "+b.itch_total+" ITCH logins. Revoking removes a broker's FIX and ITCH logins at once.")}
function openBroker(){var fld=function(lbl,id,ph,span,mono){return "<label class='fld"+(span?" span":"")+"'>"+lbl+"<input class='in"+(mono?" mono":"")+"' id="+id+" placeholder=\""+esc(ph||"")+"\"></label>"};
 dlg("<h3>Add broker</h3><p style='font-size:13px'>Issues linked FIX (order entry) and ITCH (market data) logins. Leave a password blank to generate one.</p><div class=g2>"+
  fld("Company / firm name","c_company","e.g. Alpha Equities Ltd",1)+fld("SenderCompID (FIX 49)","c_sender","e.g. ALPHAEQ",0,1)+fld("Technical contact","c_contact","email or name")+
  fld("FIX password (554)","c_fixpass","blank = generate",0,1)+fld("ITCH username (6 chars)","c_itchuser","blank = from SenderCompID",0,1)+fld("ITCH password","c_itchpass","blank = generate",1,1)+
  "</div><span class=err id=dlg_err></span><div class=acts><button class=btn type=button onclick='dlgClose()'>Cancel</button><button class='btn pri' type=button onclick='addBroker()'>Issue credentials</button></div>","wide")}
function addBroker(){var body={Company:val("c_company"),Sender:val("c_sender"),Contact:val("c_contact"),FixPass:val("c_fixpass"),ItchUser:val("c_itchuser"),ItchPass:val("c_itchpass")};
 if(!body.Sender)return dlgErr("SenderCompID is required.");
 jpost("broker/add",body).then(function(r){if(!r.ok)return dlgErr(r.error||"Not created");
  window._last={company:r.company,sender:r.sender,fix_pass:r.fix_password,itch_user:r.itch_user,itch_pass:r.itch_password};
  var ep=r.endpoints||{};
  dlg("<h3>Credentials for "+esc(r.company||r.sender)+"</h3><p>Copy them now and send them to the broker through a secure channel. You can show them again from the broker's View.</p><div class=creds>"+
   cr(extLabel(ep),ep.ext_addr,1)+cr("TargetCompID",ep.target,1)+cr("SenderCompID",r.sender,1)+cr("FIX password",r.fix_password)+cr("ITCH feed",ep.itch_ip+" : "+ep.itch_port,1)+cr("ITCH username",r.itch_user,1)+cr("ITCH password",r.itch_password)+
   "</div><div class=acts><button class=btn type=button onclick='sheetFrom(window._last)'>Open onboarding sheet ↗</button><button class='btn pri' type=button onclick='dlgClose()'>Done</button></div>","wide");
  loadBrokers()})}
function cr(k,v,plain){return "<div class=cr><span class=ck>"+esc(k)+"</span><span class='cv"+(plain?"":" sec")+"'>"+esc(v)+"</span><button class='btn sm' type=button onclick='copyCell(this)'>Copy</button></div>"}
function copyCell(btn){var cv=btn.parentNode.querySelector(".cv");if(cv&&navigator.clipboard){navigator.clipboard.writeText(cv.textContent);btn.textContent="Copied";setTimeout(function(){btn.textContent="Copy"},900)}}
function extLabel(ep){return ep.ext_tls?"FIXS endpoint (TLS)":"FIX endpoint"}
function sheetFrom(o){if(!o)return;var f=document.getElementById("sheetForm");
 f.company.value=o.company||"";f.sender.value=o.sender||"";f.fix_pass.value=o.fix_pass||"";f.itch_user.value=o.itch_user||"";f.itch_pass.value=o.itch_pass||"";csrfField(f);f.submit()}
function openDetail(sender){j("broker/detail?sender="+encodeURIComponent(sender)).then(function(d){if(d.ok===false)return toast(d.error||"Not found",true);
 window._detail={company:d.company,sender:d.sender,fix_pass:d.fix_password,itch_user:d.itch_user,itch_pass:d.itch_password};
 var ep=d.endpoints||{},chip=function(on,l){return "<span class='tag "+(on?"t-acc":"t-mut")+"'>"+l+(on?" ACTIVE":" OFF")+"</span>"};
 dlg("<div class=tbar style='align-items:flex-start'><div><h3>"+esc(d.company||d.sender)+"</h3><span class=note>SenderCompID "+esc(d.sender)+(d.created?" · issued "+esc(d.created):"")+(d.contact?" · "+esc(d.contact):"")+"</span></div><div class=tb>"+chip(d.fix_active,"FIX")+chip(d.itch_active,"ITCH")+"</div></div>"+
  "<div class=creds>"+cr(extLabel(ep),ep.ext_addr,1)+cr("Host / SNI",ep.pub_host,1)+(ep.show_int?cr("Internal endpoint",ep.int_ip+" : "+ep.int_port,1):"")+cr("TargetCompID",ep.target,1)+cr("SenderCompID",d.sender,1)+cr("FIX password",d.fix_password||"(not set)")+cr("ITCH feed",ep.itch_ip+" : "+ep.itch_port,1)+cr("ITCH username",d.itch_user||"—",1)+cr("ITCH password",d.itch_password||"(not set)")+"</div>"+
  "<div class=guide><span class=cap>How to connect</span><ol><li>"+(ep.ext_tls?"Open a <b>TLS</b> connection to <code>"+esc(ep.ext_addr)+"</code> (SocketUseSSL=Y).":"Open a TCP connection to <code>"+esc(ep.ext_addr)+"</code>.")+"</li><li>Send <b>Logon (35=A)</b> with your SenderCompID, <code>TargetCompID="+esc(ep.target)+"</code>, Username (553) and Password (554).</li><li>Send <b>NewOrderSingle (35=D)</b>; you receive <b>ExecutionReport (35=8)</b> for every change. Cancel <code>35=F</code>, amend <code>35=G</code>.</li><li>Market data comes on the <b>ITCH</b> feed with the username and password above.</li></ol></div>"+
  "<div class=acts><button class='btn red' type=button style='margin-right:auto' onclick=\"del("+jsa(d.sender)+")\">Revoke</button><button class=btn type=button onclick='sheetFrom(window._detail)'>Onboarding sheet ↗</button><button class='btn pri' type=button onclick='dlgClose()'>Close</button></div>","xwide")})}
function del(sender){confirmDlg("Revoke "+sender+"?","The broker's FIX and ITCH logins stop working at once. A session already logged on stays connected until you log it out on FIX Sessions.","Revoke",true,function(){
 jpost("broker/del",{Sender:sender}).then(function(r){if(r&&r.ok===false)return dlgErr(r.error||"Failed");dlgClose();toast(sender+" revoked");loadBrokers()})})}
// ── OMS IP allowlist ──
var _al=[];
function loadAllow(){j("allowlist").then(function(d){_al=d.list||[];renderAllow()})}
function renderAllow(){var f=flt("f_allow"),rows=_al.filter(function(r){return hit(r.ip+" "+(r.label||""),f)});
 seth("allowList",rows.map(function(r){return "<tr><td class=m>"+esc(r.ip)+"</td><td class=muted>"+(r.label?esc(r.label):"—")+"</td><td class=r><button class='btn sm red' onclick=\"delAllow("+jsa(r.ip)+")\">Remove</button></td></tr>"}).join("")||"<tr><td colspan=3 class=empty>"+(_al.length?"No entries match.":"No addresses yet — the gateway is open to every address.")+"</td></tr>");
 setc("allowCount",_al.length+" allowed"+(_al.length?" · connections from any other address are refused at the gateway":""))}
function addAllow(){var ip=val("a_ip"),label=val("a_label");setc("a_err","");if(!ip)return setc("a_err","Enter an IP address.");
 jpost("allowlist/add",{IP:ip,Label:label}).then(function(r){if(!r.ok)return setc("a_err",r.error||"Not added");document.getElementById("a_ip").value="";document.getElementById("a_label").value="";toast(ip+" added");loadAllow()})}
function delAllow(ip){confirmDlg("Remove "+ip+"?","New connections from this address are refused. Sessions already connected from it are not dropped.","Remove",true,function(){
 jpost("allowlist/del",{IP:ip}).then(function(){dlgClose();toast(ip+" removed");loadAllow()})})}
// ── Users ──
var _users=[];
function loadUsers(){j("users").then(function(d){_users=d.users||[];renderUsers()})}
function renderUsers(){var f=flt("f_user"),rows=_users.filter(function(u){return hit(u.username+" "+(u.full_name||"")+" "+(u.email||""),f)});
 seth("userList",rows.map(function(u){var me=u.username==ME_USER;
  return "<tr><td><span style='display:flex;align-items:center;gap:10px'><span class=avatar>"+esc(u.username.charAt(0).toUpperCase())+"</span><span class=mono style='font-weight:500'>"+esc(u.username)+"</span>"+(me?"<span class=faint style='font-size:11px'>(you)</span>":"")+"</span></td>"+
   "<td class=wrap style='font-size:12px;line-height:1.5'>"+(u.full_name?esc(u.full_name)+"<br><span class=faint>"+esc(u.email||"")+(u.phone?" · "+esc(u.phone):"")+"</span>":"<span class=faint>—</span>")+"</td>"+
   "<td><span class='rolebadge "+(u.role=="admin"?"":"user")+"'>"+esc(u.role)+"</span></td><td class='m muted' style='font-size:12px'>"+esc(String(u.created||"—").slice(0,10))+"</td><td class='m muted' style='font-size:12px'>"+esc(u.last_login?String(u.last_login).slice(5,16):"never")+"</td>"+
   "<td><span class=sdot style='color:"+(u.disabled?"var(--mut2)":"var(--acc)")+"'><i></i>"+(u.disabled?"Disabled":"Active")+"</span></td>"+
   "<td class=r><div class=acts><button class='btn sm' onclick=\"editUser("+jsa(u.username)+")\">Edit</button><button class='btn sm' onclick=\"userDlg('pw',"+jsa(u.username)+")\">Set password</button>"+(me?"":"<button class='btn sm red' onclick=\"delUser("+jsa(u.username)+")\">Delete</button>")+"</div></td></tr>"}).join("")||"<tr><td colspan=7 class=empty>No users match.</td></tr>");
 setc("userCount",_users.length+" account"+(_users.length==1?"":"s"))}
var _uRole="user";
function roleSeg(){return "<div class=fld>Role<div class=sidesw style='grid-template-columns:1fr 1fr'><button type=button class='b"+(_uRole=="admin"?" on":"")+"' onclick=\"_uRole='admin';this.classList.add('on');this.nextSibling.classList.remove('on')\">Admin</button><button type=button class='b"+(_uRole=="user"?" on":"")+"' onclick=\"_uRole='user';this.classList.add('on');this.previousSibling.classList.remove('on')\">User</button></div></div>"}
function editUser(name){userDlg("edit",name)}
function userDlg(mode,name){var u=_users.filter(function(x){return x.username==name})[0]||{},self=name==ME_USER;_uRole=u.role||"user";
 var h=mode=="new"?"<h3>Add user</h3><label class=fld>Username<input class='in mono' id=u_name placeholder='3–32 letters, digits . _ -'></label>"+roleSeg()+"<label class=fld>Password<input class=in type=password id=u_pass autocomplete=new-password></label><span class=note>10 to 128 characters, with at least 3 of: lower-case, upper-case, digits, symbols.</span>"
  :mode=="edit"?"<h3>Edit "+esc(name)+"</h3>"+roleSeg()+"<label class=chk><input type=checkbox id=u_dis"+(u.disabled?" checked":"")+(self?" disabled":"")+">Disabled (cannot sign in)</label>"+(self?"<span class=note>You cannot disable your own account.</span>":"")
  :"<h3>Set password for "+esc(name)+"</h3><label class=fld>New password<input class=in type=password id=u_pass autocomplete=new-password></label><span class=note>10 to 128 characters. The account is signed out everywhere.</span>";
 dlg(h+"<span class=err id=dlg_err></span><div class=acts><button class=btn type=button onclick='dlgClose()'>Cancel</button><button class='btn pri' type=button onclick=\"userSave('"+mode+"',"+jsa(name||"")+")\">"+(mode=="new"?"Create user":mode=="edit"?"Save":"Set password")+"</button></div>")}
function userSave(mode,name){var done=function(x,msg){if(!x.ok)return dlgErr(x.error||"Not saved");dlgClose();toast(msg);loadUsers();if(name==ME_USER)loadMe()};
 if(mode=="new"){var n=val("u_name");return jpost("user/add",{Username:n,Password:document.getElementById("u_pass").value,Role:_uRole}).then(function(x){done(x,"User "+n+" created")})}
 if(mode=="pw")return jpost("user/password",{Username:name,Password:document.getElementById("u_pass").value}).then(function(x){done(x,"Password set for "+name)});
 var u=_users.filter(function(x){return x.username==name})[0]||{},b={Username:name},dis=document.getElementById("u_dis").checked;
 if(_uRole!=u.role)b.Role=_uRole;if(dis!=!!u.disabled)b.Disabled=dis;
 if(b.Role===undefined&&b.Disabled===undefined)return dlgClose();
 jpost("user/update",b).then(function(x){done(x,name+" updated")})}
function delUser(name){confirmDlg("Delete "+name+"?","The account is removed and signed out everywhere. Brokers it registered stay, managed by the admins.","Delete",true,function(){
 jpost("user/delete",{Username:name}).then(function(x){if(!x.ok)return dlgErr(x.error||"Not deleted");dlgClose();toast(name+" deleted");loadUsers()})})}

// ── Message log: the latest FIX messages, filtered and decoded here ──
var MT={A:"Logon","5":"Logout","0":"Heartbeat","1":"TestRequest","2":"ResendRequest","4":"SequenceReset",D:"NewOrderSingle","8":"ExecutionReport",F:"OrderCancelRequest",G:"OrderCancelReplace","9":"OrderCancelReject","3":"Reject",j:"BusinessReject",H:"OrderStatusRequest",B:"News"};
var TN={8:"BeginString",9:"BodyLength",35:"MsgType",49:"SenderCompID",56:"TargetCompID",34:"MsgSeqNum",52:"SendingTime",50:"SenderSubID",57:"TargetSubID",98:"EncryptMethod",108:"HeartBtInt",141:"ResetSeqNumFlag",553:"Username",554:"Password",1137:"DefaultApplVerID",1128:"ApplVerID",
 1:"Account",11:"ClOrdID",41:"OrigClOrdID",55:"Symbol",54:"Side",38:"OrderQty",44:"Price",99:"StopPx",40:"OrdType",59:"TimeInForce",60:"TransactTime",126:"ExpireTime",432:"ExpireDate",37:"OrderID",17:"ExecID",150:"ExecType",39:"OrdStatus",14:"CumQty",151:"LeavesQty",6:"AvgPx",31:"LastPx",32:"LastQty",
 58:"Text",103:"OrdRejReason",102:"CxlRejReason",434:"CxlRejResponseTo",45:"RefSeqNum",371:"RefTagID",372:"RefMsgType",373:"SessionRejectReason",380:"BusinessRejectReason",453:"NoPartyIDs",448:"PartyID",447:"PartyIDSource",452:"PartyRole",
 762:"SecuritySubType",529:"OrderRestrictions",880:"TrdMatchID",1057:"AggressorIndicator",64:"SettlDate",75:"TradeDate",381:"GrossTradeAmt",111:"MaxFloor",1138:"DisplayQty",790:"OrdStatusReqID",148:"Headline",10:"CheckSum"};
var ENUM={54:{1:"Buy",2:"Sell"},40:{1:"Market",2:"Limit",3:"Stop",4:"Stop limit",Z:"Market at best"},59:{0:"Day",1:"GTC",3:"IOC",4:"FOK",6:"GTD"},150:{0:"New",4:"Canceled",5:"Replaced",8:"Rejected",C:"Expired",F:"Trade",A:"Pending new",I:"Order status"},
 39:{0:"New",1:"Partially filled",2:"Filled",4:"Canceled",8:"Rejected",C:"Expired",A:"Pending new"},35:MT,1057:{Y:"Aggressor",N:"Passive"}};
var ML={msgs:[],type:"",dir:"",paused:false,sel:-1};
function mlLoad(){if(ML.paused)return;j("fixlog?limit=500").then(function(d){ML.msgs=(d.messages||[]);mlRender()})}
function mlPause(){ML.paused=!ML.paused;setc("ml_pause",ML.paused?"Resume live":"Pause live");if(!ML.paused)mlLoad()}
function mlFields(raw){return String(raw||"").split("|").filter(function(x){return x}).map(function(kv){var i=kv.indexOf("=");return [kv.slice(0,i),kv.slice(i+1)]})}
function mlGet(f,t){for(var i=0;i<f.length;i++)if(f[i][0]==t)return f[i][1];return ""}
function mlSummary(m,f){var g=function(t){return mlGet(f,t)},t=m.msg_type,s=[];
 if(t=="D"||t=="G"||t=="F")s.push(g(55),(ENUM[54][g(54)]||"")+" "+g(38)+(g(44)?" @ "+g(44):""),"11="+g(11));
 else if(t=="8"){s.push((ENUM[150][g(150)]||g(150)),g(55),(ENUM[54][g(54)]||""));if(g(32))s.push(g(32)+" @ "+g(31));s.push("cum "+g(14)+" leaves "+g(151));if(g(58))s.push(g(58))}
 else if(t=="9"||t=="3"||t=="j")s.push(g(58)||"rejected","11="+g(11));
 else if(t=="A")s.push("HeartBtInt "+g(108));
 else if(t=="5")s.push(g(58));
 return s.filter(function(x){return x&&x.trim()}).join(" · ")}
function mlHit(m,q){if(!q)return true;var r="|"+String(m.raw).toLowerCase()+"|";return q.indexOf("=")>0?r.indexOf("|"+q+"|")>=0||r.indexOf("|"+q)>=0:r.indexOf(q)>=0||String(m.session).toLowerCase().indexOf(q)>=0}
function mlRender(){var q=flt("ml_q"),se=val("ml_sess");
 var sessions={};ML.msgs.forEach(function(m){sessions[m.session]=1});
 var ss=document.getElementById("ml_sess"),cur=ss.value;ss.innerHTML="<option value=''>All sessions</option>"+Object.keys(sessions).sort().map(function(s){return "<option>"+esc(s)+"</option>"}).join("");ss.value=cur;
 var base=ML.msgs.filter(function(m){return (!se||m.session==se)&&(!ML.dir||m.dir==ML.dir)&&mlHit(m,q)}),cnt={};
 base.forEach(function(m){cnt[m.msg_type]=(cnt[m.msg_type]||0)+1});
 seth("ml_types",["" ].concat(Object.keys(cnt).sort()).map(function(t){return "<button type=button class='mtype"+(ML.type==t?" on":"")+"' onclick=\"ML.type="+jsa(t)+";mlRender()\"><b>"+(t?esc(t):"All")+"</b> "+(t?esc(MT[t]||"Message"):"")+" "+(t?cnt[t]:base.length)+"</button>"}).join(""));
 var list=base.filter(function(m){return !ML.type||m.msg_type==ML.type});ML.view=list;
 seth("msglog",list.slice(0,150).map(function(m,i){var f=mlFields(m.raw),t=m.msg_type,col=t=="8"?"var(--acc)":(t=="3"||t=="9"||t=="j")?"var(--red)":(t=="D"||t=="F"||t=="G")?"var(--blue)":"var(--mut)";
  return "<tr class='ck"+(ML.selKey==m.time+m.raw.length?" sel":"")+"' onclick='mlOpen("+i+")'><td class='m faint'>"+esc(String(m.time).replace("T"," ").slice(11,23))+"</td><td class=m style='font-weight:600;color:"+(m.dir=="IN"?"var(--blue)":"var(--acc)")+"'>"+esc(m.dir)+"</td><td class=m>"+esc(m.session)+"</td><td style='color:"+col+";font-size:13px'>"+esc(MT[t]||t)+"</td><td class='r m muted'>"+esc(mlGet(f,"34"))+"</td><td class='m muted ell' style='max-width:420px'>"+esc(mlSummary(m,f))+"</td></tr>"}).join("")||"<tr><td colspan=6 class=empty>No messages match.</td></tr>");
 setc("ml_count",Math.min(150,list.length)+" shown of "+list.length+" matching, "+ML.msgs.length+" recent messages (heartbeats are not logged) · newest first"+(ML.paused?" · paused":" · live"))}
function mlOpen(i){var m=(ML.view||[])[i];if(!m)return;ML.selKey=m.time+m.raw.length;mlRender();var f=mlFields(m.raw);
 seth("ml_detail","<div class=tbar style='align-items:baseline'><span style='font-size:16px;font-weight:600'>"+esc(MT[m.msg_type]||m.msg_type)+"</span><span class='mono faint' style='font-size:12px'>"+esc(m.dir)+" · "+esc(String(m.time).replace("T"," ").slice(0,23))+"</span></div>"+
  "<div class=raw>"+esc(m.raw)+"</div><div>"+f.map(function(kv){var e=ENUM[kv[0]]&&ENUM[kv[0]][kv[1]];return "<div class=ftag><span>"+esc(kv[0])+"</span><span>"+esc(TN[kv[0]]||"")+"</span><span>"+esc(kv[1])+(e?" <span class=faint>("+esc(e)+")</span>":"")+"</span></div>"}).join("")+"</div>")}
document.getElementById("ml_dir").onclick=function(e){var b=e.target.closest("button");if(!b)return;ML.dir=b.dataset.d;this.querySelectorAll("button").forEach(function(x){x.classList.toggle("on",x==b)});mlRender()};
// ── Diagnostics ──
var DG={tab:"all",d:null};
function dgLoad(){j("diagnostics").then(function(d){DG.d=d;renderDiag()})}
function renderDiag(){var d=DG.d,s=_s||{};
 if(d){var rt=d.runtime||{},en=d.engine||{};
  seth("dg_kpis",[["Uptime",rt.uptime,"version "+(rt.version||"")],["Memory in use",fmtN(rt.heap_mb,1)+" MB",fmtN(Math.round(rt.sys_mb))+" MB from the system · "+rt.gc_runs+" GC runs"],["Goroutines",fmtN(rt.goroutines),rt.cpus+" CPUs · "+rt.go],["Resting orders",fmtN(en.resting_orders),fmtN(en.orders)+" orders · "+fmtN(en.trades)+" trades numbered · "+en.instruments+" instruments"]].map(function(k){return "<div class=kc><span class=l>"+k[0]+"</span><span class=v>"+esc(k[1])+"</span><span class=s>"+esc(k[2])+"</span></div>"}).join(""));
  seth("dg_cfg",(d.config||[]).map(function(c){return "<div style='display:grid;grid-template-columns:160px minmax(0,1fr);gap:10px;padding:7px 18px;border-top:1px solid var(--line);font-size:13px'><span class=muted>"+esc(c[0])+"</span><span class=mono style='font-size:12px;word-break:break-word'>"+esc(c[1])+"</span></div>"}).join(""))}
 var L=s.listeners||{},desc={FIX:"Broker order entry",FIXS:"TLS FIX gateway",Gateway:"Console & REST API",ITCH:"Market data feed"};
 seth("dg_lst",Object.keys(L).sort().map(function(n){var up=L[n];return "<div style='display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;align-items:center;padding:10px 18px;border-top:1px solid var(--line)'><div style='display:flex;flex-direction:column'><span class=mono style='font-size:13px'>"+esc(n)+"</span><span class=faint style='font-size:12px'>"+esc(desc[n.split(" ")[0]]||"")+"</span></div><span class='upc"+(up?"":" dn")+"'><i></i>"+(up?"UP":"DOWN")+"</span></div>"}).join("")+
  "<div style='display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;align-items:center;padding:10px 18px;border-top:1px solid var(--line)'><span style='font-size:13px'>FIX logon check</span><span class='tag "+(s.fix_auth?"t-acc":"t-amber")+"'>"+(s.fix_auth?"ENFORCED":"OPEN")+"</span></div>"+
  "<div style='display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;align-items:center;padding:10px 18px;border-top:1px solid var(--line)'><span style='font-size:13px'>ITCH login check</span><span class='tag "+(s.itch_auth?"t-acc":"t-amber")+"'>"+(s.itch_auth?"ENFORCED":"OPEN")+"</span></div>");
 var tabs=[["all","All"],["login","Sign-ins"],["broker","Brokers"],["market","Market"],["user","Users"],["fix","FIX sessions"]];
 seth("dg_tabs",tabs.map(function(t){return "<button type=button class='"+(DG.tab==t[0]?"on":"")+"' onclick=\"DG.tab='"+t[0]+"';renderDiag()\">"+t[1]+"</button>"}).join(""));
 var kind=function(a){a=String(a);return /^login|^logout/.test(a)?"login":/^(ISSUE|REVOKE|SESSION DISCONNECT|SESSION RESET)/.test(a)?"broker":/^(SESSION|instrument)/.test(a)?"market":/^user/.test(a)?"user":"other"};
 var col=function(a){return /REVOKE|delist|delete|disable|fail|locked|DISCONNECT/.test(a)?"t-red":/ISSUE|add|enable|login\.ok|list/.test(a)?"t-acc":/RESET|halt|SESSION/.test(a)?"t-amber":"t-mut"};
 var rows=DG.tab=="fix"?(s.events||[]).map(function(e){return "<tr><td class='m faint' style='font-size:12px;width:150px'>"+esc(String(e.time).replace("T"," ").slice(0,19))+"</td><td class=m style='font-size:12px'>"+esc(e.broker)+"</td><td><span class='tag "+(e.kind=="LOGON"?"t-acc":e.kind=="REJECT"?"t-red":"t-mut")+"'>"+esc(e.kind)+"</span></td><td class=muted>"+esc(e.detail)+"</td></tr>"})
  :(s.audit||[]).filter(function(a){return DG.tab=="all"||kind(a.action)==DG.tab}).map(function(a){var p=String(a.actor).split(" (");
   return "<tr><td class='m faint' style='font-size:12px;width:150px'>"+esc(String(a.time).replace("T"," ").slice(0,19))+"</td><td class=m style='font-size:12px'>"+esc(p[0])+"</td><td><span class='tag "+col(a.action)+"'>"+esc(a.action)+"</span></td><td class=muted>"+esc(a.target)+"</td><td class='m faint' style='font-size:12px'>"+esc(p[1]?p[1].replace(")",""):"")+"</td></tr>"});
 seth("audit",rows.join("")||"<tr><td class=empty>No entries.</td></tr>")}
function dgTest(){var b=document.getElementById("dg_run");b.disabled=true;b.textContent="Running…";setc("dg_done","");
 j("diagnostics?test=1").then(function(d){b.disabled=false;b.textContent="Run self-test";DG.d=d;renderDiag();var c=d.checks||[],bad=c.filter(function(x){return !x.ok}).length;
  seth("dg_checks",c.map(function(x){return "<div style='display:grid;grid-template-columns:18px minmax(0,1fr) auto;gap:10px;align-items:center;font-size:13px'><span style='width:10px;height:10px;border-radius:50%;background:"+(x.ok?"var(--acc)":"var(--red)")+"'></span><span>"+esc(x.name)+"</span><span class='mono faint' style='font-size:12px'>"+esc(x.result)+"</span></div>"}).join(""));
  var e=document.getElementById("dg_done");e.textContent=bad?bad+" of "+c.length+" checks failed at "+d.tested_at+".":"All "+c.length+" checks passed at "+d.tested_at+".";e.style.color=bad?"var(--red)":"var(--acc)"})}
// ── Simulator: market bots and benchmark (/admin/api/sim) ──
var SM={d:null,paused:false,mode:"trade"};
function smLoad(){return j("sim").then(function(d){SM.d=d;smRender()})}
function smSet(){document.getElementById("sm_blistf").hidden=val("sm_bset")!="list";document.getElementById("sm_bclistf").hidden=val("sm_bcl")!="list"}
(function(){try{var s=JSON.parse(localStorage.getItem("me_sim_oms")||"{}");if(s.url)document.getElementById("sm_burl").value=s.url;if(s.user)document.getElementById("sm_buser").value=s.user}catch(e){}})();
document.getElementById("sm_qmode").onclick=function(e){var b=e.target.closest("button");if(!b)return;SM.mode=b.dataset.m;this.querySelectorAll("button").forEach(function(x){x.classList.toggle("on",x==b)})};
function smKs(id,list){seth(id,list.map(function(k){return "<div><span class=l>"+esc(k[0])+"</span><span class='v"+(k[2]?" "+k[2]:"")+"'>"+esc(k[1])+"</span></div>"}).join(""))}
function smTag(id,state,cls){var e=document.getElementById(id);e.textContent=String(state).toUpperCase();e.className="tag "+cls}
function smRender(){var d=SM.d;if(!d)return;
 if(!d.ok){setc("sm_phnote",d.error||"The simulator is not available.");["sm_bstart","sm_qstart","sm_bclean"].forEach(function(i){document.getElementById(i).disabled=true});return}
 var ph=d.phase||"",open=ph=="Continuous Trading"||ph=="Pre-Open";
 smTag("sm_phase",ph,open?"t-acc":"t-amber");setc("sm_phnote",open?(ph=="Pre-Open"?"bots only quote until the market opens":"bots trade now"):"bots wait until Pre-Open or Continuous Trading");
 smBotsRender(d.bots||{});
 var q=d.bench||{},qs=q.state||"idle",qrun=qs=="running",qq=q.stats||{},acks=(qq.accepted||0)+(qq.rejected||0);
 smTag("sm_qstate",qs,qrun?"t-acc":qs=="done"?"t-blue":"t-mut");
 document.getElementById("sm_qstart").disabled=qrun;document.getElementById("sm_qstop").disabled=!qrun;
 setc("sm_qnow",fmtN(qrun?q.throughput_now:q.throughput_avg));
 document.querySelector(".sm-tp .faint").textContent=qrun?"acknowledged orders/s":(qs=="idle"?"acknowledged orders/s":"average orders/s over the run");
 smKs("sm_qk",[["Sent",fmtN(qq.sent)],["Acknowledged",fmtN(acks)],["Trades",fmtN(qq.trades),qq.trades?"good":""],["Rejected",fmtN(qq.rejected),qq.rejected?"bad":""],["Peak/s",fmtN(q.throughput_peak)],["Time",q.elapsed!=null?fmtN(q.elapsed,1)+" s":"—"]]);
 var L=(q.latency||{}),row=function(n,x){x=x||{};return "<tr><td>"+n+"</td>"+["avg","p50","p95","p99","max"].map(function(k){return "<td class=r>"+(x.n?fmtN(x[k],2):"—")+"</td>"}).join("")+"<td class=r>"+fmtN(x.n||0)+"</td></tr>"};
 seth("sm_qlat",row("Acknowledgement",L.ack)+row("First fill",L.fill));
 var sr=(q.series||[]).slice(-60),mx=Math.max.apply(null,sr.concat([1]))*1.15,w=300,h=60;
 seth("sm_qspark",sr.length<2?"<line x1=0 y1=59 x2=300 y2=59 stroke='var(--line)'/>":"<polyline fill='var(--accSoft)' stroke='none' points='0,"+h+" "+sr.map(function(v,i){return (i*w/(sr.length-1)).toFixed(1)+","+(h-2-(v/mx)*(h-6)).toFixed(1)}).join(" ")+" "+w+","+h+"'/><polyline fill='none' stroke='var(--acc)' stroke-width='1.5' vector-effect='non-scaling-stroke' points='"+sr.map(function(v,i){return (i*w/(sr.length-1)).toFixed(1)+","+(h-2-(v/mx)*(h-6)).toFixed(1)}).join(" ")+"'/>");
 var stc={New:"t-blue",Amended:"t-blue",Filled:"t-acc","Partly filled":"t-acc",Cancelled:"t-amber",Expired:"t-amber",Rejected:"t-red"};
 seth("sm_tape",(d.tape||[]).map(function(r){return "<tr><td class='m faint' style='font-size:12px'>"+esc(r.time)+"</td><td class=m style='font-size:12px'>"+esc(r.bot)+"</td><td><b>"+esc(r.symbol)+"</b></td><td style='font-weight:600;color:"+(r.side=="Buy"?"var(--acc)":"var(--red)")+"'>"+esc(r.side)+"</td><td class='r m'>"+fmtN(r.qty)+"</td><td class='r m'>"+(r.price?fmtN(r.price,2):"—")+"</td><td><span class='tag "+(stc[r.status]||"t-mut")+"'>"+esc(r.status)+"</span></td><td class='muted ell' title='"+esc(r.note||"")+"'>"+esc(r.note||"")+"</td></tr>"}).join("")||"<tr><td colspan=8 class=empty>Nothing yet. Start the market bots or run a benchmark.</td></tr>");
 seth("sm_rej",(d.rejects||[]).map(function(r){return "<div class=sm-rej><span class=muted>"+esc(r.reason)+"</span><b class=mono>"+fmtN(r.count)+"</b></div>"}).join("")||"<p class=note>None.</p>")}
function smBotsRender(b){var st=b.state||"stopped",on=st=="running"||st=="paused",held=!on&&!!b.dealer,work=b.working||0,orph=b.orphans||0,left=b.leftover||0;SM.paused=st=="paused";
 smTag("sm_bstate",st,st=="running"?(b.note||!b.connected?"t-amber":"t-acc"):st=="paused"||held?"t-amber":"t-mut");
 setc("sm_bhead",b.dealer?"dealer "+b.dealer+(b.dealer_name&&b.dealer_name!=b.dealer?" ("+b.dealer_name+")":"")+" · "+b.tenant+(b.env=="uat"?" UAT":""):"");
 document.getElementById("sm_bstart").disabled=on;document.getElementById("sm_bstop").disabled=!on;document.getElementById("sm_bclean").disabled=on||!(held||work||orph||left);
 var pb=document.getElementById("sm_bpause");pb.disabled=!on;pb.textContent=SM.paused?"Resume":"Pause";
 ["sm_burl","sm_buser","sm_bpass","sm_bremember","sm_brate","sm_bcl","sm_bclist","sm_bset","sm_blist","sm_bmax"].forEach(function(i){document.getElementById(i).disabled=on});
 smSaved(b.saved_login);
 var s=b.stats||{};
 smKs("sm_bk",[["Sent",fmtN(s.sent)],["Accepted by OMS",fmtN(s.accepted)],["Fills",fmtN(s.trades),s.trades?"good":""],["Rejected",fmtN(s.rejected),s.rejected?"bad":""],["Resting",fmtN(work+orph+left)],["Orders/min",on?fmtN(b.orders_per_min,1):"—"]]);
 var lost=orph?" "+fmtN(orph)+" bot order"+(orph==1?"":"s")+" from earlier runs "+(orph==1?"is":"are")+" still in the book with no run following "+(orph==1?"it":"them")+"; Clean Data cancels "+(orph==1?"it":"them")+".":"";
 if(left)lost+=" "+fmtN(left)+" order"+(left==1?"":"s")+" of the old engine-only bots "+(left==1?"is":"are")+" still in the book; Clean Data cancels "+(left==1?"it":"them")+".";
 if(on)setc("sm_bnote",b.note||("Trading "+(b.symbols||[]).length+" shares for "+(b.clients||[]).length+" clients through "+b.site+" · running "+fmtN(b.running_for)+" s"));
 else if(held)setc("sm_bnote","Stopped: "+fmtN(work)+" bot order"+(work==1?"":"s")+" stay in the book and the bots stay signed in, so the depth and the Blotter stay as they were. Clean Data cancels them through the OMS and signs out; Start replaces them with fresh ones."+lost);
 else setc("sm_bnote","Simulated dealers that sign in to your OMS and trade for the dealer's clients on its own order channel, so their orders show in the OMS Blotter, Execution, Positions and Purchase Power. They quote a few ticks around a moving fair price, or take the best price with an IOC order, so they trade with each other. Buys stay under the largest-buy value (clients need cash); sells only use shares the OMS says the client holds. Use a dealer login without a one-time code. With Remember ticked, the login is saved on the engine (the password is never shown or logged), so the next Start works with the password left blank."+lost);
 seth("sm_bsess",(b.clients||[]).map(function(c){return "<span class='sm-bot"+(b.connected?"":" dn")+"' title='"+esc((c.name||c.code)+" · BO "+c.bo+(c.sells?" · holds "+c.holdings+" shares":" · holdings unknown: buys only"))+"'><i></i>"+esc(c.code)+" <span class=faint>"+fmtN(c.working)+"</span></span>"}).join(""));}
function smBots(a){var body={action:a};
 if(a=="start"){var set=val("sm_bset"),cl=val("sm_bcl");body.url=val("sm_burl");body.user=val("sm_buser");body.password=document.getElementById("sm_bpass").value;
  body.rate=+val("sm_brate");body.max_value=+val("sm_bmax");body.remember=document.getElementById("sm_bremember").checked;
  if(!body.url||!body.user){toast("Enter the OMS address and the dealer login.",true);return}
  if(!body.password&&!document.getElementById("sm_bpass").placeholder){toast("Enter the dealer's password (none is saved for this login).",true);return}
  if(cl=="list"){body.clients=val("sm_bclist").split(/[\s,]+/).filter(Boolean);if(!body.clients.length){toast("Type at least one client code, or pick a number of clients.",true);return}}else body.client_count=+cl;
  if(set=="list"){body.symbols=val("sm_blist").split(/[\s,]+/).filter(Boolean);if(!body.symbols.length){toast("Type at least one symbol, or pick a set of shares.",true);return}}else body.count=+set;
  try{localStorage.setItem("me_sim_oms",JSON.stringify({url:body.url,user:body.user}))}catch(e){}}
 ["sm_bstart","sm_bpause","sm_bstop","sm_bclean"].forEach(function(i){document.getElementById(i).disabled=true});
 if(a=="start"||a=="stop")smTag("sm_bstate",a=="start"?"signing in":"stopping","t-amber");
 jpost("sim/bots",body).then(function(d){if(d.ok===false){toast(d.error||"The market bots could not "+a+".",true);smLoad();return}
  if(a=="start")document.getElementById("sm_bpass").value="";SM.d=d;smRender();
  toast({start:"Market bots signed in and trading",stop:"Market bots stopped; their orders stay in the book",pause:"Market bots paused",resume:"Market bots resumed"}[a])})}
// smSaved shows the login saved on the engine (never its password) and fills empty fields from it
function smSaved(sv){var u=document.getElementById("sm_burl"),n=document.getElementById("sm_buser"),p=document.getElementById("sm_bpass"),e=document.getElementById("sm_bsaved");
 SM.saved=sv||null;e.hidden=!sv;p.placeholder="";if(!sv)return;
 if(!u.value&&document.activeElement!==u)u.value=sv.url;if(!n.value&&document.activeElement!==n)n.value=sv.user;
 var same=n.value.trim().toLowerCase()==String(sv.user).toLowerCase()&&u.value.trim().replace(/\/+$/,"").toLowerCase()==String(sv.url).toLowerCase();
 if(same)p.placeholder="saved: leave blank to use it";
 e.innerHTML="Saved login: <b>"+esc(sv.user)+"</b> on "+esc(sv.url)+", saved "+esc(sv.saved)+". Leave the password blank to use it. <button class='btn sm' type=button onclick='smForget()'>Forget</button>"}
function smForget(){var sv=SM.saved||{};confirmDlg("Forget the saved login?","The saved password for "+(sv.user||"the dealer")+" is deleted from the engine. Bots that are signed in stay signed in.","Forget",true,function(){
 jpost("sim/bots",{action:"forget"}).then(function(d){if(d.ok===false)return dlgErr(d.error||"Failed");dlgClose();SM.d=d;smRender();toast("Saved OMS login deleted")})})}
// Clean Data: the bots' orders are cancelled through the OMS and the bots sign out; trades and the order report stay
function smBotsClean(){var b=(SM.d||{}).oms||{},n=(b.working||0)+(b.orphans||0);
 confirmDlg("Clean Data?",fmtN(n)+" market-bot order"+(n==1?"":"s")+" still in the book "+(n==1?"is":"are")+" cancelled (through the OMS, so the Blotter shows them cancelled) and the bots sign out of the OMS. Trades, prices and the order report stay.","Clean Data",true,function(){
  var ok=document.getElementById("dlg_ok");if(ok){ok.disabled=true;ok.textContent="Cleaning…"}
  jpost("sim/bots",{action:"clean"}).then(function(d){if(d.ok===false){if(ok){ok.disabled=false;ok.textContent="Clean Data"}smLoad();return dlgErr(d.error||"Clean Data failed.")}
   dlgClose();SM.d=d;smRender();toast("Clean Data: "+(d.detail||"done"))})})}
function smBench(a){var body={action:a};
 if(a=="start"){body.mode=SM.mode;body.rate=document.getElementById("sm_qmax").checked?0:+val("sm_qrate");body.seconds=+val("sm_qsec");body.symbol=val("sm_qsym");
  if(!body.symbol){toast("Type the share to benchmark, for example GP.",true);return}
  if(!document.getElementById("sm_qmax").checked&&!(body.rate>0)){toast("Set a rate above 0, or tick MAX.",true);return}}
 document.getElementById("sm_qstart").disabled=true;
 jpost("sim/bench",body).then(function(d){if(d.ok===false){toast(d.error||"The benchmark could not "+a+".",true);smLoad();return}SM.d=d;smRender();if(a=="stop")toast("Benchmark stopped")})}
function smPhase(a){confirmDlg(a=="open"?"Open the market?":"Start Pre-Open?",a=="open"?"Runs the opening auction and starts Continuous Trading for every broker, and switches the DSE clock to manual.":"Moves the market to Pre-Open for every broker, and switches the DSE clock to manual.",a=="open"?"Open market":"Start Pre-Open",false,function(){
 j("trading-session?action="+a+"&note="+encodeURIComponent("from the Simulator page"),{method:"POST"}).then(function(d){dlgClose();if(!d.ok){toast(d.error||"The phase was not changed.",true);return}toast("Market is now "+d.phase);smLoad();refresh()})})}
function smClean(){confirmDlg("Clean test data?","Stops the market bots and any benchmark, cancels every simulator order in the book (the market bots' through the OMS, signing them out), and removes the benchmark's orders from the order report (every day). Trades and prices stay.","Clean test data",true,function(){
 var b=document.getElementById("dlg_ok");if(b){b.disabled=true;b.textContent="Cleaning…"}
 j("sim/clean",{method:"POST"}).then(function(d){dlgClose();if(!d.ok){toast(d.error||"Cleaning failed.",true);smLoad();return}var r=d.result||{};toast("Cleaned: "+fmtN(r.cancelled)+" orders cancelled, "+fmtN(r.report_rows)+" order-report rows removed");smLoad()})})}

// ── Welcome: the engine at a glance, from the status poll, today's overview and the broker list ──
var WL={ov:null,instr:null,dis:{}};
try{WL.dis=JSON.parse(sessionStorage.getItem("me-wl-dismissed")||"{}")}catch(e){}
function wlLoad(){j("overview").then(function(d){if(d&&d.ok!==false){WL.ov=d;if(VIEW=="home")wlRender()}});
 if(ME_ROLE=="admin")j("instruments").then(function(d){WL.instr=d.rows||[];if(VIEW=="home")wlRender()});
 wlRender()}
// Dhaka wall clock: [hours, minutes, seconds] and the date line
function wlNow(){var p=new Intl.DateTimeFormat("en-GB",{timeZone:"Asia/Dhaka",hour:"2-digit",minute:"2-digit",second:"2-digit",hourCycle:"h23"}).formatToParts(new Date()),o={};
 p.forEach(function(x){o[x.type]=x.value});return [+o.hour,+o.minute,+o.second]}
function wlDate(){var f=function(k){return new Intl.DateTimeFormat("en-GB",{timeZone:"Asia/Dhaka",[k]:k=="day"?"numeric":"long"}).format(new Date())};
 return (f("weekday")+" · "+f("day")+" "+f("month")+" "+new Intl.DateTimeFormat("en-GB",{timeZone:"Asia/Dhaka",year:"numeric"}).format(new Date())).toUpperCase()}
function wlMin(hm){var p=String(hm||"").split(":");return p.length<2?-1:+p[0]*60+ +p[1]}
function wlIn(min){min=Math.max(0,Math.round(min));return min>=60?Math.floor(min/60)+"h "+(min%60)+"m":min+"m"}
function wlTk(v){v=+v||0;return v>=1e7?"Tk "+fmtN(v/1e7,2)+" Cr":v>=1e5?"Tk "+fmtN(v/1e5,2)+" Lakh":"Tk "+fmtN(v,0)}
// the clock and the phase pill, every second
function wlTick(){var d=_s||{},ov=WL.ov||{},sc=ov.schedule||{},ph=d.phase||ov.phase||"",t=wlNow(),m=t[0]*60+t[1],pad=function(n){return (n<10?"0":"")+n};
 var trading=ph=="Continuous Trading"||ph=="Pre-Open",note="";
 if(d.auto_clock===false||ov.auto_clock===false)note="clock manual";
 else if(ph=="Continuous Trading"&&wlMin(sc.close)>m)note="closes in "+wlIn(wlMin(sc.close)-m);
 else if(ph=="Pre-Open"&&wlMin(sc.open)>m)note="opens in "+wlIn(wlMin(sc.open)-m);
 else if(ov.trading_day&&wlMin(sc.pre_open)>m)note="pre-open at "+sc.pre_open;
 setc("wl_phase",ph||"—");document.getElementById("wl_pdot").style.background=trading?"var(--acc)":"var(--mut2)";
 setc("wl_clock",pad(t[0])+":"+pad(t[1])+":"+pad(t[2])+(note?" · "+note:""));
 return {ph:ph,trading:trading,hour:t[0]}}
// what needs a look: each item has a signature, so a dismissed one comes back when it changes
function wlAttention(){var d=_s||{},ov=WL.ov||{},tt=ov.totals||{},out=[],admin=ME_ROLE=="admin",ph=d.phase||ov.phase||"",trading=ph=="Continuous Trading"||ph=="Pre-Open";
 var live={};(d.sessions||[]).forEach(function(x){live[String(x.broker).toUpperCase()]=1});
 var down=((_b&&_b.brokers)||[]).map(function(b){return String(b.sender)}).filter(function(id){return !live[id.toUpperCase()]});
 if(down.length)out.push({id:"fix",sig:down.join(","),sev:trading?"red":"amber",title:down.length+" FIX session"+(down.length>1?"s":"")+" disconnected",
  detail:down.slice(0,4).join(", ")+(down.length>4?" and "+(down.length-4)+" more":"")+(down.length>1?" have":" has")+" not logged on"+(trading?" while the market is open.":"."),action:"View sessions",v:"sessions"});
 var lst=d.listeners||{},dl=Object.keys(lst).filter(function(k){return !lst[k]});
 if(admin&&dl.length)out.push({id:"listen",sig:dl.join(","),sev:"red",title:dl.join(", ")+(dl.length>1?" are":" is")+" down",detail:"Nothing can connect to "+(dl.length>1?"these ports":"this port")+" until the listener is back.",action:"Diagnostics",v:"diag"});
 if(admin&&d.state_age>30)out.push({id:"save",sig:"x",sev:"red",title:"The market was last saved "+Math.round(d.state_age)+" s ago",detail:"Order books and trades are normally saved every second. Check the database and the disk.",action:"Diagnostics",v:"diag"});
 var halted=(WL.instr||[]).filter(function(r){return r.status=="halted"}).map(function(r){return r.symbol});
 if(admin&&halted.length)out.push({id:"halt",sig:halted.join(","),sev:"amber",title:halted.length>1?halted.length+" instruments are halted":halted[0]+" is halted",
  detail:(halted.length>1?halted.slice(0,6).join(", ")+(halted.length>6?" …":"")+": n":"N")+"ew orders are rejected until "+(halted.length>1?"they are":"it is")+" activated. Resting orders keep their place.",action:"Open instruments",v:"instruments"});
 var tot=(tt.buy_orders||0)+(tt.sell_orders||0),rej=tt.rejected||0;
 if(rej>=10&&rej/Math.max(tot,1)>=.05)out.push({id:"rej",sig:"x",sev:"amber",title:"Rejections are high today",detail:fmtN(rej)+" of "+fmtN(tot)+" orders ("+fmtN(rej*100/tot,1)+"%) were refused on arrival.",action:"Review orders",v:"orders"});
 var q=d.fix_queued||{},qb=Object.keys(q).filter(function(k){return q[k]>0}),qn=0;qb.forEach(function(k){qn+=q[k]});
 if(admin&&qn)out.push({id:"queued",sig:qb.join(","),sev:"blue",title:fmtN(qn)+" report"+(qn>1?"s":"")+" waiting for "+qb.join(", "),detail:"Execution reports for a broker that is logged off are kept and sent when it logs on again.",action:"View sessions",v:"sessions"});
 if(admin&&d.auto_clock===false)out.push({id:"clock",sig:ph,sev:"blue",title:"The DSE clock is manual",detail:"Phases change only by hand: the market stays in "+(ph||"its phase")+" until someone moves it.",action:"Market session",v:"market"});
 return out.filter(function(a){return WL.dis[a.id]!==a.id+"|"+a.sig})}
function wlDismiss(id,sig){WL.dis[id]=id+"|"+sig;try{sessionStorage.setItem("me-wl-dismissed",JSON.stringify(WL.dis))}catch(e){}wlRender()}
function wlRender(){if(VIEW!="home")return;var d=_s||{},ov=WL.ov||{},tt=ov.totals||{},admin=ME_ROLE=="admin",now=wlTick(),col={red:"var(--red)",amber:"var(--amber)",blue:"var(--blue)",green:"var(--acc)",mut:"var(--mut2)"};
 setc("wl_date",wlDate());setc("wl_greet",(now.hour<12?"Good morning":now.hour<17?"Good afternoon":"Good evening")+(ME_USER?", "+ME_USER:""));
 var att=wlAttention(),urgent=att.filter(function(a){return a.sev=="red"}).length;
 var mk={"Continuous Trading":"The market is open and matching normally.","Pre-Open":"The market is in Pre-Open: orders are collected for the opening auction.","Post-Close":"The market is in Post-Close."}[now.ph]||(now.ph?"The market is "+now.ph.toLowerCase()+".":"");
 setc("wl_sum",mk+" "+(att.length?att.length+" item"+(att.length>1?"s need":" needs")+" a look"+(urgent?", "+urgent+" urgent":"")+".":"Nothing needs you right now."));
 // status cards
 var live={};(d.sessions||[]).forEach(function(x){live[String(x.broker).toUpperCase()]=1});
 var reg=((_b&&_b.brokers)||[]).map(function(b){return String(b.sender).toUpperCase()}),up=reg.filter(function(id){return live[id]}).length,dn=reg.length-up;
 var pt=(ov.prev&&ov.prev.totals)||null,tr=tt.trades||0,trSub=fmtN(tt.volume||0)+" shares";
 if(pt&&pt.trades){var ch=(tr-pt.trades)*100/pt.trades;trSub=(ch>=0?"▲ ":"▼ ")+fmtN(Math.abs(ch),1)+"% vs "+String(ov.prev.date).slice(5)}
 var traded=(ov.instruments||[]).filter(function(r){return r.trades>0}).length;
 var cards=[
  {l:"Engine",st:d.running===false?"DOWN":"UP",c:d.running===false?"red":"green",v:d.running===false?"Stopped":"Running",s:"uptime "+(d.uptime||"—"),go:admin?"diag":"dash"},
  reg.length?{l:"FIX sessions",st:dn?dn+" DOWN":"ALL UP",c:dn?"red":"green",v:up+" / "+reg.length,s:"logged on",go:"sessions"}
   :{l:"FIX sessions",st:"NONE",c:"mut",v:fmtN((d.sessions||[]).length),s:"no brokers registered yet",go:"creds"},
  {l:"Trades today",st:now.trading?"LIVE":"TODAY",c:now.trading?"green":"mut",v:fmtN(tr),s:trSub,go:"dash"},
  {l:"Turnover today",st:now.trading?"LIVE":"TODAY",c:now.trading?"green":"mut",v:wlTk(tt.turnover),s:!tr?"no trades yet":ov.scope=="own"?"your brokers' trades":"across "+traded+" instrument"+(traded==1?"":"s"),go:"dash"}];
 seth("wl_cards",cards.map(function(k){return "<button class=wl-card type=button onclick=\"show('"+k.go+"')\"><span class=l>"+esc(k.l)+"<span class=wl-st style='color:"+col[k.c]+"'><i></i>"+esc(k.st)+"</span></span><span class=v>"+esc(k.v)+"</span><span class=s>"+esc(k.s)+"</span></button>"}).join(""));
 // needs attention
 setc("wl_acnt",att.length?att.length+" open":"");
 seth("wl_attn",att.map(function(a){return "<div class=wl-item><span class=bar style='background:"+col[a.sev]+"'></span><div><b>"+esc(a.title)+"</b><span class=d>"+esc(a.detail)+"</span></div>"+
  "<div class=acts><button class=wl-dis type=button onclick='wlDismiss("+jsa(a.id)+","+jsa(a.sig)+")'>Dismiss</button><button class=wl-go type=button onclick=\"show('"+a.v+"')\">"+esc(a.action)+"</button></div></div>"}).join("")||"<div class=wl-clear><i></i>Nothing needs you right now.</div>");
 // quick actions: the pages this role has, with their sidebar icons
 var acts=admin?[["trade","Place an order"],["market","Market session"],["creds","Add a broker"],["access","Allow an IP"],["orders","Order report"],["diag","Run self-test"]]
  :[["trade","Place an order"],["creds","Add a broker"],["sessions","FIX sessions"],["orders","Order report"],["msglog","Message log"],["dash","Dashboard"]];
 seth("wl_acts",acts.map(function(a){var ic=document.querySelector('.navitem[data-v="'+a[0]+'"] svg');return "<button class=wl-act type=button onclick=\"show('"+a[0]+"')\"><svg viewBox='0 0 24 24'>"+(ic?ic.innerHTML:"")+"</svg><span>"+esc(a[1])+"</span></button>"}).join(""));
 // recent activity from the audit trail (a user sees their own entries)
 document.getElementById("wl_audit").style.display=admin?"":"none";
 seth("wl_recent",(d.audit||[]).slice(0,6).map(function(e){return "<div class=wl-rec><span class=t>"+esc(String(e.time||"").slice(11,16))+"</span><div><span>"+esc(wlAudit(e))+"</span><small>"+esc(String(e.actor||"").split(" (")[0])+"</small></div></div>"}).join("")||"<div class=wl-empty>No activity yet.</div>")}
// an audit entry in words
var WL_AUD={"login.ok":"Signed in to the console","login.fail":"Failed sign-in","login.locked":"Sign-in locked after too many attempts","logout":"Signed out",
 "user.add":"Added user","user.delete":"Deleted user","user.role":"Changed the role of","user.register":"Registered","user.password.self":"Changed own password","user.password.reset":"Reset the password of",
 "instruments.reload":"Reloaded instruments","instruments.import":"Imported instruments","instruments.export":"Exported instruments","instruments.delete_all":"Deleted all instruments",
 "instrument.add":"Listed","instrument.update":"Edited","instrument.halt":"Halted","instrument.resume":"Activated","instrument.delist":"Delisted",
 "ISSUE":"Issued credentials for","REVOKE":"Revoked credentials for","ALLOW+":"Allowed","ALLOW-":"Removed from the allowlist:",
 "SESSION OPEN":"Opened continuous trading","SESSION PREOPEN":"Moved to Pre-Open","SESSION CLOSE":"Moved to Close","SESSION POSTCLOSE":"Moved to Post-Close","SESSION ENDOFDAY":"Ended the trading day",
 "SESSION AUTO_ON":"Clock follows the schedule","SESSION AUTO_OFF":"Clock switched to manual","SESSION DISCONNECT":"Logged out FIX session","SESSION RESET":"Reset sequence numbers for"};
function wlAudit(e){var a=String(e.action||""),t=String(e.target||""),w=WL_AUD[a];
 if(a.indexOf("SESSION ")==0&&a!="SESSION DISCONNECT"&&a!="SESSION RESET"){var note=t.split(" · ").slice(1).join(" · ");return w?w+(note?": "+note:""):a}
 if(w)return w+(t&&!/^(instruments\.|login\.|logout|user\.password\.self)/.test(a)?" "+t:"");
 return a.charAt(0)+a.slice(1).toLowerCase().replace(/[._]/g," ")+(t?": "+t:"")}

// ── start ──
var _h0=location.hash.slice(1);
loadMe().then(function(){show(TITLES[_h0]?_h0:"home")});
refresh();loadBrokers();
setInterval(refresh,3000);
setInterval(function(){if(VIEW=="home")wlTick()},1000);
setInterval(function(){if(VIEW=="home")wlLoad()},15000);
setInterval(function(){if(VIEW=="dash"&&!_ovPaused&&OV&&OV.today)ovLoad()},3000);
setInterval(function(){if(VIEW=="dash"&&OV&&OV.today)ovSession();if(VIEW=="market")renderMarket()},1000);
setInterval(function(){if(VIEW=="trade")ttBook()},1500);
setInterval(function(){if(VIEW=="trade")ttMarket()},3000);
setInterval(function(){if(VIEW=="trade")ttMine()},5000);
setInterval(function(){if(VIEW=="orders"&&OR.today)loadOrders()},5000);
setInterval(function(){if(VIEW=="msglog")mlLoad()},3000);
setInterval(function(){if(VIEW=="sim")smLoad()},1000);
setInterval(function(){if(VIEW=="creds"||VIEW=="sessions")loadBrokers()},15000);
`
