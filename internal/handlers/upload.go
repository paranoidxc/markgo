package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/imaging"
	slugutil "github.com/1mb-dev/markgo/internal/slug"
)

var (
	blockedExtensions = map[string]bool{
		".exe": true, ".bat": true, ".cmd": true, ".sh": true,
		".com": true, ".msi": true, ".dll": true, ".so": true,
		".html": true, ".htm": true, ".xhtml": true,
		".js": true, ".mjs": true, ".cjs": true,
		".svg": true,
		".php": true, ".py": true, ".rb": true, ".pl": true,
		".cgi": true, ".jsp": true, ".asp": true, ".aspx": true,
	}
	imageExtensions = map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".webp": true,
	}
	safeFilenameRe = regexp.MustCompile(`[^a-z0-9_-]`)
)

// sanitizeFilename strips the extension, lowercases, removes unsafe chars,
// and caps length. Returns "file" if the result is empty.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext)
	name = strings.ToLower(name)
	name = safeFilenameRe.ReplaceAllString(name, "")
	if name == "" {
		name = "file"
	}
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// writeUploadFile writes one upload artefact, making sure it is world-readable
// but never executable.
func writeUploadFile(dir, name string, data []byte) error {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // uploaded files must be readable by the web server
		return err
	}
	return nil
}

// processedImage is the result of storing an image upload: the full file's name
// plus its companion thumbnail, all already written under the slug directory.
type processedImage struct {
	name   string
	thumb  string
	width  int
	height int
}

// saveProcessedImage stores an image upload as three artefacts under dir:
//
//	<stem>.jpg         full size, capped at imaging.MaxDimension (lightbox)
//	<stem>-thumb.jpg   square crop (nine-grid gallery / compose tiles)
//	<stem>-content.jpg proportional downscale (markdown bodies)
//
// Skipping a derivative the source doesn't need (a small image, an animated
// GIF) is handled by imaging.Process, which simply returns no bytes for it.
func (h *ComposeHandler) saveProcessedImage(dir, stem string, src []byte) (*processedImage, error) {
	res, err := imaging.Process(src)
	if err != nil {
		return nil, err
	}

	name := stem + res.FullExt
	if err := writeUploadFile(dir, name, res.Full); err != nil {
		return nil, err
	}

	var thumb string
	if len(res.Thumb) > 0 {
		thumb = imaging.ThumbName(name)
		if err := writeUploadFile(dir, thumb, res.Thumb); err != nil {
			return nil, err
		}
	}
	if len(res.Content) > 0 {
		if err := writeUploadFile(dir, imaging.ContentName(name), res.Content); err != nil {
			return nil, err
		}
	}

	return &processedImage{name: name, thumb: thumb, width: res.Width, height: res.Height}, nil
}

// Upload handles file upload for the compose page.
// Files are scoped to a slug directory: {config.Upload.Path}/{slug}/{filename}.
// Extension-based blocklist rejects executables, scripts, and HTML-renderable types; all others are allowed.
//
// Images are decoded and re-encoded server-side (see imaging.Process) so a
// markdown body never embeds the camera original: the returned markdown points
// at the full file, and the renderer swaps in the downscaled content
// derivative, linking back to the full image for the lightbox. Anything the
// decoder rejects is stored verbatim rather than failing the upload.
func (h *ComposeHandler) Upload(c *gin.Context) {
	slug := c.Param("slug")
	if !slugutil.WellFormed(slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid slug"})
		return
	}

	maxSize := h.config.Upload.MaxSize
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.logger.Error("Upload FormFile error", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided or file too large"})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File must have an extension"})
		return
	}
	if blockedExtensions[ext] {
		h.logger.Warn("Upload rejected: blocked extension",
			"extension", ext,
			"filename", header.Filename,
			"size", header.Size,
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "File type not allowed"})
		return
	}

	uploadDir, containErr := slugutil.ContainPath(h.config.Upload.Path, slug)
	if containErr != nil {
		if errors.Is(containErr, apperrors.ErrPathEscape) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid slug"})
			return
		}
		h.logger.Error("Upload containment check failed", "error", containErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Server configuration error"})
		return
	}

	if mkdirErr := os.MkdirAll(uploadDir, 0o755); mkdirErr != nil { //nolint:gosec // upload dir needs to be accessible
		h.logger.Error("Failed to create upload directory", "error", mkdirErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	// Read once: images go through the decoder and, on failure, reuse the same
	// bytes for the verbatim path. MaxBytesReader bounds the allocation.
	src, readErr := io.ReadAll(file)
	if readErr != nil {
		if isBodyTooLarge(readErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("File exceeds %d MB limit", maxSize>>20),
			})
			return
		}
		h.logger.Error("Failed to read upload", "error", readErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	randBytes := make([]byte, 4)
	if _, randErr := rand.Read(randBytes); randErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}
	stem := fmt.Sprintf("%d-%s-%s", time.Now().UnixMilli(), sanitizeFilename(header.Filename), hex.EncodeToString(randBytes))

	if imageExtensions[ext] {
		img, imgErr := h.saveProcessedImage(uploadDir, stem, src)
		if imgErr != nil {
			// Undecodable image (or a write failure): fall through and keep the
			// bytes verbatim rather than rejecting the author's file.
			h.logger.Warn("Upload image processing failed; storing original",
				"slug", slug, "filename", header.Filename, "extension", ext, "error", imgErr)
		} else {
			url := imaging.UploadPrefix + slug + "/" + img.name
			thumb := ""
			if img.thumb != "" {
				thumb = imaging.UploadPrefix + slug + "/" + img.thumb
			}
			h.logger.Info("Image uploaded",
				"slug", slug, "filename", img.name,
				"bytes", len(src), "width", img.width, "height", img.height)
			c.JSON(http.StatusOK, gin.H{
				"url":      url,
				"thumb":    thumb,
				"markdown": fmt.Sprintf("![%s](%s)", sanitizeFilename(header.Filename), url),
				"filename": img.name,
				"width":    img.width,
				"height":   img.height,
			})
			return
		}
	}

	filename := stem + ext
	if err := writeUploadFile(uploadDir, filename, src); err != nil {
		h.logger.Error("Failed to write uploaded file", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	url := fmt.Sprintf("%s%s/%s", imaging.UploadPrefix, slug, filename)
	var markdown string
	if imageExtensions[ext] {
		markdown = fmt.Sprintf("![%s](%s)", sanitizeFilename(header.Filename), url)
	} else {
		markdown = fmt.Sprintf("[%s](%s)", sanitizeFilename(header.Filename), url)
	}

	c.JSON(http.StatusOK, gin.H{
		"url":      url,
		"markdown": markdown,
		"filename": filename,
	})
}
