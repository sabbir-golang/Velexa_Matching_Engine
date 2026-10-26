// Browser UI test: drives headless Chrome over the DevTools protocol (no extra
// packages; Node 22+ and google-chrome/chromium). Registers a user, signs in,
// uses the console and Trade Terminal, checks XSS/CSRF/sign-out behaviour.
//
//   ME_URL=http://localhost:18010 ADMIN_PASS=... node tools/ui_test.mjs [screenshot-dir]
// Run it against a disposable engine (tools/e2e.sh shows how to start one).
import { spawn } from "node:child_process";
import { writeFileSync, mkdtempSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const B = (process.env.ME_URL || "http://localhost:18010").replace(/\/$/, "");
const OUT = process.argv[2] || mkdtempSync(join(tmpdir(), "ui-shots-"));
const CHROME = process.env.CHROME || "google-chrome";
mkdirSync(OUT, { recursive: true });
const ADMIN_PASS = process.env.ADMIN_PASS;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const prof = mkdtempSync(join(tmpdir(), "cdp-"));
const chrome = spawn(CHROME, ["--headless=new", "--disable-gpu", "--no-first-run","--remote-allow-origins=*", "--remote-debugging-port=9333",
  `--user-data-dir=${prof}`, "--window-size=1400,900", "about:blank"], { stdio: "ignore" });

let ws, id = 0;
const pending = new Map();
const problems = [];
async function connect() {
  for (let i = 0; i < 50; i++) {
    try {
      const list = await (await fetch("http://127.0.0.1:9333/json")).json();
      const page = list.find((t) => t.type === "page");
      if (page) { ws = new WebSocket(page.webSocketDebuggerUrl); break; }
    } catch {}
    await sleep(200);
  }
  await new Promise((r, j) => { // fail fast instead of waiting forever if Chrome refuses the connection
    ws.addEventListener("open", r);
    ws.addEventListener("error", () => j(new Error("Chrome refused the DevTools connection")));
    setTimeout(() => j(new Error("Chrome DevTools connection timed out")), 10000);
  });
  ws.addEventListener("message", (ev) => {
    const m = JSON.parse(ev.data);
    if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); return; }
    if (m.method === "Runtime.exceptionThrown") problems.push("exception: " + m.params.exceptionDetails.exception?.description);
    if (m.method === "Log.entryAdded" && m.params.entry.level === "error") problems.push("log: " + m.params.entry.text + " @ " + (m.params.entry.url || ""));
    if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") problems.push("console: " + m.params.args.map((a) => a.value ?? a.description).join(" "));
  });
}
function cmd(method, params = {}) {
  return new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
}
async function ev(expr) {
  const r = await cmd("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true });
  if (r.result?.exceptionDetails) throw new Error(expr.slice(0, 80) + " -> " + JSON.stringify(r.result.exceptionDetails.exception?.description));
  return r.result?.result?.value;
}
async function go(url) { await cmd("Page.navigate", { url }); await sleep(1500); }
async function waitUrl(re, ms = 6000) {
  for (let t = 0; t < ms; t += 200) { const u = await ev("location.href"); if (re.test(u)) return u; await sleep(200); }
  return await ev("location.href");
}
async function shot(name) {
  const r = await cmd("Page.captureScreenshot", { format: "png" });
  writeFileSync(join(OUT, name + ".png"), Buffer.from(r.result.data, "base64"));
}
const results = [];
const check = (name, ok, detail = "") => { results.push(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`); };

try {
  await connect();
  await cmd("Page.enable"); await cmd("Runtime.enable"); await cmd("Log.enable");

  // 1. register through the form; a mistake is caught in the browser and keeps what was typed
  await go(B + "/?as=register");
  await shot("01-register");
  await ev(`(()=>{const f=document.getElementById("regcard");const set=(n,v)=>{f.querySelector("[name="+n+"]").value=v};
    set("full_name","Test Person");set("user","tester1");set("email","tester1@example.com");set("phone","+880 1711-000001");
    set("gender","other");set("country","Bangladesh");set("pass","Trading-Desk7");set("pass2","Trading-Desk8");
    f.requestSubmit();})()`);
  await sleep(600);
  const kept = await ev(`(()=>{const f=document.getElementById("regcard");return location.search.indexOf("e=")<0&&f.querySelector("[name=email]").value=="tester1@example.com"&&/do not match/.test(f.textContent)})()`);
  check("registration mismatch is shown and the form keeps its details", kept === true);
  await ev(`(()=>{const f=document.getElementById("regcard");const set=(n,v)=>{f.querySelector("[name="+n+"]").value=v};
    set("full_name","Test Person");set("user","tester1");set("email","tester1@example.com");set("phone","+880 1711-000001");
    set("gender","other");set("country","Bangladesh");set("pass","Trading-Desk7");set("pass2","Trading-Desk7");
    f.requestSubmit();})()`);
  let u = await waitUrl(/e=registered/);
  check("register via form (CSRF field added)", /e=registered/.test(u), u);

  // 2. sign in as user
  await ev(`(()=>{const f=document.getElementById("logcard");f.querySelector("[name=user]").value="tester1";
    f.querySelector("[name=pass]").value="Trading-Desk7";f.requestSubmit();})()`);
  u = await waitUrl(/\/user$/);
  check("user sign-in lands on /user", /\/user$/.test(u), u);
  await sleep(1500);
  await shot("02-user-console");

  // 3. console fetch helpers carry the CSRF header
  const add = await ev(`jpost("broker/add",{Sender:"TESTBRK1",Company:"\\"'><img src=x onerror=\\"window.__xss=1\\">Co"})`);
  check("console POST (broker/add) accepted", add && add.ok === true, JSON.stringify(add).slice(0, 120));
  await ev(`show("creds")`); await sleep(1500);
  const xss = await ev(`window.__xss===1`);
  check("hostile company name rendered inert", xss === false);
  await shot("03-user-brokers");

  // 4. trade from the terminal while signed in
  await go(B + "/terminal");
  await sleep(1500);
  const disabled = await ev(`document.getElementById("obtn").disabled`);
  check("terminal order form enabled when signed in", disabled === false);
  await ev(`(()=>{document.getElementById("osym").value="GP";document.getElementById("oqty").value="10";
    document.getElementById("oprice").value="0";document.getElementById("ofrm").requestSubmit();})()`);
  await sleep(1500);
  const ores = await ev(`document.getElementById("ores").textContent`);
  check("order placed from terminal", /FILLED|NEW|resting|REJECTED|PARTIAL/i.test(ores) && !/Sign in/.test(ores), ores);
  await shot("04-terminal-signed-in");

  // 5. sign out (POST form) and confirm the session is gone
  await go(B + "/user");
  await ev(`document.querySelector("form[action='/admin/logout'] button").click()`);
  u = await waitUrl(/:\d+\/(\?.*)?$/);
  const me = await ev(`fetch("/admin/api/me").then(r=>r.status)`);
  check("sign-out button ends the session", me === 401, "me=" + me + " url=" + u);
  await go(B + "/terminal"); await sleep(1200);
  const ores2 = await ev(`document.getElementById("ores").textContent`);
  check("terminal says sign in when signed out", /Sign in to trade/.test(ores2), ores2);
  await shot("05-terminal-signed-out");

  // 6. admin console
  await go(B + "/?as=admin");
  await ev(`(()=>{const f=document.getElementById("logcard");f.querySelector("[name=user]").value="admin";
    f.querySelector("[name=pass]").value=${JSON.stringify(ADMIN_PASS)};f.requestSubmit();})()`);
  u = await waitUrl(/\/admin$/);
  check("admin sign-in with ME_ADMIN_PASS", /\/admin$/.test(u), u);
  await sleep(1500);
  await shot("06-admin-dashboard");
  await ev(`show("users")`); await sleep(1200);
  const rows = await ev(`document.querySelectorAll("#userList tr").length`);
  const onclick = await ev(`(document.querySelector("#userList button")||{}).getAttribute?.("onclick")||""`);
  check("users page renders with safe handlers", rows >= 2 && /editUser\("/.test(onclick), `rows=${rows} onclick=${onclick}`);
  await shot("07-admin-users");
  await ev(`show("creds")`); await sleep(1200);
  check("admin brokers view: no script ran from data", (await ev(`window.__xss===1`)) === false);
  const del = await ev(`jpost("broker/del",{Sender:"TESTBRK1"})`);
  check("admin revokes the test broker", del && del.ok === true);
  const ph = await ev(`j("trading-session?action=auto_on",{method:"POST"})`);
  check("market session control (POST) works", ph && ph.ok === true);
} catch (e) {
  results.push("ERROR " + e.message);
} finally {
  console.log(results.join("\n"));
  console.log("\nscreenshots: " + OUT);
  process.exitCode = results.every((r) => r.startsWith("PASS")) ? 0 : 1;
  console.log(problems.length ? "\nBrowser errors:\n" + problems.join("\n") : "\nBrowser errors: none");
  ws?.close(); chrome.kill();
}
