package article

import (
	"strconv"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/1mb-dev/markgo/internal/imaging"
)

// contentImageTransformer makes markdown bodies image-aware.
//
// An image whose source lives under /uploads/ is rewritten so the page shows a
// downscaled copy instead of the camera original, and wrapped in a link to the
// full file:
//
//	![shot](/uploads/post/shot.jpg)
//
// becomes
//
//	<a class="content-image-link" href="/uploads/post/shot.jpg">
//	  <img class="content-image" src="/uploads/post/shot-content.jpg"
//	       width="1280" height="853" loading="lazy" decoding="async" alt="shot">
//	</a>
//
// The link is inert without JavaScript (it opens the full image), while the
// lightbox module intercepts the click and shows it as a modal — the same
// viewer the nine-grid gallery uses. External images, images the author already
// wrapped in a link, and uploads without a derivative are left alone.
type contentImageTransformer struct {
	resolver *imaging.Resolver
}

var _ parser.ASTTransformer = (*contentImageTransformer)(nil)

// Transform rewrites every qualifying image after the document is parsed.
// Rewriting happens in a second pass because ast.Walk does not support
// replacing the node being visited.
func (t *contentImageTransformer) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	if t.resolver == nil {
		return
	}

	var images []*ast.Image
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if img, ok := n.(*ast.Image); ok {
			images = append(images, img)
		}
		return ast.WalkContinue, nil
	})

	for _, img := range images {
		t.rewrite(img)
	}
}

// rewrite swaps one image's source for its content derivative and wraps it in a
// full-size link.
func (t *contentImageTransformer) rewrite(img *ast.Image) {
	parent := img.Parent()
	if parent == nil {
		return
	}
	// The author wrapped this image in a link (to their own page, a source,
	// anything): nesting anchors is invalid, so respect their intent.
	if _, linked := parent.(*ast.Link); linked {
		return
	}

	full := string(img.Destination)
	if !t.resolver.IsUpload(full) {
		return
	}

	if variant := t.resolver.Resolve(full); variant.URL != "" {
		img.Destination = []byte(variant.URL)
		if variant.Width > 0 && variant.Height > 0 {
			// Declared dimensions let the browser reserve space before the
			// image loads — no layout jump while scrolling the feed.
			img.SetAttributeString("width", []byte(strconv.Itoa(variant.Width)))
			img.SetAttributeString("height", []byte(strconv.Itoa(variant.Height)))
		}
	}
	img.SetAttributeString("class", []byte("content-image"))
	img.SetAttributeString("loading", []byte("lazy"))
	img.SetAttributeString("decoding", []byte("async"))

	link := ast.NewLink()
	link.Destination = []byte(full)
	link.SetAttributeString("class", []byte("content-image-link"))

	parent.ReplaceChild(parent, img, link)
	link.AppendChild(link, img)
}
