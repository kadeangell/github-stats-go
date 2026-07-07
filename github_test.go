package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClient(url string) *Client {
	c := NewClient("test-token")
	c.base = url
	c.sleep = func(time.Duration) {}
	return c
}

func TestRESTRetriesOn202(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()

	var out struct {
		OK bool `json:"ok"`
	}
	if err := testClient(srv.URL).REST("/some/path", &out); err != nil {
		t.Fatalf("REST: %v", err)
	}
	if !out.OK || calls != 3 {
		t.Fatalf("got ok=%v after %d calls, want ok=true after 3", out.OK, calls)
	}
}

func TestGraphQLSurfacesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data": null, "errors": [{"message": "rate limit exceeded"}]}`))
	}))
	defer srv.Close()

	var out any
	err := testClient(srv.URL).GraphQL("query {}", &out)
	if err == nil || !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("want graphql error surfaced, got %v", err)
	}
}

// TestIndependentPagination reproduces the upstream bug scenario: the owned
// connection has two pages while the contributed connection has one. Repos
// must not be double-counted and both cursors must advance independently.
func TestIndependentPagination(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			w.Write([]byte(`{"data": {"viewer": {
				"login": "kade", "name": "Kade",
				"repositories": {
					"pageInfo": {"hasNextPage": true, "endCursor": "c1"},
					"nodes": [{"nameWithOwner": "kade/a", "stargazers": {"totalCount": 5}, "forkCount": 1,
						"languages": {"edges": [{"size": 100, "node": {"name": "Go", "color": "#00ADD8"}}]}}]
				},
				"repositoriesContributedTo": {
					"pageInfo": {"hasNextPage": false, "endCursor": null},
					"nodes": [{"nameWithOwner": "other/x", "stargazers": {"totalCount": 7}, "forkCount": 0,
						"languages": {"edges": []}}]
				}
			}}}`))
			return
		}
		w.Write([]byte(`{"data": {"viewer": {
			"login": "kade", "name": "Kade",
			"repositories": {
				"pageInfo": {"hasNextPage": false, "endCursor": "c2"},
				"nodes": [{"nameWithOwner": "kade/b", "stargazers": {"totalCount": 3}, "forkCount": 2,
					"languages": {"edges": [{"size": 300, "node": {"name": "Python", "color": "#3572A5"}}]}}]
			},
			"repositoriesContributedTo": {
				"pageInfo": {"hasNextPage": false, "endCursor": null},
				"nodes": [{"nameWithOwner": "other/x", "stargazers": {"totalCount": 7}, "forkCount": 0,
					"languages": {"edges": []}}]
			}
		}}}`))
	}))
	defer srv.Close()

	s, err := fetchRepoOverview(testClient(srv.URL), Config{Username: "kade"})
	if err != nil {
		t.Fatalf("fetchRepoOverview: %v", err)
	}
	if page != 2 {
		t.Fatalf("made %d queries, want 2", page)
	}
	if len(s.Repos) != 3 {
		t.Fatalf("got repos %v, want 3 unique", s.Repos)
	}
	if s.Stargazers != 15 { // 5 + 3 + 7, contributed repo counted once
		t.Fatalf("stargazers = %d, want 15", s.Stargazers)
	}
	if len(s.Languages) != 2 || s.Languages[0].Name != "Python" {
		t.Fatalf("languages = %+v, want Python first (largest)", s.Languages)
	}
}
