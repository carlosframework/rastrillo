// The rastrillo default icon set — vendored, never a CDN. Mirrors
// carlosframework/platform's internal/console/icons.go exactly: static,
// in-code SVG strings in the Lucide idiom (24x24 viewBox, currentColor,
// stroke, round caps and joins), safe to inline into any app's own
// html/template output via {{icon "slug"}} (register with
// template.FuncMap{"icon": rastrillo.Icon}).
//
// Vendored, never a CDN: a rastrillo app that ships this promises the
// same thing the platform's own console does -- no build step, no
// second origin a browser fetches from and the app can't vouch for or
// serve when offline. Inlining the paths is the only option, so the
// paths live here.
//
// Icons are vendored from Lucide -- https://lucide.dev -- used under the
// ISC licence:
//
//	Copyright (c) for portions of Lucide are held by Cole Bemis 2013-2022 as
//	part of Feather (MIT). All other copyright (c) for Lucide are held by
//	Lucide Contributors 2022.
//
//	Permission to use, copy, modify, and/or distribute this software for any
//	purpose with or without fee is hereby granted, provided that the above
//	copyright notice and this permission notice appear in all copies. THE
//	SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
//	REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
//	MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
//	ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
//	WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
//	ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
//	OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
//
// Emoji policy, inherited from Eleven's docs/icons.md via the console:
// emoji are CONTENT -- a mark a person or a model chose -- and never
// chrome. Interface furniture uses these icons. Adding one = paste the
// Lucide SVG's paths in below, keep the (Lucide: <slug>) comment,
// register it in the icons map, use it as {{icon "<slug>"}}.
//
// Every icon carries aria-hidden="true" because it is meant to sit
// beside its own visible text label -- the label is the accessible
// name, and a second one from the icon would only be read out twice.
// An app using an icon as a control's ONLY label must not use these
// constants bare: give that button or link an explicit aria-label.

package rastrillo

import "html/template"

// template.HTML is used HERE AND ONLY HERE in this package, for the same
// reason it's safe in the console: every string in this file is a
// compile-time constant of our own markup, never user input, a bucket
// object, or anything derived from a request.
const (
	// iconOpen is the shared opening tag: no width or height, so the icon
	// sizes from the consuming app's own CSS (e.g. a .icon { } rule) and
	// tracks the text beside it.
	iconOpen = `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" ` +
		`stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">`

	// A disclosure caret -- dropdowns, expandable sections (Lucide: chevron-down).
	iconChevronDown template.HTML = iconOpen + `<path d="m6 9 6 6 6-6"/></svg>`

	// Confirmation, success states (Lucide: check).
	iconCheck template.HTML = iconOpen + `<path d="M20 6 9 17l-5-5"/></svg>`

	// "New" / "create" actions (Lucide: plus).
	iconPlus template.HTML = iconOpen + `<path d="M5 12h14"/><path d="M12 5v14"/></svg>`

	// Search fields and filter bars (Lucide: search).
	iconSearch template.HTML = iconOpen + `<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>`
)

// icons maps each Lucide slug to its vendored markup. Slugs are the
// Lucide names so the mapping back to lucide.dev stays obvious.
var icons = map[string]template.HTML{
	"chevron-down": iconChevronDown,
	"check":        iconCheck,
	"plus":         iconPlus,
	"search":       iconSearch,
}

// Icon renders one vendored icon by its Lucide slug for use as an
// html/template FuncMap entry:
//
//	tmpl.Funcs(template.FuncMap{"icon": rastrillo.Icon})
//	// then, in the template: {{icon "check"}}
//
// An unknown slug renders nothing rather than panicking a page
// mid-response -- a typo must cost a missing icon, not a crash.
func Icon(slug string) template.HTML { return icons[slug] }
