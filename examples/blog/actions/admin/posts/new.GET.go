package actions

import (
	"net/http"

	"github.com/carlosframework/rastrillo"

	"blog/internal/blog"
)

// Handle is GET /admin/posts/new.
func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	blog.Render(ctx, w, "admin_new", http.StatusOK, blog.AdminFormView{
		Head:   blog.Head{Title: "New post"},
		Action: "/admin/posts",
	})
}
