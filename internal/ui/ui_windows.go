//go:build windows

package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/hanboyd/AirDropPlus-Go/internal/bridge"
	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/device"
	"github.com/hanboyd/AirDropPlus-Go/internal/history"
)

const (
	wmDestroy                   = 0x0002
	wmPaint                     = 0x000F
	wmClose                     = 0x0010
	wmTimer                     = 0x0113
	wmLButtonUp                 = 0x0202
	wmApp                       = 0x8000
	wmTray                      = wmApp + 1
	wmIncoming                  = wmApp + 2
	wmDevice                    = wmApp + 3
	wmClipboardUpdate           = 0x031D
	wsPopup                     = 0x80000000
	wsExTopmost                 = 0x00000008
	wsExToolwindow              = 0x00000080
	swHide                      = 0
	swShow                      = 5
	swpNoActivate               = 0x0010
	nimAdd                      = 0
	nimModify                   = 1
	nimDelete                   = 2
	nifMessage                  = 1
	nifIcon                     = 2
	nifTip                      = 4
	idiApplication              = 32512
	idiInformation              = 32516
	dtLeft                      = 0
	dtCenter                    = 1
	dtRight                     = 2
	dtVCenter                   = 4
	dtSingleLine                = 0x20
	dtEndEllipsis               = 0x8000
	transparent                 = 1
	spiGetWorkArea              = 0x0030
	dwmwaWindowCornerPreference = 33
	dwmwcRound                  = 2
)

var (
	user32                            = syscall.NewLazyDLL("user32.dll")
	gdi32                             = syscall.NewLazyDLL("gdi32.dll")
	shell32                           = syscall.NewLazyDLL("shell32.dll")
	kernel32                          = syscall.NewLazyDLL("kernel32.dll")
	dwmapi                            = syscall.NewLazyDLL("dwmapi.dll")
	procRegisterClassEx               = user32.NewProc("RegisterClassExW")
	procCreateWindowEx                = user32.NewProc("CreateWindowExW")
	procDefWindowProc                 = user32.NewProc("DefWindowProcW")
	procGetMessage                    = user32.NewProc("GetMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessage               = user32.NewProc("DispatchMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procPostMessage                   = user32.NewProc("PostMessageW")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procInvalidateRect                = user32.NewProc("InvalidateRect")
	procBeginPaint                    = user32.NewProc("BeginPaint")
	procEndPaint                      = user32.NewProc("EndPaint")
	procGetClientRect                 = user32.NewProc("GetClientRect")
	procFillRect                      = user32.NewProc("FillRect")
	procDrawText                      = user32.NewProc("DrawTextW")
	procSetBkMode                     = gdi32.NewProc("SetBkMode")
	procSetTextColor                  = gdi32.NewProc("SetTextColor")
	procCreateSolidBrush              = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject                  = gdi32.NewProc("DeleteObject")
	procCreateFont                    = gdi32.NewProc("CreateFontW")
	procAddFontResourceEx             = gdi32.NewProc("AddFontResourceExW")
	procRemoveFontResourceEx          = gdi32.NewProc("RemoveFontResourceExW")
	procSelectObject                  = gdi32.NewProc("SelectObject")
	procRoundRect                     = gdi32.NewProc("RoundRect")
	procStretchDIBits                 = gdi32.NewProc("StretchDIBits")
	procCreatePen                     = gdi32.NewProc("CreatePen")
	procMoveToEx                      = gdi32.NewProc("MoveToEx")
	procLineTo                        = gdi32.NewProc("LineTo")
	procLoadIcon                      = user32.NewProc("LoadIconW")
	procLoadCursor                    = user32.NewProc("LoadCursorW")
	procSystemParametersInfo          = user32.NewProc("SystemParametersInfoW")
	procSetTimer                      = user32.NewProc("SetTimer")
	procKillTimer                     = user32.NewProc("KillTimer")
	procAddClipboardFormatListener    = user32.NewProc("AddClipboardFormatListener")
	procRemoveClipboardFormatListener = user32.NewProc("RemoveClipboardFormatListener")
	procShellNotifyIcon               = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandle               = kernel32.NewProc("GetModuleHandleW")
	procDwmSetWindowAttribute         = dwmapi.NewProc("DwmSetWindowAttribute")
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type msg struct {
	Hwnd           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             point
	Private        uint32
}
type paintstruct struct {
	Hdc                uintptr
	Erase              int32
	RcPaint            rect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type wndclassex struct {
	Size                                                            uint32
	Style                                                           uint32
	WndProc                                                         uintptr
	ClsExtra, WndExtra                                              int32
	Instance, Icon, Cursor, Background, MenuName, ClassName, IconSm uintptr
}
type notifyicondata struct {
	Size                       uint32
	Hwnd                       uintptr
	ID, Flags, CallbackMessage uint32
	Icon                       uintptr
	Tip                        [128]uint16
	State, StateMask           uint32
	Info                       [256]uint16
	Version                    uint32
	InfoTitle                  [64]uint16
	InfoFlags                  uint32
	Guid                       [16]byte
	BalloonIcon                uintptr
}
type hit struct {
	r      rect
	action string
	id     uint64
}
type bitmapInfoHeader struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
}
type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}
type thumbnail struct {
	pixels        []byte
	width, height int32
}

type app struct {
	store        *history.Store
	bridge       *bridge.Service
	tracker      *device.Tracker
	main, popup  uintptr
	hits         []hit
	tray         notifyicondata
	visible      bool
	incoming     <-chan history.Event
	unsubscribe  func()
	thumbnails   map[uint64]thumbnail
	privateFonts []string
}

var current *app

func Run(ctx context.Context, store *history.Store, service *bridge.Service, tracker *device.Tracker) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &app{store: store, bridge: service, tracker: tracker, thumbnails: make(map[uint64]thumbnail)}
	current = a
	a.incoming, a.unsubscribe = store.Subscribe()
	defer a.unsubscribe()
	if err := a.create(); err != nil {
		return err
	}
	defer a.cleanup()
	go func() {
		for {
			select {
			case <-ctx.Done():
				procPostMessage.Call(a.main, wmClose, 0, 0)
				return
			case ev, ok := <-a.incoming:
				if !ok {
					return
				}
				if ev.Item.Source == history.FromIPhone {
					procPostMessage.Call(a.main, wmIncoming, 0, 0)
				}
			case <-tracker.Changed():
				procPostMessage.Call(a.main, wmDevice, 0, 0)
			}
		}
	}()
	var m msg
	for {
		r, _, e := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) == -1 {
			return e
		}
		if r == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

func (a *app) create() error {
	a.loadPrivateFonts()
	instance, _, _ := procGetModuleHandle.Call(0)
	class, _ := syscall.UTF16PtrFromString("AirDropPlusGoNativeUI")
	cursor, _, _ := procLoadCursor.Call(0, 32512)
	wc := wndclassex{Size: uint32(unsafe.Sizeof(wndclassex{})), WndProc: syscall.NewCallback(windowProc), Instance: instance, Cursor: cursor, ClassName: uintptr(unsafe.Pointer(class))}
	if r, _, e := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("register window class: %w", e)
	}
	mainTitle, _ := syscall.UTF16PtrFromString("AirDropPlus-Go")
	a.main, _, _ = procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(mainTitle)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if a.main == 0 {
		return fmt.Errorf("create message window")
	}
	a.popup, _, _ = procCreateWindowEx.Call(wsExTopmost|wsExToolwindow, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(mainTitle)), wsPopup, 0, 0, 390, 548, 0, 0, instance, 0)
	if a.popup == 0 {
		return fmt.Errorf("create popup")
	}
	corner := uint32(dwmwcRound)
	procDwmSetWindowAttribute.Call(a.popup, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))
	procAddClipboardFormatListener.Call(a.main)
	procSetTimer.Call(a.main, 1, 30000, 0)
	a.updateTray(true)
	return nil
}

func (a *app) cleanup() {
	procKillTimer.Call(a.main, 1)
	procRemoveClipboardFormatListener.Call(a.main)
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&a.tray)))
	for _, path := range a.privateFonts {
		p, _ := syscall.UTF16PtrFromString(path)
		procRemoveFontResourceEx.Call(uintptr(unsafe.Pointer(p)), 0x10, 0)
	}
}

func (a *app) loadPrivateFonts() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	for _, name := range []string{"CharterBT-Roman.otf", "CharterBT-Bold.otf"} {
		path := filepath.Join(filepath.Dir(exe), "fonts", name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		p, _ := syscall.UTF16PtrFromString(path)
		if n, _, _ := procAddFontResourceEx.Call(uintptr(unsafe.Pointer(p)), 0x10, 0); n > 0 {
			a.privateFonts = append(a.privateFonts, path)
		}
	}
}

func windowProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	a := current
	if a == nil {
		r, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wparam, lparam)
		return r
	}
	if hwnd == a.popup {
		return a.popupProc(message, wparam, lparam)
	}
	switch message {
	case wmTray:
		if uint32(lparam) == wmLButtonUp {
			a.toggle()
		}
		return 0
	case wmIncoming:
		a.show()
		return 0
	case wmDevice, wmTimer:
		a.updateTray(false)
		procInvalidateRect.Call(a.popup, 0, 1)
		return 0
	case wmClipboardUpdate:
		if c, err := a.bridge.ReadLocal(); err == nil {
			a.bridge.RecordLocal(c)
			procInvalidateRect.Call(a.popup, 0, 1)
		}
		return 0
	case wmClose, wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wparam, lparam)
	return r
}

func (a *app) popupProc(message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case wmPaint:
		a.paint()
		return 0
	case wmLButtonUp:
		procSetTimer.Call(a.popup, 2, 10000, 0)
		a.click(int32(int16(lparam&0xffff)), int32(int16((lparam>>16)&0xffff)))
		return 0
	case wmTimer:
		if wparam == 2 {
			a.hide()
			return 0
		}
	case wmClose:
		a.hide()
		return 0
	}
	r, _, _ := procDefWindowProc.Call(a.popup, uintptr(message), wparam, lparam)
	return r
}

func (a *app) toggle() {
	if a.visible {
		a.hide()
	} else {
		a.show()
	}
}
func (a *app) hide() {
	procKillTimer.Call(a.popup, 2)
	procShowWindow.Call(a.popup, swHide)
	a.visible = false
}
func (a *app) show() {
	var work rect
	procSystemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&work)), 0)
	x := work.Right - 406
	y := work.Bottom - 564
	procSetWindowPos.Call(a.popup, ^uintptr(0), uintptr(x), uintptr(y), 390, 548, swpNoActivate)
	procShowWindow.Call(a.popup, swShow)
	procSetTimer.Call(a.popup, 2, 10000, 0)
	procInvalidateRect.Call(a.popup, 0, 1)
	a.visible = true
}

func (a *app) updateTray(add bool) {
	iconID := uintptr(idiApplication)
	state := "iPhone 未连接"
	if a.tracker.Online() {
		iconID = idiInformation
		s := a.tracker.State()
		state = "iPhone 已连接 · " + s.IP
	}
	icon, _, _ := procLoadIcon.Call(0, iconID)
	a.tray = notifyicondata{Size: uint32(unsafe.Sizeof(notifyicondata{})), Hwnd: a.main, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTray, Icon: icon}
	copy(a.tray.Tip[:], syscall.StringToUTF16("AirDropPlus-Go · "+state))
	op := uintptr(nimModify)
	if add {
		op = nimAdd
	}
	procShellNotifyIcon.Call(op, uintptr(unsafe.Pointer(&a.tray)))
}

func (a *app) paint() {
	var ps paintstruct
	hdc, _, _ := procBeginPaint.Call(a.popup, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(a.popup, uintptr(unsafe.Pointer(&ps)))
	var bounds rect
	procGetClientRect.Call(a.popup, uintptr(unsafe.Pointer(&bounds)))
	fill(hdc, bounds, rgb(248, 247, 243))
	procSetBkMode.Call(hdc, transparent)
	fontTitle := font(-22, 600, "Bitstream Charter")
	fontCN := font(-16, 400, "TsangerJinKai02")
	fontSmall := font(-13, 400, "TsangerJinKai02")
	defer procDeleteObject.Call(fontTitle)
	defer procDeleteObject.Call(fontCN)
	defer procDeleteObject.Call(fontSmall)
	a.hits = nil
	selectFont(hdc, fontTitle)
	text(hdc, "AirDropPlus", 18, 16, 220, 48, rgb(25, 25, 27), dtLeft|dtSingleLine)
	state := "未连接"
	detail := "等待 iPhone"
	online := a.tracker.Online()
	if online {
		s := a.tracker.State()
		state = "iPhone 已连接"
		detail = s.IP + " · " + relative(s.LastSeen)
	}
	fillRound(hdc, rect{18, 56, 372, 116}, 14, rgb(255, 255, 255), rgb(224, 223, 218))
	dotColor := rgb(150, 150, 150)
	if online {
		dotColor = rgb(67, 181, 103)
	}
	fillRound(hdc, rect{34, 75, 48, 89}, 7, dotColor, dotColor)
	selectFont(hdc, fontCN)
	text(hdc, state, 58, 68, 245, 91, rgb(28, 28, 30), dtLeft|dtSingleLine)
	selectFont(hdc, fontSmall)
	text(hdc, detail, 58, 90, 245, 107, rgb(110, 109, 105), dtLeft|dtSingleLine)
	share := a.bridge.SharingPC()
	toggleColor := rgb(215, 214, 209)
	shareText := "PC 剪贴板共享：关"
	if share {
		toggleColor = rgb(41, 104, 171)
		shareText = "PC 剪贴板共享：开"
	}
	fillRound(hdc, rect{258, 73, 354, 101}, 14, toggleColor, toggleColor)
	textColor := rgb(50, 50, 52)
	if share {
		textColor = rgb(255, 255, 255)
	}
	text(hdc, shareText, 264, 77, 349, 98, textColor, dtCenter|dtVCenter|dtSingleLine)
	a.hits = append(a.hits, hit{rect{250, 66, 365, 108}, "toggle", 0})
	selectFont(hdc, fontCN)
	text(hdc, "最近剪贴板", 18, 132, 220, 157, rgb(28, 28, 30), dtLeft|dtSingleLine)
	selectFont(hdc, fontSmall)
	text(hdc, "仅保留本次运行的最近 10 条", 188, 136, 372, 154, rgb(120, 118, 113), dtRight|dtSingleLine)
	y := 166
	items := a.store.List()
	validThumbs := make(map[uint64]struct{}, len(items))
	for _, item := range items {
		validThumbs[item.ID] = struct{}{}
	}
	for id := range a.thumbnails {
		if _, ok := validThumbs[id]; !ok {
			delete(a.thumbnails, id)
		}
	}
	if len(items) == 0 {
		fillRound(hdc, rect{18, int32(y), 372, int32(y + 78)}, 12, rgb(255, 255, 255), rgb(229, 227, 221))
		text(hdc, "暂无内容。手机发送后会自动弹出。", 34, int32(y+24), 356, int32(y+55), rgb(125, 122, 116), dtCenter|dtVCenter|dtSingleLine)
	}
	for i, item := range items {
		if i >= 4 || y > 470 {
			break
		}
		h := 78
		if item.Content.Kind == clipboard.Image {
			h = 96
		}
		card := rect{18, int32(y), 372, int32(y + h)}
		fillRound(hdc, card, 12, rgb(255, 255, 255), rgb(229, 227, 221))
		selectFont(hdc, fontCN)
		if item.Content.Kind == clipboard.Image {
			a.drawImageCard(hdc, item.ID, item.Content.PNG, 30, int32(y+12), 92, int32(y+82))
			text(hdc, "图片剪贴板", 108, int32(y+17), 250, int32(y+42), rgb(28, 28, 30), dtLeft|dtSingleLine)
			selectFont(hdc, fontSmall)
			text(hdc, sourceLabel(item.Source)+" · "+relative(item.Created), 108, int32(y+44), 260, int32(y+65), rgb(115, 112, 107), dtLeft|dtSingleLine)
		} else {
			body := item.Content.Text
			if item.Content.Kind == clipboard.Files {
				body = strings.Join(item.Content.Files, ", ")
			}
			text(hdc, body, 32, int32(y+12), 350, int32(y+44), rgb(28, 28, 30), dtLeft|dtEndEllipsis)
			selectFont(hdc, fontSmall)
			text(hdc, sourceLabel(item.Source)+" · "+relative(item.Created), 32, int32(y+49), 205, int32(y+68), rgb(115, 112, 107), dtLeft|dtSingleLine)
		}
		pin := "置顶"
		if item.Pinned {
			pin = "取消置顶"
		}
		text(hdc, "复制", 252, int32(y+h-27), 286, int32(y+h-8), rgb(41, 104, 171), dtCenter|dtSingleLine)
		text(hdc, pin, 288, int32(y+h-27), 334, int32(y+h-8), rgb(41, 104, 171), dtCenter|dtSingleLine)
		text(hdc, "删除", 336, int32(y+h-27), 365, int32(y+h-8), rgb(186, 55, 48), dtCenter|dtSingleLine)
		a.hits = append(a.hits, hit{rect{244, int32(y + h - 34), 288, int32(y + h)}, "copy", item.ID}, hit{rect{288, int32(y + h - 34), 337, int32(y + h)}, "pin", item.ID}, hit{rect{337, int32(y + h - 34), 374, int32(y + h)}, "delete", item.ID})
		y += h + 9
	}
	selectFont(hdc, fontSmall)
	text(hdc, "点击托盘图标打开 · 空闲时自动隐藏", 18, 520, 372, 542, rgb(125, 122, 116), dtCenter|dtSingleLine)
}

func (a *app) click(x, y int32) {
	for _, h := range a.hits {
		if x >= h.r.Left && x < h.r.Right && y >= h.r.Top && y < h.r.Bottom {
			switch h.action {
			case "toggle":
				a.bridge.ToggleSharingPC()
			case "copy":
				for _, it := range a.store.List() {
					if it.ID == h.id {
						_ = a.bridge.CopyToPC(it.Content)
						break
					}
				}
			case "pin":
				a.store.Pin(h.id)
			case "delete":
				a.store.Delete(h.id)
			}
			procInvalidateRect.Call(a.popup, 0, 1)
			return
		}
	}
}

func font(height int32, weight int32, name string) uintptr {
	p, _ := syscall.UTF16PtrFromString(name)
	r, _, _ := procCreateFont.Call(uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(p)))
	return r
}
func selectFont(hdc, f uintptr) { procSelectObject.Call(hdc, f) }
func text(hdc uintptr, s string, l, t, r, b int32, color uint32, flags uint32) {
	p, _ := syscall.UTF16PtrFromString(s)
	rc := rect{l, t, r, b}
	procSetTextColor.Call(hdc, uintptr(color))
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(p)), uintptr(len([]rune(s))), uintptr(unsafe.Pointer(&rc)), uintptr(flags))
}
func fill(hdc uintptr, r rect, color uint32) {
	b, _, _ := procCreateSolidBrush.Call(uintptr(color))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), b)
	procDeleteObject.Call(b)
}
func fillRound(hdc uintptr, r rect, radius int32, fillColor, border uint32) {
	b, _, _ := procCreateSolidBrush.Call(uintptr(fillColor))
	p, _, _ := procCreatePen.Call(0, 1, uintptr(border))
	oldB, _, _ := procSelectObject.Call(hdc, b)
	oldP, _, _ := procSelectObject.Call(hdc, p)
	procRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius), uintptr(radius))
	procSelectObject.Call(hdc, oldB)
	procSelectObject.Call(hdc, oldP)
	procDeleteObject.Call(b)
	procDeleteObject.Call(p)
}
func (a *app) drawImageCard(hdc uintptr, id uint64, png []byte, l, t, r, b int32) {
	fillRound(hdc, rect{l, t, r, b}, 8, rgb(239, 237, 231), rgb(215, 212, 204))
	thumb, ok := a.thumbnails[id]
	if !ok {
		thumb, ok = makeThumbnail(png, 128)
		if ok {
			a.thumbnails[id] = thumb
		}
	}
	if !ok || len(thumb.pixels) == 0 {
		f := font(-12, 400, "Bitstream Charter")
		defer procDeleteObject.Call(f)
		selectFont(hdc, f)
		text(hdc, "IMAGE", l, t, r, b, rgb(107, 104, 98), dtCenter|dtVCenter|dtSingleLine)
		return
	}
	dw, dh := r-l-8, b-t-8
	scale := min(float64(dw)/float64(thumb.width), float64(dh)/float64(thumb.height))
	w, h := int32(float64(thumb.width)*scale), int32(float64(thumb.height)*scale)
	x, y := l+(dw-w)/2+4, t+(dh-h)/2+4
	info := bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: thumb.width, Height: -thumb.height, Planes: 1, BitCount: 32, SizeImage: uint32(len(thumb.pixels))}}
	procStretchDIBits.Call(hdc, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, uintptr(thumb.width), uintptr(thumb.height), uintptr(unsafe.Pointer(&thumb.pixels[0])), uintptr(unsafe.Pointer(&info)), 0, 0x00CC0020)
}

func makeThumbnail(data []byte, maxSize int) (thumbnail, bool) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return thumbnail{}, false
	}
	bounds := img.Bounds()
	if bounds.Dx() < 1 || bounds.Dy() < 1 {
		return thumbnail{}, false
	}
	w, h := bounds.Dx(), bounds.Dy()
	if w > maxSize || h > maxSize {
		if w >= h {
			h = max(1, h*maxSize/w)
			w = maxSize
		} else {
			w = max(1, w*maxSize/h)
			h = maxSize
		}
	}
	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		sy := bounds.Min.Y + y*bounds.Dy()/h
		for x := 0; x < w; x++ {
			sx := bounds.Min.X + x*bounds.Dx()/w
			rr, gg, bb, aa := img.At(sx, sy).RGBA()
			i := (y*w + x) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = byte(bb>>8), byte(gg>>8), byte(rr>>8), byte(aa>>8)
		}
	}
	return thumbnail{pixels: pixels, width: int32(w), height: int32(h)}, true
}
func rgb(r, g, b uint32) uint32 { return r | (g << 8) | (b << 16) }
func sourceLabel(s history.Source) string {
	if s == history.FromIPhone {
		return "来自 iPhone"
	}
	return "来自 PC"
}
func relative(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "刚刚"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	}
	return t.Format("15:04")
}
