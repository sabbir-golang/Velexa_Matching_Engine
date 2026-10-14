package web

// CSRFHead goes in <head> after ThemeHead on every page. It sends the
// browser's CSRF token (the me_csrf cookie) with each state-changing request:
// as the X-CSRF-Token header on same-origin fetch calls, and as a hidden "csrf"
// field on posted forms. csrfField(form) adds it to a form sent with .submit().
const CSRFHead = `<script>(function(){
function tok(){var m=document.cookie.match(/(?:^|;\s*)me_csrf=([^;]+)/);return m?decodeURIComponent(m[1]):""}
function same(u){try{return new URL(String(u&&u.url||u),location.href).origin===location.origin}catch(e){return false}}
var f=window.fetch;
window.fetch=function(u,o){o=o||{};var m=String(o.method||(u&&u.method)||"GET").toUpperCase();
 if(m!=="GET"&&m!=="HEAD"&&same(u)){var h=new Headers(o.headers||{});h.set("X-CSRF-Token",tok());o=Object.assign({},o,{headers:h,credentials:"same-origin"})}
 return f.call(this,u,o)};
window.csrfField=function(fm){var i=fm.querySelector("input[name=csrf]");if(!i){i=document.createElement("input");i.type="hidden";i.name="csrf";fm.appendChild(i)}i.value=tok()};
document.addEventListener("submit",function(e){var fm=e.target;if(fm&&String(fm.method).toLowerCase()==="post")csrfField(fm)},true);
})();</script>`
