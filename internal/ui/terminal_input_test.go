package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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

func TestTranslateKeyForTerminalStress10k(t *testing.T) {
	const iterations = 10_000

	for i := 0; i < iterations; i++ {
		r := rune('!' + (i % 94)) // printable ASCII range 33-126
		msg := tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)})
		translated := translateKeyForTerminal(msg)
		km, ok := translated.(tea.KeyMsg)
		if !ok {
			t.Fatalf("iteration %d: translated message is not a tea.KeyMsg: %T", i, translated)
		}
		if got := km.String(); got != string(r) {
			t.Fatalf("iteration %d: expected %q, got %q", i, string(r), got)
		}
	}
}
