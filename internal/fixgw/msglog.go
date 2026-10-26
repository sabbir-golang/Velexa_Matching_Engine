package fixgw

import (
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quickfixgo/quickfix"

	"velexa-sim-me/internal/config"
	"velexa-sim-me/internal/database/brokers"
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
	max  int

	// The durable copy: one line per message, rotated by size.
	file     *os.File
	path     string
	size     int64
	maxBytes int64 // rotate when the file would grow past this (ME_FIX_MSGLOG_MAX_MB)
	keep     int   // rotated files kept: path.1 … path.keep (ME_FIX_MSGLOG_KEEP)
}

var MsgLog = &msgLogStore{max: 4000}

const (
	tailBytes     = 4 << 20  // read at most this much of the file at start-up
	scrubMaxBytes = 64 << 20 // rewrite (mask passwords in) older files only up to this size
)

func initMsgLog() {
	path := config.EnvOr("ME_FIX_MSGLOG_FILE", config.StateFile("fix_messages.log"))
	if path == "" {
		return
	}
	mb, _ := strconv.Atoi(config.EnvOr("ME_FIX_MSGLOG_MAX_MB", "256"))
	keep, err := strconv.Atoi(config.EnvOr("ME_FIX_MSGLOG_KEEP", "5"))
	if mb <= 0 {
		mb = 256
	}
	if err != nil || keep < 0 {
		keep = 5
	}
	MsgLog.path, MsgLog.maxBytes, MsgLog.keep = path, int64(mb)<<20, keep

	// Older builds wrote logon passwords in clear: scrub a file of reasonable size once.
	if st, err := os.Stat(path); err == nil && st.Size() <= scrubMaxBytes {
		if b, err := os.ReadFile(path); err == nil {
			if masked := maskSecrets(string(b)); masked != string(b) {
				tmp := path + ".tmp"
				if os.WriteFile(tmp, []byte(masked), 0o600) == nil {
					_ = os.Rename(tmp, path)
				}
			}
		}
	}
	// Load the newest messages so the message log survives a restart (the ring
	// is in memory; the file is the durable record). Only the file's tail is
	// read, so a large log does not slow the start or fill memory. Line format:
	//   2006-01-02 15:04:05.000 DIR SESSION RAW   (RAW has no spaces)
	lines := tailLines(path, tailBytes)
	if len(lines) > MsgLog.max {
		lines = lines[len(lines)-MsgLog.max:]
	}
	for _, ln := range lines {
		f := strings.Fields(ln)
		if len(f) < 5 {
			continue
		}
		ts, _ := time.Parse("2006-01-02 15:04:05.000", f[0]+" "+f[1])
		raw := maskSecrets(f[4])
		cl := fixField(raw, "11")
		if cl == "" {
			cl = fixField(raw, "41")
		}
		MsgLog.ring = append(MsgLog.ring, fixMsg{
			Time: ts, Dir: f[2], Session: f[3], Raw: raw, MsgType: fixField(raw, "35"), ClOrdID: cl})
	}
	MsgLog.mu.Lock()
	defer MsgLog.mu.Unlock()
	MsgLog.openLocked()
	if MsgLog.size > MsgLog.maxBytes {
		MsgLog.rotateLocked()
	}
}

// tailLines returns the complete lines in the last n bytes of a file.
func tailLines(path string, n int64) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	off := st.Size() - n
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return nil
	}
	text := string(buf)
	if off > 0 { // the first line is cut: drop it
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// openLocked opens the log file for appending. Caller holds s.mu.
func (s *msgLogStore) openLocked() {
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("[FIX] message log %s: %v (messages kept in memory only)", s.path, err)
		s.file = nil
		return
	}
	_ = f.Chmod(0o600)
	s.file, s.size = f, 0
	if st, err := f.Stat(); err == nil {
		s.size = st.Size()
	}
}

// rotateLocked moves the log to path.1 (path.1 to path.2, … keeping s.keep
// files) and starts a new one. Caller holds s.mu.
func (s *msgLogStore) rotateLocked() {
	if s.file != nil {
		s.file.Close()
	}
	if s.keep == 0 {
		_ = os.Remove(s.path)
	} else {
		_ = os.Remove(fmt.Sprintf("%s.%d", s.path, s.keep))
		for i := s.keep - 1; i >= 1; i-- {
			_ = os.Rename(fmt.Sprintf("%s.%d", s.path, i), fmt.Sprintf("%s.%d", s.path, i+1))
		}
		_ = os.Rename(s.path, s.path+".1")
	}
	s.openLocked()
}

// secretTagRE finds the logon password tags (554 Password, 925 NewPassword) in
// a |-delimited FIX message.
var secretTagRE = regexp.MustCompile(`(^|\|)(554|925)=[^|\s]*`)

// maskSecrets replaces logon passwords with *** so they are never stored.
func maskSecrets(raw string) string {
	return secretTagRE.ReplaceAllString(raw, "${1}${2}=***")
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
	raw := maskSecrets(strings.ReplaceAll(strings.TrimRight(string(data), "\x01"), "\x01", "|"))
	mt := fixField(raw, "35")
	if mt == "0" { // skip heartbeats
		return
	}
	cl := fixField(raw, "11")
	if cl == "" {
		cl = fixField(raw, "41") // OrigClOrdID on cancel/replace/reject
	}
	m := fixMsg{Time: time.Now(), Session: session, Dir: dir, MsgType: mt, ClOrdID: cl, Raw: raw}
	line := fmt.Sprintf("%s %-3s %-14s %s\n", m.Time.UTC().Format("2006-01-02 15:04:05.000"), dir, session, raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ring = append(s.ring, m)
	if len(s.ring) > s.max {
		s.ring = s.ring[len(s.ring)-s.max:]
	}
	if s.file == nil {
		return
	}
	if s.maxBytes > 0 && s.size+int64(len(line)) > s.maxBytes {
		s.rotateLocked()
		if s.file == nil {
			return
		}
	}
	n, _ := s.file.WriteString(line)
	s.size += int64(n)
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

// RecentWhere is Recent restricted to the sessions keep accepts.
func (s *msgLogStore) RecentWhere(keep func(session string) bool, clordid string, limit int) []fixMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []fixMsg{}
	for i := len(s.ring) - 1; i >= 0 && len(out) < limit; i-- {
		m := s.ring[i]
		if !keep(m.Session) {
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
	if brokers.IsSimBench(sid.TargetCompID) {
		return nullLog{}, nil // benchmark floods would push real brokers' messages out of the log
	}
	return &sessionLog{store: f.store, session: sid.TargetCompID}, nil
}

// nullLog records nothing.
type nullLog struct{}

func (nullLog) OnIncoming([]byte)               {}
func (nullLog) OnOutgoing([]byte)               {}
func (nullLog) OnEvent(string)                  {}
func (nullLog) OnEventf(string, ...interface{}) {}
