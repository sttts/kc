package viewer

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

var registerStylesOnce sync.Once

func ensureCustomStyles() {
	registerStylesOnce.Do(func() {
		// Register Turbo Pascal inspired style (foreground-only colors).
		turbo := chroma.MustNewStyle("turbo-pascal", chroma.StyleEntries{
			chroma.Background:       "",
			chroma.Text:             "#d7d7d7",
			chroma.Comment:          "#00afff",
			chroma.Keyword:          "bold #00afff",
			chroma.Name:             "#d7d7d7",
			chroma.NameAttribute:    "#d7d7d7",
			chroma.NameTag:          "#d7d7d7",
			chroma.LiteralString:    "#ffd75f",
			chroma.LiteralStringDoc: "#ffd75f",
			chroma.LiteralNumber:    "#5fff5f",
			chroma.Operator:         "#ffffff",
			chroma.Punctuation:      "#ffffff",
			chroma.Error:            "#ff5555",
		})
		styles.Register(turbo)
	})
}

func formatTurboPascalANSI(it chroma.Iterator) string {
	var buf bytes.Buffer
	buf.WriteString("\033[44m")
	prevWasColon := false
	atLineStart := true
	for token := it(); token != chroma.EOF; token = it() {
		t := token.Type
		ansi := "37"
		bold := false
		val := token.Value
		hasNL := strings.Contains(val, "\n")

		if t == chroma.Punctuation && strings.Contains(val, ":") {
			buf.WriteString("\033[1m\033[35m")
			buf.WriteString(val)
			buf.WriteString("\033[39m\033[22m")
			prevWasColon = true
			if hasNL {
				prevWasColon, atLineStart = false, true
			} else {
				atLineStart = false
			}
			continue
		}

		forced := false
		if prevWasColon {
			if strings.TrimSpace(val) != "" && t != chroma.Punctuation {
				ansi, bold = "33", true
				prevWasColon = false
				forced = true
			}
		}

		if !forced && atLineStart && !prevWasColon && strings.TrimSpace(val) != "" {
			if !(t == chroma.Punctuation && strings.TrimSpace(val) != "-") {
				ansi, bold = "36", true
				forced = true
			}
		}

		switch {
		case t == chroma.NameTag || t.Category() == chroma.Name:
			ansi, bold = "36", true
		case !forced && (t == chroma.LiteralString || t.Category() == chroma.LiteralString):
			ansi, bold = "33", true
		case !forced && (t == chroma.LiteralNumber || t.Category() == chroma.LiteralNumber):
			ansi, bold = "32", true
		case !forced && (t == chroma.Punctuation || t == chroma.Operator):
			ansi, bold = "35", true
		case !forced && (t == chroma.Comment || t.Category() == chroma.Comment):
			ansi = "34"
		default:
			ansi = "37"
		}

		if bold {
			buf.WriteString("\033[1m")
		}
		buf.WriteString("\033[" + ansi + "m")
		buf.WriteString(val)
		buf.WriteString("\033[39m\033[22m")
		if hasNL {
			prevWasColon = false
			atLineStart = true
		} else {
			atLineStart = false
		}
	}
	buf.WriteString("\033[0m")
	return buf.String()
}

func formatTTY16mWithPanelBG(style *chroma.Style, it chroma.Iterator) string {
	var buf bytes.Buffer
	buf.WriteString("\033[44m")
	for token := it(); token != chroma.EOF; token = it() {
		entry := style.Get(token.Type)
		if entry.Bold == chroma.Yes {
			buf.WriteString("\033[1m")
		}
		if entry.Underline == chroma.Yes {
			buf.WriteString("\033[4m")
		}
		if entry.Italic == chroma.Yes {
			buf.WriteString("\033[3m")
		}
		if entry.Colour.IsSet() {
			fmt.Fprintf(&buf, "\033[38;2;%d;%d;%dm", entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue())
		}
		buf.WriteString(token.Value)
		buf.WriteString("\033[39m\033[22m\033[24m\033[23m")
	}
	buf.WriteString("\033[0m")
	return buf.String()
}

// EnsureStyles registers custom chroma styles once.
func EnsureStyles() {
	ensureCustomStyles()
}
