package style

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/rs/zerolog"
)

var (
	// ColorPurple is the color for purple
	ColorPurple = lipgloss.Color("99")
	// ColorDarkPurple is the color for dark purple
	ColorDarkPurple = lipgloss.Color("55")
	// ColorBlue is the color for blue
	ColorBlue = lipgloss.Color("#00BFFF")
	// ColorActive is the color for active
	ColorActive = lipgloss.Color("#00B894")
	// ColorPassive is the color for passive (light red to complement the active green)
	ColorPassive = lipgloss.Color("#FF6B6B")
	// ColorGrey is the color for grey
	ColorGrey = lipgloss.Color("#666666")
	// ColorLightGrey is the color for light grey
	ColorLightGrey = lipgloss.Color("#999999")
	// ColorDebug is the color for debug
	ColorDebug = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	// ColorInfo is the color for info
	ColorInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	// ColorWarn is the color for warn
	ColorWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("184"))
	// ColorWarning is the color for warning
	ColorWarning = lipgloss.Color("184")
	// ColorErrorValue is the color value for error
	ColorErrorValue = lipgloss.Color("204")
	// ColorError is the style for error
	ColorError = lipgloss.NewStyle().Foreground(ColorErrorValue)
	// ColorFatal is the color for fatal
	ColorFatal = lipgloss.NewStyle().Foreground(lipgloss.Color("134"))
	// ColorPanic is the color for panic
	ColorPanic = ColorFatal
	// ColorOrange is the color for orange
	ColorOrange = lipgloss.Color("#FF6B00")
	// TableHeaderStyle is the style for table headers
	TableHeaderStyle = lipgloss.NewStyle().Foreground(ColorPurple).Bold(true).Align(lipgloss.Center)
	// TableCellStyle is the style for table cells
	TableCellStyle = lipgloss.NewStyle().Padding(0, 1).Align(lipgloss.Center)
	// ColorMessage is the color for message
	ColorMessage = lipgloss.Color("245")
	// SpinnerTitleStyle is the style for spinner titles
	SpinnerTitleStyle = lipgloss.NewStyle()
	// MessageStyle is the style for messages
	MessageStyle = lipgloss.NewStyle().
			Align(lipgloss.Left).
			Width(150).
			Padding(1)

	// LogLevels styles - stolen from https://github.com/charmbracelet/log/blob/main/styles.go
	LogLevels = map[string]lipgloss.Style{
		zerolog.DebugLevel.String(): ColorDebug,
		zerolog.InfoLevel.String():  ColorInfo,
		zerolog.WarnLevel.String():  ColorWarn,
		zerolog.ErrorLevel.String(): ColorError,
		zerolog.FatalLevel.String(): ColorFatal,
		zerolog.PanicLevel.String(): ColorPanic,
	}
)

// TemplateFuncMap returns a template.FuncMap with the style functions
func TemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"Active":     RenderActiveString,
		"Passive":    RenderPassiveString,
		"Warning":    RenderWarningString,
		"Blue":       RenderBlueString,
		"Orange":     RenderOrangeString,
		"Purple":     RenderPurpleString,
		"DarkPurple": RenderDarkPurpleString,
		"Message":    RenderMessageString,
		"Grey":       RenderGreyString,
		"LightGrey":  RenderLightGreyString,
		"Join":       strings.Join,
	}
}

// RenderTable returns a styled table of the failover state
func RenderTable(headers []string, rows [][]string, styleFunc func(row, col int) lipgloss.Style) string {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(ColorPurple)).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return TableHeaderStyle
			}
			return TableCellStyle
		}).
		Headers(headers...).
		Rows(rows...)

	if styleFunc != nil {
		t.StyleFunc(styleFunc)
	}

	return t.Render()
}

// RenderPassiveString renders a string in the passive color
func RenderPassiveString(message string, bold bool) string {
	return lipgloss.NewStyle().
		Bold(bold).
		Foreground(ColorPassive).
		Render(message)
}

// RenderActiveString renders a string in the active color
func RenderActiveString(message string, bold bool) string {
	return lipgloss.NewStyle().
		Bold(bold).
		Foreground(ColorActive).
		Render(message)
}

// RenderWarningString renders a string in the warning color
func RenderWarningString(message string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorWarning).
		Render(message)
}

// RenderBlueString renders a string in the blue color
func RenderBlueString(message string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBlue).
		Render(message)
}

// RenderOrangeString renders a string in the orange color
func RenderOrangeString(message string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorOrange).
		Render(message)
}

// RenderPurpleString renders a string in the purple color
func RenderPurpleString(message string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPurple).
		Render(message)
}

// RenderDarkPurpleString renders a string in the dark purple color
func RenderDarkPurpleString(message string) string {
	return lipgloss.NewStyle().
		Bold(false).
		Foreground(ColorPurple).
		Faint(true).
		Render(message)
}

// RenderGreyString renders a string in the grey color
func RenderGreyString(message string, bold bool) string {
	return lipgloss.NewStyle().
		Bold(bold).
		Foreground(ColorGrey).
		Render(message)
}

// RenderLightGreyString renders a string in the light grey color
func RenderLightGreyString(message string) string {
	return lipgloss.NewStyle().
		Foreground(ColorLightGrey).
		Render(message)
}

// RenderMessageString renders a string in the message style
func RenderMessageString(message string) string {
	return MessageStyle.Render(message)
}

// RenderBoldMessage renders a string in the message style
func RenderBoldMessage(message string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Render(message)
}

// RenderErrorString renders an error string in the error color
func RenderErrorString(s string) string {
	return lipgloss.NewStyle().Foreground(ColorErrorValue).Render(s)
}

// RenderErrorStringf renders an error string in the error color
func RenderErrorStringf(format string, a ...any) string {
	return RenderErrorString(fmt.Sprintf(format, a...))
}

// RenderActiveStringf renders an active string in the active color
func RenderActiveStringf(format string, a ...any) string {
	return RenderActiveString(fmt.Sprintf(format, a...), false)
}

// RenderPassiveStringf renders a passive string in the passive color
func RenderPassiveStringf(format string, a ...any) string {
	return RenderPassiveString(fmt.Sprintf(format, a...), false)
}

// RenderWarningStringf renders a warning string in the warning color
func RenderWarningStringf(format string, a ...any) string {
	return RenderWarningString(fmt.Sprintf(format, a...))
}
