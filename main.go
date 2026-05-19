//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	windowWidth  = 40
	windowHeight = 40
	imageOpacity = 0.70
	shadowAlpha  = 120
	countdownLen = 30 * time.Second
	frameMS      = 33

	timerID = 1

	hotkeyStart = 1001
	hotkeyPause = 1002
	hotkeyReset = 1003
)

const (
	wsPopup = 0x80000000

	wsExTopmost   = 0x00000008
	wsExLayered   = 0x00080000
	wsExAppWindow = 0x00040000

	swShow = 5

	wmDestroy     = 0x0002
	wmTimer       = 0x0113
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmHotkey      = 0x0312

	mkLButton = 0x0001

	modAlt     = 0x0001
	modControl = 0x0002

	vkF6 = 0x75
	vkF7 = 0x76
	vkF8 = 0x77

	smCxScreen = 0
	smCyScreen = 1

	hwndTopmost = ^uintptr(0)

	swpNoSize     = 0x0001
	swpNoActivate = 0x0010

	ulwAlpha   = 0x00000002
	acSrcOver  = 0x00
	acSrcAlpha = 0x01

	dibRGBColors = 0
	biRGB        = 0

	idcArrow = 32512

	mbOK       = 0x00000000
	mbIconWarn = 0x00000030
)

type point struct {
	X int32
	Y int32
}

type size struct {
	CX int32
	CY int32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type rgbQuad struct {
	Blue     byte
	Green    byte
	Red      byte
	Reserved byte
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]rgbQuad
}

type countdown struct {
	duration time.Duration
	elapsed  time.Duration
	started  time.Time
	running  bool
}

type windowPosition struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

//go:embed timer.png
var embeddedTimerPNG []byte

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procSetTimer            = user32.NewProc("SetTimer")
	procKillTimer           = user32.NewProc("KillTimer")
	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = user32.NewProc("UnregisterHotKey")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procSetCapture          = user32.NewProc("SetCapture")
	procReleaseCapture      = user32.NewProc("ReleaseCapture")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procSetProcessDPIAware  = user32.NewProc("SetProcessDPIAware")

	procGetModuleHandle    = kernel32.NewProc("GetModuleHandleW")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")

	wndProcPtr = syscall.NewCallback(wndProc)

	appTimer  = countdown{duration: countdownLen}
	timerFace *image.NRGBA

	dragging        bool
	dragCursorStart point
	dragWindowStart rect
)

func main() {
	runtime.LockOSThread()
	procSetProcessDPIAware.Call()
	timerFace = loadTimerPNG()

	hInstance, _, _ := procGetModuleHandle.Call(0)
	className := mustUTF16Ptr("MapleCleanCountdownWindow")
	windowTitle := mustUTF16Ptr("Simple Timer")
	cursor, _, _ := procLoadCursor.Call(0, uintptr(idcArrow))
	icon, _, _ := procLoadIcon.Call(hInstance, uintptr(1))

	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   wndProcPtr,
		HInstance:     hInstance,
		HIcon:         icon,
		HCursor:       cursor,
		LpszClassName: className,
		HIconSm:       icon,
	}

	atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		fatalf("RegisterClassExW failed: %v", err)
	}

	screenW, _, _ := procGetSystemMetrics.Call(smCxScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCyScreen)
	x := int32(screenW)/2 - windowWidth/2
	y := int32(screenH)/3 - windowHeight/2
	if pos, ok := loadWindowPosition(); ok {
		x = pos.X
		y = pos.Y
	} else {
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
	}

	hwnd, _, err := procCreateWindowEx.Call(
		wsExLayered|wsExTopmost|wsExAppWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		wsPopup,
		uintptr(x),
		uintptr(y),
		windowWidth,
		windowHeight,
		0,
		0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		fatalf("CreateWindowExW failed: %v", err)
	}

	registerHotkeys(hwnd)
	procSetTimer.Call(hwnd, timerID, frameMS, 0)
	render(hwnd)
	procShowWindow.Call(hwnd, swShow)

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		code := int32(ret)
		if code <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func wndProc(hwnd uintptr, message uint32, wParam uintptr, lParam uintptr) uintptr {
	switch message {
	case wmTimer:
		if wParam == timerID {
			render(hwnd)
			return 0
		}
	case wmHotkey:
		switch wParam {
		case hotkeyStart:
			appTimer.start()
		case hotkeyPause:
			appTimer.pause()
		case hotkeyReset:
			appTimer.reset()
		}
		render(hwnd)
		return 0
	case wmLButtonDown:
		dragging = true
		procSetCapture.Call(hwnd)
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&dragCursorStart)))
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&dragWindowStart)))
		return 0
	case wmMouseMove:
		if dragging {
			if wParam&mkLButton == 0 {
				dragging = false
				procReleaseCapture.Call()
				saveWindowPosition(hwnd)
				return 0
			}
			var p point
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			dx := p.X - dragCursorStart.X
			dy := p.Y - dragCursorStart.Y
			procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(dragWindowStart.Left+dx), uintptr(dragWindowStart.Top+dy), 0, 0, swpNoSize|swpNoActivate)
		}
		return 0
	case wmLButtonUp:
		if dragging {
			dragging = false
			procReleaseCapture.Call()
			saveWindowPosition(hwnd)
		}
		return 0
	case wmRButtonUp:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procKillTimer.Call(hwnd, timerID)
		procUnregisterHotKey.Call(hwnd, hotkeyStart)
		procUnregisterHotKey.Call(hwnd, hotkeyPause)
		procUnregisterHotKey.Call(hwnd, hotkeyReset)
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func registerHotkeys(hwnd uintptr) {
	registerHotkey(hwnd, hotkeyStart, modAlt, vkF8, "Alt+F8 启动/继续")
	registerHotkey(hwnd, hotkeyPause, modAlt, vkF6, "Alt+F6 暂停")
	registerHotkey(hwnd, hotkeyReset, modAlt, vkF7, "Alt+F7 重置")
}

func registerHotkey(hwnd uintptr, id int, modifiers uint32, key uint32, label string) {
	ok, _, err := procRegisterHotKey.Call(hwnd, uintptr(id), uintptr(modifiers), uintptr(key))
	if ok == 0 {
		messageBox("快捷键注册失败", fmt.Sprintf("%s 注册失败。可能已经被其他程序占用。\n\n系统返回：%v", label, err))
	}
}

func (c *countdown) start() {
	if c.elapsedNow() >= c.duration {
		c.elapsed = 0
	}
	c.started = time.Now()
	c.running = true
}

func (c *countdown) pause() {
	if !c.running {
		return
	}
	c.elapsed = c.elapsedNow()
	c.running = false
}

func (c *countdown) reset() {
	c.elapsed = 0
	c.running = false
}

func (c *countdown) snapshot() (time.Duration, float64) {
	elapsed := c.elapsedNow()
	if c.running && elapsed >= c.duration {
		elapsed %= c.duration
		c.elapsed = elapsed
		c.started = time.Now()
	} else if elapsed >= c.duration {
		elapsed = c.duration
	}
	remaining := c.duration - elapsed
	progress := float64(elapsed) / float64(c.duration)
	return remaining, clamp(progress, 0, 1)
}

func (c *countdown) elapsedNow() time.Duration {
	if c.running {
		return c.elapsed + time.Since(c.started)
	}
	return c.elapsed
}

func render(hwnd uintptr) {
	remaining, _ := appTimer.snapshot()
	img := renderFrame(remaining)
	updateLayeredWindow(hwnd, img)
}

func renderFrame(remaining time.Duration) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, windowWidth, windowHeight))
	drawTimerImage(img, timerFace, imageOpacity)
	drawCountdownShadow(img, shadowClearedProgress(remaining), color.RGBA{0, 0, 0, shadowAlpha})

	seconds := int(math.Ceil(remaining.Seconds()))
	if seconds < 0 {
		seconds = 0
	}
	if seconds > 99 {
		seconds = 99
	}
	text := fmt.Sprintf("%02d", seconds)
	drawCenteredSevenText(img, text, -1, 0, color.RGBA{0, 0, 0, 175})
	drawCenteredSevenText(img, text, 1, 0, color.RGBA{0, 0, 0, 175})
	drawCenteredSevenText(img, text, 0, -1, color.RGBA{0, 0, 0, 175})
	drawCenteredSevenText(img, text, 0, 1, color.RGBA{0, 0, 0, 175})
	drawCenteredSevenText(img, text, 0, 0, color.RGBA{255, 230, 30, 255})

	return img
}

func shadowClearedProgress(remaining time.Duration) float64 {
	elapsed := countdownLen - remaining
	return clamp(float64(elapsed)/float64(countdownLen), 0, 1)
}

func loadTimerPNG() *image.NRGBA {
	decoded, err := png.Decode(bytes.NewReader(embeddedTimerPNG))
	if err != nil {
		fatalf("内置 timer.png 解码失败: %v", err)
	}

	bounds := decoded.Bounds()
	face := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(face, face.Bounds(), decoded, bounds.Min, draw.Src)
	return face
}

func drawTimerImage(dst *image.RGBA, src *image.NRGBA, opacity float64) {
	if src == nil {
		return
	}
	db := dst.Bounds()
	sb := src.Bounds()
	dw, dh := db.Dx(), db.Dy()
	sw, sh := sb.Dx(), sb.Dy()
	if sw == 0 || sh == 0 {
		return
	}

	cx := float64(dw) / 2
	cy := float64(dh) / 2
	scale := math.Min(float64(dw)/float64(sw), float64(dh)/float64(sh))

	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			sx := dx/scale + float64(sw)/2 - 0.5
			sy := dy/scale + float64(sh)/2 - 0.5
			col := sampleNRGBA(src, sx, sy)
			if col.A != 0 {
				blendPixel(dst, x, y, withAlpha(col, opacity))
			}
		}
	}
}

func drawCountdownShadow(img *image.RGBA, clearedProgress float64, shadow color.RGBA) {
	clearedProgress = clamp(clearedProgress, 0, 1)
	if clearedProgress >= 1 {
		return
	}
	b := img.Bounds()
	cx := float64(b.Dx()) / 2
	cy := float64(b.Dy()) / 2
	clearedAngle := clearedProgress * 2 * math.Pi
	feather := 0.08

	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			i := img.PixOffset(x, y)
			if img.Pix[i+3] == 0 {
				continue
			}
			coverage := 1.0
			if clearedProgress > 0 {
				dx := float64(x) + 0.5 - cx
				dy := float64(y) + 0.5 - cy
				angle := math.Atan2(dx, -dy)
				if angle < 0 {
					angle += 2 * math.Pi
				}
				if angle < clearedAngle {
					continue
				}
				if angle-clearedAngle < feather {
					coverage = (angle - clearedAngle) / feather
				}
			}
			blendPixel(img, x, y, withAlpha(shadow, coverage))
		}
	}
}

func sampleNRGBA(img *image.NRGBA, x, y float64) color.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if x < 0 || y < 0 || x > float64(w-1) || y > float64(h-1) {
		return color.RGBA{}
	}
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := minInt(x0+1, w-1)
	y1 := minInt(y0+1, h-1)
	tx := x - float64(x0)
	ty := y - float64(y0)

	c00 := nrgbaAt(img, x0, y0)
	c10 := nrgbaAt(img, x1, y0)
	c01 := nrgbaAt(img, x0, y1)
	c11 := nrgbaAt(img, x1, y1)

	return color.RGBA{
		R: byte(bilerp(float64(c00.R), float64(c10.R), float64(c01.R), float64(c11.R), tx, ty)),
		G: byte(bilerp(float64(c00.G), float64(c10.G), float64(c01.G), float64(c11.G), tx, ty)),
		B: byte(bilerp(float64(c00.B), float64(c10.B), float64(c01.B), float64(c11.B), tx, ty)),
		A: byte(bilerp(float64(c00.A), float64(c10.A), float64(c01.A), float64(c11.A), tx, ty)),
	}
}

func nrgbaAt(img *image.NRGBA, x int, y int) color.RGBA {
	i := img.PixOffset(x, y)
	return color.RGBA{R: img.Pix[i], G: img.Pix[i+1], B: img.Pix[i+2], A: img.Pix[i+3]}
}

func bilerp(c00, c10, c01, c11, tx, ty float64) int {
	top := c00 + (c10-c00)*tx
	bottom := c01 + (c11-c01)*tx
	return int(math.Round(clamp(top+(bottom-top)*ty, 0, 255)))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func drawCenteredSevenText(img *image.RGBA, text string, offsetX float64, offsetY float64, col color.RGBA) {
	layer := image.NewRGBA(image.Rect(0, 0, windowWidth, windowHeight))
	drawSevenText(layer, text, float64(windowWidth)/2, 10, col)
	bounds, ok := alphaBounds(layer)
	if !ok {
		return
	}
	dx := int(math.Round((float64(windowWidth)-float64(bounds.Dx()))/2 - float64(bounds.Min.X) + offsetX))
	dy := int(math.Round((float64(windowHeight)-float64(bounds.Dy()))/2 - float64(bounds.Min.Y) + offsetY))
	blendImageAt(img, layer, dx, dy)
}

func alphaBounds(img *image.RGBA) (image.Rectangle, bool) {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.Pix[img.PixOffset(x, y)+3] == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

func blendImageAt(dst *image.RGBA, src *image.RGBA, offsetX int, offsetY int) {
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := src.PixOffset(x, y)
			col := color.RGBA{R: src.Pix[i], G: src.Pix[i+1], B: src.Pix[i+2], A: src.Pix[i+3]}
			blendPixel(dst, x+offsetX, y+offsetY, col)
		}
	}
}

func drawSevenText(img *image.RGBA, text string, centerX float64, top float64, col color.RGBA) {
	const digitW = 12.0
	const digitH = 20.0
	const gap = 3.0
	total := float64(len(text))*digitW + float64(len(text)-1)*gap
	x := centerX - total/2
	for _, ch := range text {
		if ch >= '0' && ch <= '9' {
			drawSevenDigit(img, int(ch-'0'), x, top, digitW, digitH, 3, col)
		}
		x += digitW + gap
	}
}

func drawSevenDigit(img *image.RGBA, digit int, x, y, w, h, thick float64, col color.RGBA) {
	segments := [10][7]bool{
		{true, true, true, true, true, true, false},
		{false, true, true, false, false, false, false},
		{true, true, false, true, true, false, true},
		{true, true, true, true, false, false, true},
		{false, true, true, false, false, true, true},
		{true, false, true, true, false, true, true},
		{true, false, true, true, true, true, true},
		{true, true, true, false, false, false, false},
		{true, true, true, true, true, true, true},
		{true, true, true, true, false, true, true},
	}
	if digit < 0 || digit > 9 {
		return
	}
	s := segments[digit]
	r := thick / 2
	if s[0] {
		drawCapsuleAA(img, x+r, y+r, x+w-r, y+r, thick, col)
	}
	if s[1] {
		drawCapsuleAA(img, x+w-r, y+thick, x+w-r, y+h/2-r, thick, col)
	}
	if s[2] {
		drawCapsuleAA(img, x+w-r, y+h/2+r, x+w-r, y+h-thick, thick, col)
	}
	if s[3] {
		drawCapsuleAA(img, x+r, y+h-r, x+w-r, y+h-r, thick, col)
	}
	if s[4] {
		drawCapsuleAA(img, x+r, y+h/2+r, x+r, y+h-thick, thick, col)
	}
	if s[5] {
		drawCapsuleAA(img, x+r, y+thick, x+r, y+h/2-r, thick, col)
	}
	if s[6] {
		drawCapsuleAA(img, x+r, y+h/2, x+w-r, y+h/2, thick, col)
	}
}

func drawCapsuleAA(img *image.RGBA, x1, y1, x2, y2, thick float64, col color.RGBA) {
	r := thick / 2
	minX := int(math.Floor(math.Min(x1, x2) - r - 1))
	maxX := int(math.Ceil(math.Max(x1, x2) + r + 1))
	minY := int(math.Floor(math.Min(y1, y2) - r - 1))
	maxY := int(math.Ceil(math.Max(y1, y2) + r + 1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			d := distanceToSegment(float64(x)+0.5, float64(y)+0.5, x1, y1, x2, y2)
			coverage := clamp(r+0.5-d, 0, 1)
			if coverage > 0 {
				blendPixel(img, x, y, withAlpha(col, coverage))
			}
		}
	}
}

func distanceToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	if dx == 0 && dy == 0 {
		return math.Hypot(px-x1, py-y1)
	}
	t := ((px-x1)*dx + (py-y1)*dy) / (dx*dx + dy*dy)
	t = clamp(t, 0, 1)
	closestX := x1 + t*dx
	closestY := y1 + t*dy
	return math.Hypot(px-closestX, py-closestY)
}

func blendPixel(img *image.RGBA, x, y int, src color.RGBA) {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() || src.A == 0 {
		return
	}
	i := img.PixOffset(x, y)
	dr := int(img.Pix[i+0])
	dg := int(img.Pix[i+1])
	db := int(img.Pix[i+2])
	da := int(img.Pix[i+3])
	sa := int(src.A)
	inv := 255 - sa
	outA := sa + da*inv/255
	if outA == 0 {
		img.Pix[i+0] = 0
		img.Pix[i+1] = 0
		img.Pix[i+2] = 0
		img.Pix[i+3] = 0
		return
	}
	img.Pix[i+0] = byte((int(src.R)*sa + dr*da*inv/255) / outA)
	img.Pix[i+1] = byte((int(src.G)*sa + dg*da*inv/255) / outA)
	img.Pix[i+2] = byte((int(src.B)*sa + db*da*inv/255) / outA)
	img.Pix[i+3] = byte(outA)
}

func withAlpha(c color.RGBA, factor float64) color.RGBA {
	factor = clamp(factor, 0, 1)
	c.A = byte(math.Round(float64(c.A) * factor))
	return c
}

func updateLayeredWindow(hwnd uintptr, img *image.RGBA) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return
	}
	defer procDeleteDC.Call(hdcMem)

	bmi := bitmapInfo{}
	bmi.Header.Size = uint32(unsafe.Sizeof(bitmapInfoHeader{}))
	bmi.Header.Width = int32(width)
	bmi.Header.Height = -int32(height)
	bmi.Header.Planes = 1
	bmi.Header.BitCount = 32
	bmi.Header.Compression = biRGB

	var bits unsafe.Pointer
	hBitmap, _, _ := procCreateDIBSection.Call(hdcScreen, uintptr(unsafe.Pointer(&bmi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hBitmap == 0 || bits == nil {
		return
	}
	defer procDeleteObject.Call(hBitmap)

	oldObj, _, _ := procSelectObject.Call(hdcMem, hBitmap)
	if oldObj != 0 {
		defer procSelectObject.Call(hdcMem, oldObj)
	}

	copyToPremultipliedBGRA(unsafe.Slice((*byte)(bits), width*height*4), img)

	var wr rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	dst := point{X: wr.Left, Y: wr.Top}
	sz := size{CX: int32(width), CY: int32(height)}
	src := point{X: 0, Y: 0}
	blend := blendFunction{BlendOp: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}

	procUpdateLayeredWindow.Call(
		hwnd,
		hdcScreen,
		uintptr(unsafe.Pointer(&dst)),
		uintptr(unsafe.Pointer(&sz)),
		hdcMem,
		uintptr(unsafe.Pointer(&src)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		ulwAlpha,
	)
}

func copyToPremultipliedBGRA(dst []byte, src *image.RGBA) {
	w := src.Bounds().Dx()
	h := src.Bounds().Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := src.PixOffset(x, y)
			di := (y*w + x) * 4
			a := uint32(src.Pix[si+3])
			dst[di+0] = byte(uint32(src.Pix[si+2]) * a / 255)
			dst[di+1] = byte(uint32(src.Pix[si+1]) * a / 255)
			dst[di+2] = byte(uint32(src.Pix[si+0]) * a / 255)
			dst[di+3] = byte(a)
		}
	}
}

func loadWindowPosition() (windowPosition, bool) {
	data, err := os.ReadFile(positionConfigPath())
	if err != nil {
		return windowPosition{}, false
	}

	var pos windowPosition
	if err := json.Unmarshal(data, &pos); err != nil {
		return windowPosition{}, false
	}
	return pos, true
}

func saveWindowPosition(hwnd uintptr) {
	var wr rect
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr))); ok == 0 {
		return
	}

	data, err := json.Marshal(windowPosition{X: wr.Left, Y: wr.Top})
	if err != nil {
		return
	}
	_ = os.WriteFile(positionConfigPath(), data, 0644)
}

func positionConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "simple-timer-position.json"
	}
	return filepath.Join(filepath.Dir(exe), "simple-timer-position.json")
}

func clamp(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func mustUTF16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		fatalf("UTF-16 conversion failed: %v", err)
	}
	return p
}

func messageBox(title string, text string) {
	titlePtr := mustUTF16Ptr(title)
	textPtr := mustUTF16Ptr(text)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), mbOK|mbIconWarn)
}

func fatalf(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	messageBox("启动失败", text)
	os.Exit(1)
}
