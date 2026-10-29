package handlers

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// solidTestJPEG encodes a w×h solid-color JPEG for gallery upload tests.
func solidTestJPEG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

func TestUploadImage(t *testing.T) {
	t.Run("valid image upload writes full and thumb pair", func(t *testing.T) {
		handler, tmpDir := createTestComposeHandler(t, nil)

		router := gin.New()
		router.POST("/compose/image/:slug", handler.UploadImage)

		src := solidTestJPEG(t, 1600, 900, color.RGBA{R: 120, G: 40, B: 200, A: 255})
		req := createMultipartRequest(t, "file", "photo.jpg", src, nil)
		req.URL.Path = "/compose/image/test-article"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["url"], "/uploads/test-article/")
		assert.Contains(t, resp["url"], ".jpg")
		assert.Contains(t, resp["thumb"], "-thumb.jpg")
		assert.Contains(t, resp["filename"], "image-")

		uploadDir := filepath.Join(tmpDir, "uploads", "test-article")
		full := resp["filename"].(string)
		thumb := strings.Replace(full, ".jpg", "-thumb.jpg", 1)
		for _, name := range []string{full, thumb} {
			info, err := os.Stat(filepath.Join(uploadDir, name))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
			assert.Greater(t, info.Size(), int64(0))
		}
	})

	t.Run("non-image content rejected", func(t *testing.T) {
		handler, tmpDir := createTestComposeHandler(t, nil)

		router := gin.New()
		router.POST("/compose/image/:slug", handler.UploadImage)

		req := createMultipartRequest(t, "file", "notes.txt", []byte("plain text"), nil)
		req.URL.Path = "/compose/image/test-article"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["error"])

		// Nothing written to disk.
		_, err := os.Stat(filepath.Join(tmpDir, "uploads", "test-article"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("bad slug rejected", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		router := gin.New()
		router.POST("/compose/image/:slug", handler.UploadImage)

		req := createMultipartRequest(t, "file", "x.jpg", solidTestJPEG(t, 10, 10, color.Black), nil)
		req.URL.Path = "/compose/image/..%2F..%2Fetc"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no file provided", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		router := gin.New()
		router.POST("/compose/image/:slug", handler.UploadImage)

		req := httptest.NewRequest("POST", "/compose/image/test-article", strings.NewReader(""))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestDeleteImage(t *testing.T) {
	t.Run("deletes full and thumb", func(t *testing.T) {
		handler, tmpDir := createTestComposeHandler(t, nil)
		uploadDir := filepath.Join(tmpDir, "uploads", "test-article")
		require.NoError(t, os.MkdirAll(uploadDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(uploadDir, "1-image-ab.jpg"), []byte("f"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(uploadDir, "1-image-ab-thumb.jpg"), []byte("t"), 0o644))

		router := gin.New()
		router.DELETE("/compose/image/:slug/:filename", handler.DeleteImage)

		req := httptest.NewRequest("DELETE", "/compose/image/test-article/1-image-ab.jpg", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		_, err := os.Stat(filepath.Join(uploadDir, "1-image-ab.jpg"))
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(filepath.Join(uploadDir, "1-image-ab-thumb.jpg"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("traversal filename rejected", func(t *testing.T) {
		handler, tmpDir := createTestComposeHandler(t, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("DELETE", "/compose/image/test-article/x.jpg", nil)
		c.Params = gin.Params{
			{Key: "slug", Value: "test-article"},
			{Key: "filename", Value: "../other.jpg"},
		}
		handler.DeleteImage(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		// Nothing deleted: sibling path untouched.
		_, err := os.Stat(filepath.Join(tmpDir, "uploads", "other.jpg"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("missing image returns 404", func(t *testing.T) {
		handler, _ := createTestComposeHandler(t, nil)

		router := gin.New()
		router.DELETE("/compose/image/:slug/:filename", handler.DeleteImage)

		req := httptest.NewRequest("DELETE", "/compose/image/test-article/nope.jpg", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
