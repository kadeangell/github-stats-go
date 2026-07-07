package main

import (
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

func TestComma(t *testing.T) {
	cases := map[int]string{0: "0", 5: "5", 999: "999", 1000: "1,000", 1234567: "1,234,567", -9876: "-9,876"}
	for n, want := range cases {
		if got := comma(n); got != want {
			t.Errorf("comma(%d) = %q, want %q", n, got, want)
		}
	}
}
