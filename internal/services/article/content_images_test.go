package article

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestJPEG lays down a solid w×h JPEG at path, creating parent dirs.
func writeTestJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 90, G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
}

// newUploadsProcessor builds a processor rooted at a temp uploads dir holding
// one 1600×900 image and its 1280×720 content derivative.
func newUploadsProcessor(t *testing.T) *MarkdownContentProcessor {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "post-1")
	writeTestJPEG(t, filepath.Join(dir, "shot.jpg"), 1600, 900)
	writeTestJPEG(t, filepath.Join(dir, "shot-content.jpg"), 1280, 720)
	return NewMarkdownContentProcessor(slog.Default(), root)
}

func TestProcessMarkdown_UploadImageUsesContentDerivative(t *testing.T) {
	p := newUploadsProcessor(t)

	html, err := p.ProcessMarkdown("![shot](/uploads/post-1/shot.jpg)")
	require.NoError(t, err)

	// Inline view is the downscaled derivative, with declared dimensions so the
	// browser reserves layout space before the bytes arrive.
	assert.Contains(t, html, `src="/uploads/post-1/shot-content.jpg"`)
	assert.Contains(t, html, `width="1280"`)
	assert.Contains(t, html, `height="720"`)
	assert.Contains(t, html, `loading="lazy"`)
	assert.Contains(t, html, `decoding="async"`)
	assert.Contains(t, html, `class="content-image"`)

	// The wrapper points at the full-size file for the lightbox.
	assert.Contains(t, html, `class="content-image-link"`)
	assert.Contains(t, html, `href="/uploads/post-1/shot.jpg"`)
	assert.Contains(t, html, `alt="shot"`)
}

func TestProcessMarkdown_UploadImageWithoutDerivativeKeepsFullSource(t *testing.T) {
	root := t.TempDir()
	writeTestJPEG(t, filepath.Join(root, "post-1", "legacy.jpg"), 1600, 900)

	p := NewMarkdownContentProcessor(slog.Default(), root)
	html, err := p.ProcessMarkdown("![legacy](/uploads/post-1/legacy.jpg)")
	require.NoError(t, err)

	// Nothing to downscale: the original src stays, but it is still clickable.
	assert.Contains(t, html, `src="/uploads/post-1/legacy.jpg"`)
	assert.Contains(t, html, `class="content-image-link"`)
	assert.NotContains(t, html, `width="`)
}

func TestProcessMarkdown_LeavesNonUploadImagesAlone(t *testing.T) {
	p := newUploadsProcessor(t)

	for name, md := range map[string]string{
		"external":   "![x](https://example.com/a.jpg)",
		"relative":   "![x](../images/b.jpg)",
		"authorLink": "[![x](/uploads/post-1/shot.jpg)](/somewhere)",
	} {
		t.Run(name, func(t *testing.T) {
			html, err := p.ProcessMarkdown(md)
			require.NoError(t, err)
			assert.NotContains(t, html, "content-image-link")
		})
	}
}

func TestProcessMarkdown_NoUploadRootIsInert(t *testing.T) {
	p := NewMarkdownContentProcessor(slog.Default(), "")
	html, err := p.ProcessMarkdown("![shot](/uploads/post-1/shot.jpg)")
	require.NoError(t, err)
	assert.Contains(t, html, `src="/uploads/post-1/shot.jpg"`)
	assert.NotContains(t, html, "content-image-link")
}
