package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

const keyPickerRows = 8

type keyFilePickerEntry struct {
	name  string
	isDir bool
}

type keyFilePicker struct {
	open        bool
	dir         string
	fallbackDir string
	entries     []keyFilePickerEntry
	selected    int
	loading     bool
	err         error
	seq         uint64
}

type keyFilePickerLoadedMsg struct {
	seq     uint64
	dir     string
	entries []keyFilePickerEntry
	err     error
}

func newKeyFilePicker() keyFilePicker {
	dir := startDir()
	fallbackDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, ".ssh")
		fallbackDir = home
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if fallbackDir != "" {
		if abs, err := filepath.Abs(fallbackDir); err == nil {
			fallbackDir = abs
		}
	}

	return keyFilePicker{open: true, dir: dir, fallbackDir: fallbackDir, loading: true, seq: 1}
}

func (p keyFilePicker) load() tea.Cmd {
	dir, fallbackDir, seq := p.dir, p.fallbackDir, p.seq
	return func() tea.Msg {
		entries, err := readKeyDirectory(dir)
		if err != nil && fallbackDir != "" {
			if fallbackEntries, fallbackErr := readKeyDirectory(fallbackDir); fallbackErr == nil {
				return keyFilePickerLoadedMsg{seq: seq, dir: fallbackDir, entries: fallbackEntries}
			}
		}
		return keyFilePickerLoadedMsg{seq: seq, dir: dir, entries: entries, err: err}
	}
}

func readKeyDirectory(dir string) ([]keyFilePickerEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("unable to read %s: %w", dir, err)
	}

	result := make([]keyFilePickerEntry, 0, len(entries))
	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(fullPath); err == nil {
				isDir = info.IsDir()
			}
		}
		result = append(result, keyFilePickerEntry{name: entry.Name(), isDir: isDir})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].isDir != result[j].isDir {
			return result[i].isDir
		}
		left, right := strings.ToLower(result[i].name), strings.ToLower(result[j].name)
		if left == right {
			return result[i].name < result[j].name
		}
		return left < right
	})
	return result, nil
}

func (p keyFilePicker) withEntries(msg keyFilePickerLoadedMsg) keyFilePicker {
	if !p.open || msg.seq != p.seq || (msg.dir != p.dir && msg.dir != p.fallbackDir) {
		return p
	}
	p.dir = msg.dir
	p.fallbackDir = ""
	p.entries = msg.entries
	p.err = msg.err
	p.loading = false
	if p.selected >= len(p.entries) {
		p.selected = max(0, len(p.entries)-1)
	}
	return p
}

func (p *keyFilePicker) move(delta int) {
	if len(p.entries) == 0 || p.loading {
		return
	}
	p.selected = (p.selected + delta + len(p.entries)) % len(p.entries)
}

func (p keyFilePicker) selectedEntry() (keyFilePickerEntry, bool) {
	if p.loading || p.selected < 0 || p.selected >= len(p.entries) {
		return keyFilePickerEntry{}, false
	}
	return p.entries[p.selected], true
}

func (p keyFilePicker) entryPath(name string) string {
	return filepath.Join(p.dir, name)
}

func (p keyFilePicker) openDir(name string) (keyFilePicker, tea.Cmd) {
	return p.readDir(p.entryPath(name))
}

func (p keyFilePicker) parent() (keyFilePicker, tea.Cmd) {
	parent := filepath.Dir(p.dir)
	if parent == p.dir {
		return p, nil
	}
	return p.readDir(parent)
}

func (p keyFilePicker) readDir(dir string) (keyFilePicker, tea.Cmd) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	p.dir = dir
	p.fallbackDir = ""
	p.entries = nil
	p.selected = 0
	p.loading = true
	p.err = nil
	p.seq++
	return p, p.load()
}

func (p keyFilePicker) View(maxWidth int) string {
	width := min(64, maxWidth-2)
	if width < 20 {
		width = 20
	}
	innerWidth := max(1, borderInteriorWidth(width))

	pathLine := lipgloss.NewStyle().Foreground(colorMuted).Render(truncateHead(p.dir, innerWidth))
	lines := []string{pathLine, ""}
	switch {
	case p.loading:
		lines = append(lines, "Loading…")
	case p.err != nil:
		lines = append(lines, lipgloss.NewStyle().Foreground(colorError).Render(truncateHead(p.err.Error(), innerWidth)))
	case len(p.entries) == 0:
		lines = append(lines, lipgloss.NewStyle().Foreground(colorMuted).Render("(directory is empty)"))
	default:
		start := 0
		if p.selected >= keyPickerRows {
			start = p.selected - keyPickerRows + 1
		}
		end := min(len(p.entries), start+keyPickerRows)
		for i := start; i < end; i++ {
			entry := p.entries[i]
			name := entry.name
			if entry.isDir {
				name += string(os.PathSeparator)
			}
			prefix := "  "
			if i == p.selected {
				prefix = "> "
			}
			line := runewidth.Truncate(prefix+name, innerWidth, "…")
			style := lipgloss.NewStyle().Width(innerWidth)
			if i == p.selected {
				style = style.Foreground(colorAccent).Bold(true).Reverse(true)
			} else if entry.isDir {
				style = style.Foreground(colorDirectory)
			}
			lines = append(lines, style.Render(line))
		}
	}

	lines = append(lines, "", lipgloss.NewStyle().Foreground(colorMuted).Render("↑/↓ move · Enter open/select · Backspace parent · Esc cancel"))
	body := strings.Join(lines, "\n")
	height := lipgloss.Height(body) + 2
	return borderWithTitle(body, "Choose identity file", width, height, colorAccent)
}
