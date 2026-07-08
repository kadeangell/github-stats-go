package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func fixtureStats() *Stats {
	return &Stats{
		Name:          "Kade Angell",
		Stargazers:    1234,
		Forks:         56,
		Contributions: 7890,
		LinesAdded:    100000,
		LinesDeleted:  23456,
		Views:         42,
		Repos:         []string{"kade/a", "kade/b"},
		Languages: []Language{
			{Name: "Go", Size: 300, Color: "#00ADD8", Prop: 75},
			{Name: "Python", Size: 100, Color: "", Prop: 25},
		},
	}
}

func TestRenderOverview(t *testing.T) {
	out, err := RenderOverview("templates", fixtureStats())
	if err != nil {
		t.Fatalf("RenderOverview: %v", err)
	}
	for _, want := range []string{"Kade Angell", "1,234", "56", "7,890", "123,456", "42", "2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderLanguages(t *testing.T) {
	out, err := RenderLanguages("templates", fixtureStats())
	if err != nil {
		t.Fatalf("RenderLanguages: %v", err)
	}
	for _, want := range []string{
		"width: 75.000%",
		`<span class="lang">Go</span>`,
		`<span class="percent">25.00%</span>`,
		"#000000", // missing color falls back to black
		"animation-delay: 150ms",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderLanguagesCapsList(t *testing.T) {
	s := fixtureStats()
	s.Languages = nil
	for i := range 30 {
		s.Languages = append(s.Languages, Language{
			Name: fmt.Sprintf("Lang%02d", i), Color: "#123456", Prop: 100.0 / 30,
		})
	}
	out, err := RenderLanguages("templates", s)
	if err != nil {
		t.Fatalf("RenderLanguages: %v", err)
	}
	if got := strings.Count(out, "<li"); got != 12 {
		t.Errorf("listed %d languages, want 12", got)
	}
	// The progress bar still shows every language.
	if got := strings.Count(out, `class="progress-item"`); got != 30 {
		t.Errorf("progress bar has %d segments, want 30", got)
	}
}

func TestLoadConfig(t *testing.T) {
	if _, err := loadConfig(t.TempDir() + "/missing.json"); err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}

	path := t.TempDir() + "/config.json"
	data := `{"excludeRepos": ["me/secret-repo"], "excludeLangs": ["HTML"], "excludeForks": true}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	fc, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !toSet(fc.ExcludeRepos, false)["me/secret-repo"] {
		t.Error("excludeRepos not loaded")
	}
	if !toSet(fc.ExcludeLangs, true)["html"] {
		t.Error("excludeLangs should be lowercased")
	}
	if !fc.ExcludeForks {
		t.Error("excludeForks not loaded")
	}

	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Error("malformed config should error")
	}
}

func TestComma(t *testing.T) {
	cases := map[int]string{0: "0", 5: "5", 999: "999", 1000: "1,000", 1234567: "1,234,567", -9876: "-9,876"}
	for n, want := range cases {
		if got := comma(n); got != want {
			t.Errorf("comma(%d) = %q, want %q", n, got, want)
		}
	}
}
