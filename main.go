package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run fetches stats and writes the SVGs. Any error exits non-zero so the
// workflow never commits bad images over good ones.
func run() error {
	token := os.Getenv("ACCESS_TOKEN")
	user := os.Getenv("GITHUB_ACTOR")
	if token == "" || user == "" {
		return errors.New("ACCESS_TOKEN and GITHUB_ACTOR environment variables must be set")
	}
	cfg := Config{
		Username:     user,
		ExcludeRepos: csvSet(os.Getenv("EXCLUDED"), false),
		ExcludeLangs: csvSet(os.Getenv("EXCLUDED_LANGS"), true),
		ExcludeForks: isTruthy(os.Getenv("EXCLUDE_FORKED_REPOS")),
	}

	stats, err := FetchStats(NewClient(token), cfg)
	if err != nil {
		return err
	}
	// Sanity gate: an account with zero repos or contributions means the API
	// lied to us; refuse to overwrite good images with empty ones.
	if len(stats.Repos) == 0 || stats.Contributions == 0 {
		return fmt.Errorf("sanity check failed (repos=%d, contributions=%d); refusing to write images",
			len(stats.Repos), stats.Contributions)
	}

	overview, err := RenderOverview("templates", stats)
	if err != nil {
		return err
	}
	languages, err := RenderLanguages("templates", stats)
	if err != nil {
		return err
	}

	if err := os.MkdirAll("generated", 0o755); err != nil {
		return err
	}
	if err := os.WriteFile("generated/overview.svg", []byte(overview), 0o644); err != nil {
		return err
	}
	return os.WriteFile("generated/languages.svg", []byte(languages), 0o644)
}

func csvSet(csv string, lowercase bool) map[string]bool {
	set := map[string]bool{}
	for _, item := range strings.Split(csv, ",") {
		item = strings.TrimSpace(item)
		if lowercase {
			item = strings.ToLower(item)
		}
		if item != "" {
			set[item] = true
		}
	}
	return set
}

func isTruthy(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s != "" && s != "false"
}
