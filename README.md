# statique-pouet — Hugo integration

Hugo bindings for [statique-pouet](https://gitlab.com/iglou.eu/statiquepouet). The underlying library, its options, theming and runtime behaviour live in the main repo, this one is just the glue.

> Intentionally not packaged as a Hugo module: four partial files don't justify pulling in a Go toolchain, adding a `go.mod` to your site, or migrating a classic `theme:` setup into the modules system. A `git clone` does the same job with zero friction.

## Quickstart

From your Hugo site root:

```bash
git clone --depth 1 \
  https://gitlab.com/iglou.eu/statiquepouet-hugo.git \
  layouts/_partials/statique-pouet
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

TOML example:

```toml
[params.statiquepouet]
  lang = "fr"
  cssDark = false       # light theme only
  buttonShare = false   # hide the share button
```

## Self-hosting the assets

The default `basePath` points at the upstream Pages CDN. Convenient but tied to upstream `main`. To host the assets yourself, copy `dist/` and `lang/` from the main repo into your `static/`, then set `basePath` config:

```bash
cp -r path/to/statiquepouet/dist static/dist
cp -r path/to/statiquepouet/lang static/lang
```
