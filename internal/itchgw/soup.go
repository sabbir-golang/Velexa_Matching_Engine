package itchgw

// SoupBinTCP server used by the market, index and news feeds. Frames are
// [2-byte big-endian length][1-byte type][body]; the length counts the type byte.
//   server -> client: 'A' LoginAccept, 'J' LoginReject, 'S' SequencedData, 'H' heartbeat, 'Z' end of session
//   client -> server: 'L' LoginRequest, 'R' client heartbeat, 'O' logout

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/database/brokers"
)

// loginReq is a parsed LoginRequest: user[6] password[10] session[10] sequence[20].
type loginReq struct {
	user, pass, session string
	seq                 uint64 // 0 when blank
}

// soupConn is one client connection after login.
type soupConn struct {
	c         net.Conn
	name      string
	wmu       sync.Mutex
	stop      chan struct{}
	closeOnce sync.Once
	seq       uint64 // sequence number of the next SequencedData packet
}

func newSoupConn(c net.Conn, name string) *soupConn {
	return &soupConn{c: c, name: name, stop: make(chan struct{}), seq: 1}
}

func (s *soupConn) close() {
	s.closeOnce.Do(func() {
		close(s.stop)
		s.c.Close()
	})
}

func (s *soupConn) closed() bool {
	select {
	case <-s.stop:
		return true
	default:
		return false
	}
}

func writeSoup(c net.Conn, ptype byte, body []byte) error {
	buf := make([]byte, 3+len(body))
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(body)+1))
	buf[2] = ptype
	copy(buf[3:], body)
	_, err := c.Write(buf)
	return err
}

func (s *soupConn) writePkt(ptype byte, body []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return writeSoup(s.c, ptype, body)
}

// sendSeq sends one SequencedData packet holding one or more ITCH messages.
func (s *soupConn) sendSeq(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := writeSoup(s.c, 'S', payload); err != nil {
		return err
	}
	s.seq++
	return nil
}

func (s *soupConn) heartbeatLoop() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			if err := s.writePkt('H', nil); err != nil {
				s.close()
				return
			}
		}
	}
}

// readerLoop drains client heartbeats and watches for a logout.
func (s *soupConn) readerLoop() {
	hdr := make([]byte, 3)
	for {
		if _, err := io.ReadFull(s.c, hdr); err != nil {
			s.close()
			return
		}
		length := int(binary.BigEndian.Uint16(hdr[0:2]))
		if length > 1 {
			if _, err := io.ReadFull(s.c, make([]byte, length-1)); err != nil {
				s.close()
				return
			}
		}
		if hdr[2] == 'O' {
			log.Printf("[%s] %s: client logout", s.name, s.c.RemoteAddr())
			s.close()
			return
		}
	}
}

// packer batches ITCH messages into SequencedData packets of at most ~8 KB.
type packer struct {
	s   *soupConn
	buf []byte
}

func (p *packer) add(msgs ...[]byte) error {
	for _, m := range msgs {
		if len(p.buf)+len(m) > 8000 {
			if err := p.flush(); err != nil {
				return err
			}
		}
		p.buf = append(p.buf, m...)
	}
	return nil
}

func (p *packer) flush() error {
	if len(p.buf) == 0 {
		return nil
	}
	err := p.s.sendSeq(p.buf)
	p.buf = p.buf[:0]
	return err
}

// readLogin reads the first packet. probe is true for a connection that closed
// without sending anything (a port health check, not worth a log line).
func readLogin(c net.Conn) (req loginReq, ok, probe bool) {
	hdr := make([]byte, 3)
	if n, err := io.ReadFull(c, hdr); err != nil {
		return req, false, n == 0 && err == io.EOF
	}
	length := int(binary.BigEndian.Uint16(hdr[0:2]))
	var body []byte
	if length > 1 {
		body = make([]byte, length-1)
		if _, err := io.ReadFull(c, body); err != nil {
			return req, false, false
		}
	}
	field := func(from, to int) string {
		if len(body) < to {
			if len(body) <= from {
				return ""
			}
			to = len(body)
		}
		return strings.TrimSpace(string(body[from:to]))
	}
	req.user, req.pass, req.session = field(0, 6), field(6, 16), field(16, 26)
	req.seq, _ = strconv.ParseUint(field(26, 46), 10, 64)
	if hdr[2] != 'L' {
		log.Printf("[ITCH] %s: accepting non-'L' login packet type=%q len=%d", c.RemoteAddr(), hdr[2], length)
	}
	return req, true, false
}

// loginAccept is session[10] + sequence[20], space padded, the sequence right aligned.
func loginAccept(session string, seq uint64) []byte {
	body := make([]byte, 30)
	for i := range body {
		body[i] = ' '
	}
	copy(body[0:10], session)
	n := strconv.FormatUint(seq, 10)
	copy(body[30-len(n):30], n)
	return body
}

// authorize applies the ITCH credential allowlist when it is switched on.
func authorize(req loginReq) string {
	if brokers.Itch.Enabled {
		return brokers.Itch.Authorize(req.user, req.pass)
	}
	return ""
}

// serveSoup listens on addr and runs handle for every logged-in client.
// handle gets the connection and the login request; it must send LoginAccept.
func serveSoup(name, addr string, handle func(s *soupConn, req loginReq)) (net.Addr, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				continue
			}
			go func() {
				req, ok, probe := readLogin(c)
				if !ok {
					if !probe {
						log.Printf("[%s] %s: login read failed", name, c.RemoteAddr())
					}
					c.Close()
					return
				}
				if reason := authorize(req); reason != "" {
					log.Printf("[%s] %s: login REJECTED: %s", name, c.RemoteAddr(), reason)
					writeSoup(c, 'J', []byte{'A'}) // not authorised
					c.Close()
					return
				}
				s := newSoupConn(c, name)
				go s.heartbeatLoop()
				go s.readerLoop()
				handle(s, req)
				s.close()
			}()
		}
	}()
	return ln.Addr(), nil
}
