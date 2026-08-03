package actions

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/carlosframework/rastrillo"

	"blog/internal/blog"
)

// Handle is POST /admin/posts.
func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	// A megabyte is a generous post, and an unbounded ParseForm on a
	// public endpoint is a memory-exhaustion hole.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request.", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.PostFormValue("title"))
	body := r.PostFormValue("body")
	if title == "" {
		// Re-render with what the writer typed, rather than redirecting
		// and losing it.
		blog.Render(ctx, w, "admin_new", http.StatusBadRequest, blog.AdminFormView{
			Head:      blog.Head{Title: "New post"},
			Action:    "/admin/posts",
			FormTitle: title,
			Body:      body,
			Error:     "A post needs a title.",
		})
		return
	}

	id, err := blog.Create(ctx.DB, title, body)
	if err != nil {
		blog.Fail(ctx, w, "creating post", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/posts/%d/edit", id), http.StatusSeeOther)
}
