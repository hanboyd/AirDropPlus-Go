//go:build windows

package clipboard

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDIBPNGRoundTrip(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(2, 1, color.NRGBA{B: 255, A: 255})
	b, err := dibToPNG(imageToDIB(img))
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != img.Bounds() {
		t.Fatalf("bounds=%v", got.Bounds())
	}
	r, _, b0, _ := got.At(0, 0).RGBA()
	if r < 60000 || b0 > 1000 {
		t.Fatalf("top-left color incorrect: %v", got.At(0, 0))
	}
	_, _, b1, _ := got.At(2, 1).RGBA()
	if b1 < 60000 {
		t.Fatalf("bottom-right color incorrect: %v", got.At(2, 1))
	}
}

func TestDIBWithUnusedAlphaIsOpaque(t *testing.T) {
	dib := make([]byte, 44)
	binary.LittleEndian.PutUint32(dib[0:4], 40)
	binary.LittleEndian.PutUint32(dib[4:8], 1)
	binary.LittleEndian.PutUint32(dib[8:12], 1)
	binary.LittleEndian.PutUint16(dib[12:14], 1)
	binary.LittleEndian.PutUint16(dib[14:16], 32)
	// BGRA blue pixel; legacy BI_RGB commonly leaves the alpha byte at zero.
	dib[40], dib[41], dib[42], dib[43] = 255, 0, 0, 0
	pngData, err := dibToPNG(dib)
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatal(err)
	}
	_, _, blue, alpha := got.At(0, 0).RGBA()
	if blue < 60000 || alpha < 60000 {
		t.Fatalf("pixel should be opaque blue: %v", got.At(0, 0))
	}
}

func TestDIBV5Bitfields(t *testing.T) {
	dib := make([]byte, 128)
	binary.LittleEndian.PutUint32(dib[0:4], 124)
	binary.LittleEndian.PutUint32(dib[4:8], 1)
	binary.LittleEndian.PutUint32(dib[8:12], 1)
	binary.LittleEndian.PutUint16(dib[12:14], 1)
	binary.LittleEndian.PutUint16(dib[14:16], 32)
	binary.LittleEndian.PutUint32(dib[16:20], biBitFields)
	binary.LittleEndian.PutUint32(dib[40:44], 0x00ff0000)
	binary.LittleEndian.PutUint32(dib[44:48], 0x0000ff00)
	binary.LittleEndian.PutUint32(dib[48:52], 0x000000ff)
	binary.LittleEndian.PutUint32(dib[52:56], 0xff000000)
	binary.LittleEndian.PutUint32(dib[124:128], 0x80ff0000)
	pngData, err := dibToPNG(dib)
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatal(err)
	}
	pixel := color.NRGBAModel.Convert(got.At(0, 0)).(color.NRGBA)
	if pixel.R != 255 || pixel.G != 0 || pixel.B != 0 || pixel.A != 128 {
		t.Fatalf("unexpected V5 pixel: %v", pixel)
	}
}
