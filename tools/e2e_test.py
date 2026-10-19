#!/usr/bin/env python3
"""End-to-end system test for a running matching engine.

Drives the engine the way the OMS and a console user do: the web console API
(sign-in, CSRF, broker issue), two FIX broker sessions, the ITCH market feed,
and the public market-data API. Standard library only.

Usage (tools/e2e.sh starts an isolated engine and calls this):
    python3 tools/e2e_test.py --http 18010 --fix 15001 --itch 15501 \
        --admin-user admin --admin-pass '...'
Exit code 0 when every case passes.
"""
import argparse
import http.cookiejar
import json
import socket
import struct
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

SOH = "\x01"
RESULTS = []


def case(cid, title):
    """Decorator: run a test case, record PASS/FAIL with its detail."""
    def wrap(fn):
        def run(*a, **kw):
            t0 = time.time()
            try:
                detail = fn(*a, **kw) or ""
                RESULTS.append((cid, "PASS", title, detail, time.time() - t0))
            except AssertionError as e:
                RESULTS.append((cid, "FAIL", title, str(e), time.time() - t0))
            except Exception as e:  # noqa: BLE001 - report anything unexpected
                RESULTS.append((cid, "FAIL", title, f"{type(e).__name__}: {e}", time.time() - t0))
        run.cid = cid
        return run
    return wrap


# ---------------------------------------------------------------- HTTP client
class Console:
    def __init__(self, base):
        self.base = base
        self.jar = http.cookiejar.CookieJar()
        self.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar),
                                              NoRedirect())

    def csrf(self):
        for c in self.jar:
            if c.name == "me_csrf":
                return c.value
        self.get("/")
        return next(c.value for c in self.jar if c.name == "me_csrf")

    def req(self, method, path, body=None, form=None, headers=None):
        h = dict(headers or {})
        data = None
        if form is not None:
            data = urllib.parse.urlencode(form).encode()
            h["Content-Type"] = "application/x-www-form-urlencoded"
        elif body is not None:
            data = json.dumps(body).encode()
            h["Content-Type"] = "application/json"
        r = urllib.request.Request(self.base + path, data=data, method=method, headers=h)
        try:
            resp = self.op.open(r, timeout=10)
            return resp.status, resp.headers, resp.read().decode(errors="replace")
        except urllib.error.HTTPError as e:
            return e.code, e.headers, e.read().decode(errors="replace")

    def get(self, path):
        return self.req("GET", path)

    def post(self, path, body=None, csrf=True):
        h = {"X-CSRF-Token": self.csrf()} if csrf else {}
        return self.req("POST", path, body=body if body is not None else {}, headers=h)

    def login(self, user, pw, as_="admin"):
        tok = self.csrf()
        return self.req("POST", "/admin/login", form={"user": user, "pass": pw, "as": as_, "csrf": tok})

    def json(self, path):
        code, _, body = self.get(path)
        assert code == 200, f"GET {path}: {code} {body[:120]}"
        return json.loads(body)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **kw):
        return None


# ---------------------------------------------------------------- FIX client
class Fix:
    def __init__(self, port, sender, target="DSEME"):
        self.sock = socket.create_connection(("127.0.0.1", port), timeout=5)
        self.sender, self.target, self.seq, self.buf = sender, target, 1, b""

    def send(self, mt, fields):
        ts = time.strftime("%Y%m%d-%H:%M:%S.000", time.gmtime())
        body = f"35={mt}{SOH}49={self.sender}{SOH}56={self.target}{SOH}34={self.seq}{SOH}52={ts}{SOH}"
        body += "".join(f"{k}={v}{SOH}" for k, v in fields)
        head = f"8=FIXT.1.1{SOH}9={len(body)}{SOH}"
        msg = head + body
        msg += f"10={sum(msg.encode()) % 256:03d}{SOH}"
        self.sock.sendall(msg.encode())
        self.seq += 1

    def recv(self, timeout=2.0, until=None):
        """Read messages for up to timeout s (or until until(msg) is true)."""
        out, end = [], time.time() + timeout
        while time.time() < end:
            while True:
                i = self.buf.find(b"\x0110=")
                if i < 0:
                    break
                j = self.buf.find(b"\x01", i + 1)
                if j < 0:
                    break
                raw, self.buf = self.buf[: j + 1], self.buf[j + 1:]
                m = dict(f.split("=", 1) for f in raw.decode(errors="replace").strip(SOH).split(SOH) if "=" in f)
                if m.get("35") == "0":
                    continue
                if m.get("35") == "1":  # answer test requests so the session stays up
                    self.send("0", [(112, m.get("112", ""))])
                    continue
                out.append(m)
                if until and until(m):
                    return out
            self.sock.settimeout(max(0.05, end - time.time()))
            try:
                chunk = self.sock.recv(65536)
            except socket.timeout:
                break
            if not chunk:
                break
            self.buf += chunk
        return out

    def logon(self, password):
        self.send("A", [(98, 0), (108, 30), (141, "Y"), (553, self.sender), (554, password), (1137, 9)])
        return self.recv(3, until=lambda m: m.get("35") in ("A", "5"))

    def order(self, cl, sym, side, qty, px=None, typ="2", tif="0", extra=()):
        f = [(11, cl), (1, "BO1001"), (55, sym), (54, side), (60, time.strftime("%Y%m%d-%H:%M:%S", time.gmtime())),
             (38, qty), (40, typ), (59, tif), (762, "PUBLIC")]
        if px is not None:
            f.append((44, f"{px:.2f}"))
        f.extend(extra)
        self.send("D", f)

    def close(self):
        try:
            self.send("5", [])
            self.sock.close()
        except OSError:
            pass


def ers(msgs, cl=None):
    return [m for m in msgs if m.get("35") == "8" and (cl is None or m.get("11") == cl)]


# ---------------------------------------------------------------- ITCH client
# Body sizes (after the 1-byte type) of the DSE-ITCH messages; a SequencedData
# packet holds them back to back (docs/protocols.md, "Message layouts").
ITCH_LEN = {"T": 4, "S": 17, "L": 16, "M": 24, "P": 68, "R": 170, "H": 10, "A": 33, "F": 37,
            "E": 28, "C": 37, "B": 13, "D": 12, "U": 36, "I": 29, "Q": 33, "Z": 16}


class Itch:
    """A SoupBinTCP client of the ITCH market feed."""

    def __init__(self, port, user="", pw=""):
        self.sock = socket.create_connection(("127.0.0.1", port), timeout=5)
        body = b"L" + user.ljust(6)[:6].encode() + pw.ljust(10)[:10].encode() + b" " * 10 + b"1".rjust(20)
        self.sock.sendall(struct.pack(">H", len(body)) + body)
        self.buf, self.login = b"", None

    def read(self, seconds):
        """Return the ITCH message types received within seconds."""
        types, end = [], time.time() + seconds
        while time.time() < end:
            self.sock.settimeout(max(0.05, end - time.time()))
            try:
                chunk = self.sock.recv(65536)
            except socket.timeout:
                break
            if not chunk:
                break
            self.buf += chunk
            while len(self.buf) >= 2:
                n = struct.unpack(">H", self.buf[:2])[0]
                if len(self.buf) < 2 + n:
                    break
                pkt, self.buf = self.buf[2: 2 + n], self.buf[2 + n:]
                kind = chr(pkt[0])
                if kind in "AJ":
                    self.login = kind
                elif kind == "S":
                    p = pkt[1:]
                    while p:  # one or more ITCH messages back to back
                        t = chr(p[0])
                        n2 = ITCH_LEN.get(t)
                        assert n2 is not None and len(p) >= 1 + n2, f"bad ITCH framing at type {t!r}"
                        types.append(t)
                        p = p[1 + n2:]
        return types

    def close(self):
        self.sock.close()


# ---------------------------------------------------------------- the cases
def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--http", type=int, default=18010)
    ap.add_argument("--fix", type=int, default=15001)
    ap.add_argument("--itch", type=int, default=15501)
    ap.add_argument("--admin-user", default="admin")
    ap.add_argument("--admin-pass", required=True)
    ap.add_argument("--load", type=int, default=2000, help="orders per broker in the throughput case")
    ap.add_argument("--save", help="write the issued broker credentials here (for --after-restart)")
    ap.add_argument("--after-restart", help="only run the persistence checks, with credentials from this file")
    ap.add_argument("--after-restore", help="only check a restored backup, with credentials from this file")
    ap.add_argument("--dump-market", help="only save the live market (prices, books, counters) to this file")
    ap.add_argument("--market", help="with --after-restart: the market saved by --dump-market before the restart")
    a = ap.parse_args()
    base = f"http://127.0.0.1:{a.http}"
    adm = Console(base)
    ctx = {}
    if a.dump_market:
        with open(a.dump_market, "w") as f:
            json.dump(market_state(adm), f)
        return 0
    if a.after_restart:
        return after_restart(a, base, adm, json.load(open(a.after_restart)))
    if a.after_restore:
        return after_restore(a, base, adm, json.load(open(a.after_restore)))

    @case("SYS-01", "Engine is up: /healthz, /health and /api/status answer")
    def sys01():
        code, _, body = adm.get("/healthz")
        assert code == 200 and body.strip() == "ok", f"/healthz {code} {body!r}"
        st = adm.json("/api/status")
        assert st["instruments"] > 0, st
        return f"{st['instruments']} instruments, phase {st['phase']}"

    @case("SEC-01", "Admin sign-in with ME_ADMIN_PASS; wrong password refused")
    def sec01():
        bad = Console(base)
        code, h, _ = bad.login(a.admin_user, "Wrong-Pass-123")
        assert code == 303 and "e=1" in h.get("Location", ""), f"wrong password: {code} {h.get('Location')}"
        code, h, _ = adm.login(a.admin_user, a.admin_pass)
        assert code == 303 and h.get("Location", "").endswith("/admin"), f"admin login: {code} {h.get('Location')}"
        me = adm.json("/admin/api/me")
        assert me["role"] == "admin", me

    @case("SEC-02", "Console POST without CSRF token is refused (403)")
    def sec02():
        code, _, body = adm.post("/admin/api/trading-session?action=open", csrf=False)
        assert code == 403, f"{code} {body[:80]}"

    @case("SEC-03", "Order entry and session control need sign-in")
    def sec03():
        anon = Console(base)
        anon.csrf()
        code, _, _ = anon.post("/api/order", {"symbol": "GP", "side": "BUY", "qty": 1, "price": 1})
        assert code == 401, f"anonymous /api/order: {code}"
        code, _, _ = anon.post("/api/session/open")
        assert code == 401, f"anonymous /api/session/open: {code}"

    @case("SES-01", "Admin opens the market (POST trading-session)")
    def ses01():
        code, _, body = adm.post("/admin/api/trading-session?action=open")
        assert code == 200, f"{code} {body[:100]}"
        ph = json.loads(body)["phase"]
        assert ph == "Continuous Trading", ph
        return ph

    @case("BRK-01", "Admin issues two brokers (FIX + ITCH credentials)")
    def brk01():
        for comp in ("E2EAAA", "E2EBBB"):
            code, _, body = adm.post("/admin/api/broker/add", {"Sender": comp, "Company": comp + " Securities"})
            r = json.loads(body)
            assert code == 200 and r.get("ok"), f"{comp}: {code} {body[:120]}"
            ctx[comp] = r
        sheet_ep = adm.json("/admin/api/broker/detail?sender=E2EAAA")["endpoints"]
        assert sheet_ep["ext_addr"].endswith(f":{a.fix}"), sheet_ep
        return "sheet endpoint " + sheet_ep["ext_addr"]

    @case("BRK-02", "Duplicate SenderCompID is refused")
    def brk02():
        _, _, body = adm.post("/admin/api/broker/add", {"Sender": "E2EAAA"})
        assert json.loads(body).get("ok") is False, body

    sym = "GP"

    @case("MD-01", "Market data: order book and market watch for the test symbol")
    def md01():
        b = adm.json(f"/api/book?symbol={sym}")
        prev = b["md"]["prev"]
        assert prev > 0, b["md"]
        ctx["px"] = round(round(prev / 0.1) * 0.1, 2)  # a price on the 0.10 tick inside the band
        rows = adm.json("/api/market")["rows"]
        assert any(r.get("symbol") == sym for r in rows), "GP missing from /api/market"
        return f"prev {prev}, test price {ctx['px']}"

    @case("FIX-01", "FIX logon with issued credentials (both brokers)")
    def fix01():
        for comp in ("E2EAAA", "E2EBBB"):
            f = Fix(a.fix, comp)
            r = f.logon(ctx[comp]["fix_password"])
            assert r and r[-1].get("35") == "A", f"{comp}: {r}"
            ctx["fix_" + comp] = f

    @case("FIX-02", "FIX logon with a wrong password or unknown CompID is rejected")
    def fix02():
        f = Fix(a.fix, "E2EAAA")
        r = f.logon("not-the-password")
        assert r and r[-1].get("35") == "5" and "invalid credentials" in r[-1].get("58", ""), r
        g = Fix(a.fix, "NOBODY1")
        r = g.logon("x")
        assert r and r[-1].get("35") == "5" and "not registered" in r[-1].get("58", ""), r

    @case("FIX-03", "Resting order is acknowledged (150=0)")
    def fix03():
        s = ctx["fix_E2EAAA"]
        s.order("A-1", sym, "2", 100, ctx["px"])
        r = ers(s.recv(3, until=lambda m: m.get("35") == "8"), "A-1")
        assert r and r[0].get("150") == "0" and r[0].get("39") == "0", r
        return "OrderID " + r[0].get("37", "")

    @case("FIX-04", "Crossing order trades; both brokers get fill reports (150=F)")
    def fix04():
        a1, b1 = ctx["fix_E2EAAA"], ctx["fix_E2EBBB"]
        b1.order("B-1", sym, "1", 100, ctx["px"])
        rb = ers(b1.recv(3, until=lambda m: m.get("39") == "2"), "B-1")
        ra = ers(a1.recv(3, until=lambda m: m.get("39") == "2"), "A-1")
        assert any(m.get("150") == "F" and m.get("39") == "2" and m.get("14") == "100" for m in rb), f"buyer: {rb}"
        assert any(m.get("150") == "F" and m.get("39") == "2" for m in ra), f"seller: {ra}"
        last = adm.json(f"/api/book?symbol={sym}")["md"]["last"]
        assert abs(last - ctx["px"]) < 1e-9, f"last {last} != {ctx['px']}"
        return f"filled 100 @ {ctx['px']}, market data last {last}"

    @case("FIX-05", "Two brokers may use the same ClOrdID; each cancels only its own")
    def fix05():
        a1, b1 = ctx["fix_E2EAAA"], ctx["fix_E2EBBB"]
        lo = round(ctx["px"] - 0.5, 2)
        a1.order("SAME-1", sym, "1", 100, lo)
        b1.order("SAME-1", sym, "1", 200, lo)
        assert ers(a1.recv(2, until=lambda m: m.get("35") == "8"), "SAME-1"), "AAA ack missing"
        assert ers(b1.recv(2, until=lambda m: m.get("35") == "8"), "SAME-1"), "BBB ack missing"
        a1.send("F", [(41, "SAME-1"), (11, "SAME-1-C"), (55, sym), (54, "1"), (60, time.strftime("%Y%m%d-%H:%M:%S", time.gmtime()))])
        ra = a1.recv(2, until=lambda m: m.get("35") in ("8", "9"))
        assert any(m.get("150") == "4" and m.get("38", "100") == "100" for m in ra), f"AAA cancel: {ra}"
        b1.send("F", [(41, "SAME-1"), (11, "SAME-1-C"), (55, sym), (54, "1"), (60, time.strftime("%Y%m%d-%H:%M:%S", time.gmtime()))])
        rb = b1.recv(2, until=lambda m: m.get("35") in ("8", "9"))
        assert any(m.get("150") == "4" for m in rb), f"BBB could not cancel its own SAME-1: {rb}"

    @case("FIX-06", "Duplicate ClOrdID of a working order is rejected")
    def fix06():
        s = ctx["fix_E2EAAA"]
        lo = round(ctx["px"] - 0.5, 2)
        s.order("DUP-1", sym, "1", 100, lo)
        s.recv(2, until=lambda m: m.get("11") == "DUP-1")
        s.order("DUP-1", sym, "1", 100, lo)
        r = ers(s.recv(2, until=lambda m: m.get("11") == "DUP-1"), "DUP-1")
        assert r and r[-1].get("150") == "8" and "Duplicate ClOrdID" in r[-1].get("58", ""), r
        return r[-1].get("58")

    @case("FIX-07", "Amend outside the price band is refused with 35=9 and the reason")
    def fix07():
        s = ctx["fix_E2EAAA"]
        s.send("G", [(41, "DUP-1"), (11, "DUP-1-G"), (55, sym), (54, "1"), (38, 100), (40, "2"),
                     (44, f"{ctx['px'] * 1.5:.2f}"), (60, time.strftime("%Y%m%d-%H:%M:%S", time.gmtime()))])
        r = s.recv(2, until=lambda m: m.get("35") in ("8", "9"))
        rej = [m for m in r if m.get("35") == "9"]
        assert rej and "price band" in rej[0].get("58", "") and rej[0].get("37") != "NONE", r
        return rej[0].get("58")

    @case("FIX-08", "Amend within the band is accepted (150=5) and cancel removes it (150=4)")
    def fix08():
        s = ctx["fix_E2EAAA"]
        s.send("G", [(41, "DUP-1"), (11, "DUP-1-G2"), (55, sym), (54, "1"), (38, 50), (40, "2"),
                     (44, f"{ctx['px'] - 0.3:.2f}"), (60, time.strftime("%Y%m%d-%H:%M:%S", time.gmtime()))])
        r = ers(s.recv(2, until=lambda m: m.get("11") == "DUP-1-G2"), "DUP-1-G2")
        assert r and r[0].get("150") == "5", r
        s.send("F", [(41, "DUP-1-G2"), (11, "DUP-1-C"), (55, sym), (54, "1"), (60, time.strftime("%Y%m%d-%H:%M:%S", time.gmtime()))])
        r = ers(s.recv(2, until=lambda m: m.get("11") == "DUP-1-C"), "DUP-1-C")
        assert r and r[0].get("150") == "4", r

    @case("FIX-09", "Off-tick and out-of-band new orders are rejected with DSE text")
    def fix09():
        s = ctx["fix_E2EAAA"]
        s.order("TICK-1", sym, "1", 100, ctx["px"] + 0.05)
        r1 = ers(s.recv(2, until=lambda m: m.get("11") == "TICK-1"), "TICK-1")
        s.order("BAND-1", sym, "1", 100, ctx["px"] * 1.5)
        r2 = ers(s.recv(2, until=lambda m: m.get("11") == "BAND-1"), "BAND-1")
        assert r1 and r1[0].get("150") == "8" and "off-tick" in r1[0].get("58", ""), r1
        assert r2 and r2[0].get("150") == "8" and "price band" in r2[0].get("58", ""), r2

    @case("ITCH-01", "ITCH market feed: login, start-of-day snapshot, and a live trade is published")
    def itch01():
        feed = Itch(a.itch, "E2EAAA"[:6], ctx["E2EAAA"]["itch_password"])
        snap = feed.read(2)
        assert feed.login == "A", f"login reply {feed.login}"
        assert {"R", "P", "A"} <= set(snap), f"snapshot lacks directory/adds: {sorted(set(snap))}"
        a1, b1 = ctx["fix_E2EAAA"], ctx["fix_E2EBBB"]
        a1.order("IT-S", sym, "2", 30, ctx["px"])
        a1.recv(2, until=lambda m: m.get("11") == "IT-S")
        b1.order("IT-B", sym, "1", 30, ctx["px"])
        b1.recv(2, until=lambda m: m.get("11") == "IT-B" and m.get("39") == "2")
        live = feed.read(2)
        feed.close()
        assert "A" in live and ("E" in live or "C" in live), f"live trade not on the feed: {sorted(set(live))}"
        return f"snapshot {len(snap)} msgs; live {''.join(sorted(set(live)))}"

    @case("WEB-01", "Signed-in console user places an order from the Trade Terminal API")
    def web01():
        code, _, body = adm.post("/api/order", {"symbol": sym, "side": "BUY", "type": "LIMIT",
                                                "price": round(ctx["px"] - 1, 2), "qty": 10})
        assert code == 200, f"{code} {body[:120]}"
        rep = json.loads(body)
        assert rep["status"] in ("NEW", "FILLED", "PARTIALLY_FILLED"), rep
        return rep["status"]

    @case("PERF-01", f"Throughput: {a.load} orders per broker over FIX, all acknowledged")
    def perf01():
        a1, b1 = ctx["fix_E2EAAA"], ctx["fix_E2EBBB"]
        n = a.load
        t0 = time.time()
        for i in range(n):  # pipelined: send everything, then collect
            a1.order(f"LS-{i}", sym, "2", 10, ctx["px"])
            b1.order(f"LB-{i}", sym, "1", 10, ctx["px"])
        got_a = ers(a1.recv(30, until=lambda m: m.get("11") == f"LS-{n - 1}" and m.get("39") in ("0", "1", "2")))
        got_b = ers(b1.recv(30, until=lambda m: m.get("11") == f"LB-{n - 1}" and m.get("39") == "2"))
        dt = time.time() - t0
        acked_a = {m["11"] for m in got_a if m.get("11", "").startswith("LS-")}
        filled_b = {m["11"] for m in got_b if m.get("11", "").startswith("LB-") and m.get("39") == "2"}
        assert len(acked_a) == n, f"seller saw reports for {len(acked_a)}/{n} orders"
        assert len(filled_b) == n, f"buyer saw {len(filled_b)}/{n} orders fully filled"
        return f"{2 * n} orders, {n} trades in {dt:.2f}s = {2 * n / dt:,.0f} orders/s end to end"

    for c in [sys01, sec01, sec02, sec03, ses01, brk01, brk02, md01, fix02, fix01, fix03, fix04, fix05,
              fix06, fix07, fix08, fix09, itch01, web01, perf01]:
        needs_fix = c.cid.startswith(("FIX-0", "PERF")) and c.cid not in ("FIX-01", "FIX-02")
        if needs_fix and not ctx.get("fix_E2EBBB"):
            RESULTS.append((c.cid, "SKIP", "(FIX sessions not available)", "", 0))
            continue
        c()
    for k in ("fix_E2EAAA", "fix_E2EBBB"):
        if ctx.get(k):
            ctx[k].close()
    if a.save:
        with open(a.save, "w") as f:
            json.dump({k: ctx[k] for k in ("E2EAAA", "E2EBBB") if k in ctx}, f)
    return report()


def market_state(adm):
    """The live market as the console shows it: every instrument's prices and
    day figures, the books of the traded ones, and the order/trade counters."""
    rows = adm.json("/api/market")["rows"]
    st = adm.json("/api/status")
    books = {}
    for r in rows:
        if r.get("trades") or r.get("bid") or r.get("ask"):
            b = adm.json(f"/api/book?symbol={r['symbol']}")
            books[r["symbol"]] = {"bids": b["bids"], "asks": b["asks"], "trades": b["trades"]}
    return {"rows": {r["symbol"]: r for r in rows}, "books": books,
            "orders": st["orders"], "trades": st["trades"], "phase": st["phase"]}


def after_restart(a, base, adm, creds):
    @case("PER-01", "After a restart: admin can still sign in (stored accounts, session key)")
    def per01():
        code, h, _ = adm.login(a.admin_user, a.admin_pass)
        assert code == 303 and h.get("Location", "").endswith("/admin"), f"{code} {h.get('Location')}"

    @case("PER-02", "After a restart: issued brokers are still listed with their owner data")
    def per02():
        senders = {b["sender"] for b in adm.json("/admin/api/brokers")["brokers"]}
        assert {"E2EAAA", "E2EBBB"} <= senders, senders

    @case("PER-03", "After a restart: brokers log on with the same FIX credentials")
    def per03():
        f = Fix(a.fix, "E2EAAA")
        r = f.logon(creds["E2EAAA"]["fix_password"])
        f.close()
        assert r and r[-1].get("35") == "A", r

    @case("PER-04", "After a restart: revoking a broker stops its FIX logon")
    def per04():
        code, _, body = adm.post("/admin/api/broker/del", {"Sender": "E2EBBB"})
        assert code == 200, body
        f = Fix(a.fix, "E2EBBB")
        r = f.logon(creds["E2EBBB"]["fix_password"])
        f.close()
        assert r and r[-1].get("35") == "5", f"revoked broker still logs on: {r}"

    cases = [per01, per02, per03, per04]
    if a.market:
        before = json.load(open(a.market))
        now = {}

        @case("PER-06", "After a restart: every instrument's last, open, high, low, bid, ask, volume and trades are as before")
        def per06():
            now.update(market_state(adm))
            keys = ("last", "open", "high", "low", "prev", "change", "change_pct", "bid", "bid_qty", "ask", "ask_qty", "volume", "trades")
            diff = [f"{s}.{k}: {r.get(k)} -> {now['rows'].get(s, {}).get(k)}"
                    for s, r in before["rows"].items() for k in keys if now["rows"].get(s, {}).get(k) != r.get(k)]
            assert not diff, "; ".join(diff[:6])
            traded = sum(1 for r in before["rows"].values() if r.get("trades"))
            assert traded, "nothing had traded before the restart: the check proves nothing"
            return f"{len(before['rows'])} instruments, {traded} traded"

        @case("PER-07", "After a restart: the order books (depth, queue) and the trade tape are as before")
        def per07():
            assert now["books"] == before["books"], f"books differ: {sorted(before['books'])} vs {sorted(now['books'])}"
            levels = sum(len(b["bids"]) + len(b["asks"]) for b in before["books"].values())
            assert levels, "no resting orders before the restart: the check proves nothing"
            return f"{levels} price levels, phase {now['phase']}"

        @case("PER-08", "After a restart: order and trade numbers continue (none reused)")
        def per08():
            assert now["orders"] >= before["orders"] and now["trades"] == before["trades"], \
                f"orders {before['orders']} -> {now['orders']}, trades {before['trades']} -> {now['trades']}"
            sym, b = next((s, b) for s, b in before["books"].items() if b["asks"] or b["bids"])
            side, lv = ("BUY", b["asks"][0]) if b["asks"] else ("SELL", b["bids"][0])  # trades against the book
            code, _, body = adm.post("/api/order", {"symbol": sym, "side": side, "type": "LIMIT", "price": lv["price"], "qty": 1})
            assert code == 200, f"{code} {body[:120]}"
            rep = json.loads(body)
            assert rep["order_id"] > before["orders"], f"order number {rep['order_id']} reused (was {before['orders']})"
            assert rep["fills"] and rep["fills"][0]["id"] == before["trades"] + 1, f"trade number {rep['fills']}"
            return f"next order #{rep['order_id']}, next trade #{rep['fills'][0]['id']}"

        cases = [per01, per06, per07, per08, per02, per03, per04]
    for c in cases:
        c()
    return report()


def after_restore(a, base, adm, creds):
    """A backup restored into an empty database and data folder (a new server)."""
    @case("BAK-01", "Restored backup: admin signs in with the same password")
    def bak01():
        code, h, _ = adm.login(a.admin_user, a.admin_pass)
        assert code == 303 and h.get("Location", "").endswith("/admin"), f"{code} {h.get('Location')}"

    @case("BAK-02", "Restored backup: brokers as they were (E2EAAA kept, revoked E2EBBB still gone)")
    def bak02():
        senders = {b["sender"] for b in adm.json("/admin/api/brokers")["brokers"]}
        assert "E2EAAA" in senders and "E2EBBB" not in senders, senders

    @case("BAK-03", "Restored backup: the broker logs on with its old FIX password; audit trail kept")
    def bak03():
        f = Fix(a.fix, "E2EAAA")
        r = f.logon(creds["E2EAAA"]["fix_password"])
        f.close()
        assert r and r[-1].get("35") == "A", r
        actions = {e["action"] for e in adm.json("/admin/api/status").get("audit", [])}
        assert {"ISSUE", "REVOKE"} <= actions, f"audit entries missing: {sorted(actions)}"

    for c in (bak01, bak02, bak03):
        c()
    return report()


def report():
    w = max(len(r[2]) for r in RESULTS)
    for cid, verdict, title, detail, dt in RESULTS:
        print(f"{cid:8} {verdict:4}  {title:<{w}}  {detail}")
    failed = [r for r in RESULTS if r[1] != "PASS"]
    print(f"\n{len(RESULTS) - len(failed)}/{len(RESULTS)} passed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
