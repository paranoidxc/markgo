package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseImagesInput(t *testing.T) {
	const slug = "my-post"
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace only", "  \n \n", nil},
		{
			"single valid",
			"/uploads/my-post/a-1234-abcd.jpg",
			[]string{"/uploads/my-post/a-1234-abcd.jpg"},
		},
		{
			"multiple with blanks and spaces",
			"\n  /uploads/my-post/a.jpg \n\n/uploads/my-post/b.png\n",
			[]string{"/uploads/my-post/a.jpg", "/uploads/my-post/b.png"},
		},
		{"dedupes", "/uploads/my-post/a.jpg\n/uploads/my-post/a.jpg", []string{"/uploads/my-post/a.jpg"}},
		{"drops other slug", "/uploads/other/a.jpg", nil},
		{"drops external", "https://cdn.example.com/a.jpg", nil},
		{"drops traversal", "/uploads/my-post/../other/a.jpg", nil},
		{"drops query", "/uploads/my-post/a.jpg?v=2", nil},
		{"drops nested path", "/uploads/my-post/sub/a.jpg", nil},
		{"drops bare filename", "a.jpg", nil},
		{"keeps valid among invalid", "/uploads/other/x.jpg\n/uploads/my-post/keep.jpg", []string{"/uploads/my-post/keep.jpg"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseImagesInput(tc.raw, slug)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// newServiceWithImages creates a service pointed at a temp articles dir with
// one pre-written article carrying an images list.
func newServiceWithImages(t *testing.T, slug string, images []string) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	articles := filepath.Join(dir, "articles")
	uploads := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(articles, 0o755); err != nil {
		t.Fatal(err)
	}
	fm := "---\nslug: " + slug + "\ntitle: Gallery Post\n"
	if len(images) > 0 {
		fm += "images:\n"
		for _, img := range images {
			fm += "  - " + img + "\n"
		}
	}
	fm += "draft: false\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(articles, "2026-01-01-"+slug+".md"), []byte(fm), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewService(articles, "tester"), uploads
}

func TestPrevImages(t *testing.T) {
	s, _ := newServiceWithImages(t, "g-post", []string{"/uploads/g-post/a.jpg", "/uploads/g-post/a-thumb.jpg"})
	got := s.PrevImages("g-post")
	if len(got) != 2 || got[0] != "/uploads/g-post/a.jpg" {
		t.Fatalf("PrevImages = %v", got)
	}
	if got2 := s.PrevImages("missing"); got2 != nil {
		t.Fatalf("PrevImages(missing) = %v, want nil", got2)
	}
}

func TestImagesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	articles := filepath.Join(dir, "articles")
	if err := os.MkdirAll(articles, 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewService(articles, "tester")

	imgs := []string{"/uploads/rt-post/a.jpg", "/uploads/rt-post/b.png"}
	slug, err := s.CreatePost(&Input{Content: "hi", Title: "RT", Images: imgs})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	loaded, err := s.LoadArticle(slug)
	if err != nil {
		t.Fatalf("LoadArticle: %v", err)
	}
	if len(loaded.Images) != 2 || loaded.Images[0] != imgs[0] {
		t.Fatalf("round-trip images = %v", loaded.Images)
	}

	// Update: replace list.
	next := []string{"/uploads/rt-post/c.webp"}
	if _, err := s.UpdateArticle(slug, &Input{Content: "hi", Title: "RT", Images: next}); err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}
	loaded, err = s.LoadArticle(slug)
	if err != nil {
		t.Fatalf("LoadArticle after update: %v", err)
	}
	if len(loaded.Images) != 1 || loaded.Images[0] != next[0] {
		t.Fatalf("images after update = %v", loaded.Images)
	}

	// Update with empty list deletes the field entirely.
	if _, err := s.UpdateArticle(slug, &Input{Content: "hi", Title: "RT"}); err != nil {
		t.Fatalf("UpdateArticle empty: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "articles", "*.md"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob articles: %v", err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "images:") {
		t.Errorf("images field should be deleted, frontmatter:\n%s", content)
	}
}

func TestUnlinkRemovedImages(t *testing.T) {
	quiet := quietLogger()
	dir := t.TempDir()
	uploads := filepath.Join(dir, "uploads")
	slugDir := filepath.Join(uploads, "u-post")
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{"a.jpg", "a-thumb.jpg", "b.png", "b-thumb.png", "keep.gif", "keep-thumb.jpg"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(slugDir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prev := []string{
		"/uploads/u-post/a.jpg",
		"/uploads/u-post/b.png",
		"/uploads/u-post/keep.gif",
		"/uploads/u-post/foreign.jpg", // other file: never existed, must not error
		"https://cdn.example.com/x.jpg",
	}
	next := []string{"/uploads/u-post/keep.gif"}

	UnlinkRemovedImages(prev, next, "u-post", uploads, quiet)

	for _, f := range files {
		_, err := os.Stat(filepath.Join(slugDir, f))
		removed := f == "a.jpg" || f == "a-thumb.jpg" || f == "b.png" || f == "b-thumb.png"
		if removed && !os.IsNotExist(err) {
			t.Errorf("%s should be removed", f)
		}
		if !removed && err != nil {
			t.Errorf("%s should be kept: %v", f, err)
		}
	}
}
