package main

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
)

// Listener receives the logger's UDP broadcasts on one or more ports.
type Listener struct {
	store *Store
	mu    sync.Mutex
	conns []*net.UDPConn
	stats ListenStats
}

type PortStatus struct {
	Port  int    `json:"port"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type ListenStats struct {
	Ports    []PortStatus   `json:"ports"`
	Packets  map[string]int `json:"packets"`
	LastFrom string         `json:"last_from,omitempty"`
	LastAt   time.Time      `json:"last_at"`
	Senders  []string       `json:"senders"`
	senders  map[string]bool
}

func NewListener(s *Store) *Listener {
	return &Listener{store: s, stats: ListenStats{Packets: map[string]int{}, senders: map[string]bool{}}}
}

// Start (re)opens the listening ports.
func (l *Listener) Start(ports []int) {
	l.Stop()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stats.Ports = nil
	seen := map[int]bool{}
	for _, p := range ports {
		if p <= 0 || p > 65535 || seen[p] {
			continue
		}
		seen[p] = true
		c, err := net.ListenUDP("udp4", &net.UDPAddr{Port: p})
		if err != nil {
			l.stats.Ports = append(l.stats.Ports, PortStatus{Port: p, Error: friendlyListenError(err)})
			logs.Add("udp", "can't listen on port %d: %v", p, err)
			continue
		}
		c.SetReadBuffer(1 << 20)
		l.conns = append(l.conns, c)
		l.stats.Ports = append(l.stats.Ports, PortStatus{Port: p, OK: true})
		logs.Add("udp", "listening for logger broadcasts on UDP port %d", p)
		go l.read(c)
	}
}

func friendlyListenError(err error) string {
	return fmt.Sprintf("can't open this port, probably another program is using it (%v)", err)
}

func (l *Listener) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.conns {
		c.Close()
	}
	l.conns = nil
}

func (l *Listener) read(c *net.UDPConn) {
	buf := make([]byte, 65536)
	for {
		n, from, err := c.ReadFromUDP(buf)
		if err != nil {
			return // closed
		}
		data := append([]byte(nil), buf[:n]...)
		safely("udp", func() { l.handle(data, from.IP.String()) })
	}
}

func (l *Listener) handle(data []byte, from string) {
	p, err := ParseN1MM(data, time.Now().UTC())
	kind := "other"
	if err != nil {
		kind = "unreadable"
		logs.Add("udp", "unreadable packet from %s: %v", from, err)
	} else if p != nil {
		kind = p.Kind
	}
	l.mu.Lock()
	l.stats.Packets[kind]++
	l.stats.LastFrom = from
	l.stats.LastAt = time.Now()
	if !l.stats.senders[from] {
		l.stats.senders[from] = true
		logs.Add("udp", "first packet from %s", from)
	}
	l.mu.Unlock()
	if p != nil && p.Kind != "radio" {
		switch p.Kind {
		case "contact", "replace":
			logs.Add("udp", "%s %s %s %s from %s", p.Kind, p.QSO.Call, p.QSO.Band, p.QSO.RawMode, from)
		case "score":
			logs.Add("udp", "score %d for %s (%s)", p.Score.Score, p.Score.Call, p.Score.Contest)
		default:
			logs.Add("udp", "%s from %s", p.Kind, from)
		}
	}
	l.store.Handle(p)
}

func (l *Listener) Stats() ListenStats {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.stats
	s.Ports = append([]PortStatus(nil), l.stats.Ports...)
	s.Packets = map[string]int{}
	for k, v := range l.stats.Packets {
		s.Packets[k] = v
	}
	s.Senders = []string{}
	for k := range l.stats.senders {
		s.Senders = append(s.Senders, k)
	}
	sort.Strings(s.Senders)
	return s
}
