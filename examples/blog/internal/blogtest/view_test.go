package blogtest

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blog/internal/blog"
)

func renderPage(t *testing.T, page string, data any) string {
	t.Helper()
	rec := httptest.NewRecorder()
	blog.Render(nil, rec, page, 200, data)
	if rec.Code != 200 {
		t.Fatalf("Render(%q) = %d, want 200:\n%s", page, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// Every page file defines "content" — the same name in every file.
// Parsed into one shared tree they would overwrite each other silently,
// last file wins, and the app would render the wrong screen with no
// error anywhere. This is the test that catches a lost clone.
func TestEachPageRendersItsOwnContent(t *testing.T) {
	index := renderPage(t, "index", blog.HomeView{Head: blog.Head{Title: "The blog"}})
	post := renderPage(t, "post", blog.PostView{
		Head: blog.Head{Title: "Release notes"}, Title: "Release notes",
		Date: "Published 2 August 2026", Paragraphs: []string{"One."},
	})
	adminNew := renderPage(t, "admin_new", blog.AdminFormView{
		Head: blog.Head{Title: "New post"}, Action: "/admin/posts",
	})

	wantContains(t, index, "Notes, in the order they were written.")
	wantContains(t, post, `<article class="blog-article">`)
	wantNotContains(t, post, "Notes, in the order they were written.")
	wantContains(t, adminNew, `<form class="blog-form" method="post" action="/admin/posts">`)
	wantNotContains(t, adminNew, `<article class="blog-article">`)
}

func TestLayoutWrapsEveryPage(t *testing.T) {
	html := renderPage(t, "index", blog.HomeView{Head: blog.Head{Title: "The blog"}})

	wantContains(t, html, `<html lang="en">`)
	wantContains(t, html, `<title>The blog · The blog</title>`)
	wantContains(t, html, `<link rel="stylesheet" href="/static/tokens.css">`)
	wantContains(t, html, `<link rel="stylesheet" href="/static/blog.css">`)
	wantContains(t, html, `<div class="rst-page">`)
	wantContains(t, html, `<footer class="blog-footer">`)
	wantNotContains(t, html, "<script")
}

// A template error is a clean 500, not half a page followed by garbage:
// Render executes into a buffer before it writes anything.
func TestRenderOfAnUnknownPageIs500WithNoPartialOutput(t *testing.T) {
	rec := httptest.NewRecorder()
	blog.Render(nil, rec, "no-such-page", 200, nil)

	if rec.Code != 500 {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	wantNotContains(t, rec.Body.String(), "<html")
}

func TestParagraphsSplitsOnBlankLines(t *testing.T) {
	got := blog.Paragraphs("One.\n\nTwo.\r\n\r\nThree.\n\n\n")
	want := []string{"One.", "Two.", "Three."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Paragraphs = %q, want %q", got, want)
	}
	if len(blog.Paragraphs("   ")) != 0 {
		t.Errorf("a blank body should produce no paragraphs")
	}
}

func TestMetaLinesFormatDatesInGo(t *testing.T) {
	when := time.Date(2026, 8, 2, 9, 30, 0, 0, time.UTC)
	if got, want := blog.FormatDate(when), "2 August 2026"; got != want {
		t.Errorf("FormatDate = %q, want %q", got, want)
	}
	if got, want := blog.PublishedLine(when), "Published 2 August 2026"; got != want {
		t.Errorf("PublishedLine = %q, want %q", got, want)
	}
	if got, want := blog.DraftLine(when), "Draft · edited 2 August 2026"; got != want {
		t.Errorf("DraftLine = %q, want %q", got, want)
	}
}

func TestAdminRowsGiveDraftsNoActionPill(t *testing.T) {
	when := time.Date(2026, 8, 2, 9, 30, 0, 0, time.UTC)
	rows := blog.AdminRows([]blog.Post{
		{ID: 1, Title: "Live", Published: true, CreatedAt: when, UpdatedAt: when},
		{ID: 2, Title: "Draft", Published: false, CreatedAt: when, UpdatedAt: when},
	})

	if rows[0].ActionHref != "/posts/1" || rows[0].ActionAria != "View Live" {
		t.Errorf("published row = %+v", rows[0])
	}
	if rows[1].ActionHref != "" {
		t.Errorf("a draft has no public page, so it gets no action pill: %+v", rows[1])
	}
	if rows[1].Href != "/admin/posts/2/edit" {
		t.Errorf("the row goes where the work is: %+v", rows[1])
	}
	// The lead marker stays unused: tinting an aria-hidden circle by
	// publish state would be colour-only status.
	for _, row := range rows {
		if strings.Contains(row.Sub, "aria") {
			t.Errorf("unexpected markup in a Sub line: %q", row.Sub)
		}
	}
}

func TestEditFormResolvesTheStatusPillPair(t *testing.T) {
	draft := blog.EditForm(blog.Post{ID: 7, Title: "Draft"})
	if draft.StatusTone != "neutral" || draft.StatusLabel != "Draft" {
		t.Errorf("draft pill = %q/%q", draft.StatusTone, draft.StatusLabel)
	}
	if draft.Action != "/admin/posts/7" || draft.Sub != "Editing" {
		t.Errorf("edit view = %+v", draft)
	}

	live := blog.EditForm(blog.Post{ID: 7, Title: "Live", Published: true})
	if live.StatusTone != "positive" || live.StatusLabel != "Published" {
		t.Errorf("published pill = %q/%q", live.StatusTone, live.StatusLabel)
	}
}

func TestPaginationIsHiddenAtOnePageAndShownBeyondIt(t *testing.T) {
	if got := blog.BuildPagination("/admin/posts", "", 1, 10); got.Show {
		t.Errorf("ten posts is one page; the strip must stay away")
	}
	got := blog.BuildPagination("/admin/posts", "", 1, 11)
	if !got.Show {
		t.Fatalf("eleven posts is two pages; the strip must appear")
	}
	want := []blog.PageItem{
		{Label: "Previous", Disabled: true},
		{Label: "1", Current: true},
		{Label: "2", Href: "/admin/posts?page=2"},
		{Label: "Next", Href: "/admin/posts?page=2"},
	}
	if len(got.Items) != len(want) {
		t.Fatalf("items = %+v, want %+v", got.Items, want)
	}
	for i := range want {
		if got.Items[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, got.Items[i], want[i])
		}
	}
}

func TestPaginationCarriesTheQueryAndDisablesNextOnTheLastPage(t *testing.T) {
	got := blog.BuildPagination("/admin/posts", "go", 2, 11)
	if got.Items[0] != (blog.PageItem{Label: "Previous", Href: "/admin/posts?q=go&page=1"}) {
		t.Errorf("Previous = %+v", got.Items[0])
	}
	last := got.Items[len(got.Items)-1]
	if last != (blog.PageItem{Label: "Next", Disabled: true}) {
		t.Errorf("Next on the last page = %+v", last)
	}
}

// A gap needs 71 posts to appear, so the app builds it correctly and
// never renders it — the library's own fixtures cover that item kind.
func TestPaginationWindowsWithGapsPastSevenPages(t *testing.T) {
	got := blog.BuildPagination("/", "", 5, 100) // ten pages
	var labels []string
	gaps := 0
	for _, item := range got.Items {
		if item.Gap {
			gaps++
			labels = append(labels, "…")
			continue
		}
		labels = append(labels, item.Label)
	}
	if gaps != 2 {
		t.Errorf("items = %v, want two gaps", labels)
	}
	if strings.Join(labels, " ") != "Previous 1 … 4 5 6 … 10 Next" {
		t.Errorf("items = %q", strings.Join(labels, " "))
	}
}
