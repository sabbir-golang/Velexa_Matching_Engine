// Package latency holds the lock-free latency recorder shared by the FIX acceptor and the load tests.
package latency

import (
	"sort"
	"sync/atomic"
)

// ── latency stat: atomic counters + a lock-free ring of ns samples ──
type Stat struct {
	count   int64
	sumNs   int64
	idx     int64
	samples [8192]int64
}

func (s *Stat) Rec(ns int64) {
	atomic.AddInt64(&s.count, 1)
	atomic.AddInt64(&s.sumNs, ns)
	i := atomic.AddInt64(&s.idx, 1)
	s.samples[(i-1)&8191] = ns
}

// RecBatch records an amortized per-row cost for a batch of n rows taking durNs.
func (s *Stat) RecBatch(n, durNs int64) {
	if n <= 0 {
		return
	}
	atomic.AddInt64(&s.count, n)
	atomic.AddInt64(&s.sumNs, durNs)
	per := durNs / n
	i := atomic.AddInt64(&s.idx, 1)
	s.samples[(i-1)&8191] = per
}

func (s *Stat) Reset() {
	atomic.StoreInt64(&s.count, 0)
	atomic.StoreInt64(&s.sumNs, 0)
	atomic.StoreInt64(&s.idx, 0)
}

func (s *Stat) Snap() (count, avgNs, p50, p95, p99 int64) {
	count = atomic.LoadInt64(&s.count)
	sum := atomic.LoadInt64(&s.sumNs)
	if count > 0 {
		avgNs = sum / count
	}
	n := int(atomic.LoadInt64(&s.idx))
	if n > 8192 {
		n = 8192
	}
	if n == 0 {
		return
	}
	buf := make([]int64, n)
	copy(buf, s.samples[:n])
	sort.Slice(buf, func(i, j int) bool { return buf[i] < buf[j] })
	pct := func(p int) int64 {
		k := (p * len(buf)) / 100
		if k >= len(buf) {
			k = len(buf) - 1
		}
		return buf[k]
	}
	return count, avgNs, pct(50), pct(95), pct(99)
}
