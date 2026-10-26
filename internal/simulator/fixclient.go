package simulator

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// fixclient.go — a small FIX initiator: just enough of the session layer
// (logon, heartbeats, test requests, logout) to send orders and cancels to the
// engine's own FIX gateway and read the reports, as a broker's OMS would.

const soh = '\x01'

// fixMsg is a received message by tag (a repeating group keeps its last value).
type fixMsg map[int]string

type field struct {
	tag int
	val string
}

type client struct {
	comp, target, begin string
	conn                net.Conn
	wmu                 sync.Mutex
	seq                 int
	onMsg               func(fixMsg)
	logon               chan fixMsg
	up                  atomic.Bool
	done                chan struct{}
	once                sync.Once
}

// dialFIX connects to addr and logs on as comp. onMsg gets every application
// message (ExecutionReports, cancel rejects, session rejects) on the reader
// goroutine.
func dialFIX(addr, begin, target, comp, pass string, onMsg func(fixMsg)) (*client, error) {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the FIX gateway at %s: %w", addr, err)
	}
	c := &client{comp: comp, target: target, begin: begin, conn: conn, seq: 1, onMsg: onMsg,
		logon: make(chan fixMsg, 1), done: make(chan struct{})}
	go c.read()
	f := []field{{98, "0"}, {108, "30"}, {141, "Y"}, {553, comp}, {554, pass}}
	if begin == "FIXT.1.1" {
		f = append(f, field{1137, "9"})
	}
	if err := c.send("A", f); err != nil {
		c.close()
		return nil, err
	}
	select {
	case m := <-c.logon:
		if m[35] != "A" {
			c.close()
			return nil, fmt.Errorf("%s: logon refused: %s", comp, m[58])
		}
	case <-c.done:
		return nil, fmt.Errorf("%s: the gateway closed the connection during logon", comp)
	case <-time.After(5 * time.Second):
		c.close()
		return nil, fmt.Errorf("%s: no logon answer within 5 s", comp)
	}
	c.up.Store(true)
	go c.heartbeat()
	return c, nil
}

// send writes one message with a fresh sequence number.
func (c *client) send(mt string, fields []field) error {
	var body bytes.Buffer
	c.wmu.Lock()
	defer c.wmu.Unlock()
	fmt.Fprintf(&body, "35=%s\x0149=%s\x0156=%s\x0134=%d\x0152=%s\x01", mt, c.comp, c.target, c.seq,
		time.Now().UTC().Format("20060102-15:04:05.000"))
	for _, f := range fields {
		body.WriteString(strconv.Itoa(f.tag))
		body.WriteByte('=')
		body.WriteString(f.val)
		body.WriteByte(soh)
	}
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "8=%s\x019=%d\x01", c.begin, body.Len())
	msg.Write(body.Bytes())
	sum := 0
	for _, b := range msg.Bytes() {
		sum += int(b)
	}
	fmt.Fprintf(&msg, "10=%03d\x01", sum%256)
	c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.conn.Write(msg.Bytes()); err != nil {
		go c.close()
		return err
	}
	c.seq++
	return nil
}

func (c *client) read() {
	defer c.close()
	buf := make([]byte, 0, 64<<10)
	tmp := make([]byte, 32<<10)
	for {
		n, err := c.conn.Read(tmp)
		buf = append(buf, tmp[:n]...)
		for {
			i := bytes.Index(buf, []byte("\x0110="))
			if i < 0 {
				break
			}
			j := bytes.IndexByte(buf[i+1:], soh)
			if j < 0 {
				break
			}
			end := i + 1 + j + 1
			c.dispatch(parse(buf[:end]))
			buf = buf[end:]
		}
		if err != nil {
			return // closed by either side: the session is over
		}
	}
}

func parse(raw []byte) fixMsg {
	m := fixMsg{}
	for _, f := range bytes.Split(raw, []byte{soh}) {
		k, v, ok := bytes.Cut(f, []byte{'='})
		if !ok {
			continue
		}
		if t, err := strconv.Atoi(string(k)); err == nil {
			m[t] = string(v)
		}
	}
	return m
}

func (c *client) dispatch(m fixMsg) {
	switch m[35] {
	case "A":
		select {
		case c.logon <- m:
		default:
		}
	case "5":
		if !c.up.Load() {
			select {
			case c.logon <- m:
			default:
			}
		}
		go c.close()
	case "1":
		c.send("0", []field{{112, m[112]}})
	case "0", "2", "4":
	default:
		if c.onMsg != nil {
			c.onMsg(m)
		}
	}
}

func (c *client) heartbeat() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			c.send("0", nil)
		}
	}
}

// logout ends the session politely and waits up to a second for the gateway.
func (c *client) logout() {
	if c.up.Load() {
		c.send("5", nil)
		select {
		case <-c.done:
		case <-time.After(time.Second):
		}
	}
	c.close()
}

func (c *client) close() {
	c.once.Do(func() {
		c.up.Store(false)
		c.conn.Close()
		close(c.done)
	})
}

func (c *client) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return c.up.Load()
	}
}

func pxStr(p float64) string { return strconv.FormatFloat(p, 'f', 2, 64) }

func nowUTC() string { return time.Now().UTC().Format("20060102-15:04:05") }

// order sends a limit NewOrderSingle (35=D). side is "1" buy or "2" sell; tif
// is the FIX TimeInForce ("0" day, "3" IOC).
func (c *client) order(cl, sym, side string, qty int64, px float64, tif string) error {
	return c.send("D", []field{{11, cl}, {1, "SIMULATOR"}, {55, sym}, {54, side}, {60, nowUTC()},
		{38, strconv.FormatInt(qty, 10)}, {40, "2"}, {59, tif}, {44, pxStr(px)}})
}

// cancel sends an OrderCancelRequest (35=F) for orig.
func (c *client) cancel(cl, orig, sym, side string, qty int64) error {
	return c.send("F", []field{{11, cl}, {41, orig}, {55, sym}, {54, side}, {60, nowUTC()},
		{38, strconv.FormatInt(qty, 10)}})
}
