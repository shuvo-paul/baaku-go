package views_test

import (
	"bytes"
	"testing"

	"github.com/shuvo-paul/baaku/internal/views"
)

func TestCommitteeAdminPagesRender(t *testing.T) {
	sb := views.NewSidebarData("Baaku", "tok", "U", "u@x", "/dashboard/committee", "active", []string{"manage committee"})
	var buf bytes.Buffer
	err := views.CommitteeIndexPage(views.CommitteeIndexData{
		Sidebar: sb,
		Rows:    []views.CommitteeMemberRow{{ID: 1, PositionName: "সভাপতি", DisplayName: "লাবণ্য", PhotoURL: "/media/committee-photos/a.jpg"}},
		CSRF:    "tok",
	}).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("CommitteeIndexPage: %v", err)
	}
	for _, want := range []string{"/dashboard/committee/reorder", "sortable.esm.js", "লাবণ্য", "Drag to reorder"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("index missing %q", want)
		}
	}

	buf.Reset()
	err = views.CommitteeMemberFormPage(views.CommitteeMemberForm{
		Sidebar: sb, Title: "New Member",
		Positions: []views.Option{{Value: "", Label: "Select a position"}, {Value: "1", Label: "সভাপতি"}},
		Errors:    map[string]string{},
		CSRF:      "tok",
	}).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("CommitteeMemberFormPage: %v", err)
	}
	for _, want := range []string{"memberType", "/dashboard/committee/users/search", "photoCropper", "Select a position"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("form missing %q", want)
		}
	}

	buf.Reset()
	err = views.PositionsIndexPage(views.PositionsIndexData{
		Sidebar: sb,
		Rows:    []views.PositionRow{{ID: 1, Name: "সভাপতি", MemberCount: 2}},
		CSRF:    "tok",
	}).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("PositionsIndexPage: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("2 members")) {
		t.Error("positions index missing member count")
	}
}
