package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// The editor copies a preset straight into a KB and saves it, so every preset
// the endpoint serves must already pass the same validation the save applies.
func TestGetWikiTaxonomyPresetsServesValidTemplates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/wiki-taxonomy/presets", nil)

	(&KnowledgeBaseHandler{}).GetWikiTaxonomyPresets(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Presets []types.WikiTaxonomyPresetInfo `json:"presets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Success || len(body.Data.Presets) == 0 {
		t.Fatalf("expected at least one preset, got %+v", body)
	}
	for _, p := range body.Data.Presets {
		if p.ID == "" {
			t.Fatal("preset without an id")
		}
		cfg := &types.WikiConfig{WikiTaxonomyConfig: types.WikiTaxonomyConfig{Taxonomy: p.Taxonomy}}
		if err := types.ValidateWikiTaxonomy(cfg); err != nil {
			t.Fatalf("preset %q does not validate: %v", p.ID, err)
		}
	}
}
