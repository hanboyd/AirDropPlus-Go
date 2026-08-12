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
	"unicode/utf8"
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
	wmSetFont                   = 0x0030
	wmLButtonUp                 = 0x0202
	wmApp                       = 0x8000
	wmTray                      = wmApp + 1
	wmIncoming                  = wmApp + 2
	wmDevice                    = wmApp + 3
	wmClipboardUpdate           = 0x031D
	wmDpiChanged                = 0x02E0
	wsPopup                     = 0x80000000
	wsChild                     = 0x40000000
	wsVisible                   = 0x10000000
	wsVScroll                   = 0x00200000
	wsBorder                    = 0x00800000
	esMultiline                 = 0x0004
	esAutoVScroll               = 0x0040
	esReadOnly                  = 0x0800
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
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procMoveWindow                    = user32.NewProc("MoveWindow")
	procSetWindowText                 = user32.NewProc("SetWindowTextW")
	procSendMessage                   = user32.NewProc("SendMessageW")
	procInvalidateRect                = user32.NewProc("InvalidateRect")
	procBeginPaint                    = user32.NewProc("BeginPaint")
	procEndPaint                      = user32.NewProc("EndPaint")
	procGetClientRect                 = user32.NewProc("GetClientRect")
	procFillRect                      = user32.NewProc("FillRect")
	procDrawText                      = user32.NewProc("DrawTextW")
	procSetBkMode                     = gdi32.NewProc("SetBkMode")
	procSetTextColor                  = gdi32.NewProc("SetTextColor")
	procCreateSolidBrush              = gdi32.NewProc("CreateSolidBrush")
	procCreateBitmap                  = gdi32.NewProc("CreateBitmap")
	procCreateDIBSection              = gdi32.NewProc("CreateDIBSection")
	procDeleteObject                  = gdi32.NewProc("DeleteObject")
	procCreateFont                    = gdi32.NewProc("CreateFontW")
	procAddFontResourceEx             = gdi32.NewProc("AddFontResourceExW")
	procRemoveFontResourceEx          = gdi32.NewProc("RemoveFontResourceExW")
	procSelectObject                  = gdi32.NewProc("SelectObject")
	procRoundRect                     = gdi32.NewProc("RoundRect")
	procStretchDIBits                 = gdi32.NewProc("StretchDIBits")
	procSetStretchBltMode             = gdi32.NewProc("SetStretchBltMode")
	procSetBrushOrgEx                 = gdi32.NewProc("SetBrushOrgEx")
	procCreatePen                     = gdi32.NewProc("CreatePen")
	procMoveToEx                      = gdi32.NewProc("MoveToEx")
	procLineTo                        = gdi32.NewProc("LineTo")
	procLoadIcon                      = user32.NewProc("LoadIconW")
	procCreateIconIndirect            = user32.NewProc("CreateIconIndirect")
	procDestroyIcon                   = user32.NewProc("DestroyIcon")
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
type iconInfo struct {
	Icon               int32
	XHotspot, YHotspot uint32
	Mask, Color        uintptr
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
	edit         uintptr
	editFont     uintptr
	hits         []hit
	tray         notifyicondata
	visible      bool
	incoming     <-chan history.Event
	unsubscribe  func()
	thumbnails   map[uint64]thumbnail
	privateFonts []string
	idleIcon     uintptr
	receivedIcon uintptr
	unread       bool
	dpi          int32
	page         int
	expanded     uint64
	editItem     uint64
}

var current *app
var dpiAwarenessMode = "System DPI Aware"

func Run(ctx context.Context, store *history.Store, service *bridge.Service, tracker *device.Tracker) error {
	if ok, _, _ := procSetProcessDpiAwarenessContext.Call(^uintptr(3)); ok == 0 {
		procSetProcessDPIAware.Call()
	} else {
		dpiAwarenessMode = "Per-Monitor DPI Aware V2"
	}
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
	a.dpi = 96
	if dpi, _, _ := procGetDpiForSystem.Call(); dpi >= 96 {
		a.dpi = int32(dpi)
	}
	fmt.Printf("Native UI: %s, %d DPI\n", dpiAwarenessMode, a.dpi)
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
	a.popup, _, _ = procCreateWindowEx.Call(wsExTopmost|wsExToolwindow, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(mainTitle)), wsPopup, 0, 0, uintptr(a.px(390)), uintptr(a.px(548)), 0, 0, instance, 0)
	if a.popup == 0 {
		return fmt.Errorf("create popup")
	}
	editClass, _ := syscall.UTF16PtrFromString("EDIT")
	a.edit, _, _ = procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(editClass)), 0, wsChild|wsVisible|wsVScroll|wsBorder|esMultiline|esAutoVScroll|esReadOnly, uintptr(a.px(18)), uintptr(a.px(194)), uintptr(a.px(354)), uintptr(a.px(278)), a.popup, 0, instance, 0)
	if a.edit == 0 {
		return fmt.Errorf("create expanded text view")
	}
	a.editFont = font(-15, 400, "Microsoft YaHei UI")
	procSendMessage.Call(a.edit, wmSetFont, a.editFont, 1)
	procShowWindow.Call(a.edit, swHide)
	corner := uint32(dwmwcRound)
	procDwmSetWindowAttribute.Call(a.popup, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))
	procAddClipboardFormatListener.Call(a.main)
	procSetTimer.Call(a.main, 1, 30000, 0)
	a.idleIcon = createStatusIcon(false)
	a.receivedIcon = createStatusIcon(true)
	a.updateTray(true)
	return nil
}

func (a *app) cleanup() {
	procKillTimer.Call(a.main, 1)
	procRemoveClipboardFormatListener.Call(a.main)
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&a.tray)))
	if a.idleIcon != 0 {
		procDestroyIcon.Call(a.idleIcon)
	}
	if a.receivedIcon != 0 {
		procDestroyIcon.Call(a.receivedIcon)
	}
	if a.editFont != 0 {
		procDeleteObject.Call(a.editFont)
	}
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
			a.unread = false
			a.updateTray(false)
			a.toggle()
		}
		return 0
	case wmIncoming:
		a.unread = true
		a.page = 0
		a.updateTray(false)
		procInvalidateRect.Call(a.popup, 0, 1)
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
		a.click(a.logical(int32(int16(lparam&0xffff))), a.logical(int32(int16((lparam>>16)&0xffff))))
		return 0
	case wmDpiChanged:
		newDPI := int32(wparam & 0xffff)
		if newDPI >= 96 {
			a.dpi = newDPI
			if lparam != 0 {
				r := (*rect)(unsafe.Pointer(lparam))
				procSetWindowPos.Call(a.popup, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0)
			}
			if a.editFont != 0 {
				procDeleteObject.Call(a.editFont)
			}
			a.editFont = font(-15, 400, "Microsoft YaHei UI")
			procSendMessage.Call(a.edit, wmSetFont, a.editFont, 1)
			a.thumbnails = make(map[uint64]thumbnail)
			procInvalidateRect.Call(a.popup, 0, 1)
		}
		return 0
	case wmTimer:
		if wparam == 2 {
			if a.expanded != 0 {
				return 0
			}
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
	x := work.Right - a.px(406)
	y := work.Bottom - a.px(564)
	procSetWindowPos.Call(a.popup, ^uintptr(0), uintptr(x), uintptr(y), uintptr(a.px(390)), uintptr(a.px(548)), swpNoActivate)
	procShowWindow.Call(a.popup, swShow)
	procSetTimer.Call(a.popup, 2, 10000, 0)
	procInvalidateRect.Call(a.popup, 0, 1)
	a.visible = true
}

func (a *app) updateTray(add bool) {
	state := "iPhone 未连接"
	if a.tracker.Online() {
		s := a.tracker.State()
		state = "iPhone 已连接 · " + s.IP
	}
	icon := a.idleIcon
	if a.unread {
		icon = a.receivedIcon
		state = "已收到 iPhone 最新发送 · " + state
	}
	if icon == 0 {
		icon, _, _ = procLoadIcon.Call(0, idiApplication)
	}
	a.tray = notifyicondata{Size: uint32(unsafe.Sizeof(notifyicondata{})), Hwnd: a.main, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTray, Icon: icon}
	copy(a.tray.Tip[:], syscall.StringToUTF16("AirDropPlus-Go · "+state))
	op := uintptr(nimModify)
	if add {
		op = nimAdd
	}
	procShellNotifyIcon.Call(op, uintptr(unsafe.Pointer(&a.tray)))
}

func createStatusIcon(received bool) uintptr {
	const size = 32
	const scale = 4
	high := make([][4]byte, size*scale*size*scale)
	paint := func(shape func(float64, float64) bool, color [4]byte) {
		width := size * scale
		for y := range width {
			for x := range width {
				px, py := (float64(x)+0.5)/scale, (float64(y)+0.5)/scale
				if shape(px, py) {
					high[y*width+x] = color
				}
			}
		}
	}
	rounded := func(l, t, r, b, radius float64) func(float64, float64) bool {
		return func(x, y float64) bool {
			cx := max(l+radius, min(x, r-radius))
			cy := max(t+radius, min(y, b-radius))
			dx, dy := x-cx, y-cy
			return x >= l && x <= r && y >= t && y <= b && dx*dx+dy*dy <= radius*radius
		}
	}
	circle := func(cx, cy, radius float64) func(float64, float64) bool {
		return func(x, y float64) bool { dx, dy := x-cx, y-cy; return dx*dx+dy*dy <= radius*radius }
	}
	paint(rounded(2, 2, 30, 30, 7), [4]byte{38, 54, 72, 255})
	paint(rounded(9, 6, 23, 27, 4), [4]byte{255, 255, 255, 255})
	paint(rounded(11, 8, 21, 23.5, 2), [4]byte{38, 54, 72, 255})
	paint(circle(16, 25, 1), [4]byte{38, 54, 72, 255})
	paint(circle(24, 24, 6), [4]byte{255, 255, 255, 255})
	dot := [4]byte{174, 175, 171, 255}
	if received {
		dot = [4]byte{52, 199, 89, 255}
	}
	paint(circle(24, 24, 4.1), dot)

	pixels := make([]byte, size*size*4)
	for y := range size {
		for x := range size {
			var rr, gg, bb, aa int
			for sy := range scale {
				for sx := range scale {
					c := high[(y*scale+sy)*size*scale+x*scale+sx]
					rr += int(c[0])
					gg += int(c[1])
					bb += int(c[2])
					aa += int(c[3])
				}
			}
			i := (y*size + x) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = byte(bb/16), byte(gg/16), byte(rr/16), byte(aa/16)
		}
	}
	info := bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: size, Height: -size, Planes: 1, BitCount: 32, SizeImage: uint32(len(pixels))}}
	var bits uintptr
	color, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == 0 {
		return 0
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(bits)), len(pixels)), pixels)
	mask, _, _ := procCreateBitmap.Call(size, size, 1, 1, 0)
	if mask == 0 {
		procDeleteObject.Call(color)
		return 0
	}
	ii := iconInfo{Icon: 1, Mask: mask, Color: color}
	icon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	procDeleteObject.Call(mask)
	procDeleteObject.Call(color)
	return icon
}

func (a *app) paint() {
	var ps paintstruct
	hdc, _, _ := procBeginPaint.Call(a.popup, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(a.popup, uintptr(unsafe.Pointer(&ps)))
	fill(hdc, rect{0, 0, 390, 548}, rgb(247, 247, 245))
	procSetBkMode.Call(hdc, transparent)
	fontTitle := font(-22, 600, "Bitstream Charter")
	fontHeading := font(-16, 600, "Microsoft YaHei UI")
	fontBody := font(-15, 400, "Microsoft YaHei UI")
	fontSmall := font(-13, 400, "Microsoft YaHei UI")
	defer procDeleteObject.Call(fontTitle)
	defer procDeleteObject.Call(fontHeading)
	defer procDeleteObject.Call(fontBody)
	defer procDeleteObject.Call(fontSmall)
	a.hits = nil
	selectFont(hdc, fontTitle)
	text(hdc, "AirDropPlus", 18, 16, 220, 48, rgb(24, 24, 24), dtLeft|dtSingleLine)
	state := "未连接"
	detail := "等待 iPhone"
	online := a.tracker.Online()
	if online {
		s := a.tracker.State()
		state = "iPhone 已连接"
		detail = s.IP + " · " + relative(s.LastSeen)
	}
	fillRound(hdc, rect{18, 56, 372, 116}, 14, rgb(255, 255, 255), rgb(216, 216, 216))
	dotColor := rgb(150, 150, 150)
	if online {
		dotColor = rgb(67, 181, 103)
	}
	fillRound(hdc, rect{34, 75, 48, 89}, 7, dotColor, dotColor)
	selectFont(hdc, fontHeading)
	text(hdc, state, 58, 68, 245, 91, rgb(24, 24, 24), dtLeft|dtSingleLine)
	selectFont(hdc, fontSmall)
	text(hdc, detail, 58, 90, 245, 107, rgb(102, 102, 102), dtLeft|dtSingleLine)
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
	selectFont(hdc, fontHeading)
	text(hdc, "最近剪贴板", 18, 132, 220, 157, rgb(24, 24, 24), dtLeft|dtSingleLine)
	selectFont(hdc, fontSmall)
	text(hdc, "两页可浏览 · 超出自动归档", 188, 136, 372, 154, rgb(102, 102, 102), dtRight|dtSingleLine)
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
	if a.expanded != 0 {
		for _, item := range items {
			if item.ID == a.expanded && item.Content.Kind == clipboard.Text {
				a.paintExpanded(hdc, item, fontHeading, fontSmall)
				return
			}
		}
		a.expanded = 0
		a.editItem = 0
	}
	procShowWindow.Call(a.edit, swHide)
	const pageSize = 3
	pageCount := max(1, min(2, (len(items)+pageSize-1)/pageSize))
	if a.page >= pageCount {
		a.page = pageCount - 1
	}
	start := a.page * pageSize
	end := min(len(items), start+pageSize)
	visible := items[start:end]
	if len(items) == 0 {
		fillRound(hdc, rect{18, int32(y), 372, int32(y + 78)}, 12, rgb(255, 255, 255), rgb(216, 216, 216))
		selectFont(hdc, fontBody)
		text(hdc, "暂无内容。收到后点击托盘图标查看。", 34, int32(y+24), 356, int32(y+55), rgb(102, 102, 102), dtCenter|dtVCenter|dtSingleLine)
	}
	for _, item := range visible {
		h := 78
		card := rect{18, int32(y), 372, int32(y + h)}
		fillRound(hdc, card, 12, rgb(255, 255, 255), rgb(216, 216, 216))
		selectFont(hdc, fontBody)
		if item.Content.Kind == clipboard.Image {
			a.drawImageCard(hdc, item.ID, item.Content.PNG, 30, int32(y+10), 112, int32(y+60))
			text(hdc, "图片", 126, int32(y+13), 250, int32(y+36), rgb(51, 51, 51), dtLeft|dtSingleLine)
			selectFont(hdc, fontSmall)
			text(hdc, sourceLabel(item.Source)+" · "+relative(item.Created), 126, int32(y+37), 260, int32(y+57), rgb(102, 102, 102), dtLeft|dtSingleLine)
		} else {
			body := item.Content.Text
			if item.Content.Kind == clipboard.Files {
				body = strings.Join(item.Content.Files, ", ")
			}
			text(hdc, body, 32, int32(y+12), 350, int32(y+44), rgb(51, 51, 51), dtLeft|dtEndEllipsis)
			selectFont(hdc, fontSmall)
			text(hdc, sourceLabel(item.Source)+" · "+relative(item.Created), 32, int32(y+49), 205, int32(y+68), rgb(102, 102, 102), dtLeft|dtSingleLine)
		}
		pin := "置顶"
		if item.Pinned {
			pin = "取消置顶"
		}
		if item.Content.Kind == clipboard.Text && isLongText(item.Content.Text) {
			text(hdc, "展开", 208, int32(y+h-27), 246, int32(y+h-8), rgb(41, 104, 171), dtCenter|dtSingleLine)
			a.hits = append(a.hits, hit{rect{202, int32(y + h - 34), 250, int32(y + h)}, "expand", item.ID})
		}
		text(hdc, "复制", 252, int32(y+h-27), 286, int32(y+h-8), rgb(41, 104, 171), dtCenter|dtSingleLine)
		text(hdc, pin, 288, int32(y+h-27), 334, int32(y+h-8), rgb(41, 104, 171), dtCenter|dtSingleLine)
		text(hdc, "删除", 336, int32(y+h-27), 365, int32(y+h-8), rgb(186, 55, 48), dtCenter|dtSingleLine)
		a.hits = append(a.hits, hit{rect{244, int32(y + h - 34), 288, int32(y + h)}, "copy", item.ID}, hit{rect{288, int32(y + h - 34), 337, int32(y + h)}, "pin", item.ID}, hit{rect{337, int32(y + h - 34), 374, int32(y + h)}, "delete", item.ID})
		y += h + 9
	}
	selectFont(hdc, fontSmall)
	if pageCount > 1 {
		text(hdc, "‹ 上一页", 92, 488, 162, 511, rgb(41, 104, 171), dtCenter|dtSingleLine)
		text(hdc, fmt.Sprintf("%d / %d", a.page+1, pageCount), 164, 488, 226, 511, rgb(102, 102, 102), dtCenter|dtSingleLine)
		text(hdc, "下一页 ›", 228, 488, 298, 511, rgb(41, 104, 171), dtCenter|dtSingleLine)
		a.hits = append(a.hits, hit{rect{80, 480, 166, 516}, "prev", 0}, hit{rect{224, 480, 310, 516}, "next", 0})
	} else {
		text(hdc, "1 / 1", 164, 488, 226, 511, rgb(112, 112, 112), dtCenter|dtSingleLine)
	}
	selectFont(hdc, fontSmall)
	text(hdc, "点击托盘图标打开 · 空闲时自动隐藏", 18, 520, 372, 542, rgb(112, 112, 112), dtCenter|dtSingleLine)
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
			case "expand":
				a.expanded = h.id
				a.editItem = 0
			case "collapse":
				a.expanded = 0
				a.editItem = 0
				procShowWindow.Call(a.edit, swHide)
			case "prev":
				if a.page > 0 {
					a.page--
				}
			case "next":
				if a.page < 1 {
					a.page++
				}
			}
			procInvalidateRect.Call(a.popup, 0, 1)
			return
		}
	}
}

func (a *app) paintExpanded(hdc uintptr, item history.Item, fontHeading, fontSmall uintptr) {
	selectFont(hdc, fontHeading)
	text(hdc, "完整文本", 18, 165, 180, 188, rgb(24, 24, 24), dtLeft|dtSingleLine)
	selectFont(hdc, fontSmall)
	stamp := item.Created.Format("2006-01-02 15:04:05")
	text(hdc, sourceLabel(item.Source)+" · "+stamp, 178, 168, 372, 188, rgb(102, 102, 102), dtRight|dtSingleLine)
	procMoveWindow.Call(a.edit, uintptr(a.px(18)), uintptr(a.px(194)), uintptr(a.px(354)), uintptr(a.px(278)), 1)
	if a.editItem != item.ID {
		body := strings.ReplaceAll(strings.ReplaceAll(item.Content.Text, "\r\n", "\n"), "\n", "\r\n")
		p, _ := syscall.UTF16PtrFromString(body)
		procSetWindowText.Call(a.edit, uintptr(unsafe.Pointer(p)))
		a.editItem = item.ID
	}
	procShowWindow.Call(a.edit, swShow)
	text(hdc, "收起", 116, 486, 174, 510, rgb(41, 104, 171), dtCenter|dtSingleLine)
	text(hdc, "复制全文", 216, 486, 282, 510, rgb(41, 104, 171), dtCenter|dtSingleLine)
	a.hits = append(a.hits, hit{rect{104, 478, 184, 516}, "collapse", item.ID}, hit{rect{204, 478, 294, 516}, "copy", item.ID})
	text(hdc, "完整内容可滚动 · 归档文档含时间戳", 18, 520, 372, 542, rgb(112, 112, 112), dtCenter|dtSingleLine)
}

func isLongText(s string) bool {
	return utf8.RuneCountInString(s) > 60 || strings.ContainsAny(s, "\r\n")
}

func dpiScale(value, dpi int32) int32 {
	if dpi < 96 {
		dpi = 96
	}
	return value * dpi / 96
}

func (a *app) px(value int32) int32      { return dpiScale(value, a.dpi) }
func (a *app) logical(value int32) int32 { return value * 96 / max(a.dpi, 96) }

func uiPX(value int32) int32 {
	if current == nil {
		return value
	}
	return current.px(value)
}

func font(height int32, weight int32, name string) uintptr {
	p, _ := syscall.UTF16PtrFromString(name)
	r, _, _ := procCreateFont.Call(uintptr(uiPX(height)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(p)))
	return r
}
func selectFont(hdc, f uintptr) { procSelectObject.Call(hdc, f) }
func text(hdc uintptr, s string, l, t, r, b int32, color uint32, flags uint32) {
	p, _ := syscall.UTF16PtrFromString(s)
	rc := rect{uiPX(l), uiPX(t), uiPX(r), uiPX(b)}
	procSetTextColor.Call(hdc, uintptr(color))
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(p)), uintptr(len([]rune(s))), uintptr(unsafe.Pointer(&rc)), uintptr(flags))
}
func fill(hdc uintptr, r rect, color uint32) {
	r = rect{uiPX(r.Left), uiPX(r.Top), uiPX(r.Right), uiPX(r.Bottom)}
	b, _, _ := procCreateSolidBrush.Call(uintptr(color))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), b)
	procDeleteObject.Call(b)
}
func fillRound(hdc uintptr, r rect, radius int32, fillColor, border uint32) {
	r = rect{uiPX(r.Left), uiPX(r.Top), uiPX(r.Right), uiPX(r.Bottom)}
	radius = uiPX(radius)
	b, _, _ := procCreateSolidBrush.Call(uintptr(fillColor))
	p, _, _ := procCreatePen.Call(0, uintptr(max(uiPX(1), 1)), uintptr(border))
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
		thumb, ok = makeThumbnail(png, thumbnailCacheSize(r-l, b-t, a.dpi))
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
	procSetStretchBltMode.Call(hdc, 4)
	procSetBrushOrgEx.Call(hdc, 0, 0, 0)
	procStretchDIBits.Call(hdc, uintptr(uiPX(x)), uintptr(uiPX(y)), uintptr(uiPX(w)), uintptr(uiPX(h)), 0, 0, uintptr(thumb.width), uintptr(thumb.height), uintptr(unsafe.Pointer(&thumb.pixels[0])), uintptr(unsafe.Pointer(&info)), 0, 0x00CC0020)
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

func thumbnailCacheSize(logicalWidth, logicalHeight, dpi int32) int {
	return int(max(max(dpiScale(logicalWidth, dpi), dpiScale(logicalHeight, dpi)), 128))
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
