package logging

import (
	"fmt"
	"sync"
	"time"

	"github.com/thebanri/limoni/widgets"
)

// Entry represents a single formatted log line with metadata.
type Entry struct {
	Number    int
	Timestamp time.Time
	Level     widgets.LogLevel
	Component string
	Message   string
	Formatted string
}

// Logger is a thread-safe ring-buffer logger that directly implements
// Limoni's widgets.LogSource, widgets.LevelSource, and widgets.LineNumberSource
// for zero-allocation rendering in widgets.LogView.
type Logger struct {
	mu       sync.RWMutex
	capacity int
	entries  []Entry
	nextNum  int
	onUpdate func()
}

// New creates a new ring-buffer Logger with the given maximum line capacity.
func New(capacity int) *Logger {
	if capacity <= 0 {
		capacity = 500
	}
	return &Logger{
		capacity: capacity,
		entries:  make([]Entry, 0, capacity),
	}
}

// SetOnUpdate registers a callback invoked whenever a new log entry is appended.
func (l *Logger) SetOnUpdate(fn func()) {
	l.mu.Lock()
	l.onUpdate = fn
	l.mu.Unlock()
}

// Log records a structured entry at the specified level.
func (l *Logger) Log(level widgets.LogLevel, component, format string, args ...any) {
	now := time.Now()
	msg := fmt.Sprintf(format, args...)
	lvlTag := "INFO "
	switch level {
	case widgets.LevelTrace:
		lvlTag = "TRACE"
	case widgets.LevelDebug:
		lvlTag = "DEBUG"
	case widgets.LevelInfo:
		lvlTag = "INFO "
	case widgets.LevelWarn:
		lvlTag = "WARN "
	case widgets.LevelError:
		lvlTag = "ERROR"
	case widgets.LevelFatal:
		lvlTag = "FATAL"
	}

	l.mu.Lock()
	l.nextNum++
	num := l.nextNum
	formatted := fmt.Sprintf("%s [%s] [%-10s] %s", now.Format("15:04:05.000"), lvlTag, component, msg)
	entry := Entry{
		Number:    num,
		Timestamp: now,
		Level:     level,
		Component: component,
		Message:   msg,
		Formatted: formatted,
	}
	if len(l.entries) >= l.capacity {
		copy(l.entries, l.entries[1:])
		l.entries[len(l.entries)-1] = entry
	} else {
		l.entries = append(l.entries, entry)
	}
	cb := l.onUpdate
	l.mu.Unlock()

	if cb != nil {
		cb()
	}
}

func (l *Logger) Debug(component, format string, args ...any) {
	l.Log(widgets.LevelDebug, component, format, args...)
}

func (l *Logger) Info(component, format string, args ...any) {
	l.Log(widgets.LevelInfo, component, format, args...)
}

func (l *Logger) Warn(component, format string, args ...any) {
	l.Log(widgets.LevelWarn, component, format, args...)
}

func (l *Logger) Error(component, format string, args ...any) {
	l.Log(widgets.LevelError, component, format, args...)
}

// Clear removes all entries from the log buffer.
func (l *Logger) Clear() {
	l.mu.Lock()
	l.entries = l.entries[:0]
	l.mu.Unlock()
}

// Len implements widgets.LogSource.
func (l *Logger) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

// Line implements widgets.LogSource without allocating.
func (l *Logger) Line(i int) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if i < 0 || i >= len(l.entries) {
		return ""
	}
	return l.entries[i].Formatted
}

// Level implements widgets.LevelSource.
func (l *Logger) Level(i int) widgets.LogLevel {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if i < 0 || i >= len(l.entries) {
		return widgets.LevelInfo
	}
	return l.entries[i].Level
}

// LineNumber implements widgets.LineNumberSource.
func (l *Logger) LineNumber(i int) int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if i < 0 || i >= len(l.entries) {
		return i + 1
	}
	return l.entries[i].Number
}

// Latest returns the most recent log message for inline status display.
func (l *Logger) Latest() (Entry, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.entries) == 0 {
		return Entry{}, false
	}
	return l.entries[len(l.entries)-1], true
}
