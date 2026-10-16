package types

import (
	"fmt"
	"strings"
)

// WikiTaxonomyPresetITProduct is the built-in directory template for IT product
// knowledge bases: business knowledge ("Product") kept apart from engineering
// knowledge ("Technical"), with one fixed "Sources" shelf for per-document
// summaries. Every KB that selects the same preset ends up with the same tree,
// which is the point: reviewers meet the same layout in every wiki.
const WikiTaxonomyPresetITProduct = "it_product"

const (
	wikiTaxonomyMaxNodes       = 60
	wikiTaxonomyMaxDescription = 300
)

// WikiTaxonomyNode is one folder of a wiki directory template.
type WikiTaxonomyNode struct {
	// Name is the exact folder label. Template labels are never translated or
	// re-spelled by the planner, so two KBs always get byte-identical folders.
	Name string `json:"name" yaml:"name"`
	// Description tells the planner what belongs here. It is prompt guidance
	// only and is not stored on the folder.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Children are the fixed sub-folders. They are created up front and the
	// planner may only choose among them.
	Children []WikiTaxonomyNode `json:"children,omitempty" yaml:"children,omitempty"`
	// Open lets the planner add its own sub-folders below this node (reusing
	// existing ones first), up to WikiCategoryMaxDepth. Use it for the part of
	// the tree that is domain-specific, such as one folder per product line.
	Open bool `json:"open,omitempty" yaml:"open,omitempty"`
}

// WikiTaxonomy is a directory template applied to a wiki knowledge base. When a
// KB has one, the ingest pipeline creates its fixed folders, constrains the LLM
// planner to it, and normalizes whatever the planner returns back onto it.
type WikiTaxonomy struct {
	Roots []WikiTaxonomyNode `json:"roots" yaml:"roots"`
	// SummaryPath is where per-document summary pages are filed. Empty leaves
	// them at the wiki root, as before.
	SummaryPath []string `json:"summary_path,omitempty" yaml:"summary_path,omitempty"`
	// FallbackPath receives entity/concept pages the planner could not place
	// inside the template. Empty leaves them unfiled.
	FallbackPath []string `json:"fallback_path,omitempty" yaml:"fallback_path,omitempty"`
}

// WikiTaxonomyPresetInfo is a built-in template as the KB editor lists it: the
// starting point a user copies into a KB and then edits.
type WikiTaxonomyPresetInfo struct {
	ID       string        `json:"id"`
	Taxonomy *WikiTaxonomy `json:"taxonomy"`
}

// WikiTaxonomyPresets lists the built-in templates, each a fresh copy.
func WikiTaxonomyPresets() []WikiTaxonomyPresetInfo {
	return []WikiTaxonomyPresetInfo{
		{ID: WikiTaxonomyPresetITProduct, Taxonomy: itProductTaxonomy()},
	}
}

// WikiTaxonomyPreset returns a fresh copy of the named built-in template.
func WikiTaxonomyPreset(name string) (*WikiTaxonomy, bool) {
	switch strings.TrimSpace(name) {
	case WikiTaxonomyPresetITProduct:
		return itProductTaxonomy(), true
	default:
		return nil, false
	}
}

func itProductTaxonomy() *WikiTaxonomy {
	return &WikiTaxonomy{
		Roots: []WikiTaxonomyNode{
			{
				Name: "Product",
				Description: "Business-facing knowledge: what the product does and for whom. Features, user flows, " +
					"business rules, fees and policies, user-visible behaviour, screens and permissions.",
				Open: true,
			},
			{
				Name: "Technical",
				Description: "Engineering knowledge: how the product is built and run. Use ONLY the fixed " +
					"sub-folders below.",
				Children: []WikiTaxonomyNode{
					{Name: "Services", Description: "Systems, services and components and what each is responsible for."},
					{Name: "Interfaces", Description: "APIs, events, messages, topics, payload contracts and integration points."},
					{Name: "Data", Description: "Tables, schemas, data models and the lifecycle of stored records."},
					{Name: "Configuration", Description: "Configuration parameters, rate tables, calculation and rounding rules, feature flags."},
					{Name: "Operations", Description: "Incidents, exception handling, runbooks, troubleshooting and known issues."},
				},
			},
			{
				Name:        "Sources",
				Description: "One summary page per source document. Never place entities or concepts here.",
			},
			{
				Name:        "Unsorted",
				Description: "Only for items that clearly fit nowhere else.",
			},
		},
		SummaryPath:  []string{"Sources"},
		FallbackPath: []string{"Unsorted"},
	}
}

// WikiTaxonomyConfig is the wiki-taxonomy extension's per-KB setting. WikiConfig
// embeds it with a single line, so the extension adds one line to the core
// struct instead of fields scattered through it; encoding/json inlines the
// embedded fields, so the stored JSON and the API payload stay flat
// ("taxonomy_preset", "taxonomy" next to the core wiki_config keys).
//
// Call it through the nil-safe helpers below (WikiTaxonomyOf and friends): a
// method promoted through the embedded value would panic on a nil *WikiConfig.
type WikiTaxonomyConfig struct {
	// TaxonomyPreset names a built-in directory template (see
	// WikiTaxonomyPresetITProduct). Empty keeps free-form planning.
	TaxonomyPreset string `yaml:"taxonomy_preset,omitempty" json:"taxonomy_preset,omitempty"`
	// Taxonomy is a custom directory template. It takes precedence over
	// TaxonomyPreset when it has at least one top-level folder.
	Taxonomy *WikiTaxonomy `yaml:"taxonomy,omitempty" json:"taxonomy,omitempty"`
}

// WikiTaxonomyOf returns the directory template in force for this KB: an
// explicit Taxonomy wins over a named preset, and nil means free-form planning
// (the behaviour before templates existed). Safe on a nil config.
func WikiTaxonomyOf(c *WikiConfig) *WikiTaxonomy {
	if c == nil {
		return nil
	}
	if c.Taxonomy != nil && len(c.Taxonomy.Roots) > 0 {
		return c.Taxonomy
	}
	if t, ok := WikiTaxonomyPreset(c.TaxonomyPreset); ok {
		return t
	}
	return nil
}

// ValidateWikiTaxonomy rejects an unknown preset or a malformed custom template.
func ValidateWikiTaxonomy(c *WikiConfig) error {
	if c == nil {
		return nil
	}
	if p := strings.TrimSpace(c.TaxonomyPreset); p != "" {
		if _, ok := WikiTaxonomyPreset(p); !ok {
			return fmt.Errorf("unknown wiki taxonomy preset %q", p)
		}
	}
	if c.Taxonomy != nil && len(c.Taxonomy.Roots) > 0 {
		return c.Taxonomy.Validate()
	}
	return nil
}

// MergeWikiTaxonomy is applied when a KB update replaces the wiki config
// wholesale. A client that does not know about the extension (an older UI, the
// API, an assistant) sends neither "taxonomy" nor "taxonomy_preset"; without this
// its save would silently erase the folder layout. Clearing it is explicit: send
// "taxonomy": {"roots": []} (or a different layout).
func MergeWikiTaxonomy(prev, next *WikiConfig) *WikiConfig {
	if next == nil || prev == nil {
		return next
	}
	if next.Taxonomy == nil && strings.TrimSpace(next.TaxonomyPreset) == "" {
		next.WikiTaxonomyConfig = prev.WikiTaxonomyConfig
	}
	return next
}

// Validate checks the template is something the ingest pipeline can honour:
// labels that survive path cleaning, unique siblings, a bounded size, and
// summary/fallback paths that point at fixed folders.
func (t *WikiTaxonomy) Validate() error {
	if t == nil || len(t.Roots) == 0 {
		return fmt.Errorf("wiki taxonomy needs at least one top-level folder")
	}
	count := 0
	if err := validateTaxonomyNodes(t.Roots, 1, &count); err != nil {
		return err
	}
	if count > wikiTaxonomyMaxNodes {
		return fmt.Errorf("wiki taxonomy has %d folders, the limit is %d", count, wikiTaxonomyMaxNodes)
	}
	for name, path := range map[string][]string{"summary_path": t.SummaryPath, "fallback_path": t.FallbackPath} {
		if len(path) == 0 {
			continue
		}
		if !t.hasFixedPath(path) {
			return fmt.Errorf("wiki taxonomy %s %q is not a folder of the template", name, strings.Join(path, "/"))
		}
	}
	return nil
}

func validateTaxonomyNodes(nodes []WikiTaxonomyNode, depth int, count *int) error {
	if depth > WikiCategoryMaxDepth {
		return fmt.Errorf("wiki taxonomy is deeper than %d levels", WikiCategoryMaxDepth)
	}
	seen := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		*count++
		name := strings.TrimSpace(n.Name)
		if name == "" {
			return fmt.Errorf("wiki taxonomy folder name must not be empty")
		}
		if cleaned := CleanWikiCategoryPart(name); len(cleaned) != 1 || cleaned[0] != name {
			return fmt.Errorf("wiki taxonomy folder name %q is not usable as a folder label", n.Name)
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			return fmt.Errorf("wiki taxonomy has two sibling folders named %q", name)
		}
		seen[key] = struct{}{}
		if len([]rune(n.Description)) > wikiTaxonomyMaxDescription {
			return fmt.Errorf("wiki taxonomy folder %q description exceeds %d characters", name, wikiTaxonomyMaxDescription)
		}
		if n.Open && depth >= WikiCategoryMaxDepth {
			return fmt.Errorf("wiki taxonomy folder %q cannot be open: it is already at the maximum depth", name)
		}
		if err := validateTaxonomyNodes(n.Children, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func (t *WikiTaxonomy) hasFixedPath(path []string) bool {
	nodes := t.Roots
	for _, label := range path {
		n := findTaxonomyChild(nodes, label)
		if n == nil {
			return false
		}
		nodes = n.Children
	}
	return true
}

func findTaxonomyChild(nodes []WikiTaxonomyNode, label string) *WikiTaxonomyNode {
	label = strings.TrimSpace(label)
	for i := range nodes {
		if strings.EqualFold(strings.TrimSpace(nodes[i].Name), label) {
			return &nodes[i]
		}
	}
	return nil
}

// MaxDepth is how many folder levels a path under this template can have: the
// deepest fixed branch, or WikiCategoryMaxDepth when any node is open.
func (t *WikiTaxonomy) MaxDepth() int {
	if t == nil {
		return 0
	}
	return taxonomyNodesDepth(t.Roots, 1)
}

func taxonomyNodesDepth(nodes []WikiTaxonomyNode, depth int) int {
	best := 0
	for _, n := range nodes {
		d := depth
		switch {
		case n.Open:
			d = WikiCategoryMaxDepth
		case len(n.Children) > 0:
			d = taxonomyNodesDepth(n.Children, depth+1)
		}
		if d > best {
			best = d
		}
	}
	return best
}

// FixedPaths lists every fixed folder of the template, parents before
// children, so seeding them in order always has the parent in place.
func (t *WikiTaxonomy) FixedPaths() [][]string {
	if t == nil {
		return nil
	}
	var out [][]string
	var walk func(nodes []WikiTaxonomyNode, prefix []string)
	walk = func(nodes []WikiTaxonomyNode, prefix []string) {
		for _, n := range nodes {
			path := append(append([]string(nil), prefix...), strings.TrimSpace(n.Name))
			out = append(out, path)
			walk(n.Children, path)
		}
	}
	walk(t.Roots, nil)
	return out
}

// Conform maps a path proposed by the planner onto the template, so the stored
// tree can never drift from it however the LLM words its answer:
//
//   - the first label must be a top-level folder, matched case-insensitively and
//     rewritten to the template spelling; otherwise the item goes to FallbackPath
//     (or stays unfiled when there is none);
//   - below a fixed node only listed children are accepted; an unknown label cuts
//     the path back to its deepest valid ancestor;
//   - below an open node the planner's own labels are kept, up to the depth cap.
func (t *WikiTaxonomy) Conform(path []string) []string {
	if t == nil {
		return path
	}
	clean := CleanWikiCategoryPath(path)
	if len(clean) == 0 {
		return t.fallback()
	}
	nodes := t.Roots
	var parent *WikiTaxonomyNode
	out := make([]string, 0, WikiCategoryMaxDepth)
	for i, label := range clean {
		n := findTaxonomyChild(nodes, label)
		if n == nil {
			if i == 0 {
				return t.fallback()
			}
			if parent != nil && parent.Open {
				out = append(out, clean[i:]...)
			}
			break
		}
		out = append(out, strings.TrimSpace(n.Name))
		parent, nodes = n, n.Children
	}
	if len(out) > WikiCategoryMaxDepth {
		out = out[:WikiCategoryMaxDepth]
	}
	return out
}

func (t *WikiTaxonomy) fallback() []string {
	if len(t.FallbackPath) == 0 {
		return nil
	}
	return append([]string(nil), t.FallbackPath...)
}
