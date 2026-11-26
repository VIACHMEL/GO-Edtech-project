package boardhttp

import (
	"strings"
	"testing"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

func TestPresentConditionKeepsTableAndReplacesImageSource(t *testing.T) {
	condition := db.TaskCondition{
		HTML: `<script>alert(1)</script><table onclick="bad()"><tr><td colspan="2"><img src="https://outside.test/a" alt="Ось" onerror="bad()"></td></tr></table><a href="javascript:bad()">Текст</a>`,
		Artifacts: []db.ConditionArtifact{
			{ID: 7, Kind: "image", Key: "image_1", MIME: "image/png"},
			{ID: 9, Kind: "attachment", Key: "file_1", Name: "Данные.ods"},
		},
	}
	result := PresentCondition(condition, "/materials/")
	for _, want := range []string{`<table>`, `<td colspan="2">`, `src="/materials/7" alt="Ось"`, "Текст"} {
		if !strings.Contains(result.HTML, want) {
			t.Errorf("разметка потеряла %q", want)
		}
	}
	for _, forbidden := range []string{"outside.test", "onclick", "onerror", "javascript:", "<script", "alert"} {
		if strings.Contains(result.HTML, forbidden) {
			t.Errorf("разметка содержит %q", forbidden)
		}
	}
	if len(result.Materials) != 2 || result.Materials[1].URL != "/materials/9" || result.Materials[1].Name != "Данные.ods" {
		t.Fatalf("потеряно вложение: %+v", result.Materials)
	}
}
