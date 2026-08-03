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
	wantContains(t, body, `<label class="rst-field__label" for="title">Title <span class="rst-field__required" aria-hidden="true">*</span></label>`)
	wantContains(t, body, `<input class="rst-input" id="title" name="title" type="text" required>`)
	wantContains(t, body, `<textarea class="rst-textarea" id="body" name="body" rows="18">`)
	// An unsaved post has no status.
	wantNotContains(t, body, `class="rst-status"`)
}

func TestCreateRedirectsToTheNewPostsEditPage(t *testing.T) {
	app, db := newApp(t)

	rec := post(t, app, "/admin/posts", url.Values{
		"title": {"First post"},
		"body":  {"Hello."},
	})
	wantStatus(t, rec, http.StatusSeeOther)

	posts, err := blog.List(db, "", "", 0, blog.PageSize)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("created %d posts, want 1", len(posts))
	}
	want := fmt.Sprintf("/admin/posts/%d/edit", posts[0].ID)
	if got := rec.Header().Get("Location"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	list := get(t, app, "/admin/posts")
	wantContains(t, list.Body.String(), "First post")
}

func TestCreateWithAnEmptyTitleIs400AndCreatesNothing(t *testing.T) {
	app, db := newApp(t)

	rec := post(t, app, "/admin/posts", url.Values{
		"title": {"   "},
		"body":  {"Body the writer typed."},
	})
	wantStatus(t, rec, http.StatusBadRequest)
	body := rec.Body.String()

	wantContains(t, body, `<p class="blog-error" role="alert">A post needs a title.</p>`)
	// The submitted body is still in the field: a failed submission never
	// costs the writer what they typed.
	wantContains(t, body, "Body the writer typed.")

	n, err := blog.Count(db, "", "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("created %d posts, want 0", n)
	}
}

func TestEditShowsCurrentValuesAndTheDraftPill(t *testing.T) {
	app, db := newApp(t)
	id := seed(t, db, "Release notes", "The body.", false)

	rec := get(t, app, fmt.Sprintf("/admin/posts/%d/edit", id))
	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()

	wantContains(t, body, `class="rst-field"`)
	wantContains(t, body, `value="Release notes"`)
	wantContains(t, body, "The body.")
	wantContains(t, body, `<span class="rst-status" data-tone="neutral">Draft</span>`)
	wantContains(t, body, fmt.Sprintf(`action="/admin/posts/%d/publish"`, id))
	wantContains(t, body, fmt.Sprintf(`action="/admin/posts/%d/delete"`, id))
	// A draft has no public page to link to.
	wantNotContains(t, body, fmt.Sprintf(`href="/posts/%d"`, id))
}

func TestEditShowsThePublishedPillAfterPublishing(t *testing.T) {
	app, db := newApp(t)
	id := seed(t, db, "Release notes", "The body.", true)

	rec := get(t, app, fmt.Sprintf("/admin/posts/%d/edit", id))
	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()

	wantContains(t, body, `<span class="rst-status" data-tone="positive">Published</span>`)
	wantContains(t, body, fmt.Sprintf(`action="/admin/posts/%d/unpublish"`, id))
	wantContains(t, body, fmt.Sprintf(`href="/posts/%d"`, id))
}

func TestEditWithAMissingIdIs404(t *testing.T) {
	app, _ := newApp(t)

	rec := get(t, app, "/admin/posts/9999/edit")
	wantStatus(t, rec, http.StatusNotFound)
}

// A non-numeric id is a URL that was never ours, on the admin side as
// much as the public one: ParseID fails and the action answers 404, not
// 400.
func TestANonNumericIdIs404OnTheAdminSideToo(t *testing.T) {
	app, _ := newApp(t)

	if rec := get(t, app, "/admin/posts/abc/edit"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /admin/posts/abc/edit = %d, want 404", rec.Code)
	}
	if rec := post(t, app, "/admin/posts/abc", nil); rec.Code != http.StatusNotFound {
		t.Errorf("POST /admin/posts/abc = %d, want 404", rec.Code)
	}
}

// The 1 MiB MaxBytesReader guard opens every mutating action; this is the
// test that proves it is wired. A body over the cap makes ParseForm fail,
// and a ParseForm failure is a 400. The three state-change routes get the
// same check in admin_state_test.go.
func TestAnOversizedPostBodyIs400(t *testing.T) {
	app, db := newApp(t)
	id := seed(t, db, "T", "B", false)
	for _, target := range []string{"/admin/posts", fmt.Sprintf("/admin/posts/%d", id)} {
		req := httptest.NewRequest(http.MethodPost, target,
			strings.NewReader("title="+strings.Repeat("x", 2<<20)))
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

	rec := post(t, app, fmt.Sprintf("/admin/posts/%d", id), url.Values{
		"title": {"After"},
		"body":  {"New body."},
	})
	wantStatus(t, rec, http.StatusSeeOther)
	if got, want := rec.Header().Get("Location"), fmt.Sprintf("/admin/posts/%d/edit", id); got != want {
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

func TestUpdateWithAnEmptyTitleIs400AndChangesNothing(t *testing.T) {
	app, db := newApp(t)
	id := seed(t, db, "Before", "Old body.", false)

	rec := post(t, app, fmt.Sprintf("/admin/posts/%d", id), url.Values{
		"title": {""},
		"body":  {"New body."},
	})
	wantStatus(t, rec, http.StatusBadRequest)
	wantContains(t, rec.Body.String(), "A post needs a title.")
	// The re-render is the edit screen, status area and all.
	wantContains(t, rec.Body.String(), `<span class="rst-status" data-tone="neutral">Draft</span>`)

	after, err := blog.Get(db, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Title != "Before" || after.Body != "Old body." {
		t.Errorf("post changed: %q/%q", after.Title, after.Body)
	}
}
