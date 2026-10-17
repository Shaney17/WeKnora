package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"text/template"
)

// folderRecorder stands in for the wiki page service: it records the folder
// paths asked for and otherwise panics, so a test fails loudly if the code
// under test reaches for anything else.
type folderRecorder struct {
	interfaces.WikiPageService
	created [][]string
	failOn  string

	// Existing wiki content, for the curation pass.
	live      map[string]bool
	existing  map[string][]types.WikiIndexEntry // by page type
	existsErr error
}

func (f *folderRecorder) ExistsSlugs(_ context.Context, _ string, slugs []string) (map[string]bool, error) {
	if f.existsErr != nil {
		return nil, f.existsErr
	}
	out := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		out[s] = f.live[s]
	}
	return out, nil
}

func (f *folderRecorder) ListByTypeRecent(_ context.Context, _ string, pageType string, _ int) ([]types.WikiIndexEntry, error) {
	return f.existing[pageType], nil
}

func (f *folderRecorder) FindOrCreateFolderPath(
	_ context.Context, _ string, _ uint64, path []string,
) (string, []string, error) {
	if f.failOn != "" && strings.Join(path, "/") == f.failOn {
		return "", nil, context.DeadlineExceeded
	}
	f.created = append(f.created, path)
	return "id:" + strings.Join(path, "/"), path, nil
}

func (f *folderRecorder) ListDistinctCategoryPaths(context.Context, string, int) ([][]string, error) {
	return nil, nil
}

func presetTaxonomy(t *testing.T) *types.WikiTaxonomy {
	t.Helper()
	tax, ok := types.WikiTaxonomyPreset(types.WikiTaxonomyPresetITProduct)
	if !ok {
		t.Fatal("preset missing")
	}
	return tax
}

func TestSeedTaxonomyFoldersCreatesOnlyMissing(t *testing.T) {
	rec := &folderRecorder{}
	svc := &wikiIngestService{wikiService: rec}
	kb := &types.KnowledgeBase{ID: "kb1", TenantID: 7}
	tax := presetTaxonomy(t)

	pool := [][]string{{"Product"}, {"technical"}, {"Product", "Trading"}}
	got := svc.seedTaxonomyFolders(context.Background(), kb, tax, pool)

	created := map[string]bool{}
	for _, p := range rec.created {
		created[strings.Join(p, "/")] = true
	}
	if created["Product"] || created["Technical"] {
		t.Fatalf("existing folders must not be created again (case-insensitive): %v", rec.created)
	}
	for _, want := range []string{"Technical/Services", "Technical/Data", "Sources", "Unsorted"} {
		if !created[want] {
			t.Fatalf("expected %q to be seeded, created = %v", want, rec.created)
		}
	}
	if len(got) != len(pool)+len(rec.created) {
		t.Fatalf("returned pool has %d entries, want %d", len(got), len(pool)+len(rec.created))
	}
}

func TestSeedTaxonomyFoldersSurvivesAFailure(t *testing.T) {
	rec := &folderRecorder{failOn: "Technical/Data"}
	svc := &wikiIngestService{wikiService: rec}
	got := svc.seedTaxonomyFolders(context.Background(),
		&types.KnowledgeBase{ID: "kb1"}, presetTaxonomy(t), nil)

	for _, p := range got {
		if strings.Join(p, "/") == "Technical/Data" {
			t.Fatal("a folder that failed to create must not be reported as present")
		}
	}
	if len(got) == 0 {
		t.Fatal("one failing folder must not stop the others from being seeded")
	}
}

func TestFileSummaryPages(t *testing.T) {
	tax := presetTaxonomy(t)
	result := map[string][]string{"entity/a": {"Product"}}
	fileSummaryPages(result, map[string][]SlugUpdate{
		"summary/1": {{Type: types.WikiPageTypeSummary}},
		"entity/a":  {{Type: types.WikiPageTypeEntity}},
		"retract/x": {{Type: "retract"}},
	}, tax)

	if got := strings.Join(result["summary/1"], "/"); got != "Sources" {
		t.Fatalf("summary path = %q, want Sources", got)
	}
	if got := strings.Join(result["entity/a"], "/"); got != "Product" {
		t.Fatalf("entity assignment was disturbed: %q", got)
	}
	if _, ok := result["retract/x"]; ok {
		t.Fatal("retract-only slugs must not be filed")
	}

	result["summary/1"][0] = "Changed"
	if tax.SummaryPath[0] != "Sources" {
		t.Fatal("the summary assignment aliases the template's SummaryPath")
	}
}

func TestFileSummaryPagesNoShelfLeavesRootAlone(t *testing.T) {
	tax := &types.WikiTaxonomy{Roots: []types.WikiTaxonomyNode{{Name: "A"}}}
	result := map[string][]string{}
	fileSummaryPages(result, map[string][]SlugUpdate{"summary/1": {{Type: types.WikiPageTypeSummary}}}, tax)
	if len(result) != 0 {
		t.Fatalf("without a SummaryPath nothing should be filed: %v", result)
	}
}

func TestApplyPlannedFolderKeepsExistingPlacement(t *testing.T) {
	batch := &WikiBatchContext{PlannedFolderID: map[string]string{"s": "planned"}}

	fresh := &types.WikiPage{}
	applyPlannedFolder(fresh, batch, "s")
	if fresh.FolderID != "planned" {
		t.Fatalf("unfiled page FolderID = %q, want planned", fresh.FolderID)
	}

	moved := &types.WikiPage{FolderID: "by-hand"}
	applyPlannedFolder(moved, batch, "s")
	if moved.FolderID != "by-hand" {
		t.Fatalf("a page the user already filed was moved to %q", moved.FolderID)
	}

	applyPlannedFolder(&types.WikiPage{}, nil, "s") // must not panic
}

func TestFormatTaxonomyTemplateForPrompt(t *testing.T) {
	if got := formatTaxonomyTemplateForPrompt(nil); got != "" {
		t.Fatalf("nil template should render nothing, got %q", got)
	}
	got := formatTaxonomyTemplateForPrompt(presetTaxonomy(t))
	for _, want := range []string{
		"<fixed_template>", "</fixed_template>",
		"- Product [open]", "- Technical [fixed]", "  - Services [fixed]",
		`"Sources"`, `"Unsorted"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered template is missing %q:\n%s", want, got)
		}
	}
}

// The extension's prompt is a Go template: the template block is injected as
// data and must reach the model verbatim, with the level limit of the template.
func TestTemplateTaxonomyPlanPromptRendering(t *testing.T) {
	parsed, err := template.New("p").Parse(agent.WikiTemplateTaxonomyPlanPrompt)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var b strings.Builder
	err = parsed.Execute(&b, map[string]string{
		"MaxLevels":        "3",
		"Language":         "English",
		"TaxonomyTemplate": formatTaxonomyTemplateForPrompt(presetTaxonomy(t)),
		"ExistingTaxonomy": "Product\n  Trading",
		"Items":            "- slug: entity/x | title: X | type: entity | about: a",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := b.String()
	for _, want := range []string{"(at most 3 levels)", "- Technical [fixed]", "<existing_folders>", "entity/x", "in English"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered prompt is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<no value>") {
		t.Fatalf("prompt rendered an unset placeholder:\n%s", got)
	}
}

// Wiring contract with the core: a KB without a template is not handled by the
// extension, so the core planner runs exactly as before.
func TestPlanTemplateTaxonomyIgnoresKBsWithoutTemplate(t *testing.T) {
	svc := &wikiIngestService{}
	for name, kb := range map[string]*types.KnowledgeBase{
		"nil kb":         nil,
		"no wiki config": {ID: "kb"},
		"empty config":   {ID: "kb", WikiConfig: &types.WikiConfig{}},
		"unknown preset": {ID: "kb", WikiConfig: &types.WikiConfig{WikiTaxonomyConfig: types.WikiTaxonomyConfig{TaxonomyPreset: "nope"}}},
		"empty taxonomy": {ID: "kb", WikiConfig: &types.WikiConfig{WikiTaxonomyConfig: types.WikiTaxonomyConfig{Taxonomy: &types.WikiTaxonomy{}}}},
	} {
		t.Run(name, func(t *testing.T) {
			plan, handled := svc.planTemplateTaxonomy(context.Background(), nil, kb, nil, "English")
			if handled || plan != nil {
				t.Fatalf("handled=%v plan=%v, want the core planner to take over", handled, plan)
			}
		})
	}
}

// With a template the extension plans a summary-only batch itself (no LLM call
// is needed): the fixed folders get seeded and the summary lands on its shelf.
func TestPlanTemplateTaxonomySummaryOnlyBatch(t *testing.T) {
	rec := &folderRecorder{}
	svc := &wikiIngestService{wikiService: rec}
	kb := &types.KnowledgeBase{ID: "kb1", TenantID: 1, WikiConfig: &types.WikiConfig{
		WikiTaxonomyConfig: types.WikiTaxonomyConfig{TaxonomyPreset: types.WikiTaxonomyPresetITProduct},
	}}
	plan, handled := svc.planTemplateTaxonomy(context.Background(), nil, kb,
		map[string][]SlugUpdate{"summary/1": {{Type: types.WikiPageTypeSummary}}}, "English")
	if !handled {
		t.Fatal("a KB with a template must be handled by the extension")
	}
	if got := strings.Join(plan["summary/1"], "/"); got != "Sources" {
		t.Fatalf("summary path = %q, want Sources", got)
	}
	if len(rec.created) == 0 {
		t.Fatal("the template's fixed folders should have been seeded")
	}
}
