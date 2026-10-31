package simulator

// omsclient.go — the OMS's own order channel, the one its web front end uses:
// sign in over REST, then send orders on the dealer's order WebSocket and read
// the clients' holdings from the position WebSocket. The tenant goes in the
// X-Tenant-Subdomain header (REST) and the tenant_subdomain query (WebSocket),
// as the front end sends it.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// omsSite is where an OMS tenant lives, worked out from the address its traders
// use: https://ibl.bdtaskoms.com is tenant "ibl" (prod) with its API at
// https://api.bdtaskoms.com; a first label ending in -uat selects UAT.
type omsSite struct {
	Origin string // https://ibl.bdtaskoms.com, sent as Origin like the browser
	API    string // https://api.bdtaskoms.com
	WS     string // wss://api.bdtaskoms.com
	Tenant string
	Env    string // prod | uat
}

func parseOMSSite(raw, api string) (omsSite, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return omsSite{}, errors.New("enter the OMS address traders use, for example https://ibl.bdtaskoms.com")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return omsSite{}, fmt.Errorf("%q is not a web address like https://ibl.bdtaskoms.com", raw)
	}
	labels := strings.Split(strings.ToLower(u.Hostname()), ".")
	if net.ParseIP(u.Hostname()) != nil || len(labels) < 3 {
		return omsSite{}, fmt.Errorf("%s: enter the tenant's own address, like https://ibl.bdtaskoms.com", u.Hostname())
	}
	site := omsSite{Origin: u.Scheme + "://" + u.Host, Tenant: labels[0], Env: "prod"}
	if t, ok := strings.CutSuffix(site.Tenant, "-uat"); ok {
		site.Tenant, site.Env = t, "uat"
	}
	if site.Tenant == "" || site.Tenant == "api" || site.Tenant == "www" {
		return omsSite{}, fmt.Errorf("%s is not a tenant address: use the one traders sign in at, like https://ibl.bdtaskoms.com", u.Host)
	}
	if api = strings.TrimRight(strings.TrimSpace(api), "/"); api == "" {
		api = u.Scheme + "://api." + strings.Join(labels[1:], ".")
		if p := u.Port(); p != "" {
			api += ":" + p
		}
	}
	au, err := url.Parse(api)
	if err != nil || au.Host == "" || (au.Scheme != "https" && au.Scheme != "http") {
		return omsSite{}, fmt.Errorf("%q is not an API address like https://api.bdtaskoms.com", api)
	}
	site.API = au.Scheme + "://" + au.Host
	site.WS = "wss://" + au.Host
	if au.Scheme == "http" {
		site.WS = "ws://" + au.Host
	}
	return site, nil
}

// omsDealer is who the sign-in says the dealer is; every order carries it.
type omsDealer struct {
	UserID     int64
	TraderPK   int64
	TraderCode string // the code the dealer's order channel and FIX SenderSubID use
	BranchID   int64
	Name       string
}

// omsInvestor is one of the dealer's clients.
type omsInvestor struct {
	Code, BO, Name string
}

// omsClient talks to one OMS tenant as one dealer.
type omsClient struct {
	site       omsSite
	user, pass string // kept to sign in again before the access token runs out
	hc         *http.Client

	mu       sync.Mutex
	dealer   omsDealer
	access   string
	refresh  string
	signedIn time.Time
}

func newOMSClient(site omsSite, user, pass string) *omsClient {
	return &omsClient{site: site, user: strings.TrimSpace(user), pass: pass, hc: &http.Client{Timeout: 15 * time.Second}}
}

// call sends one REST request to the OMS API and decodes its JSON reply.
func (c *omsClient) call(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.site.API+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", c.site.Origin)
	req.Header.Set("X-Tenant-Subdomain", c.site.Tenant)
	req.Header.Set("X-Tenant-Env", c.site.Env)
	c.mu.Lock()
	if c.access != "" {
		req.Header.Set("Authorization", "Bearer "+c.access)
	}
	c.mu.Unlock()
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the OMS at %s did not answer: %v", c.site.API, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil, fmt.Errorf("the OMS answered %s with %s, not JSON", path, resp.Status)
	}
	if resp.StatusCode/100 != 2 || out["success"] == false {
		msg := firstStr(out, "message", "detail", "error")
		if msg == "" {
			msg = resp.Status
		}
		return out, fmt.Errorf("%s", msg)
	}
	return out, nil
}

// login signs the dealer in and learns who it is.
func (c *omsClient) login(ctx context.Context) error {
	out, err := c.call(ctx, http.MethodPost, "/api/users/login/", map[string]string{"username": c.user, "password": c.pass})
	if err != nil {
		return fmt.Errorf("OMS sign-in as %s failed: %v", c.user, err)
	}
	data, _ := out["data"].(map[string]any)
	if data == nil {
		return fmt.Errorf("OMS sign-in as %s: the answer has no data", c.user)
	}
	if data["mfa_required"] == true {
		return fmt.Errorf("the OMS asks %s for a one-time code (MFA): use a dealer login without MFA", c.user)
	}
	if st := str(data["users_status"]); st == "investor" {
		return fmt.Errorf("%s is an investor login: use a dealer (trader) login, which can trade for its clients", c.user)
	}
	tok, _ := data["token"].(map[string]any)
	ud, _ := data["user_detail"].(map[string]any)
	d := omsDealer{UserID: int64(num(data["id"])), TraderPK: int64(num(ud["trader_id"])), BranchID: int64(num(ud["branch_id"])),
		TraderCode: firstStr(ud, "trader_code", "tws_code"), Name: firstStr(ud, "trader_name", "name")}
	if d.Name == "" {
		d.Name = str(data["username"])
	}
	access := str(tok["access"])
	switch {
	case access == "":
		return fmt.Errorf("OMS sign-in as %s: no access token in the answer", c.user)
	case d.TraderCode == "" || d.TraderPK == 0:
		return fmt.Errorf("%s is not a dealer in the OMS (no trader code): use a dealer (trader) login", c.user)
	}
	c.mu.Lock()
	c.dealer, c.access, c.refresh, c.signedIn = d, access, str(tok["refresh"]), time.Now()
	c.mu.Unlock()
	return nil
}

// signedInFor is how long ago the current access token was issued.
func (c *omsClient) signedInFor() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Since(c.signedIn)
}

// logout ends the OMS session; a failure only means the token runs out by itself.
func (c *omsClient) logout() {
	c.mu.Lock()
	refresh := c.refresh
	c.mu.Unlock()
	if refresh == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.call(ctx, http.MethodPost, "/api/users/logout/", map[string]string{"refresh": refresh})
}

// investors lists the dealer's own clients.
func (c *omsClient) investors(ctx context.Context) ([]omsInvestor, error) {
	out, err := c.call(ctx, http.MethodGet, "/api/oms/get_investor_list_by_trader/"+strconv.FormatInt(c.dealer.TraderPK, 10)+"/", nil)
	if err != nil {
		return nil, err
	}
	rows, _ := out["data"].([]any)
	list := make([]omsInvestor, 0, len(rows))
	for _, r := range rows {
		m, _ := r.(map[string]any)
		if code := firstStr(m, "investor_code", "client_code"); code != "" {
			list = append(list, omsInvestor{Code: code, BO: str(m["bo_id"]), Name: str(m["investor_name"])})
		}
	}
	return list, nil
}

// dial opens one of the OMS's WebSockets, e.g. "order/<trader code>".
func (c *omsClient) dial(ctx context.Context, path string) (*websocket.Conn, error) {
	c.mu.Lock()
	token := c.access
	c.mu.Unlock()
	q := url.Values{"token": {token}, "individual_id": {newIndividualID()},
		"tenant_subdomain": {c.site.Tenant}, "tenant_env": {c.site.Env}}
	h := http.Header{"Origin": {c.site.Origin}}
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	ws, resp, err := d.DialContext(ctx, c.site.WS+"/"+path+"/?"+q.Encode(), h)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("%s/%s: %s", c.site.WS, path, resp.Status)
		}
		return nil, fmt.Errorf("%s/%s: %v", c.site.WS, path, err)
	}
	ws.SetReadLimit(64 << 20) // the first frame lists the day's orders
	return ws, nil
}

// newIndividualID is a random browser-session id, as the front end makes one.
func newIndividualID() string {
	b := make([]byte, 16)
	rand.Read(b)
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// str reads a JSON value as text; whole numbers print without a decimal point.
func str(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

// num reads a JSON number, or a number sent as text.
func num(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(x, "%")), 64)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(m[k]); s != "" {
			return s
		}
	}
	return ""
}
