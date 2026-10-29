package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMusicInput(t *testing.T) {
	const slug = "song-post"
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"valid mp3", "/uploads/song-post/song-123-ab.mp3", "/uploads/song-post/song-123-ab.mp3"},
		{"valid m4a", "/uploads/song-post/track.m4a", "/uploads/song-post/track.m4a"},
		{"valid uppercase ext", "/uploads/song-post/a.MP3", "/uploads/song-post/a.MP3"},
		{"non-audio rejected", "/uploads/song-post/a.txt", ""},
		{"other slug rejected", "/uploads/other/a.mp3", ""},
		{"external rejected", "https://cdn.example.com/a.mp3", ""},
		{"traversal rejected", "/uploads/song-post/../x/a.mp3", ""},
		{"query rejected", "/uploads/song-post/a.mp3?v=1", ""},
		{"nested path rejected", "/uploads/song-post/sub/a.mp3", ""},
		{"no extension rejected", "/uploads/song-post/noext", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseMusicInput(tc.raw, slug); got != tc.want {
				t.Errorf("ParseMusicInput(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestPrevMusicAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	articles := filepath.Join(dir, "articles")
	if err := os.MkdirAll(articles, 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewService(articles, "tester")

	slug, err := s.CreatePost(&Input{Content: "hi", Title: "Song", Music: "/uploads/song-rt/first.mp3"})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if got := s.PrevMusic(slug); got != "/uploads/song-rt/first.mp3" {
		t.Fatalf("PrevMusic = %q", got)
	}

	// Replace track.
	if _, err := s.UpdateArticle(slug, &Input{Content: "hi", Title: "Song", Music: "/uploads/song-rt/second.ogg"}); err != nil {
		t.Fatalf("UpdateArticle replace: %v", err)
	}
	loaded, err := s.LoadArticle(slug)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Music != "/uploads/song-rt/second.ogg" {
		t.Fatalf("Music after replace = %q", loaded.Music)
	}

	// Remove track deletes the field entirely.
	if _, err := s.UpdateArticle(slug, &Input{Content: "hi", Title: "Song"}); err != nil {
		t.Fatalf("UpdateArticle remove: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(articles, "*.md"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob articles: %v", err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytesContain(content, "music:") {
		t.Errorf("music field should be deleted, frontmatter:\n%s", content)
	}
}

func bytesContain(b []byte, sub string) bool {
	return len(b) >= len(sub) && stringsIndex(string(b), sub) >= 0
}

func stringsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestUnlinkRemovedMusic(t *testing.T) {
	logger := quietLogger()
	dir := t.TempDir()
	uploads := filepath.Join(dir, "uploads")
	slugDir := filepath.Join(uploads, "u-song")
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"old.mp3", "keep.flac"} {
		if err := os.WriteFile(filepath.Join(slugDir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name    string
		prev    string
		next    string
		gone    string
		kept    string
	}{
		{"replaced", "/uploads/u-song/old.mp3", "/uploads/u-song/keep.flac", "old.mp3", "keep.flac"},
		{"removed", "/uploads/u-song/old.mp3", "", "old.mp3", "keep.flac"},
		{"unchanged", "/uploads/u-song/old.mp3", "/uploads/u-song/old.mp3", "", "old.mp3"},
		{"foreign url ignored", "https://cdn.example.com/a.mp3", "", "", "old.mp3"},
		{"empty prev", "", "/uploads/u-song/keep.flac", "", "old.mp3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Fresh files per case — earlier removals must not leak state.
			for _, f := range []string{"old.mp3", "keep.flac"} {
				if err := os.WriteFile(filepath.Join(slugDir, f), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			UnlinkRemovedMusic(tc.prev, tc.next, "u-song", uploads, logger)
			if tc.gone != "" {
				if _, err := os.Stat(filepath.Join(slugDir, tc.gone)); !os.IsNotExist(err) {
					t.Errorf("%s should be removed", tc.gone)
				}
			}
			if tc.kept != "" {
				if _, err := os.Stat(filepath.Join(slugDir, tc.kept)); err != nil {
					t.Errorf("%s should be kept: %v", tc.kept, err)
				}
			}
		})
	}
}
