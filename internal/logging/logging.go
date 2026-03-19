package logging

import (
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

// New returns a logger with the given component as its prefix.
func New(component string) *log.Logger {
	return log.WithPrefix(component)
}

// Configure sets the global log level, time format and colour styles.
// Call once at startup before any log output is produced.
func Configure(level string) {
	parsedLevel, err := log.ParseLevel(level)
	if err != nil {
		log.Error("invalid log level, defaulting to info", "level", level, "err", err)
		parsedLevel = log.InfoLevel
	}

	log.SetLevel(parsedLevel)
	log.SetTimeFunction(func(t time.Time) time.Time { return t.UTC() })
	log.SetTimeFormat("2006-01-02T15:04:05.000Z07:00")

	styles := log.DefaultStyles()
	styles.Timestamp = lipgloss.NewStyle().Faint(true)
	styles.Message = lipgloss.NewStyle().Foreground(lipgloss.Color("213"))
	styles.Value = lipgloss.NewStyle().Foreground(lipgloss.Color("105"))
	styles.Levels[log.DebugLevel] = styles.Levels[log.DebugLevel].Foreground(lipgloss.Color("86"))
	styles.Levels[log.InfoLevel] = styles.Levels[log.InfoLevel].Foreground(lipgloss.Color("82"))
	styles.Levels[log.WarnLevel] = styles.Levels[log.WarnLevel].Foreground(lipgloss.Color("226"))
	styles.Levels[log.ErrorLevel] = styles.Levels[log.ErrorLevel].Foreground(lipgloss.Color("196"))
	styles.Levels[log.FatalLevel] = styles.Levels[log.FatalLevel].Foreground(lipgloss.Color("208"))
	log.SetStyles(styles)
}
