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
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	defaultWindowSize = 64
	minWindowSize     = 32
	maxWindowSize     = 256
	resizeGripSize    = 12
	imageOpacity      = 0.70
	shadowAlpha       = 120
	countdownLen      = 30 * time.Second
	frameMS           = 33

	timerID = 1

	hotkeyStart = 1001
	hotkeyPause = 1002
	hotkeyReset = 1003
)

const (
	wsPopup   = 0x80000000
	wsChild   = 0x40000000
	wsVisible = 0x10000000
	wsCaption = 0x00C00000
	wsBorder  = 0x00800000
	wsSysMenu = 0x00080000
	wsTabStop = 0x00010000

	wsExDlgModalFrame = 0x00000001
	wsExTopmost       = 0x00000008
	wsExLayered       = 0x00080000
	wsExAppWindow     = 0x00040000

	swShow = 5

	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmCommand     = 0x0111
	wmTimer       = 0x0113
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmHotkey      = 0x0312

	mkLButton = 0x0001

	esAutoHScroll = 0x0080

	bsPushButton    = 0x00000000
	bsDefPushButton = 0x00000001

	mfString = 0x00000000

	tpmRightButton = 0x0002
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100

	menuSettings = 2001
	menuExit     = 2002

	settingsEditStart = 3001
	settingsEditPause = 3002
	settingsEditReset = 3003
	settingsSave      = 3004
	settingsCancel    = 3005

	wmUser       = 0x0400
	hkmSetHotkey = wmUser + 1
	hkmGetHotkey = wmUser + 2

	hotkeyfShift   = 0x01
	hotkeyfControl = 0x02
	hotkeyfAlt     = 0x04

	iccHotkeyClass = 0x00000040

	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008

	vkF1 = 0x70
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

type initCommonControlsEx struct {
	Size uint32
	ICC  uint32
}

type countdown struct {
	duration time.Duration
	elapsed  time.Duration
	started  time.Time
	running  bool
}

type windowPosition struct {
	X      int32 `json:"x"`
	Y      int32 `json:"y"`
	Width  int   `json:"width"`
	Height int   `json:"height"`
	Saved  bool  `json:"saved"`
}

type hotkeyConfig struct {
	Start string `json:"start"`
	Pause string `json:"pause"`
	Reset string `json:"reset"`
}

type appConfig struct {
	Hotkeys hotkeyConfig   `json:"hotkeys"`
	Window  windowPosition `json:"window"`
}

type hotkeyBinding struct {
	id        int
	action    string
	spec      string
	modifiers uint32
	key       uint32
}

//go:embed timer.png
var embeddedTimerPNG []byte

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
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
	procSendMessage         = user32.NewProc("SendMessageW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procSetProcessDPIAware  = user32.NewProc("SetProcessDPIAware")

	procGetModuleHandle    = kernel32.NewProc("GetModuleHandleW")
	procInitCommonControls = comctl32.NewProc("InitCommonControlsEx")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")

	wndProcPtr         = syscall.NewCallback(wndProc)
	settingsWndProcPtr = syscall.NewCallback(settingsWndProc)
	appInstance        uintptr

	appTimer  = countdown{duration: countdownLen}
	timerFace *image.NRGBA
	hotkeys   []hotkeyBinding
	config    appConfig

	windowWidth  = defaultWindowSize
	windowHeight = defaultWindowSize

	dragging        bool
	resizing        bool
	dragCursorStart point
	dragWindowStart rect

	settingsHwnd      uintptr
	settingsOwnerHwnd uintptr
	settingsStartEdit uintptr
	settingsPauseEdit uintptr
	settingsResetEdit uintptr
)

func main() {
	runtime.LockOSThread()
	procSetProcessDPIAware.Call()
	initHotkeyControls()
	timerFace = loadTimerPNG()
	config = loadConfig()
	hotkeys = hotkeyBindings(config.Hotkeys)

	appInstance, _, _ = procGetModuleHandle.Call(0)
	className := mustUTF16Ptr("MapleCleanCountdownWindow")
	windowTitle := mustUTF16Ptr("Simple Timer")
	cursor, _, _ := procLoadCursor.Call(0, uintptr(idcArrow))
	icon, _, _ := procLoadIcon.Call(appInstance, uintptr(1))

	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   wndProcPtr,
		HInstance:     appInstance,
		HIcon:         icon,
		HCursor:       cursor,
		LpszClassName: className,
		HIconSm:       icon,
	}

	atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		fatalf("RegisterClassExW failed: %v", err)
	}
	registerSettingsClass(cursor, icon)

	screenW, _, _ := procGetSystemMetrics.Call(smCxScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCyScreen)
	if config.Window.Width > 0 {
		windowWidth = clampInt(config.Window.Width, minWindowSize, maxWindowSize)
	}
	if config.Window.Height > 0 {
		windowHeight = clampInt(config.Window.Height, minWindowSize, maxWindowSize)
	}
	x := int32(screenW)/2 - int32(windowWidth)/2
	y := int32(screenH)/3 - int32(windowHeight)/2
	if config.Window.Saved {
		x = config.Window.X
		y = config.Window.Y
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
		uintptr(windowWidth),
		uintptr(windowHeight),
		0,
		0,
		appInstance,
		0,
	)
	if hwnd == 0 {
		fatalf("CreateWindowExW failed: %v", err)
	}

	registerHotkeys(hwnd, hotkeys)
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
		procSetCapture.Call(hwnd)
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&dragCursorStart)))
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&dragWindowStart)))
		resizing = isInResizeGrip(dragCursorStart, dragWindowStart)
		dragging = !resizing
		return 0
	case wmMouseMove:
		if dragging || resizing {
			if wParam&mkLButton == 0 {
				dragging = false
				resizing = false
				procReleaseCapture.Call()
				saveWindowConfig(hwnd)
				return 0
			}
			var p point
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			dx := p.X - dragCursorStart.X
			dy := p.Y - dragCursorStart.Y
			if resizing {
				startW := int(dragWindowStart.Right - dragWindowStart.Left)
				startH := int(dragWindowStart.Bottom - dragWindowStart.Top)
				nextSize := clampInt(maxInt(startW+int(dx), startH+int(dy)), minWindowSize, maxWindowSize)
				windowWidth = nextSize
				windowHeight = nextSize
				procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(dragWindowStart.Left), uintptr(dragWindowStart.Top), uintptr(windowWidth), uintptr(windowHeight), swpNoActivate)
				render(hwnd)
			} else {
				procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(dragWindowStart.Left+dx), uintptr(dragWindowStart.Top+dy), 0, 0, swpNoSize|swpNoActivate)
			}
		}
		return 0
	case wmLButtonUp:
		if dragging || resizing {
			dragging = false
			resizing = false
			procReleaseCapture.Call()
			saveWindowConfig(hwnd)
		}
		return 0
	case wmRButtonUp:
		showContextMenu(hwnd)
		return 0
	case wmDestroy:
		procKillTimer.Call(hwnd, timerID)
		for _, hk := range hotkeys {
			procUnregisterHotKey.Call(hwnd, uintptr(hk.id))
		}
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func registerSettingsClass(cursor uintptr, icon uintptr) {
	className := mustUTF16Ptr("SimpleTimerSettingsWindow")
	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   settingsWndProcPtr,
		HInstance:     appInstance,
		HIcon:         icon,
		HCursor:       cursor,
		LpszClassName: className,
		HIconSm:       icon,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
}

func initHotkeyControls() {
	icc := initCommonControlsEx{
		Size: uint32(unsafe.Sizeof(initCommonControlsEx{})),
		ICC:  iccHotkeyClass,
	}
	procInitCommonControls.Call(uintptr(unsafe.Pointer(&icc)))
}

func showContextMenu(hwnd uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	procAppendMenu.Call(menu, mfString, menuSettings, uintptr(unsafe.Pointer(mustUTF16Ptr("Settings"))))
	procAppendMenu.Call(menu, mfString, menuExit, uintptr(unsafe.Pointer(mustUTF16Ptr("Exit"))))

	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton|tpmNoNotify, uintptr(p.X), uintptr(p.Y), 0, hwnd, 0)
	switch cmd {
	case menuSettings:
		openSettingsWindow(hwnd)
	case menuExit:
		procDestroyWindow.Call(hwnd)
	}
}

func openSettingsWindow(owner uintptr) {
	if settingsHwnd != 0 {
		procSetForegroundWindow.Call(settingsHwnd)
		return
	}

	var wr rect
	procGetWindowRect.Call(owner, uintptr(unsafe.Pointer(&wr)))
	x := wr.Left
	y := wr.Bottom + 8
	if y < 0 {
		y = 0
	}

	settingsOwnerHwnd = owner
	className := mustUTF16Ptr("SimpleTimerSettingsWindow")
	title := mustUTF16Ptr("Settings")
	hwnd, _, _ := procCreateWindowEx.Call(
		wsExDlgModalFrame|wsExTopmost,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsCaption|wsSysMenu|wsVisible,
		uintptr(x),
		uintptr(y),
		360,
		205,
		owner,
		0,
		appInstance,
		0,
	)
	if hwnd == 0 {
		settingsOwnerHwnd = 0
		return
	}
	settingsHwnd = hwnd
	createSettingsControls(hwnd)
	procSetForegroundWindow.Call(hwnd)
}

func createSettingsControls(hwnd uintptr) {
	createControl("STATIC", "Start", wsChild|wsVisible, 16, 22, 86, 22, hwnd, 0)
	settingsStartEdit = createControl("msctls_hotkey32", "", wsChild|wsVisible|wsBorder|wsTabStop, 104, 18, 220, 24, hwnd, settingsEditStart)
	setHotkeyControl(settingsStartEdit, config.Hotkeys.Start)

	createControl("STATIC", "Pause", wsChild|wsVisible, 16, 58, 86, 22, hwnd, 0)
	settingsPauseEdit = createControl("msctls_hotkey32", "", wsChild|wsVisible|wsBorder|wsTabStop, 104, 54, 220, 24, hwnd, settingsEditPause)
	setHotkeyControl(settingsPauseEdit, config.Hotkeys.Pause)

	createControl("STATIC", "Reset", wsChild|wsVisible, 16, 94, 86, 22, hwnd, 0)
	settingsResetEdit = createControl("msctls_hotkey32", "", wsChild|wsVisible|wsBorder|wsTabStop, 104, 90, 220, 24, hwnd, settingsEditReset)
	setHotkeyControl(settingsResetEdit, config.Hotkeys.Reset)

	createControl("BUTTON", "Save", wsChild|wsVisible|wsTabStop|bsDefPushButton, 144, 134, 82, 30, hwnd, settingsSave)
	createControl("BUTTON", "Cancel", wsChild|wsVisible|wsTabStop|bsPushButton, 240, 134, 82, 30, hwnd, settingsCancel)
}

func createControl(className string, text string, style uintptr, x int32, y int32, width int32, height int32, parent uintptr, id uintptr) uintptr {
	classPtr := mustUTF16Ptr(className)
	textPtr := mustUTF16Ptr(text)
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(classPtr)),
		uintptr(unsafe.Pointer(textPtr)),
		style,
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		parent,
		id,
		appInstance,
		0,
	)
	return hwnd
}

func settingsWndProc(hwnd uintptr, message uint32, wParam uintptr, lParam uintptr) uintptr {
	switch message {
	case wmCommand:
		switch loword(wParam) {
		case settingsSave:
			if saveSettingsFromWindow() {
				procDestroyWindow.Call(hwnd)
			}
			return 0
		case settingsCancel:
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		if hwnd == settingsHwnd {
			settingsHwnd = 0
			settingsOwnerHwnd = 0
			settingsStartEdit = 0
			settingsPauseEdit = 0
			settingsResetEdit = 0
		}
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func saveSettingsFromWindow() bool {
	newConfig := hotkeyConfig{
		Start: hotkeyControlSpec(settingsStartEdit),
		Pause: hotkeyControlSpec(settingsPauseEdit),
		Reset: hotkeyControlSpec(settingsResetEdit),
	}
	if err := validateHotkeyConfig(newConfig); err != nil {
		messageBox("Settings", err.Error())
		return false
	}

	newHotkeys := hotkeyBindings(newConfig)
	oldHotkeys := hotkeys
	unregisterHotkeys(settingsOwnerHwnd, oldHotkeys)
	if !registerHotkeys(settingsOwnerHwnd, newHotkeys) {
		unregisterHotkeys(settingsOwnerHwnd, newHotkeys)
		registerHotkeys(settingsOwnerHwnd, oldHotkeys)
		messageBox("Settings", "Hotkey registration failed. The previous hotkeys were restored.")
		return false
	}

	hotkeys = newHotkeys
	config.Hotkeys = newConfig
	saveConfig(config)
	return true
}

func validateHotkeyConfig(cfg hotkeyConfig) error {
	checks := []struct {
		name string
		spec string
	}{
		{name: "Start", spec: cfg.Start},
		{name: "Pause", spec: cfg.Pause},
		{name: "Reset", spec: cfg.Reset},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.spec) == "" {
			return fmt.Errorf("%s hotkey is empty.", check.name)
		}
		if _, _, err := parseHotkey(check.spec); err != nil {
			return fmt.Errorf("%s hotkey is invalid: %v", check.name, err)
		}
	}
	return nil
}

func setHotkeyControl(hwnd uintptr, spec string) {
	modifiers, key, err := parseHotkey(spec)
	if err != nil {
		return
	}
	flags := registerModifiersToHotkeyFlags(modifiers)
	value := uintptr((key & 0xff) | (flags << 8))
	procSendMessage.Call(hwnd, hkmSetHotkey, value, 0)
}

func hotkeyControlSpec(hwnd uintptr) string {
	value, _, _ := procSendMessage.Call(hwnd, hkmGetHotkey, 0, 0)
	key := uint32(value & 0xff)
	flags := uint32((value >> 8) & 0xff)
	if key == 0 {
		return ""
	}
	return hotkeySpecFromParts(hotkeyFlagsToRegisterModifiers(flags), key)
}

func registerModifiersToHotkeyFlags(modifiers uint32) uint32 {
	var flags uint32
	if modifiers&modShift != 0 {
		flags |= hotkeyfShift
	}
	if modifiers&modControl != 0 {
		flags |= hotkeyfControl
	}
	if modifiers&modAlt != 0 {
		flags |= hotkeyfAlt
	}
	return flags
}

func hotkeyFlagsToRegisterModifiers(flags uint32) uint32 {
	var modifiers uint32
	if flags&hotkeyfShift != 0 {
		modifiers |= modShift
	}
	if flags&hotkeyfControl != 0 {
		modifiers |= modControl
	}
	if flags&hotkeyfAlt != 0 {
		modifiers |= modAlt
	}
	return modifiers
}

func hotkeySpecFromParts(modifiers uint32, key uint32) string {
	parts := make([]string, 0, 4)
	if modifiers&modControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if modifiers&modAlt != 0 {
		parts = append(parts, "Alt")
	}
	if modifiers&modShift != 0 {
		parts = append(parts, "Shift")
	}
	if modifiers&modWin != 0 {
		parts = append(parts, "Win")
	}
	parts = append(parts, virtualKeyName(key))
	return strings.Join(parts, "+")
}

func virtualKeyName(key uint32) string {
	if key >= 'A' && key <= 'Z' {
		return string(rune(key))
	}
	if key >= '0' && key <= '9' {
		return string(rune(key))
	}
	if key >= vkF1 && key <= vkF1+23 {
		return fmt.Sprintf("F%d", key-vkF1+1)
	}
	switch key {
	case 0x20:
		return "Space"
	case 0x09:
		return "Tab"
	case 0x0D:
		return "Enter"
	case 0x1B:
		return "Esc"
	default:
		return fmt.Sprintf("VK%02X", key)
	}
}

func loword(value uintptr) int {
	return int(value & 0xffff)
}

func registerHotkeys(hwnd uintptr, hotkeys []hotkeyBinding) bool {
	okAll := true
	for _, hk := range hotkeys {
		if !registerHotkey(hwnd, hk.id, hk.modifiers, hk.key, fmt.Sprintf("%s (%s)", hk.action, hk.spec)) {
			okAll = false
		}
	}
	return okAll
}

func registerHotkey(hwnd uintptr, id int, modifiers uint32, key uint32, label string) bool {
	ok, _, err := procRegisterHotKey.Call(hwnd, uintptr(id), uintptr(modifiers), uintptr(key))
	if ok == 0 {
		messageBox("快捷键注册失败", fmt.Sprintf("%s 注册失败。可能已经被其他程序占用。\n\n系统返回：%v", label, err))
		return false
	}
	return true
}

func unregisterHotkeys(hwnd uintptr, hotkeys []hotkeyBinding) {
	for _, hk := range hotkeys {
		procUnregisterHotKey.Call(hwnd, uintptr(hk.id))
	}
}

func hotkeyBindings(cfg hotkeyConfig) []hotkeyBinding {
	defaults := defaultHotkeyConfig()
	return []hotkeyBinding{
		resolveHotkey(hotkeyStart, "启动/继续", cfg.Start, defaults.Start),
		resolveHotkey(hotkeyPause, "暂停", cfg.Pause, defaults.Pause),
		resolveHotkey(hotkeyReset, "重置", cfg.Reset, defaults.Reset),
	}
}

func defaultConfig() appConfig {
	return appConfig{
		Hotkeys: defaultHotkeyConfig(),
		Window: windowPosition{
			Width:  defaultWindowSize,
			Height: defaultWindowSize,
			Saved:  false,
		},
	}
}

func loadConfig() appConfig {
	path := configPath()
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			messageBox("配置解析失败", fmt.Sprintf("%s 格式无效，将使用默认配置。\n\n错误：%v", filepath.Base(path), err))
			cfg = defaultConfig()
		}
	} else if os.IsNotExist(err) {
		saveConfig(cfg)
	} else {
		messageBox("配置读取失败", fmt.Sprintf("无法读取 %s，将使用默认配置。\n\n错误：%v", filepath.Base(path), err))
	}

	if cfg.Window.Width == 0 {
		cfg.Window.Width = defaultWindowSize
	}
	if cfg.Window.Height == 0 {
		cfg.Window.Height = defaultWindowSize
	}
	return cfg
}

func saveConfig(cfg appConfig) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(configPath(), data, 0644)
}

func defaultHotkeyConfig() hotkeyConfig {
	return hotkeyConfig{
		Start: "Alt+F8",
		Pause: "Alt+F6",
		Reset: "Alt+F7",
	}
}

func resolveHotkey(id int, action string, configured string, fallback string) hotkeyBinding {
	if strings.TrimSpace(configured) == "" {
		configured = fallback
	}
	modifiers, key, err := parseHotkey(configured)
	if err != nil {
		messageBox("热键配置无效", fmt.Sprintf("%s 的配置 \"%s\" 无效，将回退为默认值 \"%s\"。\n\n错误：%v", action, configured, fallback, err))
		modifiers, key, _ = parseHotkey(fallback)
		configured = fallback
	}
	return hotkeyBinding{id: id, action: action, spec: configured, modifiers: modifiers, key: key}
}

func parseHotkey(spec string) (uint32, uint32, error) {
	parts := strings.Split(spec, "+")
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("空字符串")
	}

	var modifiers uint32
	for i := 0; i < len(parts)-1; i++ {
		t := strings.TrimSpace(strings.ToUpper(parts[i]))
		switch t {
		case "ALT":
			modifiers |= modAlt
		case "CTRL", "CONTROL":
			modifiers |= modControl
		case "SHIFT":
			modifiers |= modShift
		case "WIN", "WINDOWS":
			modifiers |= modWin
		default:
			return 0, 0, fmt.Errorf("不支持的修饰键: %s", parts[i])
		}
	}

	keyToken := strings.TrimSpace(strings.ToUpper(parts[len(parts)-1]))
	key, err := parseVirtualKey(keyToken)
	if err != nil {
		return 0, 0, err
	}
	return modifiers, key, nil
}

func parseVirtualKey(key string) (uint32, error) {
	if key == "" {
		return 0, fmt.Errorf("未指定主键")
	}

	if len(key) == 1 {
		ch := key[0]
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			return uint32(ch), nil
		}
	}

	if strings.HasPrefix(key, "F") {
		n, err := strconv.Atoi(strings.TrimPrefix(key, "F"))
		if err == nil && n >= 1 && n <= 24 {
			return vkF1 + uint32(n-1), nil
		}
	}

	switch key {
	case "SPACE":
		return 0x20, nil
	case "TAB":
		return 0x09, nil
	case "ENTER", "RETURN":
		return 0x0D, nil
	case "ESC", "ESCAPE":
		return 0x1B, nil
	default:
		return 0, fmt.Errorf("不支持的主键: %s", key)
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
	drawResizeGrip(img)

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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func drawCenteredSevenText(img *image.RGBA, text string, offsetX float64, offsetY float64, col color.RGBA) {
	b := img.Bounds()
	width := b.Dx()
	height := b.Dy()
	fontHeight := math.Max(20, float64(minInt(width, height))*0.50)
	digitWidth := fontHeight * 0.60
	gap := fontHeight * 0.15
	thick := math.Max(3, fontHeight*0.15)

	layer := image.NewRGBA(image.Rect(0, 0, width, height))
	drawSevenText(layer, text, float64(width)/2, 0, digitWidth, fontHeight, thick, gap, col)
	bounds, ok := alphaBounds(layer)
	if !ok {
		return
	}
	dx := int(math.Round((float64(width)-float64(bounds.Dx()))/2 - float64(bounds.Min.X) + offsetX))
	dy := int(math.Round((float64(height)-float64(bounds.Dy()))/2 - float64(bounds.Min.Y) + offsetY))
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

func drawSevenText(img *image.RGBA, text string, centerX float64, top float64, digitW float64, digitH float64, thick float64, gap float64, col color.RGBA) {
	total := float64(len(text))*digitW + float64(len(text)-1)*gap
	x := centerX - total/2
	for _, ch := range text {
		if ch >= '0' && ch <= '9' {
			drawSevenDigit(img, int(ch-'0'), x, top, digitW, digitH, thick, col)
		}
		x += digitW + gap
	}
}

func drawResizeGrip(img *image.RGBA) {
	b := img.Bounds()
	right := float64(b.Dx() - 4)
	bottom := float64(b.Dy() - 4)
	lineColor := color.RGBA{255, 230, 30, 135}
	shadowColor := color.RGBA{0, 0, 0, 120}
	for i := 0; i < 3; i++ {
		offset := float64(i * 4)
		x1 := right - offset - 8
		y1 := bottom + 1
		x2 := right + 1
		y2 := bottom - offset - 8
		drawCapsuleAA(img, x1+1, y1+1, x2+1, y2+1, 1.5, shadowColor)
		drawCapsuleAA(img, x1, y1, x2, y2, 1.5, lineColor)
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

func saveWindowConfig(hwnd uintptr) {
	var wr rect
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr))); ok == 0 {
		return
	}

	config.Window = windowPosition{
		X:      wr.Left,
		Y:      wr.Top,
		Width:  int(wr.Right - wr.Left),
		Height: int(wr.Bottom - wr.Top),
		Saved:  true,
	}
	saveConfig(config)
}

func isInResizeGrip(cursor point, window rect) bool {
	return cursor.X >= window.Right-resizeGripSize && cursor.Y >= window.Bottom-resizeGripSize
}

func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
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
