package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// RenderOverview fills templates/overview.svg with summary statistics.
func RenderOverview(templateDir string, s *Stats) (string, error) {
	tmpl, err := os.ReadFile(templateDir + "/overview.svg")
	if err != nil {
		return "", err
	}
	out := strings.NewReplacer(
		"{{ name }}", s.Name,
		"{{ stars }}", comma(s.Stargazers),
		"{{ forks }}", comma(s.Forks),
		"{{ contributions }}", comma(s.Contributions),
		"{{ lines_changed }}", comma(s.LinesAdded+s.LinesDeleted),
		"{{ views }}", comma(s.Views),
		"{{ repos }}", comma(len(s.Repos)),
	).Replace(string(tmpl))
	return out, checkComplete(out)
}

// RenderLanguages fills templates/languages.svg with the language bar and list.
func RenderLanguages(templateDir string, s *Stats) (string, error) {
	tmpl, err := os.ReadFile(templateDir + "/languages.svg")
	if err != nil {
		return "", err
	}

	// ponytail: the SVG box is a fixed 360x220 — roughly 12 list items fit.
	// The progress bar still shows every language's proportion.
	const maxLangsListed = 12
	const delayBetween = 150
	var progress, langList strings.Builder
	for i, lang := range s.Languages {
		color := lang.Color
		if color == "" {
			color = "#000000"
		}
		fmt.Fprintf(&progress,
			`<span style="background-color: %s;width: %.3f%%;" class="progress-item"></span>`,
			color, lang.Prop)
		if i >= maxLangsListed {
			continue
		}
		fmt.Fprintf(&langList, `
<li style="animation-delay: %dms;">
<svg xmlns="http://www.w3.org/2000/svg" class="octicon" style="fill:%s;"
viewBox="0 0 16 16" version="1.1" width="16" height="16"><path
fill-rule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8z"></path></svg>
<span class="lang">%s</span>
<span class="percent">%.2f%%</span>
</li>

`, i*delayBetween, color, lang.Name, lang.Prop)
	}

	out := strings.NewReplacer(
		"{{ progress }}", progress.String(),
		"{{ lang_list }}", langList.String(),
	).Replace(string(tmpl))
	return out, checkComplete(out)
}

// checkComplete guards against a template placeholder surviving into output.
func checkComplete(svg string) error {
	if i := strings.Index(svg, "{{"); i >= 0 {
		end := min(i+30, len(svg))
		return fmt.Errorf("unreplaced placeholder in output near %q", svg[i:end])
	}
	return nil
}

// comma formats n with thousands separators (stdlib has no locale formatting).
func comma(n int) string {
	s := strconv.Itoa(n)
	neg := ""
	if strings.HasPrefix(s, "-") {
		neg, s = "-", s[1:]
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	return neg + strings.Join(append([]string{s}, parts...), ",")
}
