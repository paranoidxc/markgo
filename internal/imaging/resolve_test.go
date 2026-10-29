package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// writeJPEG lays down a solid w×h JPEG at dir/name.
func writeJPEG(t *testing.T, dir, name string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 10, G: 90, B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestResolver_Resolve(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "post-1"), "shot.jpg", 1600, 900)
	writeJPEG(t, filepath.Join(root, "post-1"), "shot-content.jpg", 1280, 720)

	r := NewResolver(root)

	got := r.Resolve("/uploads/post-1/shot.jpg")
	if got.URL != "/uploads/post-1/shot-content.jpg" {
		t.Errorf("URL = %q, want the -content derivative", got.URL)
	}
	if got.Width != 1280 || got.Height != 720 {
		t.Errorf("dims = %dx%d, want 1280x720", got.Width, got.Height)
	}

	// Query strings and fragments are ignored, and the lookup is memoized —
	// the second call must answer identically after the file disappears.
	if err := os.Remove(filepath.Join(root, "post-1", "shot-content.jpg")); err != nil {
		t.Fatalf("remove derivative: %v", err)
	}
	if again := r.Resolve("/uploads/post-1/shot.jpg?cache=1"); again != got {
		t.Errorf("cached resolve = %+v, want %+v", again, got)
	}
}

func TestResolver_NoDerivative(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "post-1"), "plain.jpg", 1600, 900)
	r := NewResolver(root)

	if got := r.Resolve("/uploads/post-1/plain.jpg"); got.URL != "" {
		t.Errorf("URL = %q, want empty when no -content file exists", got.URL)
	}
	if got := r.Resolve("/uploads/post-1/missing.jpg"); got.URL != "" {
		t.Errorf("URL = %q, want empty for a missing file", got.URL)
	}
}

func TestResolver_RejectsNonUploadPaths(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "post-1"), "shot-content.jpg", 10, 10)
	r := NewResolver(root)

	for _, p := range []string{
		"https://example.com/uploads/post-1/shot.jpg",
		"/static/img/logo.png",
		"shot.jpg",
		"/uploads/../etc/passwd",
		"/uploads/post-1/../../secret.jpg",
		"/uploads/post-1/sub/shot.jpg",
		"/uploads//shot.jpg",
		"/uploads",
	} {
		if r.IsUpload(p) {
			t.Errorf("IsUpload(%q) = true, want false", p)
		}
		if got := r.Resolve(p); got.URL != "" {
			t.Errorf("Resolve(%q) = %q, want empty", p, got.URL)
		}
	}
}

func TestResolver_NilAndEmptyRootAreInert(t *testing.T) {
	var nilResolver *Resolver
	if nilResolver.IsUpload("/uploads/post-1/shot.jpg") {
		t.Error("nil resolver should not claim uploads")
	}
	if got := nilResolver.Resolve("/uploads/post-1/shot.jpg"); got.URL != "" {
		t.Errorf("nil resolver resolved %q", got.URL)
	}

	empty := NewResolver("")
	if empty.IsUpload("/uploads/post-1/shot.jpg") {
		t.Error("empty-root resolver should not claim uploads")
	}
	if got := empty.Resolve("/uploads/post-1/shot.jpg"); got.URL != "" {
		t.Errorf("empty-root resolver resolved %q", got.URL)
	}
}
