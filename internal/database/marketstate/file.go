package marketstate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"velexa-sim-me/internal/engine"
)

// FileStore keeps the states as <dir>/<name>.json and the trade log as one
// JSON-lines file per day in <dir>/trades.
type FileStore struct {
	mu       sync.Mutex
	dir      string
	tradeDir string
	seen     map[uint64]bool // trade numbers already in today's file (retries are skipped)
	seenDay  string
}

// NewFileStore keeps the state files in dir and the trade log in tradeDir.
func NewFileStore(dir, tradeDir string) *FileStore {
	return &FileStore{dir: dir, tradeDir: tradeDir}
}

func (f *FileStore) path(name string) string {
	if name == Market {
		return filepath.Join(f.dir, "engine_state.json")
	}
	return filepath.Join(f.dir, name+".json")
}

// SaveState writes the state to a temporary file, flushes it to disk and
// renames it over the old one, so a crash leaves either the old or the new.
func (f *FileStore) SaveState(name string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(f.dir, 0o750); err != nil {
		return err
	}
	path := f.path(name)
	tmp := path + ".tmp"
	w, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		w.Close()
		return err
	}
	if err := w.Sync(); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (f *FileStore) LoadState(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, err := os.ReadFile(f.path(name))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}

func (f *FileStore) SetAside(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	to := f.path(name) + ".bad-" + time.Now().Format("20060102-150405")
	return to, os.Rename(f.path(name), to)
}

func (f *FileStore) dayFile(day string) string {
	return filepath.Join(f.tradeDir, "trades-"+day+".jsonl")
}

// AppendTrades appends to the day's file and flushes it to disk.
func (f *FileStore) AppendTrades(trades []engine.TradeRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(f.tradeDir, 0o750); err != nil {
		return err
	}
	byDay := map[string][]engine.TradeRecord{}
	var days []string
	for _, t := range trades {
		d := engine.ReportDay(t.Time)
		if _, ok := byDay[d]; !ok {
			days = append(days, d)
		}
		byDay[d] = append(byDay[d], t)
	}
	for _, day := range days {
		if err := f.loadSeen(day); err != nil {
			return err
		}
		var buf bytes.Buffer
		var added []uint64
		for _, t := range byDay[day] {
			if f.seen[t.ID] {
				continue
			}
			b, err := json.Marshal(t)
			if err != nil {
				return err
			}
			buf.Write(b)
			buf.WriteByte('\n')
			added = append(added, t.ID)
		}
		if buf.Len() == 0 {
			continue
		}
		w, err := os.OpenFile(f.dayFile(day), os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		// A crash mid-write can leave a torn last line: end it, so the new
		// lines are not glued to it.
		out := buf.Bytes()
		if st, serr := w.Stat(); serr == nil && st.Size() > 0 {
			last := make([]byte, 1)
			if _, rerr := w.ReadAt(last, st.Size()-1); rerr == nil && last[0] != '\n' {
				out = append([]byte{'\n'}, out...)
			}
		}
		_, err = w.Write(out)
		if err == nil {
			err = w.Sync()
		}
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			f.seenDay = "" // part of it may be written: read the file again next time
			return err
		}
		for _, id := range added {
			f.seen[id] = true
		}
	}
	return nil
}

// loadSeen reads which trade numbers a day's file already has (kept for the
// latest day written to, so appending stays cheap).
func (f *FileStore) loadSeen(day string) error {
	if f.seenDay == day && f.seen != nil {
		return nil
	}
	recs, err := readTrades(f.dayFile(day))
	if err != nil {
		return err
	}
	f.seen = make(map[uint64]bool, len(recs))
	for _, t := range recs {
		f.seen[t.ID] = true
	}
	f.seenDay = day
	return nil
}

// readTrades reads a day's file, skipping a torn last line (a crash while
// writing) and duplicates.
func readTrades(path string) ([]engine.TradeRecord, error) {
	r, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var out []engine.TradeRecord
	seen := map[uint64]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var t engine.TradeRecord
		if json.Unmarshal(line, &t) != nil || seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		out = append(out, t)
	}
	return out, sc.Err()
}

func (f *FileStore) Trades(day string) ([]engine.TradeRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	recs, err := readTrades(f.dayFile(day))
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	return recs, err
}

// MaxTradeID reads the newest day's file: trade numbers only grow.
func (f *FileStore) MaxTradeID() (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	names, err := filepath.Glob(filepath.Join(f.tradeDir, "trades-*.jsonl"))
	if err != nil || len(names) == 0 {
		return 0, err
	}
	sort.Strings(names)
	var n uint64
	for i := len(names) - 1; i >= 0 && n == 0; i-- { // the newest file that has a trade
		recs, err := readTrades(names[i])
		if err != nil {
			return 0, fmt.Errorf("%s: %w", strings.TrimPrefix(names[i], f.tradeDir+"/"), err)
		}
		for _, t := range recs {
			if t.ID > n {
				n = t.ID
			}
		}
	}
	return n, nil
}
