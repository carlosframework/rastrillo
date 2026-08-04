// New, Show and Edit are the manifest system's generated actions now
// (manifest/posts.toml; task 10's adoption) — the generated create and
// edit-basics actions have no server-side required-field validation at
// all in this v1 (internal/generate/actions.go's package doc: "No
// server-side required-field validation exists anywhere in this slice
// — a deliberate v1 rule"), and the generated form has no page-header,
// status pill, or publish/unpublish/delete controls (those were the
// hand admin_edit.html template's own markup; the generated form.html
// doesn't carry them). Two behavioral cases this file used to cover —
// "empty title is 400" for both create and update, and "the edit
// screen shows the draft/published pill and its publish/unpublish/
// delete buttons" — no longer hold and are retired below with notes,
// per the task-10 report. Publish/unpublish/delete themselves are
// unaffected (admin_state_test.go): only their old edit-screen buttons
// are gone, pending Task 11's template ejection.
package blogtest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"blog/internal/blog"
)

func TestNewPostFormRenders(t *testing.T) {
	app, _ := newApp(t)

	rec := get(t, app, "/admin/posts/new")
	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()

	wantContains(t, body, `<form class="rst-form" method="post" action="/admin/posts">`)
	wantContains(t, body, `class="rst-field"`)
	wantContains(t, body, `<label class="rst-field__label" for="Title">Title</label>`)
	wantContains(t, body, `<input class="rst-input" id="Title" name="Title" type="text">`)
	wantContains(t, body, `<textarea class="rst-textarea" id="Body" name="Body">`)
	wantContains(t, body, `<button class="rst-btn rst-btn--primary" type="submit">Save</button>`)
	// The generated form has no page-header, status pill, or required
	// marker — templates.go's formHTML doesn't emit them, and the
	// manifest declares no required fields. See this file's own doc
	// comment.
	wantNotContains(t, body, `class="rst-field__required"`)
	wantNotContains(t, body, `required>`)
	wantNotContains(t, body, `class="rst-status"`)
}

func TestCreateRedirectsToTheNewPostsShowPage(t *testing.T) {
	app, db := newApp(t)

	rec := post(t, app, "/admin/posts", url.Values{
		"Title": {"First post"},
		"Body":  {"Hello."},
	})
	wantStatus(t, rec, http.StatusSeeOther)

	posts, err := blog.List(db, "", "", 0, blog.PageSize)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("created %d posts, want 1", len(posts))
	}
	// The generated create action redirects to the show route
	// (r.Route+"/%d"), not the old hand action's edit route.
	want := fmt.Sprintf("/admin/posts/%d", posts[0].ID)
	if got := rec.Header().Get("Location"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	list := get(t, app, "/admin/posts")
	wantContains(t, list.Body.String(), "First post")
}

// Retires TestCreateWithAnEmptyTitleIs400AndCreatesNothing: the
// generated create action has no field named as required, so an empty
// title is accepted like any other value, not rejected. See this
// file's own doc comment.
func TestCreateWithAnEmptyTitleSucceedsNoServerSideValidation(t *testing.T) {
	app, db := newApp(t)

	rec := post(t, app, "/admin/posts", url.Values{
		"Title": {"   "},
		"Body":  {"Body the writer typed."},
	})
	wantStatus(t, rec, http.StatusSeeOther)

	n, err := blog.Count(db, "", "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("created %d posts, want 1: v1's generated create has no required-field validation", n)
	}
}

func TestShowPageRendersFields(t *testing.T) {
	app, db := newApp(t)
	id := seed(t, db, "Release notes", "The body.", false)

	rec := get(t, app, fmt.Sprintf("/admin/posts/%d", id))
	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()

	wantContains(t, body, `<h1>Release notes</h1>`)
	wantContains(t, body, "The body.")
	wantContains(t, body, fmt.Sprintf(`href="/admin/posts/%d/edit"`, id))
	wantContains(t, body, `<header class="rst-page-header">`)
}

func TestShowWithAMissingIdIs404(t *testing.T) {
	app, _ := newApp(t)

	rec := get(t, app, "/admin/posts/9999")
	wantStatus(t, rec, http.StatusNotFound)
}

func TestEditShowsCurrentValues(t *testing.T) {
	app, db := newApp(t)
	id := seed(t, db, "Release notes", "The body.", false)

	rec := get(t, app, fmt.Sprintf("/admin/posts/%d/edit", id))
	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()

	wantContains(t, body, `class="rst-field"`)
	wantContains(t, body, `value="Release notes"`)
	wantContains(t, body, "The body.")
	wantContains(t, body, fmt.Sprintf(`action="/admin/posts/%d/edit-basics"`, id))
}

func TestEditWithAMissingIdIs404(t *testing.T) {
	app, _ := newApp(t)

	rec := get(t, app, "/admin/posts/9999/edit")
	wantStatus(t, rec, http.StatusNotFound)
}

// A non-numeric id is a URL that was never ours, on the admin side as
// much as the public one: parseID fails and the action answers 404, not
// 400.
func TestANonNumericIdIs404OnTheAdminSideToo(t *testing.T) {
	app, _ := newApp(t)

	if rec := get(t, app, "/admin/posts/abc/edit"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /admin/posts/abc/edit = %d, want 404", rec.Code)
	}
	if rec := get(t, app, "/admin/posts/abc"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /admin/posts/abc = %d, want 404", rec.Code)
	}
	if rec := post(t, app, "/admin/posts/abc/edit-basics", nil); rec.Code != http.StatusNotFound {
		t.Errorf("POST /admin/posts/abc/edit-basics = %d, want 404", rec.Code)
	}
}

// The 1 MiB MaxBytesReader guard opens every mutating action; this is the
// test that proves it is wired. A body over the cap makes ParseForm fail,
// and a ParseForm failure is a 400. The three state-change routes get the
// same check in admin_state_test.go.
func TestAnOversizedPostBodyIs400(t *testing.T) {
	app, db := newApp(t)
	id := seed(t, db, "T", "B", false)
	for _, target := range []string{"/admin/posts", fmt.Sprintf("/admin/posts/%d/edit-basics", id)} {
		req := httptest.NewRequest(http.MethodPost, target,
			strings.NewReader("Title="+strings.Repeat("x", 2<<20)))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		wantStatus(t, rec, http.StatusBadRequest)
	}
}

func TestUpdateChangesTheFieldsAndRedirectsBack(t *testing.T) {
	app, db := newApp(t)
	id := seed(t, db, "Before", "Old body.", false)
	before, err := blog.Get(db, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// Timestamps are RFC3339 seconds, so wait for the clock to cross a
	// second boundary — otherwise "updated_at moved" is unobservable.
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)) + 20*time.Millisecond)

	rec := post(t, app, fmt.Sprintf("/admin/posts/%d/edit-basics", id), url.Values{
		"Title": {"After"},
		"Body":  {"New body."},
	})
	wantStatus(t, rec, http.StatusSeeOther)
	// The generated edit-basics action redirects to the show route
	// (r.Route+"/%d"), not the old hand action's edit route.
	if got, want := rec.Header().Get("Location"), fmt.Sprintf("/admin/posts/%d", id); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	after, err := blog.Get(db, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Title != "After" || after.Body != "New body." {
		t.Errorf("post = %q/%q, want After/New body.", after.Title, after.Body)
	}
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("updated_at did not move: %v then %v", before.UpdatedAt, after.UpdatedAt)
	}
	if !after.CreatedAt.Equal(before.CreatedAt) {
		t.Errorf("created_at moved: %v then %v", before.CreatedAt, after.CreatedAt)
	}
}

// Retires TestUpdateWithAnEmptyTitleIs400AndChangesNothing: the
// generated edit-basics action's Basics group has no Money field, so
// generation time already knows no parse can fail and emits no
// validation branch at all (actions.go's updatePOST) — an empty title
// is written through like any other value.
func TestUpdateWithAnEmptyTitleSucceedsNoServerSideValidation(t *testing.T) {
	app, db := newApp(t)
	id := seed(t, db, "Before", "Old body.", false)

	rec := post(t, app, fmt.Sprintf("/admin/posts/%d/edit-basics", id), url.Values{
		"Title": {""},
		"Body":  {"New body."},
	})
	wantStatus(t, rec, http.StatusSeeOther)

	after, err := blog.Get(db, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Title != "" || after.Body != "New body." {
		t.Errorf("post = %q/%q, want empty title/New body.: v1's generated update has no required-field validation", after.Title, after.Body)
	}
}
