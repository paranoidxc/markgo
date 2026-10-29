package imaging

import (
	"image"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// UploadPrefix is the URL path under which uploads are served.
const UploadPrefix = "/uploads/"

// ContentVariant is the inline-display form of a full-size upload: the URL of
// its downscaled derivative plus that derivative's intrinsic size. A zero
// value means "no derivative available — use the full image".
type ContentVariant struct {
	URL    string
	Width  int
	Height int
}

// Resolver maps upload URLs found in markdown bodies to the pre-generated
// content derivative written by the upload handler (see ContentName).
//
// Lookups hit the filesystem once per URL and are then memoized, so rendering
// an article costs one stat per image on first load and nothing afterwards.
// Resolution is deliberately read-only: a derivative that was never written —
// an animated GIF, a file uploaded before derivatives existed, an external
// image — simply yields the zero ContentVariant and the caller keeps the full
// URL it already had.
type Resolver struct {
	root string

	mu    sync.RWMutex
	cache map[string]ContentVariant
}

// NewResolver creates a resolver rooted at the uploads directory. A nil
// resolver and a resolver with an empty root both report "no derivative", so
// callers can use them interchangeably.
func NewResolver(uploadRoot string) *Resolver {
	return &Resolver{
		root:  uploadRoot,
		cache: make(map[string]ContentVariant),
	}
}

// IsUpload reports whether p addresses a file stored under the uploads
// directory (either server-absolute /uploads/... or a bare uploads/... path).
func (r *Resolver) IsUpload(p string) bool {
	if r == nil || r.root == "" {
		return false
	}
	_, _, ok := splitUploadPath(p)
	return ok
}

// Resolve returns the content derivative for an upload URL, or the zero value
// when there is none. Unknown or non-upload URLs resolve to the zero value.
func (r *Resolver) Resolve(fullURL string) ContentVariant {
	if r == nil || r.root == "" {
		return ContentVariant{}
	}
	slug, name, ok := splitUploadPath(fullURL)
	if !ok {
		return ContentVariant{}
	}

	key := slug + "/" + name
	r.mu.RLock()
	variant, cached := r.cache[key]
	r.mu.RUnlock()
	if cached {
		return variant
	}

	variant = r.lookup(slug, name)

	r.mu.Lock()
	r.cache[key] = variant
	r.mu.Unlock()
	return variant
}

// lookup stats the derivative and reads its pixel dimensions from the header.
func (r *Resolver) lookup(slug, name string) ContentVariant {
	derived := ContentName(name)
	if derived == name {
		return ContentVariant{}
	}
	path := filepath.Join(r.root, slug, derived)
	f, err := os.Open(path)
	if err != nil {
		return ContentVariant{}
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return ContentVariant{}
	}
	return ContentVariant{
		URL:    UploadPrefix + slug + "/" + derived,
		Width:  cfg.Width,
		Height: cfg.Height,
	}
}

// splitUploadPath extracts the slug and filename from an uploads path,
// rejecting anything that could escape the uploads directory. Query strings and
// fragments are dropped; URL-escaped names are unescaped so on-disk names that
// contain spaces still resolve.
func splitUploadPath(p string) (slug, name string, ok bool) {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "uploads/")
	p = strings.TrimPrefix(p, "uploads") // "/uploads"
	if p == "" {
		return "", "", false
	}

	parts := strings.Split(p, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	slug, name = parts[0], parts[1]
	if slug == "" || name == "" {
		return "", "", false
	}
	if unescaped, err := url.PathUnescape(name); err == nil {
		name = unescaped
	}
	for _, seg := range []string{slug, name} {
		if seg == "." || seg == ".." || strings.ContainsAny(seg, "\\\x00") {
			return "", "", false
		}
		if filepath.Base(seg) != seg {
			return "", "", false
		}
	}
	return slug, name, true
}
