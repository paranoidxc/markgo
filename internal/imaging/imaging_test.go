package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

// solidJPEG encodes a w×h JPEG filled with the given color.
func solidJPEG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// solidPNG encodes a w×h PNG filled with the given color; alpha is taken
// from the color's A channel.
func solidPNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestProcess_JPEGDownscalesFull(t *testing.T) {
	src := solidJPEG(t, 3000, 1000, color.RGBA{R: 200, G: 30, B: 40, A: 255})
	res, err := Process(src)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.FullExt != ".jpg" {
		t.Errorf("FullExt = %q, want .jpg", res.FullExt)
	}
	if res.ThumbExt != ".jpg" {
		t.Errorf("ThumbExt = %q, want .jpg", res.ThumbExt)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(res.Full))
	if err != nil {
		t.Fatalf("decode processed full: %v", err)
	}
	if max(cfg.Width, cfg.Height) != MaxDimension {
		t.Errorf("full max edge = %d, want %d", max(cfg.Width, cfg.Height), MaxDimension)
	}
	// Aspect preserved: 3000x1000 -> 2048x682
	if cfg.Width != 2048 || cfg.Height != 682 {
		t.Errorf("full dims = %dx%d, want 2048x682", cfg.Width, cfg.Height)
	}
	tcfg, _, err := image.DecodeConfig(bytes.NewReader(res.Thumb))
	if err != nil {
		t.Fatalf("decode thumb: %v", err)
	}
	if tcfg.Width != ThumbSize || tcfg.Height != ThumbSize {
		t.Errorf("thumb dims = %dx%d, want %dx%d", tcfg.Width, tcfg.Height, ThumbSize, ThumbSize)
	}
	if res.Width != 2048 || res.Height != 682 {
		t.Errorf("result dims = %dx%d, want 2048x682", res.Width, res.Height)
	}
}

func TestProcess_JPEGUnderLimitNotResized(t *testing.T) {
	src := solidJPEG(t, 800, 600, color.RGBA{G: 255, A: 255})
	res, err := Process(src)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Width != 800 || res.Height != 600 {
		t.Errorf("dims = %dx%d, want 800x600 (no upscale/downscale)", res.Width, res.Height)
	}
}

func TestProcess_PNGWithAlphaStaysPNG(t *testing.T) {
	// Right half fully transparent -> alpha detected, PNG preserved.
	src := solidPNG(t, 400, 400, color.NRGBA{R: 10, G: 20, B: 30, A: 0})
	// Overwrite the left half opaque by building a two-tone image instead.
	img := image.NewNRGBA(image.Rect(0, 0, 400, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			if x < 200 {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	src = buf.Bytes()

	res, err := Process(src)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.FullExt != ".png" {
		t.Errorf("FullExt = %q, want .png (transparency preserved)", res.FullExt)
	}
	if res.ThumbExt != ".png" {
		t.Errorf("ThumbExt = %q, want .png", res.ThumbExt)
	}
	decoded, err := png.Decode(bytes.NewReader(res.Full))
	if err != nil {
		t.Fatalf("decode processed full: %v", err)
	}
	// Center-crop square of 400x400 is 400x400; check a transparent pixel.
	if _, _, a, _ := decoded.At(300, 200).RGBA(); a != 0 {
		t.Errorf("alpha lost: transparent pixel has alpha %d", a)
	}
}

func TestProcess_AnimatedGIFPassthrough(t *testing.T) {
	// Build a 2-frame GIF.
	pal := color.Palette{color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}}
	g := &gif.GIF{}
	for i := 0; i < 2; i++ {
		img := image.NewPaletted(image.Rect(0, 0, 100, 100), pal)
		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				img.SetColorIndex(x, y, uint8(i))
			}
		}
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("encode gif: %v", err)
	}

	res, err := Process(buf.Bytes())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.FullExt != ".gif" {
		t.Errorf("FullExt = %q, want .gif", res.FullExt)
	}
	if !bytes.Equal(res.Full, buf.Bytes()) {
		t.Error("animated GIF full should pass through byte-for-byte")
	}
	if res.ThumbExt != ".jpg" {
		t.Errorf("ThumbExt = %q, want .jpg", res.ThumbExt)
	}
	tcfg, _, err := image.DecodeConfig(bytes.NewReader(res.Thumb))
	if err != nil {
		t.Fatalf("decode thumb: %v", err)
	}
	if tcfg.Width != tcfg.Height {
		t.Errorf("thumb not square: %dx%d", tcfg.Width, tcfg.Height)
	}
}

func TestProcess_InvalidData(t *testing.T) {
	if _, err := Process([]byte("not an image")); err == nil {
		t.Fatal("expected error for garbage input")
	}
}

func TestProcess_ThumbCenterCrop(t *testing.T) {
	// 400x200 red image with a green 100-wide center (x 150..250): the
	// center-crop square is x 100..300, so edges stay red and the middle
	// is green.
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			c := color.RGBA{R: 255, A: 255}
			if x >= 150 && x < 250 {
				c = color.RGBA{G: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}

	res, err := Process(buf.Bytes())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	thumb, err := jpeg.Decode(bytes.NewReader(res.Thumb))
	if err != nil {
		t.Fatalf("decode thumb: %v", err)
	}
	b := thumb.Bounds()
	// Center pixel must lean green (g dominates r).
	r, g, _, _ := thumb.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA()
	if g <= r {
		t.Errorf("center pixel not green: r=%d g=%d", r>>8, g>>8)
	}
	// Left-edge pixel must lean red (crop excludes x<100).
	r, g, _, _ = thumb.At(b.Min.X+2, b.Min.Y+b.Dy()/2).RGBA()
	if r <= g {
		t.Errorf("left edge pixel not red: r=%d g=%d", r>>8, g>>8)
	}
}

func TestProcess_SmallImageThumbNotUpscaled(t *testing.T) {
	src := solidJPEG(t, 64, 64, color.RGBA{B: 255, A: 255})
	res, err := Process(src)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	tcfg, _, err := image.DecodeConfig(bytes.NewReader(res.Thumb))
	if err != nil {
		t.Fatalf("decode thumb: %v", err)
	}
	if tcfg.Width != 64 || tcfg.Height != 64 {
		t.Errorf("thumb dims = %dx%d, want 64x64 (no upscale)", tcfg.Width, tcfg.Height)
	}
}

func TestThumbName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1729-photo-ab12cd34.jpg", "1729-photo-ab12cd34-thumb.jpg"},
		{"shot.PNG", "shot-thumb.png"},
		{"anim.gif", "anim-thumb.jpg"},
		{"noext", "noext"},
		{"weird.tar.gz", "weird.tar.gz"}, // non-image ext -> unchanged
	}
	for _, c := range cases {
		if got := ThumbName(c.in); got != c.want {
			t.Errorf("ThumbName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
