//go:build rastrillo_actions

package actions

import (
	"net/http"
	"strings"

	"github.com/carlosframework/rastrillo"

	"blog/internal/blog"
)

// Handle is GET /admin/posts.
func Handle(ctx *rastrillo.Ctx, w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := blog.PageParam(r)

	all, err := blog.Count(ctx.DB, "", "")
	if err != nil {
		blog.Fail(ctx, w, "counting posts", err)
		return
	}
	total, err := blog.Count(ctx.DB, q, "")
	if err != nil {
		blog.Fail(ctx, w, "counting matching posts", err)
		return
	}
	posts, err := blog.List(ctx.DB, q, "", blog.Offset(page), blog.PageSize)
	if err != nil {
		blog.Fail(ctx, w, "loading posts", err)
		return
	}

	blog.Render(ctx, w, "admin_list", http.StatusOK, blog.AdminListView{
		Head:       blog.Head{Title: "Posts"},
		Query:      q,
		Rows:       blog.AdminRows(posts),
		Pagination: blog.BuildPagination("/admin/posts", q, page, total),
		// The true blank state gets the empty-state card; a search that
		// matched nothing gets a plain note instead. Telling a writer
		// with forty posts that their blog is empty is a lie.
		Empty:   all == 0,
		NoMatch: all > 0 && total == 0,
	})
}
