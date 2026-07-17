# StatiquePouet — Hugo integration

Render a fediverse comment thread under your articles. Hugo bindings for [StatiquePouet](https://gitlab.com/iglou.eu/statiquepouet).

## Quick start

**1. Add the theme as a git submodule.** From your site root:

```bash
git submodule add https://gitlab.com/iglou.eu/statiquepouet-hugo.git themes/statiquepouet-hugo
```

**2. Enable it.** In your Hugo config, list your own theme first and StatiquePouet second:

```toml
theme = ["your-theme", "statiquepouet-hugo"]
```

**3. Wire the partials.** In `layouts/_default/baseof.html`:

```go-html-template
<!doctype html>
<html lang="{{ .Site.Language.Lang }}">
<head>
  …
  {{ partial       "statiquepouet-fediverse-context.html" . }}
  {{ partialCached "statiquepouet-css.html" . }}
</head>
<body>
  {{ block "main" . }}{{ end }}
  …
  {{ partialCached "statiquepouet-js.html" . }}
</body>
</html>
```

In `layouts/_default/single.html`, where the comments should appear:

```go-html-template
{{ define "main" }}
  <article>…</article>
  {{ partialCached "statiquepouet-content.html" . }}
{{ end }}
```

**4. Point an article at a fediverse post.** Add a `fediverseContext` key to its front matter:

```yaml
---
title: My article
fediverseContext: "https://mastodon.social/@you/123456..."
---
```

That's it. The article now shows the thread of replies to that fediverse post. Articles without the key render nothing, so the partials are safe to call on every page.

## Installation methods

The submodule in the quick start is the recommended default. Three methods work, all install the same self-contained theme:

| Method | Command | Notes |
|---|---|---|
| **git submodule** | `git submodule add <repo> themes/statiquepouet-hugo` | Tracks upgrades in your repo. Recommended. |
| **git clone** | `git clone --depth 1 <repo> themes/statiquepouet-hugo` | Simplest, no submodule bookkeeping. |
| **Hugo Module** | `hugo mod get gitlab.com/iglou.eu/statiquepouet-hugo` | For sites already using the modules system. |

`<repo>` is `https://gitlab.com/iglou.eu/statiquepouet-hugo.git`.

For the clone and submodule methods, enable the theme via composition as shown in the quick start. No theme of your own? `theme = "statiquepouet-hugo"` works too. Neither method needs a Go toolchain on your site (only the optional announce tool does).

For the Hugo Module method, your site must itself be a module (`hugo mod init <path>` once), then import it:

```toml
[[module.imports]]
  path = "gitlab.com/iglou.eu/statiquepouet-hugo"
```

Heads-up for module users: the optional [`announce/`](announce/) tool is a separate nested Go module, so `hugo mod get` does **not** fetch it. If you want it, use the clone or submodule method instead.

Whichever method you choose, Hugo merges the layouts so StatiquePouet's partials become available. They are all prefixed `statiquepouet-`, so they override nothing of yours. Out of the box the JS, CSS and i18n bundles are served from the upstream GitLab Pages CDN, no other setup needed.

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

The default `basePath` points at the upstream Pages CDN. Convenient but tied to upstream `main`. To host the assets yourself, copy `dist/` and `lang/` from the main repo into your `static/`, then set the `basePath` config:

```bash
cp -r path/to/statiquepouet/dist static/dist
cp -r path/to/statiquepouet/lang static/lang
```

## Mastodon auto-announce

The repo ships [`announce/`](announce/), a small Go program that toots articles whose `fediverseContext` is empty and writes the resulting URL back into their front matter. It links Hugo as a Go library (`hugolib.NewHugoSites` + a no-render build) to enumerate pages and resolve permalinks, so per-page `slug:` / `url:` overrides and the site's `permalinks` config are honoured. Idempotent (skips already-announced articles, sends `Idempotency-Key` on the POST), skips gracefully when the token is unset (forks without secrets don't break).

Optional per-article front-matter key `fediverseHashtags` (list of strings, without the leading `#`) appends Mastodon hashtags at the end of the toot:

```yaml
fediverseContext: ""
fediverseHashtags:
  - blog
  - lowtech
```

Site-wide defaults go under `[params.statiquepouet]` as a `fediverseHashtags` list. They are prepended to every toot, in addition to each article's own tags. Case-insensitive duplicates are dropped, the defaults keeping their leading position:

```toml
[params.statiquepouet]
  fediverseHashtags = ["blog", "france"]
```

Setup:
1. Mastodon UI, Preferences, Development, New application, scope `write:statuses` only. Copy the token.
2. Export `MASTODON_TOKEN`, `MASTODON_INSTANCE`. Optional: `STATUS_VISIBILITY` (default `public`), `STATUS_TEMPLATE` (default `New article: %title%\n%url%`), `SITE_ROOT` (default `.`). See the file header for details.
3. Wire it before `hugo build`. npm `prebuild`, Makefile, or the git `pre-commit` hook below.

Requires `go` in `PATH` (Hugo is embedded as a library).

### Local: git pre-commit hook

The `announce/` folder ships a ready [`announce.pre-commit.sh`](announce/announce.pre-commit.sh) hook. When you commit an article whose `fediverseContext` is empty, it toots it and stages the resulting URL into that same commit, so the next build renders the thread. It is configured entirely through `git config` under the `statiquepouet.*` namespace, no env file to manage.

Install it from your site root, once:

```bash
cp themes/statiquepouet-hugo/announce/announce.pre-commit.sh .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

Then configure it. The token stays in `.git/config` (via `--local`), so it is never committed and never lands in your global `~/.gitconfig`:

```bash
git config --local statiquepouet.token <write:statuses token>   # required, secret
git config statiquepouet.instance mastodon.example              # required, bare hostname
git config statiquepouet.visibility unlisted                    # optional
```

| Key | Required | Effect |
|---|---|---|
| `statiquepouet.token` | yes | OAuth2 token, scope `write:statuses`. Set it with `--local` so it stays out of git and out of `~/.gitconfig`. |
| `statiquepouet.instance` | yes | Bare instance hostname, e.g. `mastodon.example`. |
| `statiquepouet.enabled` | no | `0` to turn the hook off without uninstalling it. Default on. |
| `statiquepouet.binary` | no | Path to a prebuilt announce binary. Set it to skip the `go` build on every commit (`go build -o announce ./announce` from the theme repo). |
| `statiquepouet.visibility` | no | `public` / `unlisted` / `private` / `direct`. Default `public`. |
| `statiquepouet.template` | no | Toot body, tokens `%title%` and `%url%`. Default `New article: %title%\n%url%`. |
| `statiquepouet.dir` | no | Path to the announce module, relative to the site root. Default `themes/statiquepouet-hugo/announce`. |
| `statiquepouet.siteRoot` | no | Hugo site root. Defaults to the git repo root. Set it only for a monorepo. |

The hook never blocks your commit: if it is disabled, has no staged article awaiting a toot, or is not configured, it exits quietly. To avoid paying for a Hugo build on every commit, it first checks whether a staged markdown file under `content/` actually carries an empty `fediverseContext`, and only then runs the program. Because it stages the URLs into the commit in progress, the thread renders on your very next build, unlike a pre-push hook.

### CI (GitLab / GitHub)

The `announce/` folder ships ready CI files, [`announce.gitlab-ci.yml`](announce/announce.gitlab-ci.yml) and [`announce.github-actions.yml`](announce/announce.github-actions.yml), with the same flow: run the program on `golang:latest`, refresh its dependencies with `go mod tidy`, then commit and push the front-matter URLs back to your repo. No editing of the file is needed, everything is set through CI variables. Run it before your Hugo build.

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
| `ANNOUNCE_DIR` | Path to the announce module, relative to the site root. Default `themes/statiquepouet-hugo/announce`. Set it only if you cloned the theme under a different directory name. |
| `SITE_ROOT` | Hugo site root. Set it only for a monorepo where the site lives in a subfolder (defaults to the repo root). |

The push back uses the platform's built-in token, no extra credential to create, and such pushes do not retrigger the pipeline.

**GitLab.** Include it from your `.gitlab-ci.yml`, then enable the job-token push once under Settings, CI/CD, Job token permissions, "Allow Git push requests to the repository". The job targets branch pipelines (`$CI_COMMIT_BRANCH`).

```yaml
include:
  - local: 'themes/statiquepouet-hugo/announce/announce.gitlab-ci.yml'
```

**GitHub.** If you have no workflow yet, copy the file as a standalone workflow. Workflows only run from `.github/workflows/` at the repo root, so copy it there.

```bash
cp themes/statiquepouet-hugo/announce/announce.github-actions.yml .github/workflows/announce.yml
```

If you already have a build/deploy workflow, do not add a second workflow. Copy only the `announce` job (the `jobs.announce:` block, from `announce:` down to the end of the file) into your existing workflow's `jobs:` and wire it as a gate before your build:

- Add `needs: announce` to your build job. A failed announce then stops the build and deploy.
- Give the announce job `permissions: contents: write` (set it on the job, since a build workflow's top-level permissions are usually read-only). Keep your build job's `pages: write` / `id-token: write` on that job.
- Set your build job's checkout to `ref: ${{ github.ref_name }}`. The announce job pushes the front-matter URLs back to the branch, and checking out the branch tip (rather than the triggering SHA) makes the build render the thread in the same run instead of the next one.

To store the Mastodon secret and variables in a named GitHub Environment instead of at repository level, add `environment: { name: <env> }` to the announce job and create that environment under Settings, Environments. Declaring the environment on the job is what unlocks its scoped `vars.*` and `secrets.*`.

## License

Public domain via the [Unlicense](UNLICENSE).
