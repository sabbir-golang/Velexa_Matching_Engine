package itchgw

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"velexa-sim-me/internal/engine"
)

// payload length of each ITCH message type (without the type byte), as the OMS
// itch service's ITCH_LENGTH table has it.
var itchLen = map[byte]int{'T': 4, 'S': 17, 'L': 16, 'M': 24, 'P': 68, 'R': 170, 'H': 10, 'A': 33, 'F': 37,
	'E': 28, 'C': 37, 'B': 13, 'D': 12, 'U': 36, 'I': 29, 'Q': 33, 'Z': 16}

type itchMsg struct {
	typ  byte
	body []byte
}

func (m itchMsg) u32(off int) uint32 { return binary.BigEndian.Uint32(m.body[off:]) }
func (m itchMsg) u64(off int) uint64 { return binary.BigEndian.Uint64(m.body[off:]) }

// soupClient is a minimal SoupBinTCP client that splits packets into ITCH messages.
type soupClient struct {
	t   *testing.T
	c   net.Conn
	seq uint64 // number of sequenced packets received
}

func dial(t *testing.T, addr net.Addr, session string, seq uint64) (*soupClient, string, uint64) {
	t.Helper()
	c, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 46)
	for i := range body {
		body[i] = ' '
	}
	copy(body[0:6], "ITFIC1")
	copy(body[6:16], "password")
	copy(body[16:26], session)
	if seq > 0 {
		n := fmt.Sprint(seq)
		copy(body[46-len(n):], n)
	}
	if err := writeSoup(c, 'L', body); err != nil {
		t.Fatal(err)
	}
	sc := &soupClient{t: t, c: c}
	ptype, b := sc.packet(time.Second)
	if ptype != 'A' || len(b) < 30 {
		t.Fatalf("expected LoginAccept, got %q (%d bytes)", ptype, len(b))
	}
	var n uint64
	fmt.Sscan(string(b[10:30]), &n)
	return sc, string(b[0:10]), n
}

func (s *soupClient) packet(wait time.Duration) (byte, []byte) {
	s.c.SetReadDeadline(time.Now().Add(wait))
	hdr := make([]byte, 3)
	if _, err := io.ReadFull(s.c, hdr); err != nil {
		return 0, nil
	}
	b := make([]byte, int(binary.BigEndian.Uint16(hdr[0:2]))-1)
	if _, err := io.ReadFull(s.c, b); err != nil {
		s.t.Fatalf("short packet: %v", err)
	}
	return hdr[2], b
}

// messages reads sequenced packets for up to wait and returns the ITCH messages.
func (s *soupClient) messages(wait time.Duration) []itchMsg {
	var out []itchMsg
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		ptype, b := s.packet(150 * time.Millisecond)
		if ptype != 'S' {
			continue
		}
		s.seq++
		for len(b) > 0 {
			n, ok := itchLen[b[0]]
			if !ok || len(b) < 1+n {
				s.t.Fatalf("bad ITCH framing at type %q with %d bytes left", b[0], len(b))
			}
			out = append(out, itchMsg{b[0], b[1 : 1+n]})
			b = b[1+n:]
		}
	}
	return out
}

func testFeed(t *testing.T) (*engine.Engine, *marketFeed, net.Addr) {
	t.Helper()
	eng := engine.NewEngine([]engine.Instrument{
		{Symbol: "GP", Name: "Grameenphone", Prev: 100},
		{Symbol: "ACI", Name: "Aci Ltd.", Prev: 250},
	}, engine.NewBus())
	go eng.Run()
	f := &marketFeed{eng: eng, session: "SIMTEST001"}
	addr, err := serveSoup("ITCH-test", "127.0.0.1:0", f.serve)
	if err != nil {
		t.Fatal(err)
	}
	return eng, f, addr
}

func place(t *testing.T, eng *engine.Engine, cl string, sym string, side engine.Side, px float64, qty int64) {
	t.Helper()
	o := eng.NewOrder(sym, side, engine.Limit, px, qty, "FIX:test")
	o.ClOrdID = cl
	eng.SubmitAndWait(o)
}

func kinds(ms []itchMsg) string {
	b := make([]byte, len(ms))
	for i, m := range ms {
		b[i] = m.typ
	}
	return string(b)
}

// The start-of-day set must arrive in the order the itch decoders need.
func TestStartOfDayOrder(t *testing.T) {
	eng, f, addr := testFeed(t)
	place(t, eng, "B0", "GP", engine.Buy, 99.9, 500)
	time.Sleep(50 * time.Millisecond)

	c, session, seq := dial(t, addr, "", 0)
	defer c.c.Close()
	if session != f.session || seq != 1 {
		t.Fatalf("login accept = %q/%d", session, seq)
	}
	ms := c.messages(600 * time.Millisecond)
	if len(ms) == 0 || ms[0].typ != 'T' {
		t.Fatalf("the first message must be the seconds message, got %s", kinds(ms))
	}
	first := map[byte]int{}
	for i, m := range ms {
		if _, seen := first[m.typ]; !seen {
			first[m.typ] = i
		}
	}
	for _, need := range []byte{'S', 'L', 'P', 'R', 'H', 'A'} {
		if _, ok := first[need]; !ok {
			t.Fatalf("no %q in the start-of-day set: %s", need, kinds(ms))
		}
	}
	// per orderbook: P, then R, then H, then the reference-price A (order 0), then real orders
	state := map[uint32]string{}
	var ref, real, index int
	for _, m := range ms {
		switch m.typ {
		case 'P':
			state[m.u32(4)] += "P"
		case 'R':
			ob := m.u32(4)
			state[ob] += "R"
			if m.body[8+1+12+12+3+8+8+4+4+4+4+4] == 'I' {
				index++
			}
		case 'H':
			state[m.u32(4)] += "H"
		case 'A':
			ob := m.u32(4 + 8 + 1 + 8)
			if m.u64(4) == 0 {
				ref++
				state[ob] += "A"
			} else {
				real++
				if state[ob] != "PRHA" {
					t.Errorf("order for ob %d arrived after %q, want PRHA first", ob, state[ob])
				}
			}
		}
	}
	for ob, st := range state {
		if st != "PRHA" {
			t.Errorf("orderbook %d got %q, want PRHA", ob, st)
		}
	}
	if len(state) != 4 { // GP, ACI, DSEX, DS30
		t.Errorf("expected 4 orderbooks incl. 2 indices, got %d", len(state))
	}
	if index != 2 || ref != 4 {
		t.Errorf("index R=%d reference A=%d, want 2 and 4", index, ref)
	}
	if real != 1 {
		t.Errorf("the resting order should be sent once, got %d", real)
	}
}

// Live events carry the engine's order numbers: A on rest, one E per trade on the
// resting order, D on cancel.
func TestLiveEventsUseRealOrderNumbers(t *testing.T) {
	eng, _, addr := testFeed(t)
	c, _, _ := dial(t, addr, "", 0)
	defer c.c.Close()
	c.messages(400 * time.Millisecond) // start of day

	place(t, eng, "B1", "GP", engine.Buy, 100, 100)
	place(t, eng, "S1", "GP", engine.Sell, 100, 30) // trades 30 against B1
	place(t, eng, "B2", "ACI", engine.Buy, 249.9, 10)
	eng.Cancel("B2")
	ms := c.messages(600 * time.Millisecond)

	var adds, execs, dels int
	var restingID uint64
	for _, m := range ms {
		switch m.typ {
		case 'A':
			adds++
			if restingID == 0 {
				restingID = m.u64(4)
			}
		case 'E':
			execs++
			if m.u64(4) != restingID || m.u64(12) != 30 {
				t.Errorf("E on order %d qty %d, want order %d qty 30", m.u64(4), m.u64(12), restingID)
			}
			if m.u64(20) == 0 {
				t.Error("E without a match number")
			}
		case 'D':
			dels++
		}
	}
	if adds != 2 || execs != 1 || dels != 1 {
		t.Fatalf("A/E/D = %d/%d/%d, want 2/1/1 (stream: %s)", adds, execs, dels, kinds(ms))
	}
}

// Logging in again with the session and next sequence continues the stream; no
// second start-of-day set is sent.
func TestResumeContinuesTheStream(t *testing.T) {
	eng, f, addr := testFeed(t)
	c, session, _ := dial(t, addr, "", 0)
	c.messages(400 * time.Millisecond)
	next := c.seq + 1
	c.c.Close()
	time.Sleep(100 * time.Millisecond) // the server stores its resume point on close

	place(t, eng, "B1", "GP", engine.Buy, 100, 100) // while the client is away

	c2, session2, seq2 := dial(t, addr, session, next)
	defer c2.c.Close()
	if session2 != f.session || seq2 != next {
		t.Fatalf("resume accept = %q/%d, want %q/%d", session2, seq2, f.session, next)
	}
	ms := c2.messages(500 * time.Millisecond)
	var adds, dirs int
	for _, m := range ms {
		switch m.typ {
		case 'A':
			adds++
		case 'R', 'P':
			dirs++
		}
	}
	if adds != 1 || dirs != 0 {
		t.Fatalf("resume should replay only the missed order: A=%d directory=%d (%s)", adds, dirs, kinds(ms))
	}
}

func TestHaltSendsTradingState(t *testing.T) {
	eng, _, addr := testFeed(t)
	c, _, _ := dial(t, addr, "", 0)
	defer c.c.Close()
	c.messages(400 * time.Millisecond)

	engine.SetStatus("GP", "halted")
	eng.PublishTradingState("GP")
	var got byte
	for _, m := range c.messages(500 * time.Millisecond) {
		if m.typ == 'H' && m.u32(4) == uint32(eng.OrderBookID("GP")) {
			got = m.body[8]
		}
	}
	engine.SetStatus("GP", "active")
	if got != 'V' {
		t.Fatalf("trading state after a halt = %q, want 'V'", got)
	}
}

func TestIndexFeedSendsValues(t *testing.T) {
	eng, _, _ := testFeed(t)
	f := &indexFeed{eng: eng, session: "IDXTEST001", every: 100 * time.Millisecond}
	addr, err := serveSoup("ITCH-INDEX-test", "127.0.0.1:0", f.serve)
	if err != nil {
		t.Fatal(err)
	}
	c, _, _ := dial(t, addr, "", 0)
	defer c.c.Close()
	got := map[uint32]uint64{}
	for _, m := range c.messages(400 * time.Millisecond) {
		if m.typ == 'Z' {
			got[m.u32(4)] = m.u64(8)
		}
	}
	if got[engine.IndexOBDSEX] != 600000 || got[engine.IndexOBDS30] != 220000 {
		t.Fatalf("index values before any trade = %v, want DSEX 6000.00 and DS30 2200.00", got)
	}
}

func TestNewsFeedAnnouncesSessionsAndHalts(t *testing.T) {
	eng, _, _ := testFeed(t)
	f := &newsFeed{eng: eng, session: "NWSTEST001"}
	addr, err := serveSoup("ITCH-NEWS-test", "127.0.0.1:0", f.serve)
	if err != nil {
		t.Fatal(err)
	}
	eng.SetPhase("Pre-Open") // before the client connects: replayed
	c, _, _ := dial(t, addr, "", 0)
	defer c.c.Close()
	engine.SetStatus("ACI", "halted")
	eng.PublishTradingState("ACI")
	engine.SetStatus("ACI", "active")

	// 'N' is variable length, so parse this feed's packets by hand.
	var titles []string
	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		ptype, b := c.packet(150 * time.Millisecond)
		if ptype != 'S' {
			continue
		}
		for len(b) > 0 {
			if b[0] != 'N' {
				b = b[1+itchLen[b[0]]:]
				continue
			}
			p := b[1+4+4+4+30:]
			end := 0
			for p[end] != 0 {
				end++
			}
			titles = append(titles, string(p[:end]))
			for k := 0; k < 3; k++ { // skip the three strings
				for p[0] != 0 {
					p = p[1:]
				}
				p = p[1:]
			}
			b = p
		}
	}
	want := []string{"Pre-opening session started", "Trading suspended: ACI"}
	if len(titles) != 2 || titles[0] != want[0] || titles[1] != want[1] {
		t.Fatalf("news titles = %q, want %q", titles, want)
	}
}
