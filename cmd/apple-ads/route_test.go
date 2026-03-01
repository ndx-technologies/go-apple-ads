package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestRoute(t *testing.T) {
	commands := []string{
		"timeline",
		"merge-csv",
		"get update-commands-csv",

		"analyse campaigns",
		"analyse adgroups",
		"analyse keywords",
		"analyse searchterms",

		"analyse keywords cannibalisation",
		"analyse keywords language-mismatch",
	}

	tests := []struct {
		args []string
		cmd  string
		rest []string
	}{
		{
			args: []string{"timeline", "-id"},
			cmd:  "timeline", rest: []string{"-id"},
		},
		{
			args: []string{"merge-csv", "--path", "data/"},
			cmd:  "merge-csv", rest: []string{"--path", "data/"},
		},
		{
			args: []string{"get", "update-commands-csv", "--output", "commands.csv"},
			cmd:  "get update-commands-csv", rest: []string{"--output", "commands.csv"},
		},
		{
			args: []string{"analyse", "campaigns", "--date", "2023-01-01"},
			cmd:  "analyse campaigns", rest: []string{"--date", "2023-01-01"},
		},
		{
			args: []string{"analyse", "adgroups", "-v"},
			cmd:  "analyse adgroups", rest: []string{"-v"},
		},
		{
			args: []string{"analyse", "keywords", "--campaign-id", "1234"},
			cmd:  "analyse keywords", rest: []string{"--campaign-id", "1234"},
		},
		{
			args: []string{"analyse", "searchterms", "--limit", "100"},
			cmd:  "analyse searchterms", rest: []string{"--limit", "100"},
		},
		{
			args: []string{"analyse", "keywords", "cannibalisation", "-id"},
			cmd:  "analyse keywords cannibalisation", rest: []string{"-id"},
		},
		{
			args: []string{"analyse", "keywords", "language-mismatch", "--lang", "en"},
			cmd:  "analyse keywords language-mismatch", rest: []string{"--lang", "en"},
		},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			if cmd, rest, err := route(tt.args, commands); cmd != tt.cmd || !slices.Equal(rest, tt.rest) || err != nil {
				t.Error(tt, cmd, rest, err)
			}
		})
	}

	t.Run("unknown", func(t *testing.T) {
		tests := [][]string{
			{""},
			{"unknown"},
			{"analyse", "unknown"},
			{"analyse", "keywords", "unknown"},
			{"analyse-keywords", "cannibalisation"},
			{"analyse-keywords-cannibalisation"},
			{"analyse", "keywords", "language", "mismatch"},
		}
		for _, tt := range tests {
			t.Run(strings.Join(tt, " "), func(t *testing.T) {
				_, _, err := route(tt, commands)
				if !errors.Is(err, ErrUnknownCommand{}) {
					t.Error("no error")
				}
			})
		}
	})
}
