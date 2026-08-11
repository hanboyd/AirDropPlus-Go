//go:build windows

package clipboard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	cfTextUnicode = 13
	cfDIB         = 8
	cfDIBV5       = 17
	cfHDROP       = 15
	gmemMoveable  = 0x0002
	biRGB         = 0
	biBitFields   = 3
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	procEmptyClipboard         = user32.NewProc("EmptyClipboard")
	procIsClipboardFormatAvail = user32.NewProc("IsClipboardFormatAvailable")
	procGetClipboardData       = user32.NewProc("GetClipboardData")
	procSetClipboardData       = user32.NewProc("SetClipboardData")
	procGlobalAlloc            = kernel32.NewProc("GlobalAlloc")
	procGlobalLock             = kernel32.NewProc("GlobalLock")
	procGlobalUnlock           = kernel32.NewProc("GlobalUnlock")
	procGlobalSize             = kernel32.NewProc("GlobalSize")
)

type Windows struct{}

func New() *Windows { return &Windows{} }

func formatAvailable(format uintptr) bool {
	r, _, _ := procIsClipboardFormatAvail.Call(format)
	return r != 0
}

func open() error {
	r, _, err := procOpenClipboard.Call(0)
	if r == 0 {
		return fmt.Errorf("OpenClipboard: %w", err)
	}
	return nil
}

func clipboardBytes(format uintptr) ([]byte, error) {
	if !formatAvailable(format) {
		return nil, ErrEmpty
	}
	if err := open(); err != nil {
		return nil, err
	}
	defer procCloseClipboard.Call()
	h, _, err := procGetClipboardData.Call(format)
	if h == 0 {
		return nil, fmt.Errorf("GetClipboardData: %w", err)
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return nil, fmt.Errorf("GlobalLock: %w", err)
	}
	defer procGlobalUnlock.Call(h)
	n, _, _ := procGlobalSize.Call(h)
	if n == 0 {
		return nil, ErrEmpty
	}
	return append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(p)), n)...), nil
}

func (w *Windows) Read() (Content, error) {
	if formatAvailable(cfTextUnicode) {
		b, err := clipboardBytes(cfTextUnicode)
		if err != nil {
			return Content{}, err
		}
		units := unsafe.Slice((*uint16)(unsafe.Pointer(&b[0])), len(b)/2)
		end := 0
		for end < len(units) && units[end] != 0 {
			end++
		}
		return Content{Kind: Text, Text: string(utf16.Decode(units[:end]))}, nil
	}
	if formatAvailable(cfHDROP) {
		b, err := clipboardBytes(cfHDROP)
		if err != nil {
			return Content{}, err
		}
		files, err := decodeHDROP(b)
		if err != nil {
			return Content{}, err
		}
		return Content{Kind: Files, Files: files}, nil
	}
	imageFormat := uintptr(0)
	if formatAvailable(cfDIBV5) {
		imageFormat = cfDIBV5
	} else if formatAvailable(cfDIB) {
		imageFormat = cfDIB
	}
	if imageFormat != 0 {
		b, err := clipboardBytes(imageFormat)
		if err != nil {
			return Content{}, err
		}
		p, err := dibToPNG(b)
		if err != nil {
			return Content{}, err
		}
		return Content{Kind: Image, PNG: p}, nil
	}
	return Content{}, ErrEmpty
}

func (w *Windows) WriteText(text string) error {
	units := append(utf16.Encode([]rune(text)), 0)
	b := unsafe.Slice((*byte)(unsafe.Pointer(&units[0])), len(units)*2)
	return setClipboardBytes(cfTextUnicode, b)
}

func (w *Windows) WritePNG(data []byte) error {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode PNG: %w", err)
	}
	return setClipboardBytes(cfDIB, imageToDIB(img))
}

func setClipboardBytes(format uintptr, data []byte) error {
	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return fmt.Errorf("GlobalAlloc: %w", err)
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return fmt.Errorf("GlobalLock: %w", err)
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
	procGlobalUnlock.Call(h)
	if err := open(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("EmptyClipboard: %w", err)
	}
	if r, _, err := procSetClipboardData.Call(format, h); r == 0 {
		return fmt.Errorf("SetClipboardData: %w", err)
	}
	return nil
}

func decodeHDROP(b []byte) ([]string, error) {
	if len(b) < 20 {
		return nil, fmt.Errorf("invalid CF_HDROP header")
	}
	off := int(binary.LittleEndian.Uint32(b[0:4]))
	wide := binary.LittleEndian.Uint32(b[16:20]) != 0
	if off < 20 || off >= len(b) || !wide {
		return nil, fmt.Errorf("unsupported CF_HDROP data")
	}
	u := make([]uint16, (len(b)-off)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[off+i*2:])
	}
	var out []string
	for start, i := 0, 0; i < len(u); i++ {
		if u[i] != 0 {
			continue
		}
		if i == start {
			break
		}
		out = append(out, string(utf16.Decode(u[start:i])))
		start = i + 1
	}
	if len(out) == 0 {
		return nil, ErrEmpty
	}
	return out, nil
}

func imageToDIB(src image.Image) []byte {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	stride := w * 4
	out := make([]byte, 40+stride*h)
	binary.LittleEndian.PutUint32(out[0:4], 40)
	binary.LittleEndian.PutUint32(out[4:8], uint32(w))
	binary.LittleEndian.PutUint32(out[8:12], uint32(h))
	binary.LittleEndian.PutUint16(out[12:14], 1)
	binary.LittleEndian.PutUint16(out[14:16], 32)
	binary.LittleEndian.PutUint32(out[20:24], uint32(stride*h))
	for y := 0; y < h; y++ {
		row := 40 + (h-1-y)*stride
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			i := row + x*4
			out[i], out[i+1], out[i+2], out[i+3] = c.B, c.G, c.R, c.A
		}
	}
	return out
}

func dibToPNG(b []byte) ([]byte, error) {
	if len(b) < 40 {
		return nil, fmt.Errorf("DIB header is too short")
	}
	header := int(binary.LittleEndian.Uint32(b[0:4]))
	w := int(int32(binary.LittleEndian.Uint32(b[4:8])))
	rawH := int32(binary.LittleEndian.Uint32(b[8:12]))
	bits := int(binary.LittleEndian.Uint16(b[14:16]))
	compression := binary.LittleEndian.Uint32(b[16:20])
	if header < 40 || header > len(b) || w <= 0 || rawH == 0 || (bits != 24 && bits != 32) || (compression != biRGB && compression != biBitFields) {
		return nil, fmt.Errorf("unsupported DIB: header=%d width=%d height=%d bits=%d compression=%d", header, w, rawH, bits, compression)
	}
	if compression == biBitFields && bits != 32 {
		return nil, fmt.Errorf("unsupported bitfield DIB depth: %d", bits)
	}
	h := int(rawH)
	topDown := h < 0
	if topDown {
		h = -h
	}
	pixelOffset := header
	redMask, greenMask, blueMask, alphaMask := uint32(0x00ff0000), uint32(0x0000ff00), uint32(0x000000ff), uint32(0xff000000)
	if compression == biBitFields {
		const maskOffset = 40
		if maskOffset+12 > len(b) {
			return nil, fmt.Errorf("truncated DIB color masks")
		}
		redMask = binary.LittleEndian.Uint32(b[maskOffset : maskOffset+4])
		greenMask = binary.LittleEndian.Uint32(b[maskOffset+4 : maskOffset+8])
		blueMask = binary.LittleEndian.Uint32(b[maskOffset+8 : maskOffset+12])
		alphaMask = 0
		if header >= 56 && maskOffset+16 <= len(b) {
			alphaMask = binary.LittleEndian.Uint32(b[maskOffset+12 : maskOffset+16])
		}
		if header == 40 {
			pixelOffset += 12
		}
	}
	stride := ((w*bits + 31) / 32) * 4
	if pixelOffset+stride*h > len(b) {
		return nil, fmt.Errorf("truncated DIB")
	}
	useBIAlpha := false
	if bits == 32 && compression == biRGB {
		for i := pixelOffset + 3; i < pixelOffset+stride*h; i += 4 {
			if b[i] != 0 {
				useBIAlpha = true
				break
			}
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := h - 1 - y
		if topDown {
			sy = y
		}
		row := pixelOffset + sy*stride
		for x := 0; x < w; x++ {
			i := row + x*(bits/8)
			if compression == biBitFields {
				value := binary.LittleEndian.Uint32(b[i : i+4])
				a := byte(255)
				if alphaMask != 0 {
					a = maskByte(value, alphaMask)
				}
				img.SetNRGBA(x, y, color.NRGBA{
					R: maskByte(value, redMask), G: maskByte(value, greenMask),
					B: maskByte(value, blueMask), A: a,
				})
				continue
			}
			a := byte(255)
			if bits == 32 && useBIAlpha {
				a = b[i+3]
			}
			img.SetNRGBA(x, y, color.NRGBA{R: b[i+2], G: b[i+1], B: b[i], A: a})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func maskByte(value, mask uint32) byte {
	if mask == 0 {
		return 0
	}
	shift := 0
	for mask&(1<<shift) == 0 {
		shift++
	}
	max := mask >> shift
	component := (value & mask) >> shift
	return byte((uint64(component)*255 + uint64(max)/2) / uint64(max))
}
