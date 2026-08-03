//go:build rastrillo_actions

package actions

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/carlosframework/rastrillo"

	"blog/internal/blog"
)

// Handle is GET /admin/posts/{id}/edit.
func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
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
	blog.Render(ctx, w, "admin_edit", http.StatusOK, blog.EditForm(post))
}
