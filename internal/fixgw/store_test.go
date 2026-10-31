package fixgw

import (
	"strconv"
	"testing"

	"github.com/quickfixgo/quickfix"
)

func TestBoundedStoreKeepsTheNewestMessages(t *testing.T) {
	st, _ := boundedStoreFactory{keep: 100}.Create(quickfix.SessionID{BeginString: "FIXT.1.1", SenderCompID: "DSEME", TargetCompID: "BRK"})
	for seq := 1; seq <= 250; seq++ {
		if err := st.SaveMessageAndIncrNextSenderMsgSeqNum(seq, []byte(strconv.Itoa(seq))); err != nil {
			t.Fatal(err)
		}
	}
	if n := st.NextSenderMsgSeqNum(); n != 251 {
		t.Fatalf("next sender seq %d", n)
	}
	b := st.(*boundedStore)
	if len(b.msgs) != 100 {
		t.Fatalf("kept %d messages, want 100", len(b.msgs))
	}
	if old, _ := st.GetMessages(1, 150); len(old) != 0 {
		t.Fatalf("messages 1-150 should be gone, got %d", len(old))
	}
	got, _ := st.GetMessages(140, 155)
	if len(got) != 5 || string(got[0]) != "151" || string(got[4]) != "155" {
		t.Fatalf("140-155: %q", got)
	}
	// A jump in sequence numbers (SequenceReset) drops everything too old.
	st.SetNextSenderMsgSeqNum(100000)
	st.SaveMessageAndIncrNextSenderMsgSeqNum(100000, []byte("x"))
	if len(b.msgs) != 1 {
		t.Fatalf("after a jump: %d messages kept", len(b.msgs))
	}
	st.Reset()
	if st.NextSenderMsgSeqNum() != 1 || st.NextTargetMsgSeqNum() != 1 || len(b.msgs) != 0 {
		t.Fatal("reset")
	}
}

func TestResendKeepSetting(t *testing.T) {
	for v, want := range map[string]int{"": defaultResendKeep, "500": 500, "0": 0, "lots": defaultResendKeep, "-3": defaultResendKeep} {
		t.Setenv("ME_FIX_RESEND_KEEP", v)
		if got := resendKeep(); got != want {
			t.Errorf("ME_FIX_RESEND_KEEP=%q: %d, want %d", v, got, want)
		}
	}
}
