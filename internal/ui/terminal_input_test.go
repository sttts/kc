package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"
)

func TestTranslateKeyForTerminal(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.Msg
		want string
	}{
		{
			name: "space",
			msg:  tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}),
			want: " ",
		},
		{
			name: "tab",
			msg:  tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}),
			want: "\t",
		},
		{
			name: "backspace",
			msg:  tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}),
			want: "\x7f",
		},
		{
			name: "ctrl+a",
			msg:  tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}),
			want: "\x01",
		},
		{
			name: "printable",
			msg:  tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}),
			want: "x",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			translated := translateKeyForTerminal(tc.msg)
			km, ok := translated.(tea.KeyMsg)
			if !ok {
				t.Fatalf("translated message is not a tea.KeyMsg: %T", translated)
			}
			if got := km.String(); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
