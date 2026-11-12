package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAmount(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want float64
		ok   bool
	}{
		{"empty", "", 0, true},
		{"spaces", "  ", 0, true},
		{"integer", "42", 42, true},
		{"decimal", "12.5", 12.5, true},
		{"negative", "-1", 0, false},
		{"not a number", "abc", 0, false},
		{"inf", "Inf", 0, false},
		{"nan", "NaN", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAmount(tc.raw)
			if tc.ok {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestCreatePost_Amount(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Content: "Sponsored write-up.",
		Amount:  "88.5",
	})
	require.NoError(t, err)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Contains(t, string(content), "amount: 88.5")

	// Round-trip: LoadArticle returns the shortest form
	input, err := svc.LoadArticle(slug)
	require.NoError(t, err)
	assert.Equal(t, "88.5", input.Amount)
}

func TestCreatePost_AmountZeroOmitted(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	_, err := svc.CreatePost(&Input{
		Content: "Plain thought.",
		Amount:  "0",
	})
	require.NoError(t, err)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)
	content, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.NotContains(t, string(content), "amount:")
}

func TestCreatePost_AmountInvalid(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	_, err := svc.CreatePost(&Input{
		Content: "Oops.",
		Amount:  "12,50",
	})
	require.Error(t, err)
}

func TestUpdateArticle_Amount(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "Test Author")

	slug, err := svc.CreatePost(&Input{
		Content: "First version.",
		Amount:  "10",
	})
	require.NoError(t, err)

	// Edit: change the amount
	loaded, err := svc.LoadArticle(slug)
	require.NoError(t, err)
	loaded.Amount = "25.75"
	loaded.Content = "Second version."
	_, err = svc.UpdateArticle(slug, loaded)
	require.NoError(t, err)

	after, err := svc.LoadArticle(slug)
	require.NoError(t, err)
	assert.Equal(t, "25.75", after.Amount)

	// Edit: clear the amount — key must be deleted
	after.Amount = ""
	_, err = svc.UpdateArticle(slug, after)
	require.NoError(t, err)

	final, err := svc.LoadArticle(slug)
	require.NoError(t, err)
	assert.Equal(t, "", final.Amount)

	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	require.Len(t, files, 1)
	content, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.NotContains(t, string(content), "amount:")
}
