//go:build windows

// Package tray 用原生 Win32 Shell_NotifyIcon 实现托盘，不引入第三方 GUI 库。
// 隐藏窗口与钩子共用同一个消息循环线程；耗时操作一律丢到 goroutine，
// 避免阻塞消息循环导致输入卡顿。
package tray

import (
	"fmt"
	"syscall"
	"unsafe"

	"keypulse/internal/version"
)

const (
	wmApp           = 0x8000
	wmTray          = wmApp + 1
	wmDestroy       = 0x0002
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2
	nifMsg    = 1
	nifIcon   = 2
	nifTip    = 4
	nifInfo   = 0x10

	niifInfo    = 0x1
	niifNosound = 0x10

	idiApplication = 32512
	mfString       = 0x0000
	mfChecked      = 0x0008 // MF_CHECKED：实时菜单项的勾选标记
	mfSeparator    = 0x0800
	tpmReturnCmd   = 0x0100
	tpmRightButton = 0x0002
	tpmNonNotify   = 0x0080
	swShowNormal   = 1

	idOpen   = 1001
	idPause  = 1002
	idExport = 1003
	idQuit   = 1004
	idLive   = 1005
	idBoard  = 1006
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	registerClassEx  = user32.NewProc("RegisterClassExW")
	createWindowEx   = user32.NewProc("CreateWindowExW")
	destroyWindow    = user32.NewProc("DestroyWindow")
	defWindowProc    = user32.NewProc("DefWindowProcW")
	loadIcon         = user32.NewProc("LoadIconW")
	createPopupMenu  = user32.NewProc("CreatePopupMenu")
	appendMenu       = user32.NewProc("AppendMenuW")
	trackPopupMenu   = user32.NewProc("TrackPopupMenu")
	destroyMenu      = user32.NewProc("DestroyMenu")
	getCursorPos     = user32.NewProc("GetCursorPos")
	setForegroundWin = user32.NewProc("SetForegroundWindow")
	postQuitMsg      = user32.NewProc("PostQuitMessage")
	shellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	shellExecute     = shell32.NewProc("ShellExecuteW")
	getModuleHandle  = kernel32.NewProc("GetModuleHandleW")

	wndProcCB uintptr
	hwnd      uintptr
	classBuf  []uint16
	titleBuf  []uint16
	nid       notifyIconDataW
	cfg       Config
)

type notifyIconDataW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  uintptr
	lpszClassName uintptr
	hIconSm       uintptr
}

type point struct {
	x int32
	y int32
}

type Config struct {
	OnOpen        func()
	OnToggle      func() bool // 返回切换后的 paused 状态
	OnExport      func()
	Paused        func() bool
	OnLiveToggle  func() bool // 切换实时模式，返回切换后的状态；只做原子操作，IO 自行异步
	Live          func() bool // 实时模式当前是否开启，用于菜单勾选
	OnBoardToggle func() bool // 切换本地看板，返回切换后的状态；启停一律异步
	Board         func() bool // 本地看板当前是否开启
}

func New(c Config) error {
	cfg = c
	inst, _, _ := getModuleHandle.Call(0)
	classBuf = syscall.StringToUTF16("KeyPulseHiddenWindow")
	titleBuf = syscall.StringToUTF16("KeyPulse")
	wndProcCB = syscall.NewCallback(wndProc)

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProcCB,
		hInstance:     inst,
		lpszClassName: uintptr(unsafe.Pointer(&classBuf[0])),
	}
	r, _, e := registerClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		return fmt.Errorf("注册窗口类失败: %v", e)
	}

	h, _, e := createWindowEx.Call(0,
		uintptr(unsafe.Pointer(&classBuf[0])),
		uintptr(unsafe.Pointer(&titleBuf[0])),
		0, 0, 0, 0, 0, 0, 0, inst, 0)
	if h == 0 {
		return fmt.Errorf("创建隐藏窗口失败: %v", e)
	}
	hwnd = h

	p := false
	if cfg.Paused != nil {
		p = cfg.Paused()
	}
	icon := makeIcon(p, iconSize())
	if icon == 0 {
		icon, _, _ = loadIcon.Call(0, idiApplication) // GDI 失败时退回系统图标
	} else {
		currentIcon = icon
	}
	nid = notifyIconDataW{
		cbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:             h,
		uID:              1,
		uFlags:           nifMsg | nifIcon | nifTip,
		uCallbackMessage: wmTray,
		hIcon:            icon,
	}
	setTip(p)
	r, _, e = shellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if r == 0 {
		return fmt.Errorf("创建托盘图标失败: %v", e)
	}
	return nil
}

func setTip(paused bool) {
	tip := version.Label() + " · 采集中"
	if paused {
		tip = version.Label() + " · 已暂停"
	}
	for i := range nid.szTip {
		nid.szTip[i] = 0
	}
	copy(nid.szTip[:], syscall.StringToUTF16(tip))
}

// 回调参数一律用 uintptr，避免小于指针宽度的整数在 x64 fastcall
// 寄存器传参时的取值歧义（Go 的 NewCallback 对此有严格 ABI 约束）。
func wndProc(h uintptr, msg uintptr, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmTray:
		switch lparam & 0xFFFF {
		case wmLButtonDblClk:
			if cfg.OnOpen != nil {
				go cfg.OnOpen()
			}
		case wmRButtonUp:
			showMenu()
		}
		return 0
	case wmDestroy:
		postQuitMsg.Call(0)
		return 0
	}
	r, _, _ := defWindowProc.Call(h, uintptr(msg), wparam, lparam)
	return r
}

func addMenuItem(menu uintptr, flags, id uintptr, text string) {
	buf := syscall.StringToUTF16(text)
	appendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(&buf[0])))
}

type menuEntry struct {
	flags uintptr
	id    uintptr
	text  string
}

// menuItems 每次右键时按当前状态现算菜单。看板页内已有实时推送开关，
// 托盘再摆一个「实时模式」会被误认成两个功能，故看板开启时隐藏它。
func menuItems(paused, liveOn, boardOn bool) []menuEntry {
	pauseText := "暂停采集"
	if paused {
		pauseText = "恢复采集"
	}
	liveFlags := uintptr(mfString)
	if liveOn {
		liveFlags |= mfChecked
	}
	boardFlags := uintptr(mfString)
	if boardOn {
		boardFlags |= mfChecked
	}
	items := []menuEntry{
		{mfString, idOpen, "打开统计报告（离线）"},
		{mfString, idPause, pauseText},
	}
	if !boardOn {
		items = append(items, menuEntry{liveFlags, idLive, "实时模式"})
	}
	return append(items,
		menuEntry{boardFlags, idBoard, "本地看板（实时）"},
		menuEntry{mfString, idExport, "导出 CSV"},
		menuEntry{mfSeparator, 0, ""},
		menuEntry{mfString, idQuit, "退出"},
	)
}

func showMenu() {
	menu, _, _ := createPopupMenu.Call()
	if menu == 0 {
		return
	}
	defer destroyMenu.Call(menu)

	p, liveOn, boardOn := false, false, false
	if cfg.Paused != nil {
		p = cfg.Paused()
	}
	if cfg.Live != nil {
		liveOn = cfg.Live()
	}
	if cfg.Board != nil {
		boardOn = cfg.Board()
	}
	for _, it := range menuItems(p, liveOn, boardOn) {
		addMenuItem(menu, it.flags, it.id, it.text)
	}

	var pt point
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	setForegroundWin.Call(hwnd)
	cmd, _, _ := trackPopupMenu.Call(menu,
		tpmReturnCmd|tpmNonNotify|tpmRightButton,
		uintptr(uint32(pt.x)), uintptr(uint32(pt.y)), 0, hwnd, 0)

	switch cmd {
	case idOpen:
		if cfg.OnOpen != nil {
			go cfg.OnOpen()
		}
	case idPause:
		if cfg.OnToggle != nil {
			now := cfg.OnToggle()
			setTip(now)
			if !updateIcon(now) {
				shellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
			}
		}
	case idLive:
		if cfg.OnLiveToggle != nil {
			cfg.OnLiveToggle()
		}
	case idBoard:
		if cfg.OnBoardToggle != nil {
			cfg.OnBoardToggle()
		}
	case idExport:
		if cfg.OnExport != nil {
			go cfg.OnExport()
		}
	case idQuit:
		postQuitMsg.Call(0)
	}
}

// Notify 弹一条静默托盘气泡（无提示音）。使用 nid 的副本而不是改全局结构，
// 否则 szInfo / uFlags 会污染后续的 setTip 与 updateIcon。
// 气泡用于钩子自动恢复（调用方做 5 分钟节流）与实时模式开关反馈。
func Notify(title, msg string) {
	if hwnd == 0 {
		return
	}
	n := nid
	n.uFlags = nifInfo
	n.dwInfoFlags = niifInfo | niifNosound
	n.uVersion = 0 // 与 uTimeout 共用 union，0 = 系统默认展示时长
	setUTF16(n.szInfoTitle[:], title)
	setUTF16(n.szInfo[:], msg)
	shellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&n)))
}

func setUTF16(dst []uint16, s string) {
	for i := range dst {
		dst[i] = 0
	}
	src := syscall.StringToUTF16(s)
	if len(src) >= len(dst) {
		src = src[:len(dst)-1]
	}
	copy(dst, src)
	dst[len(dst)-1] = 0
}

// OpenFile 用系统默认程序打开本地文件（报告 / CSV）。
func OpenFile(path string) {
	verb := syscall.StringToUTF16("open")
	p := syscall.StringToUTF16(path)
	shellExecute.Call(0,
		uintptr(unsafe.Pointer(&verb[0])),
		uintptr(unsafe.Pointer(&p[0])),
		0, 0, swShowNormal)
}

// OpenURL 用系统默认浏览器打开 URL（本地看板只传 127.0.0.1 回环地址）。
func OpenURL(url string) {
	verb := syscall.StringToUTF16("open")
	u := syscall.StringToUTF16(url)
	shellExecute.Call(0,
		uintptr(unsafe.Pointer(&verb[0])),
		uintptr(unsafe.Pointer(&u[0])),
		0, 0, swShowNormal)
}

func Remove() {
	shellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	if currentIcon != 0 {
		destroyIcon.Call(currentIcon)
		currentIcon = 0
	}
	if hwnd != 0 {
		destroyWindow.Call(hwnd)
		hwnd = 0
	}
}
