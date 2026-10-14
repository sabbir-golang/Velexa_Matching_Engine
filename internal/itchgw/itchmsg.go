package itchgw

// Builders for the DSE-ITCH messages the feeds send. Each returns the type byte
// followed by the fixed payload, all integers big-endian, text space padded. The
// layouts are the ones the OMS itch service decodes (src/ITCH/**/decoders.cpp).
// Every message but 'T' starts with a nanosecond timestamp within the current
// second, which the receiver adds to the last 'T' (seconds since midnight, Dhaka).

import (
	"time"
)

var dhaka = time.FixedZone("Asia/Dhaka", 6*3600)

// secOf is the seconds-since-midnight (Dhaka) of t, as carried by 'T'.
func secOf(t time.Time) uint32 {
	t = t.In(dhaka)
	return uint32(t.Hour()*3600 + t.Minute()*60 + t.Second())
}

func nsOf(t time.Time) uint32 { return uint32(t.In(dhaka).Nanosecond()) }

func mSeconds(sec uint32) []byte { // 'T' payload 4
	x := &ibuf{}
	x.u8('T')
	x.u32(sec)
	return x.b
}

func mSystemEvent(ns uint32, code byte) []byte { // 'S' payload 17
	x := &ibuf{}
	x.u8('S')
	x.u32(ns)
	x.fix("", 8) // group
	x.u8(code)
	x.u32(0) // orderbook
	return x.b
}

func mPriceTick(ns uint32) []byte { // 'L' payload 16
	x := &ibuf{}
	x.u8('L')
	x.u32(ns)
	x.u32(1) // tick_size_table_id
	x.u32(1) // tick_size (raw; /100 = 0.01)
	x.u32(0) // price_start
	return x.b
}

func mQuantityTick(ns uint32) []byte { // 'M' payload 24
	x := &ibuf{}
	x.u8('M')
	x.u32(ns)
	x.u32(1) // tick_size_table_id
	x.u64(1) // tick_size
	x.u64(0) // quantity_start
	return x.b
}

func mCompany(ns, company uint32, name, id string) []byte { // 'P' payload 68
	x := &ibuf{}
	x.u8('P')
	x.u32(ns)
	x.u32(company)
	x.fix(name, 30)
	x.fix(id, 30)
	return x.b
}

// rdir is the content of an orderbook directory ('R') message.
type rdir struct {
	ob       uint32
	isin     string
	sec      string // sec_code: the ticker or index name
	group    string // trading group: PUBLIC, SPUBLIC, ATBPUB, DEBT, INDEX ...
	market   byte   // 'P' public, 'I' index
	listing  byte   // category letter A/B/N/Z/S
	sector   string
	instr    string // instrument type: EQ, MF, CB, DB, IN
	name     string
	minQty   uint64
	delisted bool
}

func mOrderbook(ns uint32, d rdir) []byte { // 'R' payload 170
	x := &ibuf{}
	x.u8('R')
	x.u32(ns)
	x.u32(d.ob)
	x.u8('P') // price_type
	x.fix(d.isin, 12)
	x.fix(d.sec, 12)
	x.fix("BDT", 3)
	x.fix(d.group, 8)
	if d.minQty == 0 {
		d.minQty = 1
	}
	x.u64(d.minQty)
	x.u32(1) // quantity_tick_size_table_id
	x.u32(1) // price_tick_size_table_id
	x.u32(2) // price_decimals
	x.u32(0) // delisting_date
	x.u32(0) // delisting_time
	x.u8(d.market)
	x.u32(d.ob) // company (matches 'P')
	x.u8(d.listing)
	x.fix(d.sector, 12)
	x.fix(d.instr, 12)
	x.fix(d.name, 60)
	x.u32(0) // maturity_date
	x.u32(0) // yield_decimals
	return x.b
}

func mTradingState(ns, ob uint32, state byte) []byte { // 'H' payload 10
	x := &ibuf{}
	x.u8('H')
	x.u32(ns)
	x.u32(ob)
	x.u8(state) // 'T' tradable, 'V' suspended
	x.u8(' ')   // reason
	return x.b
}

func mAdd(ns uint32, order uint64, verb byte, qty uint64, ob, price uint32) []byte { // 'A' payload 33
	x := &ibuf{}
	x.u8('A')
	x.u32(ns)
	x.u64(order)
	x.u8(verb)
	x.u64(qty)
	x.u32(ob)
	x.u32(price)
	x.u32(0) // yield
	return x.b
}

// mReference is the reference (previous close) price: an 'A' with order 0 and quantity 0.
func mReference(ns, ob, price uint32) []byte { return mAdd(ns, 0, 'B', 0, ob, price) }

func mExec(ns uint32, order, qty, match uint64) []byte { // 'E' payload 28
	x := &ibuf{}
	x.u8('E')
	x.u32(ns)
	x.u64(order)
	x.u64(qty)
	x.u64(match)
	return x.b
}

// mExecPrice is 'C' (execution with price). The receiver ignores repeated match
// numbers, so both sides of an auction trade can send it safely.
func mExecPrice(ns uint32, order, qty, match uint64, ob, price uint32) []byte { // 'C' payload 37
	x := &ibuf{}
	x.u8('C')
	x.u32(ns)
	x.u64(order)
	x.u64(qty)
	x.u64(match)
	x.u8('Y') // printable
	x.u32(price)
	x.u32(ob)
	return x.b
}

func mDelete(ns uint32, order uint64) []byte { // 'D' payload 12
	x := &ibuf{}
	x.u8('D')
	x.u32(ns)
	x.u64(order)
	return x.b
}

// mClose is 'Q', the closing trade message the receiver stores as the close price.
func mClose(ns uint32, qty uint64, ob, price uint32, match uint64) []byte { // 'Q' payload 33
	x := &ibuf{}
	x.u8('Q')
	x.u32(ns)
	x.u64(qty)
	x.u32(ob)
	x.u8('Y')
	x.u32(price)
	x.u32(0) // yield
	x.u64(match)
	return x.b
}

func mIndexValue(ns, ob uint32, raw uint64) []byte { // 'Z' payload 16 (index feed)
	x := &ibuf{}
	x.u8('Z')
	x.u32(ns)
	x.u32(ob)
	x.u64(raw)
	return x.b
}
