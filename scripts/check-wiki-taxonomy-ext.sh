#!/usr/bin/env bash
# Verifies the wiki-taxonomy extension survived a merge from upstream:
#   * every one-line hook in a core file is still there,
#   * the extension's own files exist,
#   * (informational) which files differ from the base ref outside the extension.
#
# Usage: scripts/check-wiki-taxonomy-ext.sh [--test] [--base <ref>]
#   --test        also run the extension's Go and frontend tests
#   --base <ref>  ref to compare against for the stray-edit report (default upstream/main)
#
# See docs/extensions/wiki-taxonomy.md.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

MARK='\[wiki-taxonomy-ext\]'
run_tests=0
base=upstream/main
while [ $# -gt 0 ]; do
  case "$1" in
    --test) run_tests=1 ;;
    --base) base="${2:?--base needs a ref}"; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done

# file:expected number of marked hook lines
hooks=(
  "internal/types/wiki_page.go:1"
  "internal/types/prompt_instructions.go:1"
  "internal/application/service/wiki_ingest_taxonomy.go:1"
  "internal/application/service/wiki_ingest_batch.go:1"
  "internal/application/service/knowledgebase.go:1"
  "internal/router/router.go:1"
  "frontend/src/views/knowledge/KnowledgeBaseEditorModal.vue:7"
)

# files owned by the extension (a trailing / means a directory)
owned=(
  "internal/types/wiki_taxonomy.go"
  "internal/types/wiki_taxonomy_test.go"
  "internal/agent/prompts_wiki_template.go"
  "internal/application/service/wiki_ingest_taxonomy_template.go"
  "internal/application/service/wiki_ingest_taxonomy_template_test.go"
  "internal/handler/wiki_taxonomy_presets.go"
  "internal/handler/wiki_taxonomy_presets_test.go"
  "internal/router/routes_wiki_taxonomy.go"
  "frontend/src/extensions/wikiTaxonomy/"
  "docs/extensions/wiki-taxonomy.md"
  "scripts/check-wiki-taxonomy-ext.sh"
)

fail=0

echo "== hooks in core files"
for entry in "${hooks[@]}"; do
  file="${entry%%:*}"; want="${entry##*:}"
  have=$(grep -cE "$MARK" "$file" || true)
  if [ "$have" -eq "$want" ]; then
    printf '  ok       %s (%s)\n' "$file" "$have"
  else
    printf '  MISSING  %s: found %s hook(s), expected %s\n' "$file" "$have" "$want"
    fail=1
  fi
done

echo "== extension files"
for path in "${owned[@]}"; do
  if [ -e "$path" ]; then printf '  ok       %s\n' "$path"; else printf '  MISSING  %s\n' "$path"; fail=1; fi
done

echo "== files that differ from $base outside the extension"
if git rev-parse --verify -q "$base" >/dev/null; then
  allowed=$(printf '%s\n' "${owned[@]}" "${hooks[@]%%:*}")
  stray=0
  while IFS= read -r changed; do
    skip=0
    while IFS= read -r a; do
      case "$a" in */) [[ "$changed" == "$a"* ]] && skip=1 ;; *) [ "$changed" = "$a" ] && skip=1 ;; esac
    done <<< "$allowed"
    if [ "$skip" -eq 0 ]; then printf '  stray    %s\n' "$changed"; stray=1; fi
  done < <(git diff --name-only "$base"...HEAD)
  [ "$stray" -eq 0 ] && echo "  none"
else
  echo "  (skipped: $base not found, run: git fetch upstream)"
fi

if [ "$fail" -ne 0 ]; then
  echo "FAILED: re-apply the missing hooks, see docs/extensions/wiki-taxonomy.md" >&2
  exit 1
fi

if [ "$run_tests" -eq 1 ]; then
  echo "== go tests"
  go test ./internal/types/ ./internal/handler/ ./internal/router/ ./internal/agent/ \
    && go test ./internal/application/service/ \
         -run 'Taxonomy|SeedTaxonomy|FileSummary|ApplyPlannedFolder|PlanTemplate'
  echo "== frontend"
  (cd frontend && npx tsx --test src/extensions/wikiTaxonomy/*.test.ts && npx vue-tsc --noEmit -p tsconfig.app.json)
fi

echo "OK"
