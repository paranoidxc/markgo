package compose

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	slugutil "github.com/1mb-dev/markgo/internal/slug"
	"gopkg.in/yaml.v3"
)

// audioExtensions lists the file extensions accepted for the compose music
// area. The generic upload endpoint serves any non-blocklisted file; music
// entries are additionally restricted to playable audio formats.
var audioExtensions = map[string]bool{
	".mp3": true, ".m4a": true, ".aac": true, ".ogg": true,
	".oga": true, ".opus": true, ".wav": true, ".flac": true, ".weba": true,
}

// ParseMusicInput converts the compose form's `music` field (a single
// /uploads/<slug>/<filename> URL) into a validated value scoped to slug.
// The hidden input is client-controlled, so the same discipline as
// ParseImagesInput applies: only this slug's uploads with a safe filename
// and an audio extension survive; anything else becomes "" (no track).
func ParseMusicInput(raw, slug string) string {
	entry := strings.TrimSpace(raw)
	if entry == "" {
		return ""
	}
	expectedPrefix := "/uploads/" + slug + "/"
	if !strings.HasPrefix(entry, expectedPrefix) {
		return ""
	}
	filename := strings.TrimPrefix(entry, expectedPrefix)
	if !safeUploadFilename(filename) {
		return ""
	}
	if !audioExtensions[strings.ToLower(filepath.Ext(filename))] {
		return ""
	}
	return entry
}

// PrevMusic returns the music track currently stored in the article's
// frontmatter. Callers read this before UpdateArticle so they can unlink
// the replaced file (see UnlinkRemovedMusic).
func (s *Service) PrevMusic(slug string) string {
	_, content, err := s.findFileBySlug(slug)
	if err != nil {
		return ""
	}
	parts := strings.SplitN(string(content), "---", 3)
	if len(parts) < 3 {
		return ""
	}
	var fm struct {
		Music string `yaml:"music"`
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		return ""
	}
	return fm.Music
}

// UnlinkRemovedMusic deletes the previous music file when it differs from
// the submitted one (removed or replaced). Mirrors UnlinkRemovedImages'
// discipline: only /uploads/<slug>/ shaped URLs are ours to remove,
// containment failures are logged and skipped, and every error path is
// best-effort.
func UnlinkRemovedMusic(prev, next, slug, uploadPath string, logger *slog.Logger) {
	if prev == "" || prev == next {
		return
	}
	expectedPrefix := "/uploads/" + slug + "/"
	if !strings.HasPrefix(prev, expectedPrefix) {
		return
	}
	filename := strings.TrimPrefix(prev, expectedPrefix)
	if filename == "" || strings.ContainsRune(filename, '/') {
		return
	}
	slugDir, err := slugutil.ContainPath(uploadPath, slug)
	if err != nil {
		logger.Warn("unlink removed music: containment failed",
			"slug", slug, "music", prev, "error", err)
		return
	}
	target := filepath.Join(slugDir, filename)
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("unlink removed music: remove failed",
			"slug", slug, "path", target, "error", err)
	}
}
