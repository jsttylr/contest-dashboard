package main

import (
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"
)

type LogLine struct {
	Seq  int64  `json:"seq"`
	Time string `json:"time"`
	Tag  string `json:"tag"`
	Text string `json:"text"`
}

// LogBuf keeps the most recent lines so the settings page can show what the listener is doing.
type LogBuf struct {
	mu    sync.Mutex
	lines []LogLine
	seq   int64
	max   int
}

var logs = &LogBuf{max: 2000}

func (b *LogBuf) Add(tag, format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	log.Printf("[%s] %s", tag, text)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	b.lines = append(b.lines, LogLine{Seq: b.seq, Time: time.Now().Format("15:04:05.000"), Tag: tag, Text: text})
	if len(b.lines) > b.max {
		b.lines = b.lines[len(b.lines)-b.max:]
	}
}

// Since returns lines newer than seq.
func (b *LogBuf) Since(seq int64) []LogLine {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []LogLine{}
	for _, l := range b.lines {
		if l.Seq > seq {
			out = append(out, l)
		}
	}
	return out
}

// safely runs f and logs a panic instead of letting it close the program.
func safely(where string, f func()) {
	defer func() {
		if e := recover(); e != nil {
			logs.Add("app", "BUG in %s: %v\n%s", where, e, debug.Stack())
		}
	}()
	f()
}
