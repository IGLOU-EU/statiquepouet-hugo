# StatiquePouet — Hugo integration

Hugo bindings for [StatiquePouet](https://gitlab.com/iglou.eu/statiquepouet). The underlying library, its options, theming and runtime behaviour live in the main repo, this one is just the glue.

> Intentionally not packaged as a Hugo module: four partial files don't justify pulling in a Go toolchain, adding a `go.mod` to your site, or migrating a classic `theme:` setup into the modules system. A `git clone` does the same job with zero friction.

## Quickstart

From your Hugo site root:

```bash
git clone --depth 1 https://gitlab.com/iglou.eu/statiquepouet-hugo.git themes/<name>/layouts/_partials/statiquepouet-hugo
```

Or, if your theme is itself a git repo, add it as a submodule so upgrades stay tracked:

```bash
git submodule add https://gitlab.com/iglou.eu/statiquepouet-hugo.git themes/<name>/layouts/_partials/statiquepouet-hugo
```

Wire the four partials. In `layouts/_default/baseof.html`:

```go-html-template
<!doctype html>
<html lang="{{ .Site.Language.Lang }}">
<head>
  …
  {{ partial       "statiquepouet-hugo/fediverse-context.html" . }}
  {{ partialCached "statiquepouet-hugo/css.html" . }}
</head>
<body>
  {{ block "main" . }}{{ end }}
  …
  {{ partialCached "statiquepouet-hugo/js.html" . }}
</body>
</html>
```

In `layouts/_default/single.html` (or your equivalent), where the comments should appear:

```go-html-template
{{ define "main" }}
  <article>…</article>
  {{ partialCached "statiquepouet-hugo/content.html" . }}
{{ end }}
```

Mark articles that should display a thread with a `fediverse_context` key in their front matter:

```yaml
---
title: My article
fediverse_context: "https://mastodon.social/@you/123456..."
---
```

Without it, all four partials render nothing (safe to call them unconditionally on every page).

That's it. Out of the box the JS, CSS and i18n bundles are served from the upstream GitLab Pages CDN, no other setup needed.

## Configuration

Under `[params.statiquepouet]` in your Hugo config. Every key has a default, override only what you need.

| Key | Type | Default | Effect |
|---|---|---|---|
| `basePath` | string | `"https://statiquepouet.gitlab.iglou.eu/"` | URL prefix to `dist/` and `lang/`. Defaults to the upstream Pages, republished on every push to `main`. Trailing slash added if missing. |
| `lang` | string | _none_ | `STATIQUE_POUET_CONFIG.lang`. Unset → `null` → the lib falls back to `<html lang>` → `navigator.language` → `"en"`. |
| `commentsContainerId` | string | `"comments-content"` | ID of the `<div>` that hosts the rendered thread. |
| `cssLight` / `cssDark` | bool | `true` | Emit the corresponding theme `<link>`. |
| `cssAutoTheme` | bool | `true` | Wrap themes in `prefers-color-scheme` media queries. `false` = both loaded unconditionally (toggle theme yourself via a class on `<body>` etc.). |
| `buttonLoad` / `buttonReply` / `buttonShare` | bool | `true` | Emit each button. |
| `buttonLoadText` | string | `"Load comments"` | Label of the load button. |
| `buttonReplyText` | string | `"Reply"` | Label of the reply button. |
| `buttonShareText` | string | `"Share"` | Label of the share button. |

TOML example:

```toml
[params.statiquepouet]
  lang = "fr"
  cssDark = false                       # light theme only
  buttonShare = false                   # hide the share button
  buttonLoadText = "Charger les commentaires"
  buttonReplyText = "Répondre"
```

## Self-hosting the assets

The default `basePath` points at the upstream Pages CDN. Convenient but tied to upstream `main`. To host the assets yourself, copy `dist/` and `lang/` from the main repo into your `static/`, then set `basePath` config:

```bash
cp -r path/to/statiquepouet/dist static/dist
cp -r path/to/statiquepouet/lang static/lang
```

## Mastodon auto-announce

The repo ships [`announce/`](announce/), a small Go program that toots articles whose `fediverse_context` is empty and writes the resulting URL back into their front matter. It links Hugo as a Go library (`hugolib.NewHugoSites` + a no-render build) to enumerate pages and resolve permalinks, so per-page `slug:` / `url:` overrides and the site's `permalinks` config are honoured. Idempotent (skips already-announced articles, sends `Idempotency-Key` on the POST), skips gracefully when the token is unset (forks without secrets don't break).

Optional per-article front-matter key `fediverse_hashtags` (list of strings, without the leading `#`) appends Mastodon hashtags at the end of the toot:

```yaml
fediverse_context: ""
fediverse_hashtags:
  - blog
  - lowtech
```

Site-wide defaults go under `[params.statiquepouet]` as a `fediverse_hashtags` list. They are prepended to every toot, in addition to each article's own tags. Case-insensitive duplicates are dropped, the defaults keeping their leading position:

```toml
[params.statiquepouet]
  fediverse_hashtags = ["blog", "france"]
```

Setup:
1. Mastodon UI, Preferences, Development, New application, scope `write:statuses` only. Copy the token.
2. Export `MASTODON_TOKEN`, `MASTODON_INSTANCE`. Optional: `STATUS_VISIBILITY` (default `public`), `STATUS_TEMPLATE` (default `New article: %title%\n%url%`), `SITE_ROOT` (default `.`). See the file header for details.
3. Wire it before `hugo build`. npm `prebuild`, Makefile, or a git `pre-push` hook.

Requires `go` in `PATH` (Hugo is embedded as a library). From the site root:

```bash
# .git/hooks/pre-push
#!/bin/sh
set -e
go run ./themes/<name>/layouts/_partials/statiquepouet-hugo/announce
if ! git diff --quiet content/; then
  git add content/
  git commit -m "chore: announce articles to Mastodon"
fi
```

You can also build the binary once (`go build -o announce ./announce` from the theme repo) and ship it in `PATH` to skip the `go run` cost on every invocation.

### CI (GitLab / GitHub)

The `announce/` folder ships ready CI files, [`announce.gitlab-ci.yml`](announce/announce.gitlab-ci.yml) and [`announce.github-actions.yml`](announce/announce.github-actions.yml), with the same flow: run the program on `golang:latest`, refresh its dependencies with `go mod tidy`, then commit and push the front-matter URLs back to your repo. No editing of the file is needed everything is set through CI variables. Run it before your Hugo build.

Configure these (GitLab: Settings, CI/CD, Variables. GitHub: Settings, Secrets and variables, Actions). The job fails fast if either is unset:

| Variable | Notes |
|---|---|
| `MASTODON_TOKEN` | Token, scope `write:statuses`. A GitHub secret, mark masked on GitLab. |
| `MASTODON_INSTANCE` | Bare hostname, e.g. `mastodon.example`. |

Optional:

| Variable | Notes |
|---|---|
| `STATUS_VISIBILITY` | `public` / `unlisted` / `private` / `direct`. Default `public`. |
| `STATUS_TEMPLATE` | Toot body, tokens `%title%` and `%url%`. Default `New article: %title%\n%url%`. |
| `ANNOUNCE_DIR` | Skip the scan and point at the module directly. |
| `SITE_ROOT` | Hugo site root. Set it only for a monorepo where the site lives in a subfolder (defaults to the repo root). |

The push back uses the platform's built-in token, no extra credential to create, and such pushes do not retrigger the pipeline.

**GitLab.** Include it from your `.gitlab-ci.yml`, then enable the job-token push once under Settings, CI/CD, Job token permissions, "Allow Git push requests to the repository". The job targets branch pipelines (`$CI_COMMIT_BRANCH`).

```yaml
include:
  - local: 'themes/<name>/layouts/_partials/statiquepouet-hugo/announce/announce.gitlab-ci.yml'
```

**GitHub.** Workflows only run from `.github/workflows/` at the repo root, so copy the file there. On GitHub `MASTODON_TOKEN` is a secret and the rest are repository variables (`vars.*`).

```bash
cp themes/<name>/layouts/_partials/statiquepouet-hugo/announce/announce.github-actions.yml .github/workflows/announce.yml
```
