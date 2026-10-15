package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Store keeps every contact in memory and in an append-only journal file.
// A big contest is a few thousand contacts, so memory is no concern and a
// journal survives a crash or power cut without a database engine.
type Store struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	qsos    map[string]*QSO
	radios  map[string]*RadioInfo
	scores  []Score
	version int64
	changed chan struct{} // closed and replaced on every change
}

type journalEntry struct {
	Op    string `json:"op"` // put, del, score, drop
	QSO   *QSO   `json:"qso,omitempty"`
	ID    string `json:"id,omitempty"`
	Score *Score `json:"score,omitempty"`
	Key   string `json:"key,omitempty"` // contest key for drop
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, qsos: map[string]*QSO{}, radios: map[string]*RadioInfo{}, changed: make(chan struct{})}
	if path == "" {
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var e journalEntry
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				s.apply(e)
			}
		}
		f.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// Rewrite the journal compactly so it doesn't grow with every edit forever.
	if err := s.rewrite(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) apply(e journalEntry) {
	switch e.Op {
	case "put":
		if e.QSO != nil {
			s.qsos[e.QSO.ID] = e.QSO
		}
	case "del":
		delete(s.qsos, e.ID)
	case "score":
		if e.Score != nil {
			s.scores = append(s.scores, *e.Score)
			if len(s.scores) > 50 {
				s.scores = s.scores[len(s.scores)-50:]
			}
		}
	case "drop":
		for id, q := range s.qsos {
			if contestKey(q) == e.Key {
				delete(s.qsos, id)
			}
		}
	}
}

func (s *Store) rewrite() error {
	if s.path == "" {
		return nil
	}
	if s.f != nil {
		s.f.Close()
		s.f = nil
	}
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, q := range s.sortedLocked() {
		enc.Encode(journalEntry{Op: "put", QSO: q})
	}
	for i := range s.scores {
		enc.Encode(journalEntry{Op: "score", Score: &s.scores[i]})
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.f, err = os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0o644)
	return err
}

func (s *Store) record(e journalEntry) {
	s.apply(e)
	if s.f != nil {
		b, _ := json.Marshal(e)
		if _, err := s.f.Write(append(b, '\n')); err != nil {
			logs.Add("store", "can't save to %s: %v", s.path, err)
		}
	}
	s.bumpLocked()
}

func (s *Store) bumpLocked() {
	s.version++
	close(s.changed)
	s.changed = make(chan struct{})
}

// Handle applies one parsed packet.
func (s *Store) Handle(p *Packet) {
	if p == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch p.Kind {
	case "contact", "replace":
		s.record(journalEntry{Op: "put", QSO: p.QSO})
	case "delete":
		if _, ok := s.qsos[p.DeleteID]; ok {
			s.record(journalEntry{Op: "del", ID: p.DeleteID})
		}
	case "score":
		s.record(journalEntry{Op: "score", Score: p.Score})
	case "radio":
		r := p.Radio
		key := r.Station + "#" + itoa(r.RadioNr)
		old := s.radios[key]
		s.radios[key] = r
		// Radio packets arrive several times a second; only wake pages for real changes.
		if old == nil || old.FreqKHz != r.FreqKHz || old.Mode != r.Mode || old.TX != r.TX ||
			old.Running != r.Running || old.Operator != r.Operator || r.Seen.Sub(old.Seen) > 30*time.Second {
			s.bumpLocked()
		}
	}
}

func (s *Store) AddQSOs(qs []*QSO) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, q := range qs {
		s.apply(journalEntry{Op: "put", QSO: q})
	}
	if err := s.rewrite(); err != nil {
		logs.Add("store", "can't save: %v", err)
	}
	s.bumpLocked()
}

// DropContest deletes every contact in one contest.
func (s *Store) DropContest(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, q := range s.qsos {
		if contestKey(q) == key {
			delete(s.qsos, id)
			n++
		}
	}
	if err := s.rewrite(); err != nil {
		logs.Add("store", "can't save: %v", err)
	}
	s.bumpLocked()
	return n
}

// Snapshot is a consistent copy for building a summary.
type Snapshot struct {
	Version int64
	QSOs    []*QSO
	Radios  []RadioInfo
	Scores  []Score
}

func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{Version: s.version, QSOs: s.sortedLocked(), Scores: append([]Score(nil), s.scores...)}
	for _, r := range s.radios {
		snap.Radios = append(snap.Radios, *r)
	}
	sort.Slice(snap.Radios, func(i, j int) bool {
		a, b := snap.Radios[i], snap.Radios[j]
		if a.Station != b.Station {
			return a.Station < b.Station
		}
		return a.RadioNr < b.RadioNr
	})
	return snap
}

func (s *Store) sortedLocked() []*QSO {
	out := make([]*QSO, 0, len(s.qsos))
	for _, q := range s.qsos {
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.Before(out[j].Time)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Wait returns a channel that closes on the next change.
func (s *Store) Wait() (int64, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version, s.changed
}

func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f != nil {
		s.f.Close()
		s.f = nil
	}
}
