// Package ui formats terminal output.
package ui

import (
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

type Style struct {
	color bool
}

// Detect enables colour only for a terminal, honouring NO_COLOR.
func Detect(f *os.File) Style {
	if os.Getenv("NO_COLOR") != "" {
		return Style{}
	}
	fi, err := f.Stat()
	return Style{color: err == nil && fi.Mode()&os.ModeCharDevice != 0}
}

func (s Style) wrap(code, t string) string {
	if !s.color || t == "" {
		return t
	}
	return "\x1b[" + code + "m" + t + "\x1b[0m"
}

func (s Style) Green(t string) string  { return s.wrap("32", t) }
func (s Style) Orange(t string) string { return s.wrap("38;5;209", t) }
func (s Style) Blue(t string) string   { return s.wrap("38;5;111", t) }
func (s Style) Dim(t string) string    { return s.wrap("2", t) }
func (s Style) Bold(t string) string   { return s.wrap("1", t) }

type Cell struct {
	Text  string
	Color func(string) string
}

func Plain(t string) Cell { return Cell{Text: t} }

// Table pads cells by visible width, which tabwriter cannot do once colour
// escapes are in the text.
func (s Style) Table(w io.Writer, indent string, rows [][]Cell) {
	var widths []int
	for _, r := range rows {
		for i, c := range r {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(c.Text))
		}
	}
	for _, r := range rows {
		var b strings.Builder
		b.WriteString(indent)
		for i, c := range r {
			t := c.Text
			if c.Color != nil {
				t = c.Color(t)
			}
			b.WriteString(t)
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.Text)+2))
			}
		}
		io.WriteString(w, strings.TrimRight(b.String(), " ")+"\n")
	}
}
