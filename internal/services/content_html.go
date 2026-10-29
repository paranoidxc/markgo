package services

import (
	"sync"

	"github.com/microcosm-cc/bluemonday"
)

// contentHTMLPolicy sanitizes rendered markdown before it reaches a template
// (the safeHTML template function) or a preview endpoint.
//
// UGCPolicy is the baseline, but it drops two things the renderer deliberately
// emits and the reading experience depends on:
//
//   - class on <a>/<img>: the click-to-zoom wrapper around upload images
//     (.content-image-link / .content-image) and the styling hooks that go
//     with it. Without them every body image opens full size in a new tab
//     instead of the lightbox.
//   - loading/decoding on <img>: native lazy loading, which keeps a long post
//     from pulling every image before the reader scrolls to it.
//
// Both are inert attributes — no script execution, no URL handling — so
// re-allowing them does not widen the XSS surface. width/height already pass
// UGC's image policy, which is what reserves layout space before the bytes
// arrive.
var contentHTMLPolicy = sync.OnceValue(func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class").OnElements("img", "a")
	p.AllowAttrs("loading", "decoding").OnElements("img")
	return p
})

// ContentHTMLPolicy returns the shared sanitizer for rendered markdown.
// Callers that sanitize HTML fragments themselves (the compose preview) must
// use this policy rather than a fresh UGCPolicy, or their output will not match
// what the published page renders.
func ContentHTMLPolicy() *bluemonday.Policy {
	return contentHTMLPolicy()
}
