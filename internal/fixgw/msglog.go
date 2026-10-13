package fixgw

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/quickfixgo/quickfix"

	"velexa-sim-me/internal/config"
)

// fixmsglog.go — a FIX message log for the Matching Engine: every inbound and
// outbound FIX message is captured (per session) into a queryable ring + an
// append-only file, so any order can be traced end to end. Wired in as the
// acceptor's quickfix LogFactory (replacing NullLog). Heartbeats (35=0) are
// skipped to keep the trace focused on orders/execs/session events.

type fixMsg struct {
	Time    time.Time `json:"time"`
	Session string    `json:"session"` // broker (SessionID TargetCompID)
	Dir     string    `json:"dir"`     // IN | OUT
	MsgType string    `json:"msg_type"`
	ClOrdID string    `json:"clordid"`
	Raw     string    `json:"raw"` // FIX message, SOH shown as |
}

type msgLogStore struct {
	mu   sync.Mutex
	ring []fixMsg
	file *os.File
	max  int
}

var MsgLog = &msgLogStore{max: 4000}

func initMsgLog() {
	path := config.EnvOr("ME_FIX_MSGLOG_FILE", config.StateFile("fix_messages.log"))
	if path == "" {
		return
	}
	// Load recent history so the message log survives a restart (the ring is
	// in-memory; the file is the durable record). Line format:
	//   2006-01-02 15:04:05.000 DIR SESSION RAW   (RAW has no spaces)
	if b, err := os.ReadFile(path); err == nil {
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		if len(lines) > MsgLog.max {
			lines = lines[len(lines)-MsgLog.max:]
		}
		for _, ln := range lines {
			f := strings.Fields(ln)
			if len(f) < 5 {
				continue
			}
			ts, _ := time.Parse("2006-01-02 15:04:05.000", f[0]+" "+f[1])
			raw := f[4]
			cl := fixField(raw, "11")
			if cl == "" {
				cl = fixField(raw, "41")
			}
			MsgLog.ring = append(MsgLog.ring, fixMsg{
				Time: ts, Dir: f[2], Session: f[3], Raw: raw, MsgType: fixField(raw, "35"), ClOrdID: cl})
		}
	}
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640); err == nil {
		MsgLog.file = f
	}
}

// fixField extracts a tag's value from a |-delimited FIX string ("" if absent).
func fixField(raw, tag string) string {
	for _, f := range strings.Split(raw, "|") {
		if strings.HasPrefix(f, tag+"=") {
			return f[len(tag)+1:]
		}
	}
	return ""
}

func (s *msgLogStore) add(session, dir string, data []byte) {
	raw := strings.ReplaceAll(strings.TrimRight(string(data), "\x01"), "\x01", "|")
	mt := fixField(raw, "35")
	if mt == "0" { // skip heartbeats
		return
	}
	cl := fixField(raw, "11")
	if cl == "" {
		cl = fixField(raw, "41") // OrigClOrdID on cancel/replace/reject
	}
	m := fixMsg{Time: time.Now(), Session: session, Dir: dir, MsgType: mt, ClOrdID: cl, Raw: raw}
	s.mu.Lock()
	s.ring = append(s.ring, m)
	if len(s.ring) > s.max {
		s.ring = s.ring[len(s.ring)-s.max:]
	}
	f := s.file
	s.mu.Unlock()
	if f != nil {
		fmt.Fprintf(f, "%s %-3s %-14s %s\n", m.Time.UTC().Format("2006-01-02 15:04:05.000"), dir, session, raw)
	}
}

// Recent returns up to limit newest messages, optionally filtered by broker
// (exact, case-insensitive) and ClOrdID (substring).
func (s *msgLogStore) Recent(broker, clordid string, limit int) []fixMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []fixMsg{}
	for i := len(s.ring) - 1; i >= 0 && len(out) < limit; i-- {
		m := s.ring[i]
		if broker != "" && !strings.EqualFold(m.Session, broker) {
			continue
		}
		if clordid != "" && !strings.Contains(strings.ToUpper(m.ClOrdID), strings.ToUpper(clordid)) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ── quickfix.Log / LogFactory ──────────────────────────────────────────────
type sessionLog struct {
	store   *msgLogStore
	session string
}

func (l *sessionLog) OnIncoming(b []byte) { l.store.add(l.session, "IN", b) }

func (l *sessionLog) OnOutgoing(b []byte) { l.store.add(l.session, "OUT", b) }

func (l *sessionLog) OnEvent(string) {}

func (l *sessionLog) OnEventf(string, ...interface{}) {}

type msgLogFactory struct{ store *msgLogStore }

func (f msgLogFactory) Create() (quickfix.Log, error) {
	return &sessionLog{store: f.store, session: "-"}, nil
}

func (f msgLogFactory) CreateSessionLog(sid quickfix.SessionID) (quickfix.Log, error) {
	return &sessionLog{store: f.store, session: sid.TargetCompID}, nil
}
