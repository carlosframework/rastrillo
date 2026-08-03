package actions

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/carlosframework/rastrillo"

	"blog/internal/blog"
)

// Handle is POST /admin/posts/{id}.
func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request.", http.StatusBadRequest)
		return
	}
	id, ok := blog.ParseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	post, err := blog.Get(ctx.DB, id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		blog.Fail(ctx, w, "loading post", err)
		return
	}

	title := strings.TrimSpace(r.PostFormValue("title"))
	body := r.PostFormValue("body")
	if title == "" {
		view := blog.EditForm(post)
		view.FormTitle = title
		view.Body = body
		view.Error = "A post needs a title."
		blog.Render(ctx, w, "admin_edit", http.StatusBadRequest, view)
		return
	}

	if err := blog.Update(ctx.DB, id, title, body); err != nil {
		blog.Fail(ctx, w, "updating post", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/posts/%d/edit", id), http.StatusSeeOther)
}
