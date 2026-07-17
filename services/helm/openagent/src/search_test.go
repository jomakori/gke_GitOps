package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSearchTerms(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "basic terms",
			query: "terraform module istio gateway",
			want:  []string{"terraform", "module", "istio", "gateway"},
		},
		{
			name:  "filters stop words",
			query: "what is the terraform module for the cluster",
			want:  []string{"terraform", "module", "cluster"},
		},
		{
			name:  "deduplicates",
			query: "terraform terraform module module",
			want:  []string{"terraform", "module"},
		},
		{
			name:  "strips punctuation",
			query: "\"terraform\", 'module', (istio) [gateway]",
			want:  []string{"terraform", "module", "istio", "gateway"},
		},
		{
			name:  "min word length 4",
			query: "a bc def ghi jklm",
			want:  []string{"jklm"},
		},
		{
			name:  "case insensitive",
			query: "Terraform MODULE Istio",
			want:  []string{"terraform", "module", "istio"},
		},
		{
			name:  "empty string",
			query: "",
			want:  nil,
		},
		{
			name:  "only stop words and short words",
			query: "what is the meaning of this",
			want:  []string{"meaning"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSearchTerms(tt.query)
			if len(got) != len(tt.want) {
				t.Errorf("extractSearchTerms(%q) = %v (%d terms), want %v (%d terms)", tt.query, got, len(got), tt.want, len(tt.want))
				return
			}
			for i, term := range got {
				if term != tt.want[i] {
					t.Errorf("extractSearchTerms(%q)[%d] = %q, want %q", tt.query, i, term, tt.want[i])
				}
			}
		})
	}
}

func TestRepoNameFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"standard owner/repo", "/mnt/repos/jomakori/gke_GitOps", "jomakori/gke_GitOps"},
		{"trailing slash", "/mnt/repos/jomakori/devops_Terraform/", "jomakori/devops_Terraform"},
		{"deep path", "/data/cache/github/jomakori/k8s-maklab-cluster", "jomakori/k8s-maklab-cluster"},
		{"relative path", "repos/owner/repo", "owner/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repoNameFromPath(tt.path)
			if got != tt.want {
				t.Errorf("repoNameFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestDiscoverRepos(t *testing.T) {
	t.Run("discovers repos with .git dirs", func(t *testing.T) {
		dir := t.TempDir()

		os.MkdirAll(filepath.Join(dir, "jomakori", "gke_GitOps", ".git"), 0755)
		os.MkdirAll(filepath.Join(dir, "jomakori", "devops_Terraform", ".git"), 0755)
		os.MkdirAll(filepath.Join(dir, "other", "not-a-repo"), 0755)

		repos := discoverRepos(dir)
		if len(repos) != 2 {
			t.Errorf("discoverRepos: expected 2 repos, got %d: %v", len(repos), repos)
		}
	})

	t.Run("skips dot-prefixed dirs", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".hidden", "repo", ".git"), 0755)
		os.MkdirAll(filepath.Join(dir, "owner", ".hidden-repo", ".git"), 0755)

		repos := discoverRepos(dir)
		if len(repos) != 0 {
			t.Errorf("discoverRepos: expected 0 repos, got %d", len(repos))
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		repos := discoverRepos(dir)
		if len(repos) != 0 {
			t.Errorf("discoverRepos: expected 0 repos, got %d", len(repos))
		}
	})

	t.Run("nonexistent path", func(t *testing.T) {
		repos := discoverRepos("/nonexistent/path/12345")
		if repos != nil {
			t.Errorf("discoverRepos: expected nil, got %v", repos)
		}
	})
}

func TestSearchReposJSON(t *testing.T) {
	t.Run("empty basePath", func(t *testing.T) {
		_, err := searchReposJSON("", []string{"test"})
		if err == nil {
			t.Error("expected error for empty basePath")
		}
	})

	t.Run("nonexistent basePath", func(t *testing.T) {
		results, err := searchReposJSON("/nonexistent/repo/cache/path", []string{"test"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})

	t.Run("gradient degradation when rg missing", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "owner", "repo", ".git"), 0755)
		origPath := os.Getenv("PATH")
		os.Setenv("PATH", "")
		defer os.Setenv("PATH", origPath)

		results, err := searchReposJSON(dir, []string{"test"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results when rg missing, got %v", results)
		}
	})
}

func TestHandleSearch(t *testing.T) {
	t.Run("rejects POST", func(t *testing.T) {
		handler := handleSearch("/tmp/test")
		req := httptest.NewRequest(http.MethodPost, "/search?q=test", nil)
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", w.Code)
		}
	})

	t.Run("missing q parameter", func(t *testing.T) {
		handler := handleSearch("/tmp/test")
		req := httptest.NewRequest(http.MethodGet, "/search", nil)
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("empty q returns empty array", func(t *testing.T) {
		handler := handleSearch("/tmp/test")
		req := httptest.NewRequest(http.MethodGet, "/search?q=the+what+is", nil)
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		body := w.Body.String()
		if body != "[]\n" {
			t.Errorf("expected empty JSON array, got %q", body)
		}
	})

	t.Run("nonexistent repo path returns empty array", func(t *testing.T) {
		handler := handleSearch("/nonexistent/path/for/search")
		req := httptest.NewRequest(http.MethodGet, "/search?q=terraform+module", nil)
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: body=%q", w.Code, w.Body.String())
		}
	})

	t.Run("repo path without rg binary returns empty array", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "owner", "repo", ".git"), 0755)

		origPath := os.Getenv("PATH")
		os.Setenv("PATH", "")
		defer os.Setenv("PATH", origPath)

		handler := handleSearch(dir)
		req := httptest.NewRequest(http.MethodGet, "/search?q=terraform+module", nil)
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: body=%q", w.Code, w.Body.String())
		}
	})
}

func TestRepoCachePathConfig(t *testing.T) {
	t.Setenv("AGENT_API_URL", "http://localhost:8083/a2a")

	t.Run("default empty", func(t *testing.T) {
		cfg := loadConfig()
		if cfg.RepoCachePath != "" {
			t.Errorf("RepoCachePath should default to empty, got %q", cfg.RepoCachePath)
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("REPO_CACHE_PATH", "/mnt/repos")
		cfg := loadConfig()
		if cfg.RepoCachePath != "/mnt/repos" {
			t.Errorf("RepoCachePath = %q, want /mnt/repos", cfg.RepoCachePath)
		}
	})
}
