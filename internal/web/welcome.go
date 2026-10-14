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
 <form class="card reg" method="post" action="/register" id="regcard" style="display:none">
  <h2>Create account</h2>
  <p class="sub">New accounts start with an empty workspace and see only their own brokers.</p>
  <div class="two">
   <div><label>Full name *</label><input name="full_name" autocomplete="name" required maxlength="80"></div>
   <div><label>Username *</label><input name="user" autocomplete="username" required placeholder="letters, digits . _ -" minlength="3" maxlength="32"></div>
   <div><label>Email *</label><input name="email" type="email" autocomplete="email" required placeholder="you@example.com"></div>
   <div><label>Phone *</label><input name="phone" type="tel" autocomplete="tel" required placeholder="+880 1XXX-XXXXXX"></div>
   <div><label>Gender *</label><select name="gender" required><option value="">Select…</option><option value="male">Male</option><option value="female">Female</option><option value="other">Other</option></select></div>
   <div><label>Country *</label><select name="country" id="country" required></select></div>
   <div><label>Password *</label><input name="pass" type="password" autocomplete="new-password" required minlength="8" placeholder="min 8 characters"></div>
   <div><label>Confirm password *</label><input name="pass2" type="password" autocomplete="new-password" required minlength="8"></div>
  </div>
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
var start="__AS__";
if(start=="register"){document.getElementById("regerr").textContent=document.getElementById("err").textContent;pick("register")}else pick(start);
</script>
</body></html>`
