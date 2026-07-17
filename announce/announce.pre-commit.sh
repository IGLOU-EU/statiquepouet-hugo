#!/bin/sh
# StatiquePouet Mastodon auto-announce, as a git pre-commit hook.
#
# When you commit an article whose `fediverseContext` is empty, this hook
# toots it and stages the resulting URL back into the same commit, so the
# next build renders the thread. Same core flow as the CI jobs
# (announce.gitlab-ci.yml / announce.github-actions.yml), driven by git
# config instead of CI variables.
#
# Install it into your site (from the site root):
#
#   cp themes/statiquepouet-hugo/announce/announce.pre-commit.sh .git/hooks/pre-commit
#   chmod +x .git/hooks/pre-commit
#
# Configure it with git config (see the README for the full list):
#
#   git config --local statiquepouet.token <write:statuses token>   # secret, stays in .git/config
#   git config statiquepouet.instance mastodon.example
#   git config statiquepouet.visibility unlisted                    # optional
#
# Turn it off without uninstalling:   git config statiquepouet.enabled 0
# Skip rebuilding on every commit:    git config statiquepouet.binary /path/to/announce
#
# Requires `go` in PATH unless statiquepouet.binary points at a prebuilt
# binary (Hugo is embedded as a library). The hook never blocks your commit:
# if it is disabled, has no new content to announce, or is not configured,
# it exits 0.

# Master switch. Absent config means enabled.
TRUE=1
FALSE=0
HOOKS_ENABLED="$(git config --int --get statiquepouet.enabled 2>/dev/null || echo "$TRUE")"
[ "$HOOKS_ENABLED" -eq "$FALSE" ] && exit 0

# Cheap guard: only pay for the Hugo build when a staged markdown file under
# content/ carries an empty fediverseContext, i.e. an article awaiting its toot.
new_content="$FALSE"
oldifs=$IFS
IFS='
'
for f in $(git diff --cached --name-only --diff-filter=d -- content 2>/dev/null); do
  case "$f" in
    *.md) ;;
    *) continue ;;
  esac
  if git show ":$f" 2>/dev/null | grep -Eq "^fediverseContext[[:space:]]*[:=][[:space:]]*(\"\"|'')?[[:space:]]*\$"; then
    new_content="$TRUE"
    break
  fi
done
IFS=$oldifs
[ "$new_content" -eq "$TRUE" ] || exit 0

# Configuration, all under the statiquepouet.* git config namespace. The
# announce program reads the MASTODON_*/STATUS_*/SITE_ROOT env, so translate.
token="$(git config --get statiquepouet.token 2>/dev/null || true)"
[ -n "$token" ] || { echo "announce: statiquepouet.token unset, skipping (git config --local statiquepouet.token <token>)"; exit 0; }
export MASTODON_TOKEN="$token"

instance="$(git config --get statiquepouet.instance 2>/dev/null || true)"
[ -n "$instance" ] || { echo "announce: statiquepouet.instance unset, skipping" >&2; exit 0; }
export MASTODON_INSTANCE="$instance"

visibility="$(git config --get statiquepouet.visibility 2>/dev/null || true)"
[ -n "$visibility" ] && export STATUS_VISIBILITY="$visibility"

template="$(git config --get statiquepouet.template 2>/dev/null || true)"
[ -n "$template" ] && export STATUS_TEMPLATE="$template"

REPO_ROOT="$(git rev-parse --show-toplevel)"
site_root="$(git config --get statiquepouet.siteRoot 2>/dev/null || true)"
SITE_ROOT="${site_root:-$REPO_ROOT}"
export SITE_ROOT

# A prebuilt binary (statiquepouet.binary) skips the go build on every commit,
# else fall back to `go run` against the module.
content_diff() { git diff --name-only -- content 2>/dev/null || true; }
before="$(content_diff)"

binary="$(git config --get statiquepouet.binary 2>/dev/null || true)"
if [ -n "$binary" ] && command -v "$binary" >/dev/null 2>&1; then
  echo "announce: running $binary"
  "$binary"
  rc=$?
else
  module_dir="$SITE_ROOT/$(git config --get statiquepouet.dir 2>/dev/null || echo themes/statiquepouet-hugo/announce)"
  if [ ! -f "$module_dir/go.mod" ]; then
    echo "announce: $module_dir not found, skipping (set statiquepouet.dir or statiquepouet.binary)" >&2
    exit 0
  fi
  echo "announce: building and running at $module_dir"
  ( cd "$module_dir" && go run . )
  rc=$?
fi
[ "$rc" -eq 0 ] || echo "announce: program exited $rc, committing whatever it wrote back" >&2

# Stage only the files the program just wrote back (worktree entries that
# appeared since `before`), never sweeping in unrelated unstaged edits.
after="$(content_diff)"
if [ -n "$after" ]; then
  if [ -n "$before" ]; then
    touched="$(printf '%s\n' "$after" | grep -vxF "$before" || true)"
  else
    touched="$after"
  fi
  if [ -n "$touched" ]; then
    oldifs=$IFS
    IFS='
'
    for f in $touched; do
      git add -- "$f"
    done
    IFS=$oldifs
    echo "announce: staged front-matter URLs into this commit"
  fi
fi

exit 0
