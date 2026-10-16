package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	historyFile = "history.jsonl"
	// historyKeep is how many entries a change keeps; the file is trimmed
	// back to it once it grows a fifth past.
	historyKeep = 1000
)

type HistoryEntry struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
	Tone string    `json:"tone"`
}

// Record adds an entry to the change's history.
func (a *App) Record(id, text, tone string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	a.histMu.Lock()
	defer a.histMu.Unlock()
	dir := a.store.Dir(id)
	if _, err := os.Stat(dir); err != nil {
		return
	}
	path := filepath.Join(dir, historyFile)
	data, err := json.Marshal(HistoryEntry{At: time.Now(), Text: text, Tone: tone})
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.Write(append(data, '\n'))
	f.Close()
	if entries := readHistory(path); len(entries) > historyKeep*6/5 {
		writeHistory(path, entries[len(entries)-historyKeep:])
	}
}

// History is the change's history, newest first.
func (a *App) History(id string) []HistoryEntry {
	a.histMu.Lock()
	defer a.histMu.Unlock()
	entries := readHistory(filepath.Join(a.store.Dir(id), historyFile))
	slices.Reverse(entries)
	return entries
}

func readHistory(path string) []HistoryEntry {
	f, err := os.Open(path)
	if err != nil {
		return []HistoryEntry{}
	}
	defer f.Close()
	out := []HistoryEntry{}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	for s.Scan() {
		var e HistoryEntry
		if json.Unmarshal(s.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func writeHistory(path string, entries []HistoryEntry) {
	var b strings.Builder
	for _, e := range entries {
		if data, err := json.Marshal(e); err == nil {
			b.Write(data)
			b.WriteByte('\n')
		}
	}
	if os.WriteFile(path+".tmp", []byte(b.String()), 0o644) == nil {
		os.Rename(path+".tmp", path)
	}
}
