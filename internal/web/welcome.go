package web

// WelcomeHTML is the landing page at "/": a welcome board with User and Admin
// sign-in. Both post to /admin/login; "as" tells the server which option was used.
const WelcomeHTML = `<!doctype html><html><head><meta charset="utf-8">` + ThemeHead + CSRFHead + `
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Welcome — Matching Engine</title>` + AdminFontLink + `
<style>
:root{--bg:#080b11;--panel:#121821;--line:#232c39;--ink:#e8eef6;--mut:#7d8a9c;--acc:#4dd0b1}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;
background:radial-gradient(1100px 560px at 20% -10%,#15202e,#080b11);color:var(--ink);
font-family:Inter,system-ui,-apple-system,sans-serif}
.wrap{display:grid;grid-template-columns:1.15fr 1fr;gap:28px;max-width:960px;width:100%;align-items:stretch}
.board{padding:34px 8px 20px 4px;display:flex;flex-direction:column;justify-content:center}
.brand{display:flex;align-items:center;gap:11px;margin-bottom:26px}
.logo{width:34px;height:34px;border-radius:10px;background:linear-gradient(135deg,#4dd0b1,#2f9e86);
display:flex;align-items:center;justify-content:center;color:#04231f;font-weight:800;font-size:17px}
.brand b{font-size:15px;letter-spacing:-.01em}.brand small{display:block;color:var(--mut);font-size:11px;font-family:"IBM Plex Mono",monospace;letter-spacing:.06em}
h1{font-size:34px;line-height:1.15;margin:0 0 14px;letter-spacing:-.03em}
h1 span{color:var(--acc)}
.lead{color:var(--mut);font-size:14.5px;line-height:1.65;margin:0 0 26px;max-width:460px}
.feat{display:grid;gap:10px;margin:0 0 26px;padding:0;list-style:none}
.feat li{display:flex;gap:11px;align-items:flex-start;font-size:13.5px;color:#b8c3d2;line-height:1.5}
.feat i{flex:0 0 auto;width:22px;height:22px;border-radius:7px;background:#14262a;color:var(--acc);display:flex;align-items:center;justify-content:center;font-style:normal;font-size:12px;font-weight:700}
.term{display:inline-flex;align-items:center;gap:8px;color:var(--ink);text-decoration:none;font-size:13px;font-weight:600;
border:1px solid var(--line);border-radius:10px;padding:10px 15px;width:max-content;transition:border .15s}
.term:hover{border-color:var(--acc);color:var(--acc)}
.card{background:var(--panel);border:1px solid var(--line);border-radius:16px;padding:30px 28px;box-shadow:0 24px 60px rgba(0,0,0,.55);align-self:center}
.card h2{font-size:17px;margin:0 0 4px;letter-spacing:-.02em}
.sub{color:var(--mut);font-size:12.5px;margin:0 0 20px;min-height:18px}
.seg{display:grid;grid-template-columns:1fr 1fr;gap:4px;background:#0b1119;border:1px solid var(--line);border-radius:11px;padding:4px;margin-bottom:6px}
.seg button{margin:0;padding:9px;background:none;color:var(--mut);border:0;border-radius:8px;font-weight:600;font-size:13px;cursor:pointer;width:auto}
.seg button.on{background:#17222f;color:var(--ink);box-shadow:inset 0 0 0 1px var(--acc)}
label{display:block;font-size:12px;color:var(--mut);margin:15px 0 6px;font-weight:600}
input{width:100%;padding:11px 13px;background:#0b1119;border:1px solid var(--line);border-radius:10px;color:var(--ink);font-size:14px;outline:none;transition:border .15s}
input:focus{border-color:var(--acc)}
.go{width:100%;margin-top:22px;padding:12px;background:var(--acc);color:#04231f;border:0;border-radius:10px;font-weight:700;font-size:14px;cursor:pointer}
.go:hover{filter:brightness(1.07)}
.ok{color:#4dd0b1;font-size:12.5px;margin-top:14px;min-height:16px}
.err{color:#f87171;font-size:12.5px;margin-top:14px;min-height:16px}
.card.reg{padding:26px 26px}
.two{display:grid;grid-template-columns:1fr 1fr;gap:0 14px}
.two label{margin-top:12px}
select{width:100%;padding:11px 13px;background:#0b1119;border:1px solid var(--line);border-radius:10px;color:var(--ink);font-size:14px;outline:none}
select:focus{border-color:var(--acc)}
.wrap.regmode{grid-template-columns:.8fr 1.4fr}
@media(max-width:560px){.two{grid-template-columns:1fr}}
.ferr{display:block;color:#f87171;font-size:11.5px;margin-top:5px;line-height:1.35}.ferr:empty{display:none}
.hint{display:block;color:var(--mut);font-size:11.5px;margin-top:5px;line-height:1.35}
input.bad,select.bad{border-color:#f87171!important}
.alt{text-align:center;color:var(--mut);font-size:12.5px;margin-top:16px}
.alt a{color:var(--acc);text-decoration:none;font-weight:600}
.ft{color:var(--mut);font-size:11px;text-align:center;margin-top:18px;font-family:"IBM Plex Mono",monospace;letter-spacing:.05em}
@media(max-width:820px){.wrap{grid-template-columns:1fr}.board{padding:6px 2px}h1{font-size:27px}}
` + welcomeLightCSS + welcomeThemeBtnCSS + `</style></head><body>
` + ThemeButtonFixed + `
<div class="wrap">
 <section class="board">
  <div class="brand"><span class="logo">M</span><div><b>Matching Engine</b><small>DSEME · VELEXA</small></div></div>
  <h1>Welcome to the <span>Velexa</span> matching engine</h1>
  <p class="lead">A DSE-style matching engine with FIX order entry, an ITCH market-data feed, and a live order book. Sign in to monitor the market, or to administer brokers and instruments.</p>
  <ul class="feat">
   <li><i>U</i><span><b>User</b> &mdash; your own workspace: register your brokers, manage their credentials and FIX sessions, trace their messages, watch the market.</span></li>
   <li><i>A</i><span><b>Admin</b> &mdash; full control: broker credentials, IP allowlist, instruments, market session and console users.</span></li>
  </ul>
  <a class="term" href="/terminal">Open Trade Terminal &rarr;</a>
 </section>
 <form class="card" id="logcard" method="post" action="/admin/login">
  <h2>Sign in</h2>
  <p class="sub" id="who"></p>
  <div class="seg" role="tablist">
   <button type="button" id="t_user" onclick="pick('user')">User login</button>
   <button type="button" id="t_admin" onclick="pick('admin')">Admin login</button>
  </div>
  <input type="hidden" name="as" id="as" value="__AS__">
  <label>Username</label><input name="user" id="u" autofocus autocomplete="username">
  <label>Password</label><input name="pass" type="password" autocomplete="current-password">
  <div class="__ERRCLASS__" id="err">__ERR__</div>
  <button class="go" type="submit" id="go">Sign in</button>
  <div class="alt" id="toReg">New here? <a href="#" onclick="pick('register');return false">Create an account</a></div>
  <div class="ft">VELEXA · DSEME</div>
 </form>
 <form class="card reg" method="post" action="/register" id="regcard" style="display:none" novalidate>
  <h2>Create account</h2>
  <p class="sub">New accounts start with an empty workspace and see only their own brokers.</p>
  <div class="two">
   <div><label for="r_full_name">Full name *</label><input name="full_name" id="r_full_name" autocomplete="name" required maxlength="80"><small class="ferr"></small></div>
   <div><label for="r_user">Username *</label><input name="user" id="r_user" autocomplete="username" required placeholder="letters, digits . _ -" maxlength="32"><small class="ferr"></small></div>
   <div><label for="r_email">Email *</label><input name="email" id="r_email" type="email" autocomplete="email" required placeholder="you@example.com" maxlength="120"><small class="ferr"></small></div>
   <div><label for="r_phone">Phone *</label><input name="phone" id="r_phone" type="tel" autocomplete="tel" required placeholder="+880 1XXX-XXXXXX"><small class="ferr"></small></div>
   <div><label for="r_gender">Gender *</label><select name="gender" id="r_gender" required><option value="">Select…</option><option value="male">Male</option><option value="female">Female</option><option value="other">Other</option></select><small class="ferr"></small></div>
   <div><label for="country">Country *</label><select name="country" id="country" required></select><small class="ferr"></small></div>
   <div><label for="r_pass">Password *</label><input name="pass" id="r_pass" type="password" autocomplete="new-password" required maxlength="128" placeholder="at least 10 characters"><small class="ferr"></small></div>
   <div><label for="r_pass2">Confirm password *</label><input name="pass2" id="r_pass2" type="password" autocomplete="new-password" required maxlength="128"><small class="ferr"></small></div>
  </div>
  <small class="hint">Password: 10 to 128 characters, with at least 3 of: lower-case letters, upper-case letters, digits, symbols.</small>
  <div class="err" id="regerr"></div>
  <button class="go" type="submit">Register</button>
  <div class="alt">Already registered? <a href="#" onclick="pick('user');return false">Sign in</a></div>
  <div class="ft">VELEXA · DSEME</div>
 </form>
</div>
<script>
var REG=__REG__==1;
var COUNTRIES="Afghanistan|Albania|Algeria|Argentina|Armenia|Australia|Austria|Azerbaijan|Bahrain|Bangladesh|Belarus|Belgium|Bhutan|Bolivia|Brazil|Brunei|Bulgaria|Cambodia|Cameroon|Canada|Chile|China|Colombia|Croatia|Cuba|Cyprus|Czechia|Denmark|Ecuador|Egypt|Estonia|Ethiopia|Finland|France|Georgia|Germany|Ghana|Greece|Hungary|Iceland|India|Indonesia|Iran|Iraq|Ireland|Israel|Italy|Japan|Jordan|Kazakhstan|Kenya|Kuwait|Laos|Latvia|Lebanon|Libya|Lithuania|Luxembourg|Malaysia|Maldives|Mexico|Morocco|Myanmar|Nepal|Netherlands|New Zealand|Nigeria|Norway|Oman|Pakistan|Palestine|Peru|Philippines|Poland|Portugal|Qatar|Romania|Russia|Saudi Arabia|Serbia|Singapore|Slovakia|Slovenia|South Africa|South Korea|Spain|Sri Lanka|Sudan|Sweden|Switzerland|Syria|Taiwan|Tanzania|Thailand|Tunisia|Turkey|Uganda|Ukraine|United Arab Emirates|United Kingdom|United States|Uruguay|Uzbekistan|Venezuela|Vietnam|Yemen|Zimbabwe|Other".split("|");
document.getElementById("country").innerHTML="<option value=''>Select…</option>"+COUNTRIES.map(function(c){return "<option>"+c+"</option>"}).join("");
function pick(a){
 var reg=a=="register";
 document.querySelector(".wrap").classList.toggle("regmode",reg);
 document.getElementById("regcard").style.display=reg?"block":"none";
 document.getElementById("logcard").style.display=reg?"none":"block";
 if(reg)return;
 document.getElementById("as").value=a;
 document.getElementById("t_user").classList.toggle("on",a=="user");document.getElementById("t_admin").classList.toggle("on",a=="admin");
 document.getElementById("who").textContent=a=="admin"?"Administrator access — full control.":"Your own brokers and the market.";
 document.getElementById("go").textContent=a=="admin"?"Sign in as admin":"Sign in as user";
 document.getElementById("toReg").style.display=(REG&&a=="user")?"block":"none"}
// ── Registration: checked here with the server's rules before it is sent, so a
// mistake never empties the form; what only the server can check (a taken name
// or email) comes back with the details kept. Passwords are never stored.
var RF=["full_name","user","email","phone","gender","country"],DRAFT="me-reg-draft",regTried=false;
function rel(n){return document.querySelector('#regcard [name="'+n+'"]')}
function setErr(n,msg){var el=rel(n),e=el.parentNode.querySelector(".ferr");el.classList.toggle("bad",!!msg);el.setAttribute("aria-invalid",msg?"true":"false");if(e)e.textContent=msg||""}
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
 if(f){setErr(f,msg);re.textContent=note;re.style.color="var(--mut)";rel(f).focus()}
 else re.textContent=msg+(note?" "+note:"");
}else pick(start);
</script>
</body></html>`
