package itchgw

// index.go — the DSE-ITCH index feed (SoupBinTCP, default :5502).
//
// The OMS itch service takes index definitions (market type 'I' orderbooks) and
// their previous values from the market feed; the index feed only carries the
// current values, as 'Z' messages. This feed sends every index once a second.
// The up / down / unchanged / trades / volume / value figures shown next to an
// index are computed by the receiver from the market feed's executions.

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"velexa-sim-me/internal/engine"
)

type indexFeed struct {
	eng     *engine.Engine
	session string
	every   time.Duration
}

// NewIndexFeed returns the index feed; ME_ITCH_INDEX_PORT=off disables it.
func NewIndexFeed(eng *engine.Engine) Runner {
	return &indexFeed{eng: eng, session: fmt.Sprintf("IDX%07d", time.Now().Unix()%10000000), every: time.Second}
}

func (f *indexFeed) Run() {
	port := strings.TrimSpace(os.Getenv("ME_ITCH_INDEX_PORT"))
	if port == "" {
		port = "5502"
	}
	if strings.EqualFold(port, "off") {
		return
	}
	if _, err := serveSoup("ITCH-INDEX", ":"+port, f.serve); err != nil {
		log.Printf("[ITCH-INDEX] %v", err)
		return
	}
	log.Printf("[ITCH-INDEX] SoupBinTCP index feed listening on :%s", port)
	select {}
}

func (f *indexFeed) serve(s *soupConn, req loginReq) {
	// Index values are a fresh snapshot every second, so a returning client simply
	// continues at the sequence it asks for; nothing needs replaying.
	seq := uint64(1)
	if req.session == f.session && req.seq > 1 {
		seq = req.seq
	}
	s.seq = seq
	if s.writePkt('A', loginAccept(f.session, seq)) != nil {
		return
	}
	var lastSec uint32
	now := time.Now()
	ns, sec := stamp(now, &lastSec)
	p := &packer{s: s}
	if p.add(sec, mSystemEvent(ns, 'O')) != nil || f.sendValues(p, &lastSec) != nil {
		return
	}
	t := time.NewTicker(f.every)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			if f.sendValues(p, &lastSec) != nil {
				return
			}
		}
	}
}

func (f *indexFeed) sendValues(p *packer, lastSec *uint32) error {
	ns, sec := stamp(time.Now(), lastSec)
	if sec != nil {
		p.add(sec)
	}
	for _, q := range f.eng.Indices() {
		p.add(mIndexValue(ns, uint32(q.OB), uint64(q.Value*100+0.5)))
	}
	return p.flush()
}
