#!/bin/sh
# StatiquePouet Mastodon auto-announce, as a git pre-push hook.
#
# The push-time counterpart of announce.pre-commit.sh. When you push a commit
# that delivers an article whose `fediverseContext` is empty, this hook toots
# it and records the resulting URL in a follow-up commit, so your next push
# renders the thread. Same core flow as the CI jobs (announce.gitlab-ci.yml /
# announce.github-actions.yml) and the pre-commit hook, driven by git config
# instead of CI variables.
#
# You only need ONE announce hook, not both:
#   - pre-commit: the URL lands in the same commit, thread renders on your
#     next build.
#   - pre-push (this one): announces at push time, scoped to the commits you
#     are actually pushing, the URL lands in a follow-up commit.
# Install whichever fits your workflow. They share the same statiquepouet.*
# config, so switching between them needs no reconfiguration.
#
# git offers no client-side post-push hook, so this fires just before the
# transfer. It stays conservative to match a successful push: it looks only at
# the commits this push delivers and skips a non-fast-forward ref (which the
# remote would reject anyway), but a push rejected server-side after the hook
# passes can still leave a toot out. If that risk matters, announce from CI
# instead, which runs only once the push has landed.
#
# Install it into your site (from the site root):
#
#   cp themes/statiquepouet-hugo/announce/announce.pre-push.sh .git/hooks/pre-push
#   chmod +x .git/hooks/pre-push
#
# Configure it with git config, exactly like the pre-commit hook (see the
# README for the full list):
#
#   git config --local statiquepouet.token <write:statuses token>   # secret, stays in .git/config
#   git config statiquepouet.instance mastodon.example
#   git config statiquepouet.visibility unlisted                    # optional
#
# Turn it off without uninstalling:   git config statiquepouet.enabled 0
# Skip rebuilding on every push:      git config statiquepouet.binary /path/to/announce
#
# Requires `go` in PATH unless statiquepouet.binary points at a prebuilt
# binary (Hugo is embedded as a library). The hook never blocks your push:
# if it is disabled, has no content to announce, or is not configured,
# it exits 0.

# Master switch. Absent config means enabled.
TRUE=1
FALSE=0
HOOKS_ENABLED="$(git config --int --get statiquepouet.enabled 2>/dev/null || echo "$TRUE")"
[ "$HOOKS_ENABLED" -eq "$FALSE" ] && exit 0

# The front-matter key that marks an announceable article. Overridable for
# forks that renamed it; it must match the key your theme partials and the
# announce program use (both default to fediverseContext).
context_key="$(git config --get statiquepouet.contextKey 2>/dev/null || true)"
[ -n "$context_key" ] || context_key="fediverseContext"
empty_ctx="^${context_key}[[:space:]]*[:=][[:space:]]*(\"\"|'')?[[:space:]]*\$"

# The content markdown files this push actually delivers. Reads the ref
# updates git feeds on stdin (local_ref local_sha remote_ref remote_sha), so
# we only ever announce what is coming, not the whole working tree.
pushed_content() {
  while read -r _local_ref local_sha _remote_ref remote_sha; do
    # A deletion pushes an all-zero local sha: nothing to announce.
    case "$local_sha" in *[!0]*) ;; *) continue ;; esac
    case "$remote_sha" in
      *[!0]*)
        # The remote already has this ref. Only a fast-forward will land, so
        # skip a non-fast-forward push rather than toot for a rejected one.
        if git merge-base --is-ancestor "$remote_sha" "$local_sha" 2>/dev/null; then
          git log --name-only --pretty=format: --diff-filter=d "$remote_sha..$local_sha" -- content 2>/dev/null
        fi
        ;;
      *)
        # All zeros: a new ref on the remote. "Coming" is whatever is not yet
        # on any remote-tracking branch, never the entire history.
        git log --name-only --pretty=format: --diff-filter=d "$local_sha" --not --remotes -- content 2>/dev/null
        ;;
    esac
  done
}

# Cheap guard: only pay for the Hugo build when a pushed markdown file under
# content/ carries an empty fediverseContext, i.e. an article awaiting its
# toot. The whole file is inspected, not just the lines this push changed, so
# an article still awaiting its toot triggers even when the commit touched
# only its body.
# Resolve the list with the default IFS so `read` inside pushed_content still
# splits the space-separated ref fields, then iterate on newlines only.
pushed_files="$(pushed_content | sort -u)"

new_content="$FALSE"
oldifs=$IFS
IFS='
'
for f in $pushed_files; do
  case "$f" in
    *.md) ;;
    *) continue ;;
  esac
  [ -f "$f" ] || continue
  if grep -Eq "$empty_ctx" "$f" 2>/dev/null; then
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

# A prebuilt binary (statiquepouet.binary) skips the go build on every push,
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
[ "$rc" -eq 0 ] || echo "announce: program exited $rc, recording whatever it wrote back" >&2

# Record only the files the program just wrote back (worktree entries that
# appeared since `before`) as a partial follow-up commit, never sweeping in
# unrelated working-tree edits. --no-verify keeps this automated commit from
# re-triggering the announce/lint hooks. The thread renders on your next push.
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
    # word-split $touched (IFS=newline) so each path is a separate pathspec
    # shellcheck disable=SC2086
    if git commit --no-verify -m "chore(announce): record fediverse thread URLs" -- $touched >/dev/null 2>&1; then
      echo "announce: recorded thread URLs in a follow-up commit, push again to render them"
    else
      echo "announce: could not commit the thread URLs, they wait in your working tree" >&2
    fi
    IFS=$oldifs
  fi
fi

exit 0
