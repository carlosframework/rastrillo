package act_index_get

import (
	"net/http"

	"github.com/carlosframework/rastrillo"

	"blog/internal/blog"
)

// Handle is GET /.
func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	// Go's mux treats "/" as a prefix pattern, so this action receives
	// every unmatched GET. Without this guard the homepage answers
	// /nonsense with a 200. Every rastrillo app with an index action
	// needs it — friction finding F6.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	page := blog.PageParam(r)
	total, err := blog.CountPublished(ctx.DB)
	if err != nil {
		blog.Fail(ctx, w, "counting published posts", err)
		return
	}
	posts, err := blog.ListPublished(ctx.DB, blog.Offset(page), blog.PageSize)
	if err != nil {
		blog.Fail(ctx, w, "loading published posts", err)
		return
	}

	blog.Render(ctx, w, "index", http.StatusOK, blog.HomeView{
		Head:       blog.Head{Title: "The blog"},
		Rows:       blog.PublicRows(posts),
		Pagination: blog.BuildPagination("/", "", "", page, total),
	})
}
