// =====================================
// Colorfy string by ANSI color
//
// inspired by github.com/fatih/color
// =====================================

package utils

import "fmt"

const ANSIColorEscape = "\x1b"

// Base attributes
const (
	ANSIColorReset int = iota
	ANSIColorBold
	ANSIColorFaint
	ANSIColorItalic
	ANSIColorUnderline
	ANSIColorBlinkSlow
	ANSIColorBlinkRapid
	ANSIColorReverseVideo
	ANSIColorConcealed
	ANSIColorCrossedOut
)

// Foreground text colors
const (
	ANSIColorFgBlack int = iota + 30
	ANSIColorFgRed
	ANSIColorFgGreen
	ANSIColorFgYellow
	ANSIColorFgBlue
	ANSIColorFgMagenta
	ANSIColorFgCyan
	ANSIColorFgWhite
)

// Foreground Hi-Intensity text colors
const (
	ANSIColorFgHiBlack int = iota + 90
	ANSIColorFgHiRed
	ANSIColorFgHiGreen
	ANSIColorFgHiYellow
	ANSIColorFgHiBlue
	ANSIColorFgHiMagenta
	ANSIColorFgHiCyan
	ANSIColorFgHiWhite
)

// Background text colors
const (
	ANSIColorBgBlack int = iota + 40
	ANSIColorBgRed
	ANSIColorBgGreen
	ANSIColorBgYellow
	ANSIColorBgBlue
	ANSIColorBgMagenta
	ANSIColorBgCyan
	ANSIColorBgWhite
)

// Background Hi-Intensity text colors
const (
	ANSIColorBgHiBlack int = iota + 100
	ANSIColorBgHiRed
	ANSIColorBgHiGreen
	ANSIColorBgHiYellow
	ANSIColorBgHiBlue
	ANSIColorBgHiMagenta
	ANSIColorBgHiCyan
	ANSIColorBgHiWhite
)

// Color wrap with ANSI color
func Color(color int, s string) string {
	return fmt.Sprintf("\033[1;%dm%s\033[0m", color, s)
}
