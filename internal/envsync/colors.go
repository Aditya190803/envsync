package envsync

import (
	"fmt"
	"os"
	"strings"
)

// ANSI color codes for terminal output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// colorsEnabled returns true when ANSI colors should be used.
// Colors are disabled when NO_COLOR is set or TERM is "dumb".
func colorsEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	return true
}

// colorize wraps text in an ANSI color code, if colors are enabled.
func colorize(color, text string) string {
	if !colorsEnabled() {
		return text
	}
	return color + text + colorReset
}

// cOK returns a green "OK" string.
func cOK() string { return colorize(colorGreen+colorBold, "OK") }

// cFAIL returns a red "FAIL" string.
func cFAIL() string { return colorize(colorRed+colorBold, "FAIL") }

// cSuccess formats a success message in green.
func cSuccess(format string, a ...any) string {
	return colorize(colorGreen, fmt.Sprintf(format, a...))
}

// cError formats an error message in red.
func cError(format string, a ...any) string {
	return colorize(colorRed, fmt.Sprintf(format, a...))
}

// cWarn formats a warning message in yellow.
func cWarn(format string, a ...any) string {
	return colorize(colorYellow, fmt.Sprintf(format, a...))
}

// cInfo formats an info message in cyan.
func cInfo(format string, a ...any) string {
	return colorize(colorCyan, fmt.Sprintf(format, a...))
}

// cBold formats text in bold.
func cBold(text string) string {
	return colorize(colorBold, text)
}

// cDim formats text in dim.
func cDim(text string) string {
	return colorize(colorDim, text)
}
