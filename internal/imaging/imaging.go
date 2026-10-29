// Package imaging provides server-side image processing for compose gallery
// uploads: a downscaled re-encode of the full image (compression) and a square
// center-crop thumbnail for the nine-grid display.
//
// Supported inputs: JPEG, PNG, GIF, WebP (decode-only, via golang.org/x/image).
// Outputs: JPEG for opaque sources and animated-GIF fulls, PNG when the source
// is a PNG with transparency (alpha preserved on full and thumb).
package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register the WebP decoder
)

const (
	// MaxDimension caps the longest edge of the stored full image. Larger
	// sources are proportionally downscaled before re-encode.
	MaxDimension = 2048
	// ThumbSize is the edge length of the square thumbnail. Sources whose
	// short edge is smaller are never upscaled.
	ThumbSize = 480
	// MaxPixels guards against decompression bombs: sources decoding to more
	// pixels than this are rejected outright.
	MaxPixels = 40 << 20 // 40 megapixels

	jpegQuality  = 85
	thumbQuality = 82
)

// Result carries the processed outputs of one uploaded image.
type Result struct {
	// Full is the re-encoded (and possibly downscaled) full-size image.
	Full []byte
	// FullExt is the extension to store Full under: ".jpg", ".png", or
	// ".gif" (animated GIFs pass through byte-for-byte).
	FullExt string
	// Thumb is the square center-crop thumbnail.
	Thumb []byte
	// ThumbExt is the extension to store Thumb under: ".jpg" or ".png".
	ThumbExt string

	Width  int
	Height int
}

// Process decodes src and returns the compressed full image plus a square
// center-crop thumbnail. Animated GIFs are preserved byte-for-byte as the
// full image (re-encoding would drop animation); their thumbnail is rendered
// from the first frame.
func Process(src []byte) (*Result, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("unsupported or corrupt image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("invalid image dimensions %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Width*cfg.Height > MaxPixels {
		return nil, fmt.Errorf("image too large: %dx%d exceeds %d megapixels", cfg.Width, cfg.Height, MaxPixels>>20)
	}

	// Animated GIF: passthrough the original bytes, thumb from first frame.
	if format == "gif" {
		if frames, err := gif.DecodeAll(bytes.NewReader(src)); err == nil && len(frames.Image) > 1 {
			thumb, thumbExt, err := buildThumb(frames.Image[0], false)
			if err != nil {
				return nil, err
			}
			return &Result{
				Full:     src,
				FullExt:  ".gif",
				Thumb:    thumb,
				ThumbExt: thumbExt,
				Width:    cfg.Width,
				Height:   cfg.Height,
			}, nil
		}
	}

	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("unsupported or corrupt image: %w", err)
	}

	alpha := hasAlpha(img)

	// Full image: downscale when over MaxDimension, then re-encode.
	full := img
	fullW, fullH := img.Bounds().Dx(), img.Bounds().Dy()
	if max(fullW, fullH) > MaxDimension {
		full = resizeToFit(img, MaxDimension)
		fullW, fullH = full.Bounds().Dx(), full.Bounds().Dy()
	}
	fullBytes, fullExt, err := encodeFull(full, format, alpha)
	if err != nil {
		return nil, err
	}

	thumb, thumbExt, err := buildThumb(img, format == "png" && alpha)
	if err != nil {
		return nil, err
	}

	return &Result{
		Full:     fullBytes,
		FullExt:  fullExt,
		Thumb:    thumb,
		ThumbExt: thumbExt,
		Width:    fullW,
		Height:   fullH,
	}, nil
}

// ThumbName derives the thumbnail filename for an uploaded image filename,
// mirroring the pairing the upload handler writes to disk:
//
//	"a-b.jpg" -> "a-b-thumb.jpg"
//	"a.png"   -> "a-thumb.png"
//	"a.gif"   -> "a-thumb.jpg" (animated fulls pass through; thumbs are JPEG)
//
// Files without a recognizable extension are returned unchanged so callers
// never derive a name that was never written.
func ThumbName(filename string) string {
	ext := lowerExt(filename)
	if ext == "" {
		return filename
	}
	base := filename[:len(filename)-len(ext)]
	thumbExt := ext
	if ext == ".gif" {
		thumbExt = ".jpg"
	}
	if thumbExt != ".jpg" && thumbExt != ".png" {
		return filename
	}
	return base + "-thumb" + thumbExt
}

// buildThumb center-crops img to a square (short edge, centered) and scales
// it to ThumbSize without upscaling. keepAlpha selects PNG output so
// transparent PNGs keep their transparency in the grid.
func buildThumb(img image.Image, keepAlpha bool) ([]byte, string, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	side := min(w, h)

	// Center square crop rectangle.
	crop := image.Rect(b.Min.X+(w-side)/2, b.Min.Y+(h-side)/2, b.Min.X+(w-side)/2+side, b.Min.Y+(h-side)/2+side)

	target := min(ThumbSize, side)
	dst := image.NewRGBA(image.Rect(0, 0, target, target))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, crop, xdraw.Over, nil)

	var buf bytes.Buffer
	var err error
	if keepAlpha {
		err = png.Encode(&buf, dst)
		return buf.Bytes(), ".png", err
	}
	// JPEG has no alpha — flatten onto white first.
	err = jpeg.Encode(&buf, flatten(dst), &jpeg.Options{Quality: thumbQuality})
	return buf.Bytes(), ".jpg", err
}

// encodeFull re-encodes the (possibly resized) full image. PNG sources with
// transparency stay PNG; everything else becomes JPEG flattened on white
// (WebP decode-only support and static GIFs included).
func encodeFull(img image.Image, format string, alpha bool) ([]byte, string, error) {
	var buf bytes.Buffer
	var err error
	if format == "png" && alpha {
		err = png.Encode(&buf, img)
		return buf.Bytes(), ".png", err
	}
	err = jpeg.Encode(&buf, flatten(img), &jpeg.Options{Quality: jpegQuality})
	return buf.Bytes(), ".jpg", err
}

// flatten composites img over an opaque white background, because the JPEG
// encoder renders transparent pixels as black.
func flatten(img image.Image) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)
	return dst
}

// resizeToFit proportionally scales img so its longest edge is maxEdge.
func resizeToFit(img image.Image, maxEdge int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := float64(maxEdge) / float64(max(w, h))
	nw := max(1, int(float64(w)*scale))
	nh := max(1, int(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
	return dst
}

// hasAlpha reports whether any pixel is translucent. Transparent PNGs are
// kept lossless-ish; opaque sources go the smaller JPEG route.
func hasAlpha(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xffff {
				return true
			}
		}
	}
	return false
}

// lowerExt returns the lowercased extension including the dot, or "".
func lowerExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if len(ext) < 2 || len(ext) > 6 {
		return ""
	}
	return ext
}
