package web

// WelcomeHTML is the landing page at "/": a welcome board with User and Admin
// sign-in. Both post to /admin/login; "as" tells the server which option was used.
const WelcomeHTML = `<!doctype html><html><head><meta charset="utf-8">` + ThemeHead + CSRFHead + `
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Welcome — Matching Engine</title>` + AdminFontLink + `
<style>
/* Tokens from the welcome design (dark; light below). surface = card, sunk = quiet fill, field = input fill. */
:root{--bg:#050706;--surface:#111714;--sunk:#0c110f;--border:#24302a;--text:#e5ece8;--muted:#9eaea5;--faint:#6f7e76;--accent:#3dbd84;--accentSoft:#173328;--onAccent:#06100b;--sell:#ee6a60;--field:#18211d;color-scheme:dark}
:root[data-theme="light"]{--bg:#eef1f6;--surface:#ffffff;--sunk:#f6f8fb;--border:#e1e5ec;--text:#15201a;--muted:#56645c;--faint:#87938c;--accent:#14804f;--accentSoft:#e2f1e8;--onAccent:#ffffff;--sell:#c23f37;--field:#eaf0fb;color-scheme:light}
*{box-sizing:border-box}
html,body{margin:0}
body{min-height:100vh;background:var(--bg);color:var(--text);font-family:"IBM Plex Sans",system-ui,-apple-system,sans-serif;font-size:14px;line-height:1.45;-webkit-font-smoothing:antialiased}
a{color:var(--accent);text-decoration:none}a:hover{text-decoration:underline}
input,button,select{font-family:inherit}
#flow{position:fixed;inset:0;width:100%;height:100%;display:block;z-index:0}
.wrap{position:relative;z-index:2;min-height:100vh;display:flex;flex-wrap:wrap;align-items:center;justify-content:center;gap:48px 64px;padding:72px 24px;pointer-events:none}
.board{flex:1 1 380px;max-width:480px;display:flex;flex-direction:column;gap:22px;pointer-events:auto}
.brand{display:flex;align-items:center;gap:10px}
.logo{width:34px;height:34px;border-radius:8px;background:var(--accent);color:var(--onAccent);display:grid;place-items:center;font-weight:700;font-size:16px}
.brand b{display:block;font-weight:600;font-size:15px}.brand small{display:block;font-family:"IBM Plex Mono",monospace;font-size:11px;letter-spacing:.06em;color:var(--faint)}
h1{margin:0;font-size:40px;font-weight:700;line-height:1.12;letter-spacing:-.025em;text-wrap:balance}
h1 span{color:var(--accent)}
/* text over the moving lines gets a quiet pad of the page colour, so it stays readable */
.lead{margin:0;font-size:15px;color:var(--muted);text-wrap:pretty;background:var(--bg);box-shadow:0 0 0 8px var(--bg);border-radius:4px}
.feat{display:flex;flex-direction:column;gap:12px;margin:0;padding:0;list-style:none;background:var(--bg);box-shadow:0 0 0 8px var(--bg);border-radius:4px}
.feat li{display:grid;grid-template-columns:22px 1fr;gap:12px;align-items:start;font-size:14px;color:var(--muted);text-wrap:pretty}
.feat i{width:22px;height:22px;border-radius:6px;background:var(--accentSoft);color:var(--accent);display:grid;place-items:center;font-style:normal;font-size:11px;font-weight:700;margin-top:1px}
.feat b{color:var(--text);font-weight:600}
.term{display:inline-flex;align-items:center;gap:8px;width:max-content;font-size:14px;font-weight:600;color:var(--text);background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:9px 16px;transition:border-color .15s,color .15s}
.term:hover{border-color:var(--accent);color:var(--accent);text-decoration:none}
.term svg{width:14px;height:14px}
.card{flex:0 1 420px;width:100%;max-width:420px;background:var(--surface);border:1px solid var(--border);border-radius:14px;padding:28px;display:flex;flex-direction:column;gap:18px;box-shadow:0 24px 60px -24px rgba(20,32,26,.25);pointer-events:auto;margin:0}
:root:not([data-theme="light"]) .card{box-shadow:0 24px 60px -24px rgba(0,0,0,.6)}
.card.reg{flex-basis:640px;max-width:640px}
.head{display:flex;flex-direction:column;gap:2px}
.head h2{margin:0;font-size:18px;font-weight:600}
.sub{font-size:13px;color:var(--muted);margin:0;min-height:18px}
.seg{display:grid;grid-template-columns:1fr 1fr;gap:4px;border:1px solid var(--border);border-radius:10px;padding:4px}
.seg button{font-size:13px;font-weight:600;border-radius:7px;padding:8px;cursor:pointer;border:1px solid transparent;background:transparent;color:var(--muted)}
.seg button.on{border-color:var(--accent);background:var(--accentSoft);color:var(--text)}
label,.fld{display:flex;flex-direction:column;gap:6px;font-size:12px;font-weight:500;color:var(--muted)}
input,select{width:100%;min-width:0;font-size:14px;color:var(--text);background:var(--field);border:1px solid var(--border);border-radius:8px;padding:10px 12px;outline:none;transition:border-color .15s,box-shadow .15s}
input:focus,select:focus{border-color:var(--accent);box-shadow:0 0 0 3px var(--accentSoft)}
.pw{position:relative;display:flex}
.pw input{padding-right:52px}
.pw .show{position:absolute;right:6px;top:50%;transform:translateY(-50%);font-size:11px;font-weight:600;letter-spacing:.02em;color:var(--muted);background:transparent;border:0;padding:6px;cursor:pointer}
.pw .show:hover{color:var(--text)}
.msg{min-height:18px;font-size:13px}
.ok{color:var(--accent)}.err{color:var(--sell)}
.go{font-size:14px;font-weight:600;color:var(--onAccent);background:var(--accent);border:0;border-radius:8px;padding:12px;cursor:pointer}
.go:hover{filter:brightness(1.06)}
.alt{text-align:center;font-size:13px;color:var(--muted)}
.alt a{font-weight:600;cursor:pointer}
.ft{text-align:center;font-family:"IBM Plex Mono",monospace;font-size:11px;letter-spacing:.08em;color:var(--faint)}
.two{display:grid;grid-template-columns:1fr 1fr;gap:14px 16px}
.f{display:flex;flex-direction:column;gap:6px}
.ferr{display:block;color:var(--sell);font-size:11.5px;line-height:1.35}.ferr:empty{display:none}
.hint{display:block;color:var(--faint);font-size:12px;line-height:1.4}
input.bad,select.bad{border-color:var(--sell)!important}
.theme-btn{display:grid;place-items:center;width:36px;height:36px;border-radius:8px;border:1px solid var(--border);background:var(--surface);color:var(--muted);cursor:pointer;padding:0;transition:color .15s}
.theme-btn:hover{color:var(--text)}.theme-btn .moon{display:none}[data-theme="light"] .theme-btn .sun{display:none}[data-theme="light"] .theme-btn .moon{display:block}
@media(max-width:560px){.two{grid-template-columns:1fr}h1{font-size:30px}.wrap{padding:64px 16px 32px;gap:32px}.card{padding:22px}}
</style></head><body>
` + ThemeButtonFixed + `
<canvas id="flow" aria-hidden="true"></canvas>
<div class="wrap">
 <section class="board">
  <div class="brand"><span class="logo">M</span><div><b>Matching Engine</b><small>DSEME · VELEXA</small></div></div>
  <h1>Welcome to the <span>Velexa</span> matching engine</h1>
  <p class="lead">A DSE-style matching engine with FIX order entry, an ITCH market-data feed, and a live order book. Sign in to monitor the market, or to administer brokers and instruments.</p>
  <ul class="feat">
   <li><i>U</i><span><b>User</b> &mdash; your own workspace: register your brokers, manage their credentials and FIX sessions, trace their messages, watch the market.</span></li>
   <li><i>A</i><span><b>Admin</b> &mdash; full control: broker credentials, IP allowlist, instruments, market session and console users.</span></li>
  </ul>
  <a class="term" href="/terminal">Open Trade Terminal <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14M13 6l6 6-6 6"/></svg></a>
 </section>
 <form class="card" id="logcard" method="post" action="/admin/login">
  <div class="head"><h2>Sign in</h2><p class="sub" id="who"></p></div>
  <div class="seg" role="tablist">
   <button type="button" id="t_user" onclick="pick('user')">User login</button>
   <button type="button" id="t_admin" onclick="pick('admin')">Admin login</button>
  </div>
  <input type="hidden" name="as" id="as" value="__AS__">
  <label>Username<input name="user" id="u" autofocus autocomplete="username"></label>
  <label>Password<span class="pw"><input name="pass" type="password" autocomplete="current-password"><button class="show" type="button" onclick="pwShow(this)">SHOW</button></span></label>
  <div class="msg __ERRCLASS__" id="err">__ERR__</div>
  <button class="go" type="submit" id="go">Sign in</button>
  <div class="alt" id="toReg">New here? <a href="#" onclick="pick('register');return false">Create an account</a></div>
  <div class="ft">VELEXA · DSEME</div>
 </form>
 <form class="card reg" method="post" action="/register" id="regcard" style="display:none" novalidate>
  <div class="head"><h2>Create an account</h2><p class="sub">User accounts manage their own brokers. Admin access is granted by an existing admin.</p></div>
  <div class="two">
   <div class="f"><label for="r_full_name">Full name *</label><input name="full_name" id="r_full_name" autocomplete="name" required maxlength="80"><small class="ferr"></small></div>
   <div class="f"><label for="r_user">Username *</label><input name="user" id="r_user" autocomplete="username" required placeholder="letters, digits . _ -" maxlength="32"><small class="ferr"></small></div>
   <div class="f"><label for="r_email">Email *</label><input name="email" id="r_email" type="email" autocomplete="email" required placeholder="you@example.com" maxlength="120"><small class="ferr"></small></div>
   <div class="f"><label for="r_phone">Phone *</label><input name="phone" id="r_phone" type="tel" autocomplete="tel" required placeholder="+880 1XXX-XXXXXX"><small class="ferr"></small></div>
   <div class="f"><label for="r_gender">Gender *</label><select name="gender" id="r_gender" required><option value="">Select…</option><option value="male">Male</option><option value="female">Female</option><option value="other">Other</option></select><small class="ferr"></small></div>
   <div class="f"><label for="country">Country *</label><select name="country" id="country" required></select><small class="ferr"></small></div>
   <div class="f"><label for="r_pass">Password *</label><span class="pw"><input name="pass" id="r_pass" type="password" autocomplete="new-password" required maxlength="128" placeholder="at least 10 characters"><button class="show" type="button" onclick="pwShow(this)">SHOW</button></span><small class="ferr"></small></div>
   <div class="f"><label for="r_pass2">Confirm password *</label><span class="pw"><input name="pass2" id="r_pass2" type="password" autocomplete="new-password" required maxlength="128"><button class="show" type="button" onclick="pwShow(this)">SHOW</button></span><small class="ferr"></small></div>
  </div>
  <small class="hint">Password: 10 to 128 characters, with at least 3 of: lower-case letters, upper-case letters, digits, symbols.</small>
  <div class="msg err" id="regerr"></div>
  <button class="go" type="submit">Create account</button>
  <div class="alt">Already have an account? <a href="#" onclick="pick('user');return false">Sign in</a></div>
  <div class="ft">VELEXA · DSEME</div>
 </form>
</div>
<script>
var REG=__REG__==1;
var COUNTRIES="Afghanistan|Albania|Algeria|Argentina|Armenia|Australia|Austria|Azerbaijan|Bahrain|Bangladesh|Belarus|Belgium|Bhutan|Bolivia|Brazil|Brunei|Bulgaria|Cambodia|Cameroon|Canada|Chile|China|Colombia|Croatia|Cuba|Cyprus|Czechia|Denmark|Ecuador|Egypt|Estonia|Ethiopia|Finland|France|Georgia|Germany|Ghana|Greece|Hungary|Iceland|India|Indonesia|Iran|Iraq|Ireland|Israel|Italy|Japan|Jordan|Kazakhstan|Kenya|Kuwait|Laos|Latvia|Lebanon|Libya|Lithuania|Luxembourg|Malaysia|Maldives|Mexico|Morocco|Myanmar|Nepal|Netherlands|New Zealand|Nigeria|Norway|Oman|Pakistan|Palestine|Peru|Philippines|Poland|Portugal|Qatar|Romania|Russia|Saudi Arabia|Serbia|Singapore|Slovakia|Slovenia|South Africa|South Korea|Spain|Sri Lanka|Sudan|Sweden|Switzerland|Syria|Taiwan|Tanzania|Thailand|Tunisia|Turkey|Uganda|Ukraine|United Arab Emirates|United Kingdom|United States|Uruguay|Uzbekistan|Venezuela|Vietnam|Yemen|Zimbabwe|Other".split("|");
document.getElementById("country").innerHTML="<option value=''>Select…</option>"+COUNTRIES.map(function(c){return "<option>"+c+"</option>"}).join("");
function pick(a){
 var reg=a=="register";
 document.getElementById("regcard").style.display=reg?"flex":"none";
 document.getElementById("logcard").style.display=reg?"none":"flex";
 if(reg)return;
 document.getElementById("as").value=a;
 document.getElementById("t_user").classList.toggle("on",a=="user");document.getElementById("t_admin").classList.toggle("on",a=="admin");
 document.getElementById("who").textContent=a=="admin"?"Full console for exchange operators.":"Your own brokers and the market.";
 document.getElementById("go").textContent=a=="admin"?"Sign in as admin":"Sign in as user";
 document.getElementById("toReg").style.display=(REG&&a=="user")?"block":"none"}
// SHOW / HIDE on a password field
function pwShow(b){var i=b.parentNode.querySelector("input"),on=i.type=="password";i.type=on?"text":"password";b.textContent=on?"HIDE":"SHOW"}
// ── Registration: checked here with the server's rules before it is sent, so a
// mistake never empties the form; what only the server can check (a taken name
// or email) comes back with the details kept. Passwords are never stored.
var RF=["full_name","user","email","phone","gender","country"],DRAFT="me-reg-draft",regTried=false;
function rel(n){return document.querySelector('#regcard [name="'+n+'"]')}
function setErr(n,msg){var el=rel(n),e=el.closest(".f").querySelector(".ferr");el.classList.toggle("bad",!!msg);el.setAttribute("aria-invalid",msg?"true":"false");if(e)e.textContent=msg||""}
function pwClasses(p){var c=0;if(/[a-z]/.test(p))c++;if(/[A-Z]/.test(p))c++;if(/[0-9]/.test(p))c++;if(/[^a-zA-Z0-9]/.test(p))c++;return c}
function regCheck(n){var v=rel(n).value,t=v.trim(),u=rel("user").value.trim();
 switch(n){
 case "full_name":return t.length<2?"Enter your full name (at least 2 characters).":"";
 case "user":return !/^[A-Za-z0-9._-]{3,32}$/.test(t)?"3 to 32 characters: letters, digits, dot, underscore or dash.":"";
 case "email":return !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(t)||t.length>120?"Enter a valid email address, e.g. you@example.com.":"";
 case "phone":var d=t.replace(/[^0-9]/g,"").length;return !/^\+?[0-9 ()-]+$/.test(t)||d<7||d>15?"Enter a phone number with 7 to 15 digits, e.g. +880 1712-345678.":"";
 case "gender":return v?"":"Select your gender.";
 case "country":return v?"":"Select your country.";
 case "pass":if(v.length<10)return "At least 10 characters.";if(pwClasses(v)<3)return "Use at least 3 of: lower-case, upper-case, digits, symbols.";return "";
 case "pass2":return !v?"Enter the password again.":v!=rel("pass").value?"The two passwords do not match.":""}return ""}
var REGN=RF.concat(["pass","pass2"]);
function regValidate(){var first=null;REGN.forEach(function(n){var m=regCheck(n);setErr(n,m);if(m&&!first)first=n});if(first)rel(first).focus();return !first}
REGN.forEach(function(n){var el=rel(n);var re=function(){if(regTried||el.classList.contains("bad"))setErr(n,regCheck(n));if(n=="pass"&&rel("pass2").value&&regTried)setErr("pass2",regCheck("pass2"))};el.addEventListener("input",re);el.addEventListener("change",re);el.addEventListener("blur",function(){if(el.value)setErr(n,regCheck(n))})});
document.getElementById("regcard").addEventListener("submit",function(e){regTried=true;var re=document.getElementById("regerr");re.textContent="";re.style.color="";
 if(!regValidate()){e.preventDefault();return}
 try{var d={};RF.forEach(function(n){d[n]=rel(n).value});sessionStorage.setItem(DRAFT,JSON.stringify(d))}catch(x){}});
// the server's answer: which field it is about
var REGFIELD={reg_mismatch:"pass2",reg_weak:"pass",reg_name:"user",reg_fullname:"full_name",reg_email:"email",reg_phone:"phone",reg_gender:"gender",reg_country:"country",reg_taken:"user"};
var start="__AS__",code=(location.search.match(/[?&]e=([a-z_]+)/)||[])[1]||"";
if(code=="registered"){try{sessionStorage.removeItem(DRAFT)}catch(x){}}
if(start=="register"){pick("register");
 var msg=document.getElementById("err").textContent,d=null;try{d=JSON.parse(sessionStorage.getItem(DRAFT)||"null")}catch(x){}
 if(d)RF.forEach(function(n){if(d[n]!=null)rel(n).value=d[n]});
 var f=REGFIELD[code];
 // passwords are never kept: after a round trip they are the only fields to fill again
 var note=d?"Your details are kept — please enter the password twice again.":"";
 var re=document.getElementById("regerr");
 if(f){setErr(f,msg);re.textContent=note;re.style.color="var(--muted)";rel(f).focus()}
 else re.textContent=msg+(note?" "+note:"");
}else pick(start);
// ── Background: orders flowing in from both sides to the book in the middle.
// A click sends a ripple through them. Still when the system asks for less motion.
(function(){var cv=document.getElementById("flow");if(!cv||!cv.getContext)return;
 var c=cv.getContext("2d"),w=0,h=0,paths=[],bursts=[],root=document.documentElement,
  still=!!(window.matchMedia&&matchMedia("(prefers-reduced-motion: reduce)").matches);
 function build(){var n=Math.round(Math.max(24,Math.min(80,h/9)));paths=[];
  for(var i=0;i<n;i++)paths.push({left:i%2==0,y:(i/n)*h*1.4-h*.2,t:Math.random(),v:.0015+Math.random()*.002})}
 function size(){var d=window.devicePixelRatio||1;w=window.innerWidth;h=window.innerHeight;cv.width=w*d;cv.height=h*d;c.setTransform(d,0,0,d,0,0);build();draw()}
 function bp(t,a,b,d,e){var u=1-t;return u*u*u*a+3*u*u*t*b+3*u*t*t*d+t*t*t*e}
 function draw(){var dark=root.getAttribute("data-theme")!="light",cx=w/2,cy=h/2;c.clearRect(0,0,w,h);
  c.strokeStyle=dark?"rgba(255,255,255,0.22)":"rgba(26,31,42,0.2)";c.fillStyle=dark?"rgba(120,230,175,0.8)":"rgba(20,128,79,0.7)";c.lineWidth=1.1;
  bursts.forEach(function(b){b.r+=15;b.life-=.015});bursts=bursts.filter(function(b){return b.life>0});
  paths.forEach(function(p){var x0=p.left?0:w,x1=p.left?cx*.5:w-cx*.5,x2=p.left?cx*.8:w-cx*.8;
   c.beginPath();c.moveTo(x0,p.y);c.bezierCurveTo(x1,p.y,x2,cy,cx,cy);c.setLineDash([1,4]);c.stroke();c.setLineDash([]);
   if(!still){p.t+=p.v;if(p.t>1){p.t=0;p.y+=(Math.random()-.5)*10}}
   var x=bp(p.t,x0,x1,x2,cx),y=bp(p.t,p.y,p.y,cy,cy);
   bursts.forEach(function(b){var dx=x-b.x,dy=y-b.y,dd=Math.hypot(dx,dy)||1;if(dd<b.r+120&&dd>b.r-120){var f=(1-Math.abs(dd-b.r)/120)*b.life;x+=dx/dd*f*80;y+=dy/dd*f*80}});
   c.fillRect(x-1.5,y-1.5,3,3)})}
 window.addEventListener("resize",size);size();
 if(still)new MutationObserver(draw).observe(root,{attributes:true,attributeFilter:["data-theme"]});
 else{cv.addEventListener("click",function(e){bursts.push({x:e.clientX,y:e.clientY,r:0,life:1})});
  (function loop(){draw();requestAnimationFrame(loop)})()}
})();
</script>
</body></html>`
