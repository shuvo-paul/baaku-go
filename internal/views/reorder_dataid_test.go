package views_test

import (
	"strings"
	"testing"

	"github.com/shuvo-paul/baaku/internal/views"
)

// Regression: data-id must render the bare id. templ.JSONString JSON-encodes
// the int, so data-id became &#34;123&#34; and row.dataset.id was the string
// `"123"` (with quotes). decodeReorderIDs parsed that as int 0, so the UPDATE
// matched no row and a drag snapped back on reload. Reported as: drag and drop
// reordering isn't working.
func TestCommitteeRowDataIDIsBareID(t *testing.T) {
	comp := views.CommitteeIndexPage(views.CommitteeIndexData{
		Rows: []views.CommitteeMemberRow{
			{ID: 123, DisplayName: "A", PositionName: "P"},
			{ID: 456, DisplayName: "B", PositionName: "P"},
		},
		CSRF: "tok",
	})
	var b strings.Builder
	if err := comp.Render(t.Context(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := b.String()
	for _, want := range []string{`data-id="123"`, `data-id="456"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(html, "&#34;") {
		t.Error("data-id is JSON-quoted; dataset.id would carry stray quotes")
	}
}

// Same bare-id requirement for the plans reorder table.
func TestPlanRowDataIDIsBareID(t *testing.T) {
	comp := views.AdminPlansPage(views.AdminPlansData{
		Rows: []views.AdminPlanRow{{ID: 123, Name: "A"}},
		CSRF: "tok",
	})
	var b strings.Builder
	if err := comp.Render(t.Context(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := b.String()
	if !strings.Contains(html, `data-id="123"`) {
		t.Error("missing data-id=\"123\"")
	}
	if strings.Contains(html, "&#34;") {
		t.Error("data-id is JSON-quoted; dataset.id would carry stray quotes")
	}
}
