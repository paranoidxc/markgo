package compose

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/1mb-dev/markgo/internal/imaging"
	slugutil "github.com/1mb-dev/markgo/internal/slug"
	"gopkg.in/yaml.v3"
)

// ParseImagesInput converts the compose form's `images` field (newline-
// separated /uploads/<slug>/<filename> URLs) into a validated, deduplicated
// list scoped to slug.
//
// The hidden input is client-controlled, so every entry is checked strictly:
// it must match the exact /uploads/<slug>/ prefix for the article being
// edited and carry a safe single-segment filename. Anything else — other
// slugs' paths, traversal, query strings, external URLs — is dropped rather
// than rejected, so a stale or tampered value can never publish a reference
// to (or later delete) a file this article doesn't own.
func ParseImagesInput(raw, slug string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	expectedPrefix := "/uploads/" + slug + "/"
	seen := make(map[string]struct{})
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		entry := strings.TrimSpace(line)
		if entry == "" {
			continue
		}
		if !strings.HasPrefix(entry, expectedPrefix) {
			continue
		}
		filename := strings.TrimPrefix(entry, expectedPrefix)
		if !safeUploadFilename(filename) {
			continue
		}
		if _, dup := seen[entry]; dup {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}

// safeUploadFilename reports whether filename is a single safe path segment:
// non-empty, no separators, no traversal, no query/fragment, and it
// round-trips through URL path unescaping (uploaded filenames are generated
// from [a-z0-9_-] plus a timestamp and hex suffix, so anything needing
// escapes was not produced by the upload endpoint).
func safeUploadFilename(filename string) bool {
	if filename == "" || strings.ContainsRune(filename, '/') || strings.ContainsRune(filename, '\\') {
		return false
	}
	if strings.ContainsAny(filename, "?#") || strings.Contains(filename, "..") {
		return false
	}
	decoded, err := url.PathUnescape(filename)
	if err != nil || decoded != filename {
		return false
	}
	return true
}

// PrevImages returns the images list currently stored in the article's
// frontmatter. Callers read this before UpdateArticle so they can unlink
// upload files dropped from the list (see UnlinkRemovedImages).
func (s *Service) PrevImages(slug string) []string {
	_, content, err := s.findFileBySlug(slug)
	if err != nil {
		return nil
	}
	parts := strings.SplitN(string(content), "---", 3)
	if len(parts) < 3 {
		return nil
	}
	var fm struct {
		Images []string `yaml:"images"`
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		return nil
	}
	return fm.Images
}

// UnlinkRemovedImages deletes the upload files (and their -thumb companions)
// that were removed from an article's images list between prev and next.
// Mirrors UnlinkOwnedUpload's discipline: only /uploads/<slug>/ shaped URLs
// are ours to remove, containment failures are logged and skipped, and every
// error path is best-effort — cleanup must never fail the caller.
func UnlinkRemovedImages(prev, next []string, slug, uploadPath string, logger *slog.Logger) {
	if len(prev) == 0 {
		return
	}
	kept := make(map[string]struct{}, len(next))
	for _, u := range next {
		kept[u] = struct{}{}
	}
	for _, entry := range prev {
		if _, stillThere := kept[entry]; stillThere {
			continue
		}
		unlinkOwnedGalleryFile(entry, slug, uploadPath, logger)
	}
}

// unlinkOwnedGalleryFile removes one gallery image and its thumbnail when the
// URL has the /uploads/<slug>/<filename> shape owned by slug.
func unlinkOwnedGalleryFile(imageURL, slug, uploadPath string, logger *slog.Logger) {
	expectedPrefix := "/uploads/" + slug + "/"
	if !strings.HasPrefix(imageURL, expectedPrefix) {
		return
	}
	filename := strings.TrimPrefix(imageURL, expectedPrefix)
	if filename == "" || strings.ContainsRune(filename, '/') {
		return
	}
	slugDir, err := slugutil.ContainPath(uploadPath, slug)
	if err != nil {
		logger.Warn("unlink removed image: containment failed",
			"slug", slug, "image", imageURL, "error", err)
		return
	}
	for _, name := range []string{filename, imaging.ThumbName(filename)} {
		target := filepath.Join(slugDir, name)
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("unlink removed image: remove failed",
				"slug", slug, "path", target, "error", err)
		}
	}
}
