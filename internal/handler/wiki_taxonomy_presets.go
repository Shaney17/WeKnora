package handler

// Wiki-taxonomy extension: the read-only list of built-in directory templates.
// Lives in its own file (a method on KnowledgeBaseHandler may sit in any file of
// the package) so the large core handler stays untouched.
// See docs/extensions/wiki-taxonomy.md.

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// GetWikiTaxonomyPresets lists the built-in wiki directory templates. The KB
// editor copies one into a knowledge base as a starting point and lets the user
// edit it; the saved copy, not the preset, is what ingest follows. Read-only and
// global (no KB id), so only the Viewer role is required.
func (h *KnowledgeBaseHandler) GetWikiTaxonomyPresets(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"presets": types.WikiTaxonomyPresets()},
	})
}
