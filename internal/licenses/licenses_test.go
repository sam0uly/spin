package licenses

import (
	"strings"
	"testing"
)

func TestKnown(t *testing.T) {
	known := Known()
	if len(known) == 0 {
		t.Fatal("Known() must not be empty")
	}
	for i := 1; i < len(known); i++ {
		if known[i-1] >= known[i] {
			t.Errorf("Known() must be sorted: %v", known)
		}
	}
	for _, id := range known {
		if id == NoneID {
			t.Errorf("None must not be part of the known license set")
		}
		if IsKnown(id) == false {
			t.Errorf("Known() ids must satisfy IsKnown: %q", id)
		}
	}
}

func TestOptions(t *testing.T) {
	opts := Options()
	known := Known()
	if len(opts) != len(known)+1 {
		t.Fatalf("Options() must be Known() + None; got %d options", len(opts))
	}
	for i, id := range known {
		if opts[i] != id {
			t.Errorf("Options()[%d] = %q, want %q", i, opts[i], id)
		}
	}
	if opts[len(opts)-1] != NoneID {
		t.Errorf("last option must be %q, got %q", NoneID, opts[len(opts)-1])
	}
}

func TestIsKnown(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"MIT", true},
		{"Apache-2.0", true},
		{"CC0-1.0", true},
		{"None", false},
		{"none", false},
		{"NONE", false},
		{"Proprietary", false},
		{"proprietary", false},
		{"", false},
		{"mit", false}, // exact SPDX matching only
		{"Apache-3.0", false},
		{"Custom License", false},
	}
	for _, tc := range cases {
		if got := IsKnown(tc.id); got != tc.want {
			t.Errorf("IsKnown(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// TestRender_Known ensures every built-in license renders a non-empty
// text without error, so a bad embed or id never breaks generation.
func TestRender_Known(t *testing.T) {
	for _, id := range Known() {
		t.Run(id, func(t *testing.T) {
			text, err := Render(id, "Jane Doe", 2026)
			if err != nil {
				t.Fatalf("Render(%q): %v", id, err)
			}
			if strings.TrimSpace(text) == "" {
				t.Errorf("Render(%q) produced empty text", id)
			}
		})
	}
}

func TestRender_MITSubstitutes(t *testing.T) {
	text, err := Render("MIT", "Jane Doe", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Copyright (c) 2026 Jane Doe") {
		t.Errorf("expected substituted copyright line; got:\n%s", text)
	}
	if strings.Contains(text, "<year>") || strings.Contains(text, "<copyright holders>") {
		t.Errorf("placeholder tokens must be substituted; got:\n%s", text)
	}
}

func TestRender_ApacheSubstitutes(t *testing.T) {
	text, err := Render("Apache-2.0", "Jane Doe", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Copyright 2026 Jane Doe") {
		t.Errorf("expected [yyyy]/[name of copyright owner] substitution; got:\n%s", text)
	}
}

func TestRender_BSDSubstitutes(t *testing.T) {
	for _, id := range []string{"BSD-2-Clause", "BSD-3-Clause"} {
		text, err := Render(id, "Jane Doe", 2026)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "Copyright (c) 2026 Jane Doe") {
			t.Errorf("%s: expected <year>/<owner> substitution; got:\n%s", id, text)
		}
	}
}

func TestRender_MissingHolderKeepsToken(t *testing.T) {
	text, err := Render("MIT", "", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Copyright (c) 2026 <copyright holders>") {
		t.Errorf("missing holder must leave the token in place; got:\n%s", text)
	}
}

func TestRender_ZeroYearKeepsToken(t *testing.T) {
	text, err := Render("MIT", "Jane Doe", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Copyright (c) <year> Jane Doe") {
		t.Errorf("zero year must leave the token in place; got:\n%s", text)
	}
}

func TestRender_NoTokenLicensesUnaffected(t *testing.T) {
	for _, id := range []string{"Unlicense", "CC0-1.0"} {
		text, err := Render(id, "Jane Doe", 2026)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, "Jane Doe") || strings.Contains(text, "2026") {
			t.Errorf("%s has no copyright tokens and must not be substituted; got:\n%s", id, text)
		}
	}
}

func TestRender_UnknownID(t *testing.T) {
	_, err := Render("Not-A-License", "Jane Doe", 2026)
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
	if !strings.Contains(err.Error(), "Not-A-License") {
		t.Errorf("error should name the id; got: %v", err)
	}
}
