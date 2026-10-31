package fixgw

import (
	"log"
	"strconv"
	"time"

	"github.com/quickfixgo/quickfix"

	"velexa-sim-me/internal/config"
)

// store.go — the FIX sessions' message store. quickfix keeps every message it
// sends so it can answer a ResendRequest (35=2). Its in-memory store keeps all
// of them for as long as the broker stays logged on, so memory grows with every
// execution report of the day. This store keeps only the newest resendKeep()
// per session; a resend request for anything older is answered with a
// SequenceReset-GapFill (35=4, 123=Y) for the missing range, which quickfix
// already does for any message the store does not have. Sessions reset their
// sequence numbers at every logon, so resend requests only cover gaps inside a
// live connection, normally a few recent messages.

// defaultResendKeep is how many sent messages a session keeps by default.
const defaultResendKeep = 20000

// resendKeep reads ME_FIX_RESEND_KEEP: how many sent messages each session
// keeps; 0 keeps every message (quickfix's own in-memory store).
func resendKeep() int {
	v := config.EnvOr("ME_FIX_RESEND_KEEP", strconv.Itoa(defaultResendKeep))
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		log.Printf("[FIX] ME_FIX_RESEND_KEEP=%q is not a number of messages: keeping %d", v, defaultResendKeep)
		return defaultResendKeep
	}
	return n
}

// newStoreFactory returns the message store for the acceptor's sessions.
func newStoreFactory() quickfix.MessageStoreFactory {
	if keep := resendKeep(); keep > 0 {
		return boundedStoreFactory{keep: keep}
	}
	return quickfix.NewMemoryStoreFactory()
}

type boundedStoreFactory struct{ keep int }

func (f boundedStoreFactory) Create(quickfix.SessionID) (quickfix.MessageStore, error) {
	s := &boundedStore{keep: f.keep}
	return s, s.Reset()
}

// boundedStore is quickfix's in-memory store with a limit on the messages it
// keeps. Like that store it is used only by its session's goroutine.
type boundedStore struct {
	sender, target int
	created        time.Time
	keep           int
	msgs           map[int][]byte
	oldest         int // no message below this sequence number is kept
}

func (s *boundedStore) NextSenderMsgSeqNum() int { return s.sender + 1 }
func (s *boundedStore) NextTargetMsgSeqNum() int { return s.target + 1 }
func (s *boundedStore) IncrNextSenderMsgSeqNum() error {
	s.sender++
	return nil
}
func (s *boundedStore) IncrNextTargetMsgSeqNum() error {
	s.target++
	return nil
}
func (s *boundedStore) SetNextSenderMsgSeqNum(next int) error {
	s.sender = next - 1
	return nil
}
func (s *boundedStore) SetNextTargetMsgSeqNum(next int) error {
	s.target = next - 1
	return nil
}
func (s *boundedStore) CreationTime() time.Time     { return s.created }
func (s *boundedStore) SetCreationTime(t time.Time) { s.created = t }
func (s *boundedStore) Refresh() error              { return nil }
func (s *boundedStore) Close() error                { return nil }

func (s *boundedStore) Reset() error {
	s.sender, s.target, s.created = 0, 0, time.Now()
	s.msgs, s.oldest = map[int][]byte{}, 0
	return nil
}

// SaveMessage keeps msg and forgets the messages more than keep below it.
func (s *boundedStore) SaveMessage(seq int, msg []byte) error {
	s.msgs[seq] = msg
	cut := seq - s.keep // forget everything up to here
	if cut < s.oldest {
		return nil
	}
	if cut-s.oldest > 2*s.keep { // a jump in sequence numbers: scan instead of counting up
		for k := range s.msgs {
			if k <= cut {
				delete(s.msgs, k)
			}
		}
	} else {
		for k := s.oldest; k <= cut; k++ {
			delete(s.msgs, k)
		}
	}
	s.oldest = cut + 1
	return nil
}

func (s *boundedStore) SaveMessageAndIncrNextSenderMsgSeqNum(seq int, msg []byte) error {
	if err := s.SaveMessage(seq, msg); err != nil {
		return err
	}
	return s.IncrNextSenderMsgSeqNum()
}

func (s *boundedStore) IterateMessages(begin, end int, cb func([]byte) error) error {
	for seq := max(begin, s.oldest); seq <= end; seq++ {
		if m, ok := s.msgs[seq]; ok {
			if err := cb(m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *boundedStore) GetMessages(begin, end int) ([][]byte, error) {
	var out [][]byte
	err := s.IterateMessages(begin, end, func(m []byte) error {
		out = append(out, m)
		return nil
	})
	return out, err
}
