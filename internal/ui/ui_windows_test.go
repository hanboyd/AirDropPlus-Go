//go:build windows

package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestLongTextDetection(t *testing.T) {
	if isLongText("short text") {
		t.Fatal("short text was treated as long")
	}
	if !isLongText("first line\nsecond line") {
		t.Fatal("multiline text should expand")
	}
	if !isLongText("这是一个用于验证完整展开功能的长文本。它超过六十个字符以后，应当显示展开入口，而不是只留下无法继续阅读的省略号。这部分文字还会继续增加，确保长度超过界面预览阈值。") {
		t.Fatal("long CJK text should expand")
	}
}

func TestThumbnailContainsDecodedImagePixels(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	img.SetNRGBA(2, 1, color.NRGBA{R: 12, G: 100, B: 220, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	thumb, ok := makeThumbnail(b.Bytes(), 128)
	if !ok || thumb.width != 4 || thumb.height != 2 || len(thumb.pixels) != 32 {
		t.Fatalf("invalid thumbnail: ok=%v size=%dx%d bytes=%d", ok, thumb.width, thumb.height, len(thumb.pixels))
	}
	if thumb.pixels[(1*4+2)*4+2] != 12 || thumb.pixels[(1*4+2)*4+1] != 100 || thumb.pixels[(1*4+2)*4] != 220 {
		t.Fatal("thumbnail did not retain decoded pixel content")
	}
}
