package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type searchResult struct {
	Repo string `json:"repo"`
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type rgMatch struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

const (
	maxSearchResults  = 50
	searchTimeoutSecs = 15
	rgBinary          = "rg"
)

func handleSearch(repoCachePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, `{"error":"missing ?q= parameter"}`, http.StatusBadRequest)
			return
		}

		terms := extractSearchTerms(query)
		if len(terms) == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]searchResult{})
			return
		}

		results, err := searchReposJSON(repoCachePath, terms)
		if err != nil {
			log.Printf("[Search] error: %v", err)
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func searchReposJSON(basePath string, terms []string) ([]searchResult, error) {
	if basePath == "" {
		return nil, fmt.Errorf("REPO_CACHE_PATH not set")
	}
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		log.Printf("[Search] repo cache path %s does not exist, skipping", basePath)
		return nil, nil
	}
	if _, err := exec.LookPath(rgBinary); err != nil {
		log.Printf("[Search] ripgrep binary %q not found in PATH, skipping", rgBinary)
		return nil, nil
	}

	start := time.Now()
	defer func() {
		log.Printf("[Search] completed in %v", time.Since(start))
	}()

	repos := discoverRepos(basePath)
	if len(repos) == 0 {
		log.Printf("[Search] no .git repos found under %s", basePath)
		return nil, nil
	}

	var patterns []string
	for _, t := range terms {
		patterns = append(patterns, regexp.QuoteMeta(t))
	}
	pattern := strings.Join(patterns, "|")

	return ripgrepSearch(repos, pattern)
}

func extractSearchTerms(query string) []string {
	words := strings.Fields(query)
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
		"were": true, "be": true, "been": true, "being": true, "have": true,
		"has": true, "had": true, "do": true, "does": true, "did": true,
		"will": true, "would": true, "could": true, "should": true, "may": true,
		"might": true, "can": true, "shall": true, "to": true, "of": true,
		"in": true, "for": true, "on": true, "with": true, "at": true,
		"by": true, "from": true, "as": true, "into": true, "through": true,
		"during": true, "before": true, "after": true, "above": true,
		"below": true, "between": true, "and": true, "but": true, "or": true,
		"nor": true, "not": true, "so": true, "yet": true, "both": true,
		"either": true, "neither": true, "each": true, "every": true,
		"all": true, "any": true, "few": true, "more": true, "most": true,
		"other": true, "some": true, "such": true, "no": true, "only": true,
		"own": true, "same": true, "than": true, "too": true, "very": true,
		"just": true, "about": true, "up": true, "out": true, "if": true,
		"then": true, "now": true, "here": true, "there": true, "when": true,
		"where": true, "why": true, "how": true, "which": true, "who": true,
		"whom": true, "what": true, "this": true, "that": true, "these": true,
		"those": true, "it": true, "its": true, "my": true, "your": true,
		"our": true, "their": true, "i": true, "me": true, "we": true,
		"you": true, "he": true, "she": true, "they": true, "him": true,
		"her": true, "them": true, "us": true, "get": true, "need": true,
		"want": true, "like": true, "know": true, "see": true, "look": true,
		"find": true, "tell": true, "give": true, "show": true, "let": true,
		"make": true, "take": true, "use": true, "check": true, "help": true,
		"please": true, "thanks": true, "mean": true, "say": true,
	}
	seen := map[string]bool{}
	var terms []string

	for _, w := range words {
		w = strings.Trim(strings.ToLower(w), `"',.!?;:()[]{}#@*&^%$+=<>/\|~`+"`")
		if len(w) < 4 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		terms = append(terms, w)
	}

	return terms
}

func discoverRepos(basePath string) []string {
	var repos []string

	ownerEntries, err := os.ReadDir(basePath)
	if err != nil {
		log.Printf("[Search] failed to read %s: %v", basePath, err)
		return nil
	}

	for _, owner := range ownerEntries {
		if !owner.IsDir() || strings.HasPrefix(owner.Name(), ".") {
			continue
		}
		ownerPath := filepath.Join(basePath, owner.Name())

		repoEntries, err := os.ReadDir(ownerPath)
		if err != nil {
			continue
		}

		for _, repo := range repoEntries {
			if !repo.IsDir() || strings.HasPrefix(repo.Name(), ".") {
				continue
			}
			repoPath := filepath.Join(ownerPath, repo.Name())
			gitDir := filepath.Join(repoPath, ".git")
			if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
				repos = append(repos, repoPath)
			}
		}
	}

	log.Printf("[Search] discovered %d repos under %s", len(repos), basePath)
	return repos
}

func ripgrepSearch(repos []string, pattern string) ([]searchResult, error) {
	var allResults []searchResult

	for _, repoPath := range repos {
		if len(allResults) >= maxSearchResults {
			break
		}

		repoName := repoNameFromPath(repoPath)
		matches, err := rgSearchRepo(repoPath, pattern, repoName, maxSearchResults-len(allResults))
		if err != nil {
			log.Printf("[Search] rg error on %s: %v", repoName, err)
			continue
		}
		allResults = append(allResults, matches...)
	}

	return allResults, nil
}

func rgSearchRepo(repoPath, pattern, repoName string, limit int) ([]searchResult, error) {
	args := []string{
		"--json",
		"--no-config",
		"--hidden",
		"--no-messages",
		"--no-ignore-vcs",
		"--max-count", "3",
		"--max-columns", "200",
		"-C", "1",
		"--",
		pattern,
		repoPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(searchTimeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, rgBinary, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("rg exited: %w", err)
	}

	var results []searchResult
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var m rgMatch
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		if m.Type != "match" {
			continue
		}

		results = append(results, searchResult{
			Repo: repoName,
			File: m.Data.Path.Text,
			Line: m.Data.LineNumber,
			Text: strings.TrimRight(m.Data.Lines.Text, "\n"),
		})

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

func repoNameFromPath(path string) string {
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	if len(parts) >= 2 {
		return fmt.Sprintf("%s/%s", parts[len(parts)-2], parts[len(parts)-1])
	}
	return filepath.Base(path)
}
