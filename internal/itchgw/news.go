package itchgw

// news.go — the DSE-ITCH news feed (SoupBinTCP, default :5503).
//
// It publishes exchange announcements as 'N' messages: the market moving between
// sessions, an instrument being suspended or resumed, and new listings. They
// come from the engine's market-event journal, so a client that connects later
// still receives the day's announcements first.

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"velexa-sim-me/internal/engine"
)

type newsFeed struct {
	eng     *engine.Engine
	session string
}

// NewNewsFeed returns the news feed; ME_ITCH_NEWS_PORT=off disables it.
func NewNewsFeed(eng *engine.Engine) Runner {
	return &newsFeed{eng: eng, session: fmt.Sprintf("NWS%07d", time.Now().Unix()%10000000)}
}

func (f *newsFeed) Run() {
	port := strings.TrimSpace(os.Getenv("ME_ITCH_NEWS_PORT"))
	if port == "" {
		port = "5503"
	}
	if strings.EqualFold(port, "off") {
		return
	}
	if _, err := serveSoup("ITCH-NEWS", ":"+port, f.serve); err != nil {
		log.Printf("[ITCH-NEWS] %v", err)
		return
	}
	log.Printf("[ITCH-NEWS] SoupBinTCP news feed listening on :%s", port)
	select {}
}

// mNews is 'N': ts4, ob4, news_id4, firm_id30, then title, reference and text,
// each NUL terminated (at most 80, 255 and 511 bytes of text).
func mNews(ns, ob, id uint32, firm, title, ref, text string) []byte {
	x := &ibuf{}
	x.u8('N')
	x.u32(ns)
	x.u32(ob)
	x.u32(id)
	x.fix(firm, 30)
	cstr := func(s string, max int) {
		if len(s) > max {
			s = s[:max]
		}
		x.b = append(x.b, s...)
		x.b = append(x.b, 0)
	}
	cstr(title, 80)
	cstr(ref, 255)
	cstr(text, 511)
	return x.b
}

var sessionNews = map[byte][2]string{
	'S': {"Pre-opening session started", "Orders are being accepted for the opening call. No trades take place until the market opens."},
	'Q': {"Continuous trading started", "The market is open for continuous trading."},
	'M': {"Market closed", "Continuous trading has ended for the day."},
	'V': {"Post-closing session started", "Orders are accepted at the closing price only."},
	'E': {"End of trading day", "The trading day has ended. Unexecuted day orders have expired."},
}

// item turns a journal event into an announcement, or nil when it is not news.
func (f *newsFeed) item(ev engine.Event) []string {
	switch ev.Kind {
	case engine.EvSession:
		if n, ok := sessionNews[ev.Code]; ok {
			return []string{"DSE", n[0], "DSE/SESSION", n[1]}
		}
	case engine.EvState:
		sym := f.eng.SymbolForOB(int(ev.OB))
		if sym == "" {
			return nil
		}
		if ev.Code == 'V' {
			return []string{sym, "Trading suspended: " + sym, "DSE/HALT/" + sym,
				"Trading in " + sym + " has been suspended until further notice."}
		}
		return []string{sym, "Trading resumed: " + sym, "DSE/RESUME/" + sym, "Trading in " + sym + " has resumed."}
	case engine.EvList:
		sym := f.eng.SymbolForOB(int(ev.OB))
		if sym == "" {
			return nil
		}
		return []string{sym, "New listing: " + sym, "DSE/LISTING/" + sym, sym + " is listed and open for trading."}
	}
	return nil
}

func (f *newsFeed) serve(s *soupConn, req loginReq) {
	j := f.eng.Journal()
	cursor := j.Oldest()
	seq := uint64(1)
	if req.session == f.session && req.seq > 1 {
		seq, cursor = req.seq, j.Next() // returning client: only new announcements
	}
	s.seq = seq
	if s.writePkt('A', loginAccept(f.session, seq)) != nil {
		return
	}
	var lastSec uint32
	p := &packer{s: s}
	ns, sec := stamp(time.Now(), &lastSec)
	if p.add(sec, mSystemEvent(ns, 'O')) != nil || p.flush() != nil {
		return
	}
	idle := time.NewTicker(time.Second)
	defer idle.Stop()
	for !s.closed() {
		wake := j.Wait()
		evs, next, ok := j.Read(cursor, 1000)
		if !ok {
			cursor = j.Oldest()
			continue
		}
		sent := false
		for _, ev := range evs {
			n := f.item(ev)
			if n == nil {
				continue
			}
			ns, sec := stamp(time.Unix(0, ev.At), &lastSec)
			if sec != nil {
				p.add(sec)
			}
			// one announcement per packet: the receiver drops an 'N' cut by a packet edge
			if p.add(mNews(ns, uint32(ev.OB), uint32(ev.Seq), n[0], n[1], n[2], n[3])) != nil || p.flush() != nil {
				return
			}
			sent = true
		}
		cursor = next
		if len(evs) > 0 || sent {
			continue
		}
		select {
		case <-wake:
		case <-idle.C:
			lastSec = secOf(time.Now())
			if p.add(mSeconds(lastSec)) != nil || p.flush() != nil {
				return
			}
		case <-s.stop:
			return
		}
	}
}
