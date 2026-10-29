package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/1mb-dev/markgo/internal/imaging"
	slugutil "github.com/1mb-dev/markgo/internal/slug"
)

// UploadImage handles POST /compose/image/:slug for the compose page's image
// gallery. Accepts one image per request, processes it through the imaging
// package (compression + square thumbnail), and stores the pair under
// uploads/<slug>/.
//
// Response (JSON):
//
//	{ "url": "/uploads/<slug>/<name>.jpg", "thumb": "/uploads/<slug>/<name>-thumb.jpg", "filename": "<name>.jpg", "width": 2048, "height": 1536 }
//
// The client appends url to the hidden images field; the companion thumb URL
// is derived at render time from the imaging.ThumbName convention.
func (h *ComposeHandler) UploadImage(c *gin.Context) {
	slug := c.Param("slug")
	if !slugutil.WellFormed(slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid slug format"})
		return
	}

	// MaxBytesReader enforces the configured per-file limit; FormFile parses
	// the multipart body lazily.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.config.Upload.MaxSize)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("Image exceeds %d MB limit", h.config.Upload.MaxSize>>20),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file provided"})
		return
	}
	defer file.Close()

	src, err := io.ReadAll(file)
	if err != nil {
		h.logger.Error("Image upload read failed", "slug", slug, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read image"})
		return
	}

	res, err := imaging.Process(src)
	if err != nil {
		h.logger.Info("Image upload rejected", "slug", slug, "filename", header.Filename, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slugDir, err := slugutil.ContainPath(h.config.Upload.Path, slug)
	if err != nil {
		h.logger.Error("Image upload slug containment failed", "slug", slug, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid upload path"})
		return
	}
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		h.logger.Error("Image upload dir creation failed", "slug", slug, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	randBytes := make([]byte, 4)
	if _, err := rand.Read(randBytes); err != nil {
		h.logger.Error("Image upload randomness failed", "slug", slug, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}
	filename := fmt.Sprintf("%d-image-%s%s", time.Now().UnixMilli(), hex.EncodeToString(randBytes), res.FullExt)
	thumbFilename := imaging.ThumbName(filename)

	fullPath := filepath.Join(slugDir, filename)
	thumbPath := filepath.Join(slugDir, thumbFilename)
	if err := os.WriteFile(fullPath, res.Full, 0o644); err != nil {
		h.logger.Error("Image upload write failed", "slug", slug, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save image"})
		return
	}
	if err := os.WriteFile(thumbPath, res.Thumb, 0o644); err != nil {
		h.logger.Error("Image thumbnail write failed", "slug", slug, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save thumbnail"})
		return
	}

	urlPath := "/uploads/" + slug + "/" + filename
	h.logger.Info("Gallery image uploaded", "slug", slug, "filename", filename, "bytes", len(res.Full), "thumb_bytes", len(res.Thumb))

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"url":      urlPath,
		"thumb":    "/uploads/" + slug + "/" + thumbFilename,
		"filename": filename,
		"width":    res.Width,
		"height":   res.Height,
	})
}

// DeleteImage handles DELETE /compose/image/:slug/:filename for removing a
// gallery image before publish. Deletes the stored pair (full + thumb) when
// the filename is a single safe segment owned by slug.
func (h *ComposeHandler) DeleteImage(c *gin.Context) {
	slug := c.Param("slug")
	filename := c.Param("filename")
	if !slugutil.WellFormed(slug) || filename == "" || filepath.Base(filename) != filename || filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	slugDir, err := slugutil.ContainPath(h.config.Upload.Path, slug)
	if err != nil {
		h.logger.Error("Image delete slug containment failed", "slug", slug, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid upload path"})
		return
	}

	removed := 0
	for _, name := range []string{filename, imaging.ThumbName(filename)} {
		if err := os.Remove(filepath.Join(slugDir, name)); err == nil {
			removed++
		}
	}
	if removed == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}
	h.logger.Info("Gallery image deleted", "slug", slug, "filename", filename, "removed", removed)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteUpload handles DELETE /compose/upload/:slug/:filename for removing a
// single generic upload (e.g. the compose music area's replaced track). No
// thumbnail pairing is assumed — exactly one file is removed.
func (h *ComposeHandler) DeleteUpload(c *gin.Context) {
	slug := c.Param("slug")
	filename := c.Param("filename")
	if !slugutil.WellFormed(slug) || filename == "" || filepath.Base(filename) != filename || filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	slugDir, err := slugutil.ContainPath(h.config.Upload.Path, slug)
	if err != nil {
		h.logger.Error("Upload delete slug containment failed", "slug", slug, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid upload path"})
		return
	}

	if err := os.Remove(filepath.Join(slugDir, filename)); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}
		h.logger.Error("Upload delete failed", "slug", slug, "filename", filename, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file"})
		return
	}
	h.logger.Info("Upload file deleted", "slug", slug, "filename", filename)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
