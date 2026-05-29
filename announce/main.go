// Announce Hugo articles on Mastodon, then write the resulting toot URL
// back into each article's `fediverse_context` front matter so that
// StatiquePouet can render the comment thread for that article.
//
//	MASTODON_TOKEN     OAuth2 access token with `write:statuses` scope.
//	                   Generated via Preferences > Development on the
//	                   instance. If unset, the program exits 0 silently
//	                   so that CI on forks without the secret does not
//	                   fail the build.
//	MASTODON_INSTANCE  Bare hostname of the instance (e.g.
//	                   `mastodon.example`). Scheme and trailing slash
//	                   are tolerated and stripped.
//	STATUS_VISIBILITY  `public`, `unlisted`, `private`, `direct`.
//	                   Default: `public`. Use `direct` while testing
//	                   (the toot is delivered to your inbox only).
//	STATUS_TEMPLATE    Body of the toot. Tokens `%title%` and `%url%`
//	                   are substituted. Default:
//	                       New article: %title%
//	                       %url%
//	SITE_ROOT          Hugo site root (directory containing `hugo.toml`).
//	                   Default: current directory.
//
// References:
//
//	POST /api/v1/statuses        https://docs.joinmastodon.org/methods/statuses/#create
//	Idempotency-Key (1h TTL)     same page, header section
//	Obtaining an access token    https://docs.joinmastodon.org/client/token/
//
// Exit codes:
//
//	0  every eligible article announced (or none eligible).
//	1  at least one POST failed; surviving articles still got processed.
//	2  configuration error (missing env var, missing dependency).
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/config/allconfig"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugofs/hglob"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/gohugoio/hugo/parser/pageparser"
	"github.com/gohugoio/hugo/resources/page"
)

const (
	fediverseKey      = "fediverse_context"
	hashtagsKey       = "fediverse_hashtags"
	paramsSection     = "statiquepouet" // [params.statiquepouet] in the Hugo config
	defaultStatusBody = "New article: %title%\n%url%"
	defaultVisibility = "public"
	apiTimeout        = 30 * time.Second
)

func main() {
	os.Exit(run())
}

func run() int {
	token := os.Getenv("MASTODON_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "MASTODON_TOKEN absent, skipping announce step")
		return 0
	}

	instance := normaliseInstance(os.Getenv("MASTODON_INSTANCE"))
	if instance == "" {
		fmt.Fprintln(os.Stderr, "announce: MASTODON_INSTANCE is required (e.g. mastodon.example)")
		return 2
	}

	visibility := envOr("STATUS_VISIBILITY", defaultVisibility)
	tmpl := envOr("STATUS_TEMPLATE", defaultStatusBody)
	siteRoot, err := filepath.Abs(envOr("SITE_ROOT", "."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "announce: resolve SITE_ROOT: %s\n", err)
		return 2
	}

	sites, err := newHugoSites(siteRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "announce: init hugo: %s\n", err)
		return 2
	}

	contentRoots := sites.PathSpec.BaseFs.Content.RealDirs("")
	if len(contentRoots) == 0 {
		fmt.Fprintln(os.Stderr, "announce: no content directories resolved from hugo config")
		return 2
	}
	candidates, err := scanCandidates(contentRoots)
	if err != nil {
		fmt.Fprintf(os.Stderr, "announce: scan content: %s\n", err)
		return 2
	}
	if len(candidates) == 0 {
		return 0
	}

	if err := buildHugoSites(sites, candidates); err != nil {
		fmt.Fprintf(os.Stderr, "announce: build hugo site: %s\n", err)
		return 2
	}

	defaultTags := extractHashtags(sites.Sites[0].Site().Params().GetNested(paramsSection, hashtagsKey))

	client := &http.Client{Timeout: apiTimeout}
	exit := 0
	for _, p := range sites.Pages() {
		if p.File() == nil || p.Draft() {
			continue
		}
		tootURL, err := announcePage(client, token, instance, visibility, tmpl, siteRoot, defaultTags, p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "announce %s: %s\n", relPath(siteRoot, p), err)
			exit = 1
			continue
		}
		if tootURL != "" {
			fmt.Printf("announced: %s -> %s\n", relPath(siteRoot, p), tootURL)
		}
	}
	return exit
}

func newHugoSites(siteRoot string) (*hugolib.HugoSites, error) {
	cfg := config.New()
	cfg.Set("workingDir", siteRoot)
	cfg.Set("buildDrafts", true)
	cfg.Set("buildFuture", true)
	cfg.Set("buildExpired", true)

	configs, err := allconfig.LoadConfig(allconfig.ConfigSourceDescriptor{
		Flags:       cfg,
		Fs:          hugofs.Os,
		Environment: "production",
	})
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	cfg.Set("publishDir", configs.Base.PublishDir)
	cfg.Set("publishDirStatic", configs.Base.PublishDir)
	cfg.Set("publishDirDynamic", configs.Base.PublishDir)
	hfs := hugofs.NewFromSourceAndDestination(hugofs.Os, hugofs.Os, cfg)

	h, err := hugolib.NewHugoSites(deps.DepsCfg{
		Configs: configs,
		Fs:      hfs,
		StdOut:  io.Discard,
		StdErr:  os.Stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("new hugo sites: %w", err)
	}
	return h, nil
}

func scanCandidates(contentRoots []string) (map[string]bool, error) {
	out := make(map[string]bool)
	for _, root := range contentRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				// errors (permission, vanished symlink) are skipped.
				if path == root {
					return walkErr
				}
				return nil
			}
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			cfm, err := pageparser.ParseFrontMatterAndContent(bytes.NewReader(raw))
			if err != nil {
				return nil
			}
			val, ok := cfm.FrontMatter[fediverseKey]
			if !ok {
				return nil
			}
			if s, _ := val.(string); strings.TrimSpace(s) != "" {
				return nil
			}
			out[path] = true
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func buildHugoSites(h *hugolib.HugoSites, candidates map[string]bool) error {
	filter := hglob.NewFilenameFilterForInclusionFunc(func(filename string) bool {
		return candidates[filename]
	})
	if err := h.Build(hugolib.BuildCfg{SkipRender: true, ContentInclusionFilter: filter}); err != nil {
		return fmt.Errorf("build: %w", err)
	}
	return nil
}

func relPath(siteRoot string, p page.Page) string {
	abs := p.File().Filename()
	rel, err := filepath.Rel(siteRoot, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func normaliseInstance(s string) string {
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	return strings.TrimRight(s, "/")
}

func announcePage(client *http.Client, token, instance, visibility, tmpl, siteRoot string, defaultTags []string, p page.Page) (string, error) {
	params := p.Params()
	rawCtx, hasKey := params[fediverseKey]
	if !hasKey {
		return "", nil
	}
	if s, _ := rawCtx.(string); strings.TrimSpace(s) != "" {
		return "", nil // already announced
	}

	title := p.Title()
	rel := relPath(siteRoot, p)
	if title == "" {
		fmt.Fprintf(os.Stderr, "skip %s, missing title in front matter\n", rel)
		return "", nil
	}

	body := strings.ReplaceAll(tmpl, "%title%", title)
	body = strings.ReplaceAll(body, "%url%", p.Permalink())

	if tags := mergeHashtags(defaultTags, extractHashtags(params[hashtagsKey])); len(tags) > 0 {
		body = strings.TrimRight(body, "\n") + "\n\n" + strings.Join(tags, " ")
	}

	fullPath := p.File().Filename()
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}

	idem := idempotencyKey(rel, body)
	tootURL, err := postStatus(client, token, instance, visibility, body, idem)
	if err != nil {
		return "", err
	}

	if err := writeBackFediverseContext(fullPath, raw, tootURL); err != nil {
		return "", fmt.Errorf("write back: %w", err)
	}
	return tootURL, nil
}

var errStaleIdempotency = errors.New("idempotency-key resolves to a deleted status")

func postStatus(client *http.Client, token, instance, visibility, status, idem string) (string, error) {
	url, err := doPostStatus(client, token, instance, visibility, status, idem)
	if errors.Is(err, errStaleIdempotency) && idem != "" {
		return doPostStatus(client, token, instance, visibility, status, "")
	}
	return url, err
}

func doPostStatus(client *http.Client, token, instance, visibility, status, idem string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"status":     status,
		"visibility": visibility,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, "https://"+instance+"/api/v1/statuses", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound && idem != "" {
			return "", errStaleIdempotency
		}
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}

	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if out.URL == "" {
		return "", errors.New("no .url in Mastodon response")
	}
	return out.URL, nil
}

func idempotencyKey(path, body string) string {
	slug := strings.ReplaceAll(strings.TrimSuffix(path, ".md"), "/", "-")
	sum := sha256.Sum256([]byte(body))
	return "statique-pouet-" + slug + "-" + hex.EncodeToString(sum[:4])
}

func extractHashtags(v any) []string {
	var raw []any
	switch t := v.(type) {
	case []any:
		raw = t
	case []string:
		raw = make([]any, len(t))
		for i, s := range t {
			raw[i] = s
		}
	default:
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, x := range raw {
		s, ok := x.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "#")
		if s == "" {
			continue
		}
		out = append(out, "#"+s)
	}
	return out
}

func mergeHashtags(defaults, page []string) []string {
	out := make([]string, 0, len(defaults)+len(page))
	seen := make(map[string]bool, len(defaults)+len(page))
	for _, tag := range append(append([]string{}, defaults...), page...) {
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, tag)
	}
	return out
}

var fediverseLineRe = regexp.MustCompile(`(?m)^(fediverse_context[ \t]*[:=]).*$`)

func writeBackFediverseContext(path string, original []byte, url string) error {
	if !fediverseLineRe.Match(original) {
		return errors.New("fediverse_context line not found in file")
	}
	// JSON like escape for value inside double quotes. YAML and
	// TOML accept this for characters we care about `\` or `"`.
	safe := strings.ReplaceAll(url, `\`, `\\`)
	safe = strings.ReplaceAll(safe, `"`, `\"`)
	suffix := ` "` + safe + `"`

	updated := fediverseLineRe.ReplaceAllFunc(original, func(match []byte) []byte {
		sub := fediverseLineRe.FindSubmatch(match)
		return append(append([]byte{}, sub[1]...), suffix...)
	})
	return os.WriteFile(path, updated, 0o644)
}
