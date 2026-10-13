package web

// Dark / light theme shared by every page. The choice is kept in localStorage
// ("me-theme") so the console, the Trade Terminal and the benchmark page, which
// are framed inside the console, change together.

// ThemeHead goes first in <head>: it sets data-theme before the page paints (no flash),
// handles clicks on any #themeBtn, and follows changes made in another tab or frame.
const ThemeHead = `<script>(function(){var d=document.documentElement;
function get(){try{return localStorage.getItem('me-theme')==='light'?'light':'dark'}catch(e){return 'dark'}}
d.setAttribute('data-theme',get());
document.addEventListener('click',function(e){var b=e.target.closest&&e.target.closest('#themeBtn');if(!b)return;
var t=d.getAttribute('data-theme')==='light'?'dark':'light';d.setAttribute('data-theme',t);try{localStorage.setItem('me-theme',t)}catch(x){}});
window.addEventListener('storage',function(e){if(e.key==='me-theme')d.setAttribute('data-theme',get())});})();</script>`

// ThemeButton is the sun / moon button: the sun shows in dark mode (switch to light), the moon in light mode.
const ThemeButton = `<button class="theme-btn" id="themeBtn" type="button" aria-label="Switch dark / light theme" title="Switch dark / light theme">` +
	`<svg class="sun" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4.2"/><path d="M12 2v2.5M12 19.5V22M4.2 4.2l1.8 1.8M18 18l1.8 1.8M2 12h2.5M19.5 12H22M4.2 19.8 6 18M18 6l1.8-1.8"/></svg>` +
	`<svg class="moon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg></button>`

const adminLightCSS = `:root[data-theme="light"]{--bg:#f3f5f9;--side:#fff;--panel:#fff;--panel2:#f6f8fb;--line:#e2e7ef;--line2:#d4dbe6;--ink:#1a2330;--mut:#566376;--mut2:#8591a3;--acc:#0fa585;--amber:#b7791f;--grn:#15803d;--red:#dc2626;--blue:#2563eb;color-scheme:light}
[data-theme="light"] .navitem:hover{background:#eef2f7}
[data-theme="light"] .navitem.on{background:#e4f5f0}
[data-theme="light"] .filterbar .fi,[data-theme="light"] input.f,[data-theme="light"] .in-search{background:#fff}
[data-theme="light"] select.f{background-color:#fff}[data-theme="light"] select.f option{background:#fff}
[data-theme="light"] .btn,[data-theme="light"] .code-tag{background:#f1f4f8}
[data-theme="light"] .btn.acc{background:var(--acc)}
[data-theme="light"] .exp-menu button:hover{background:#eef2f7}
[data-theme="light"] .exp-menu{box-shadow:0 14px 40px rgba(20,30,50,.15)}
[data-theme="light"] .reveal{background:#e9f8f1;border-color:#bfe5d3}
[data-theme="light"] .benchframe{background:#fff;box-shadow:0 6px 20px rgba(20,30,50,.1)}
[data-theme="light"] .guide{color:#3d4a5c}[data-theme="light"] .guide .st .n{background:#e3ebfb;color:#2c4a8a}
[data-theme="light"] .modal{background:rgba(20,28,40,.45)}[data-theme="light"] .mcard{box-shadow:0 30px 80px rgba(20,30,50,.25)}
[data-theme="light"] aside.open{box-shadow:0 0 50px rgba(20,30,50,.25)}
`

const welcomeLightCSS = `:root[data-theme="light"]{--bg:#f3f5f9;--panel:#fff;--line:#e2e7ef;--ink:#1a2330;--mut:#566376;--acc:#0fa585;color-scheme:light}
[data-theme="light"] body{background:radial-gradient(1100px 560px at 20% -10%,#e3eef0,#f3f5f9)}
[data-theme="light"] .feat li{color:#3d4a5c}[data-theme="light"] .feat i{background:#e0f3ee}
[data-theme="light"] .card{box-shadow:0 24px 60px rgba(20,30,50,.12)}
[data-theme="light"] .seg,[data-theme="light"] input,[data-theme="light"] select{background:#fff}
[data-theme="light"] .seg button.on{background:#e4f5f0}
[data-theme="light"] .ok{color:#0b7f66}[data-theme="light"] .err{color:#dc2626}
`

const terminalLightCSS = `:root[data-theme="light"]{--bg:#f3f5f9;--surf:#fff;--surf2:#f6f8fb;--line:rgba(20,30,50,.12);--ink:#1a2330;--ink2:#566376;--ink3:#8591a3;--brand:#0a8f68;--up:#0f9d58;--down:#d93025;color-scheme:light}
html.embed[data-theme="light"]{--bg:#f3f5f9;--surf:#fff;--surf2:#f6f8fb;--line:#e2e7ef;--ink:#1a2330;--ink2:#566376;--ink3:#8591a3;--brand:#0fa585;--up:#15803d;--down:#dc2626}
[data-theme="light"] tbody tr:hover{background:rgba(20,30,50,.04)}[data-theme="light"] .mwt tbody tr:nth-child(even){background:rgba(20,30,50,.03)}
[data-theme="light"] select option{background:#fff}[data-theme="light"] *{scrollbar-color:#c3ccd9 transparent}
[data-theme="light"] .tab.on,[data-theme="light"] .btn.buy{color:#fff}
html.embed .theme-btn{display:none}
`

const benchLightCSS = `:root[data-theme="light"]{--bg:#f3f5f9;--panel:#fff;--panel2:#f6f8fb;--bd:#e2e7ef;--tx:#1a2330;--mut:#566376;--grn:#0f9d58;--blu:#2563eb;--amb:#b7791f;--red:#dc2626;--vio:#6d4fd6;color-scheme:light}
[data-theme="light"] .req,[data-theme="light"] .mult{background:#e9f8f1;border-color:#bfe5d3}
[data-theme="light"] .mult.miss{background:#fdeeee;border-color:#f3c6c6}
[data-theme="light"] button,[data-theme="light"] .sel,[data-theme="light"] .hw .hn{background:#f1f4f8}
[data-theme="light"] button:hover{border-color:#8aa6cf}
[data-theme="light"] button.on,[data-theme="light"] button.start{background:var(--grn);border-color:var(--grn);color:#fff}
[data-theme="light"] button.stop{background:var(--red);border-color:var(--red);color:#fff}
[data-theme="light"] button.clean{background:#fff4de;color:var(--amb)}
[data-theme="light"] button.rate.sel{background:#e3ecfd;color:#1d3f87}
[data-theme="light"] .clean{background:#fff4de;border-color:#e8cf9a}
[data-theme="light"] .bar{background:#e8edf4}[data-theme="light"] td{border-bottom-color:#edf0f5}
[data-theme="light"] .f{background:#e4f6ec}[data-theme="light"] .n{background:#e6edf8;color:#34507d}[data-theme="light"] .r{background:#fdeaea}
[data-theme="light"] .lpstage{background:#f6f8fb;border-color:#dfe5ee}[data-theme="light"] .lpp95{color:#1a2330}
[data-theme="light"] .stage .arrow,[data-theme="light"] .lparrow{color:#9aa7b8}
header .theme-btn{margin-left:auto}
`

// ThemeButtonFixed is the button pinned to the top-right corner (welcome page).
const ThemeButtonFixed = `<div class="theme-btn-wrap" style="position:fixed;top:18px;right:18px;z-index:10">` + ThemeButton + `</div>`

// Button styles per page: each page names its own line / text / hover variables.
const adminThemeBtnCSS = `.theme-btn{display:inline-flex;align-items:center;justify-content:center;width:36px;height:36px;border-radius:10px;border:1px solid var(--line);background:transparent;color:var(--mut);cursor:pointer;padding:0;transition:.15s}.theme-btn:hover{color:var(--ink);background:var(--panel2)}.theme-btn .moon{display:none}[data-theme="light"] .theme-btn .sun{display:none}[data-theme="light"] .theme-btn .moon{display:block}`
const welcomeThemeBtnCSS = `.theme-btn{display:inline-flex;align-items:center;justify-content:center;width:36px;height:36px;border-radius:10px;border:1px solid var(--line);background:transparent;color:var(--mut);cursor:pointer;padding:0;transition:.15s}.theme-btn:hover{color:var(--ink);background:var(--panel)}.theme-btn .moon{display:none}[data-theme="light"] .theme-btn .sun{display:none}[data-theme="light"] .theme-btn .moon{display:block}`
const terminalThemeBtnCSS = `.theme-btn{display:inline-flex;align-items:center;justify-content:center;width:36px;height:36px;border-radius:10px;border:1px solid var(--line);background:transparent;color:var(--ink2);cursor:pointer;padding:0;transition:.15s}.theme-btn:hover{color:var(--ink);background:var(--surf)}.theme-btn .moon{display:none}[data-theme="light"] .theme-btn .sun{display:none}[data-theme="light"] .theme-btn .moon{display:block}`
const benchThemeBtnCSS = `.theme-btn{display:inline-flex;align-items:center;justify-content:center;width:36px;height:36px;border-radius:10px;border:1px solid var(--bd);background:transparent;color:var(--mut);cursor:pointer;padding:0;transition:.15s}.theme-btn:hover{color:var(--tx);background:var(--panel2)}.theme-btn .moon{display:none}[data-theme="light"] .theme-btn .sun{display:none}[data-theme="light"] .theme-btn .moon{display:block}`
