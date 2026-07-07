package main

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

type Config struct {
	Username     string
	ExcludeRepos map[string]bool
	ExcludeLangs map[string]bool // lowercased names
	ExcludeForks bool
}

type Language struct {
	Name  string
	Size  int
	Color string
	Prop  float64 // percentage of total bytes
}

type Stats struct {
	Name          string
	Stargazers    int
	Forks         int
	Contributions int
	LinesAdded    int
	LinesDeleted  int
	Views         int
	Repos         []string
	Languages     []Language // sorted by size, descending
}

// FetchStats gathers everything the templates need. Any failure in the core
// GraphQL data is fatal; per-repo REST quirks degrade gracefully (logged).
func FetchStats(c *Client, cfg Config) (*Stats, error) {
	s, err := fetchRepoOverview(c, cfg)
	if err != nil {
		return nil, fmt.Errorf("fetching repo overview: %w", err)
	}
	s.Contributions, err = fetchContributions(c)
	if err != nil {
		return nil, fmt.Errorf("fetching contributions: %w", err)
	}
	s.LinesAdded, s.LinesDeleted, err = fetchLinesChanged(c, cfg.Username, s.Repos)
	if err != nil {
		return nil, fmt.Errorf("fetching lines changed: %w", err)
	}
	s.Views, err = fetchViews(c, s.Repos)
	if err != nil {
		return nil, fmt.Errorf("fetching views: %w", err)
	}
	return s, nil
}

// ---- repo overview (stars, forks, languages) --------------------------------

type repoNode struct {
	NameWithOwner string `json:"nameWithOwner"`
	Stargazers    struct {
		TotalCount int `json:"totalCount"`
	} `json:"stargazers"`
	ForkCount int `json:"forkCount"`
	Languages struct {
		Edges []struct {
			Size int `json:"size"`
			Node struct {
				Name  string `json:"name"`
				Color string `json:"color"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"languages"`
}

type repoConn struct {
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []repoNode `json:"nodes"`
}

type overviewResp struct {
	Viewer struct {
		Login                     string   `json:"login"`
		Name                      string   `json:"name"`
		Repositories              repoConn `json:"repositories"`
		RepositoriesContributedTo repoConn `json:"repositoriesContributedTo"`
	} `json:"viewer"`
}

func overviewQuery(ownedCursor, contribCursor string) string {
	return fmt.Sprintf(`{
  viewer {
    login
    name
    repositories(first: 100, orderBy: {field: UPDATED_AT, direction: DESC}, isFork: false, after: %s) {
      pageInfo { hasNextPage endCursor }
      nodes {
        nameWithOwner
        stargazers { totalCount }
        forkCount
        languages(first: 10, orderBy: {field: SIZE, direction: DESC}) {
          edges { size node { name color } }
        }
      }
    }
    repositoriesContributedTo(first: 100, includeUserRepositories: false, orderBy: {field: UPDATED_AT, direction: DESC}, contributionTypes: [COMMIT, PULL_REQUEST, REPOSITORY, PULL_REQUEST_REVIEW], after: %s) {
      pageInfo { hasNextPage endCursor }
      nodes {
        nameWithOwner
        stargazers { totalCount }
        forkCount
        languages(first: 10, orderBy: {field: SIZE, direction: DESC}) {
          edges { size node { name color } }
        }
      }
    }
  }
}`, cursorArg(ownedCursor), cursorArg(contribCursor))
}

func cursorArg(c string) string {
	if c == "" {
		return "null"
	}
	return fmt.Sprintf("%q", c)
}

func fetchRepoOverview(c *Client, cfg Config) (*Stats, error) {
	s := &Stats{}
	langs := map[string]*Language{}
	seen := map[string]bool{}

	var ownedCursor, contribCursor string
	ownedDone, contribDone := false, false

	for !ownedDone || !contribDone {
		var resp overviewResp
		if err := c.GraphQL(overviewQuery(ownedCursor, contribCursor), &resp); err != nil {
			return nil, err
		}
		s.Name = resp.Viewer.Name
		if s.Name == "" {
			s.Name = resp.Viewer.Login
		}

		var nodes []repoNode
		if !ownedDone {
			nodes = append(nodes, resp.Viewer.Repositories.Nodes...)
		}
		if !contribDone && !cfg.ExcludeForks {
			nodes = append(nodes, resp.Viewer.RepositoriesContributedTo.Nodes...)
		}
		for _, repo := range nodes {
			name := repo.NameWithOwner
			if seen[name] || cfg.ExcludeRepos[name] {
				continue
			}
			seen[name] = true
			s.Repos = append(s.Repos, name)
			s.Stargazers += repo.Stargazers.TotalCount
			s.Forks += repo.ForkCount

			for _, edge := range repo.Languages.Edges {
				lname := edge.Node.Name
				if cfg.ExcludeLangs[strings.ToLower(lname)] {
					continue
				}
				l := langs[lname]
				if l == nil {
					l = &Language{Name: lname, Color: edge.Node.Color}
					langs[lname] = l
				}
				l.Size += edge.Size
			}
		}

		// Cursors advance independently — fixes the upstream lockstep bug.
		if !ownedDone {
			ownedDone = !resp.Viewer.Repositories.PageInfo.HasNextPage
			ownedCursor = resp.Viewer.Repositories.PageInfo.EndCursor
		}
		if !contribDone {
			contribDone = !resp.Viewer.RepositoriesContributedTo.PageInfo.HasNextPage || cfg.ExcludeForks
			contribCursor = resp.Viewer.RepositoriesContributedTo.PageInfo.EndCursor
		}
	}

	total := 0
	for _, l := range langs {
		total += l.Size
	}
	for _, l := range langs {
		if total > 0 {
			l.Prop = 100 * float64(l.Size) / float64(total)
		}
		s.Languages = append(s.Languages, *l)
	}
	sort.Slice(s.Languages, func(i, j int) bool {
		if s.Languages[i].Size != s.Languages[j].Size {
			return s.Languages[i].Size > s.Languages[j].Size
		}
		return s.Languages[i].Name < s.Languages[j].Name
	})
	sort.Strings(s.Repos)
	return s, nil
}

// ---- contributions -----------------------------------------------------------

func fetchContributions(c *Client) (int, error) {
	var years struct {
		Viewer struct {
			ContributionsCollection struct {
				ContributionYears []int `json:"contributionYears"`
			} `json:"contributionsCollection"`
		} `json:"viewer"`
	}
	err := c.GraphQL(`query { viewer { contributionsCollection { contributionYears } } }`, &years)
	if err != nil {
		return 0, err
	}

	var parts []string
	for _, y := range years.Viewer.ContributionsCollection.ContributionYears {
		parts = append(parts, fmt.Sprintf(
			`year%d: contributionsCollection(from: "%d-01-01T00:00:00Z", to: "%d-01-01T00:00:00Z") { contributionCalendar { totalContributions } }`,
			y, y, y+1))
	}
	if len(parts) == 0 {
		return 0, nil
	}

	var byYear struct {
		Viewer map[string]struct {
			ContributionCalendar struct {
				TotalContributions int `json:"totalContributions"`
			} `json:"contributionCalendar"`
		} `json:"viewer"`
	}
	query := fmt.Sprintf("query { viewer { %s } }", strings.Join(parts, "\n"))
	if err := c.GraphQL(query, &byYear); err != nil {
		return 0, err
	}
	total := 0
	for _, y := range byYear.Viewer {
		total += y.ContributionCalendar.TotalContributions
	}
	return total, nil
}

// ---- per-repo REST stats -----------------------------------------------------

// forEachRepo runs fn concurrently per repo (the client bounds actual HTTP
// concurrency) and returns the first error.
func forEachRepo(repos []string, fn func(repo string) error) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(repos))
	for _, repo := range repos {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			if err := fn(repo); err != nil {
				errCh <- err
			}
		}(repo)
	}
	wg.Wait()
	close(errCh)
	return <-errCh
}

func fetchLinesChanged(c *Client, username string, repos []string) (int, int, error) {
	var mu sync.Mutex
	added, deleted := 0, 0
	err := forEachRepo(repos, func(repo string) error {
		var contributors []struct {
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
			Weeks []struct {
				A int `json:"a"`
				D int `json:"d"`
			} `json:"weeks"`
		}
		if err := c.REST("/repos/"+repo+"/stats/contributors", &contributors); err != nil {
			// ponytail: tolerate per-repo quirks (persistent 202s, malformed
			// payloads) like upstream did — a slightly-low count today
			// self-heals tomorrow, a failed run would block all updates.
			log.Printf("warning: skipping lines-changed for %s: %v", repo, err)
			return nil
		}
		for _, contributor := range contributors {
			if contributor.Author == nil || contributor.Author.Login != username {
				continue
			}
			mu.Lock()
			for _, w := range contributor.Weeks {
				added += w.A
				deleted += w.D
			}
			mu.Unlock()
		}
		return nil
	})
	return added, deleted, err
}

func fetchViews(c *Client, repos []string) (int, error) {
	var mu sync.Mutex
	total := 0
	err := forEachRepo(repos, func(repo string) error {
		var traffic struct {
			Views []struct {
				Count int `json:"count"`
			} `json:"views"`
		}
		if err := c.REST("/repos/"+repo+"/traffic/views", &traffic); err != nil {
			var httpErr *HTTPError
			if errors.As(err, &httpErr) && httpErr.Status == 403 {
				// No push access to this repo's traffic data; expected for
				// contributed repos.
				return nil
			}
			return err
		}
		mu.Lock()
		for _, v := range traffic.Views {
			total += v.Count
		}
		mu.Unlock()
		return nil
	})
	return total, err
}
