package main

import (
	"encoding/json"
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
	fc, err := loadConfig("config.json")
	if err != nil {
		return err
	}
	cfg := Config{
		Username:     user,
		ExcludeRepos: toSet(fc.ExcludeRepos, false),
		ExcludeLangs: toSet(fc.ExcludeLangs, true),
		ExcludeForks: fc.ExcludeForks,
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

// fileConfig is the public, in-repo config (config.json). It intentionally
// holds nothing secret — repos/langs to exclude are visible to anyone.
type fileConfig struct {
	ExcludeRepos []string `json:"excludeRepos"` // "owner/name"
	ExcludeLangs []string `json:"excludeLangs"` // case-insensitive
	ExcludeForks bool     `json:"excludeForks"`
}

func loadConfig(path string) (fileConfig, error) {
	var fc fileConfig
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fc, nil // no config file is fine: nothing excluded
	}
	if err != nil {
		return fc, err
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		return fc, fmt.Errorf("parsing %s: %w", path, err)
	}
	return fc, nil
}

func toSet(items []string, lowercase bool) map[string]bool {
	set := map[string]bool{}
	for _, item := range items {
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
