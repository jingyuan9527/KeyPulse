//go:build windows

// Package hook 挂载 Windows 全局低层钩子并独占一个 OS 线程跑消息循环。
//
// 无感运行的关键约束：
//  1. 回调里绝不写文件、不分配内存、不调用 time.Now、不发 channel；
//  2. WM_MOUSEMOVE 是频率最高的事件（每秒上千次），在 switch 里直接落空过滤；
//  3. 任何路径都必须调用 CallNextHookEx，否则用户的输入会被吞掉；
//  4. 钩子会因回调超时被系统静默卸载，所以回调只做原子操作；
//  5. 心跳检测与重挂都在钩子所属 OS 线程执行：低层全局钩子的回调被投递到
//     安装它的线程消息队列，换线程重装会导致回调永远不被取出执行。
package hook

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"keypulse/internal/counter"
)

const (
	whKeyboardLL = 13
	whMouseLL    = 14

	wmKeyDown     = 0x0100
	wmKeyUp       = 0x0101
	wmSysKeyDown  = 0x0104
	wmSysKeyUp    = 0x0105
	wmLButtonDown = 0x0201
	wmRButtonDown = 0x0204
	wmMButtonDown = 0x0207
	wmMouseWheel  = 0x020A
	wmXButtonDown = 0x020B
	wmMouseHWheel = 0x020E
	wmQuit        = 0x0012
	wmHeartbeat   = 0x8000 + 100 // WM_APP 范围，不与托盘的 wmTray(0x8001) 冲突

	belowNormalPriorityClass = 0x00004000

	// 重挂冷却：正常 5 秒；连续掉线（多为锁屏 / UAC 安全桌面 / 高完整性窗口
	// 造成的"有输入但钩子收不到"假阳性）逐级退避到最多 60 秒，避免风暴。
	// 一旦某个心跳周期确认钩子有回调，立即复位。
	reinstallMinGap = 5 * time.Second
	reinstallMaxGap = 60 * time.Second
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	setHookEx        = user32.NewProc("SetWindowsHookExW")
	unhookEx         = user32.NewProc("UnhookWindowsHookEx")
	callNext         = user32.NewProc("CallNextHookEx")
	getMessage       = user32.NewProc("GetMessageW")
	peekMsg          = user32.NewProc("PeekMessageW")
	translateMsg     = user32.NewProc("TranslateMessage")
	dispatchMsg      = user32.NewProc("DispatchMessageW")
	postThreadMsg    = user32.NewProc("PostThreadMessageW")
	getLastInputInfo = user32.NewProc("GetLastInputInfo")
	getThreadID      = kernel32.NewProc("GetCurrentThreadId")
	getProcess       = kernel32.NewProc("GetCurrentProcess")
	setPriorityCls   = kernel32.NewProc("SetPriorityClass")

	callNextAddr uintptr
	paused       atomic.Bool
	active       *Hook

	// seenFlag 由两个回调无条件置 1、由心跳 Swap 读并清零。
	// 用 Store 而非 Add：每次回调只有一条原子写，没有 RMW，鼠标移动
	// 每秒上千次时也不会形成争用热点。
	seenFlag atomic.Uint32

	// recoveryHandler 存 func(Recovery)，只允许在 Install 之前设置；
	// 用 atomic.Value 保证写入对钩子线程的可见性。
	recoveryHandler atomic.Value
)

// Recovery 是一次"检测到系统有输入但钩子零回调 → 自动重挂成功"的记录。
// 只含时间与估算缺口，不含任何按键内容。注意这不等于证明钩子被卸载：
// 锁屏、UAC 安全桌面、高完整性进程窗口同样会让本进程收不到回调。
type Recovery struct {
	At     time.Time
	GapSec int    // 距上次确认钩子存活的秒数，即可能漏计的时间窗
	Total  int    // 进程启动以来累计恢复次数
	Reason string // 固定标识，不采集任何上下文
}

// SetRecoveryHandler 注册恢复通知回调。回调会在独立 goroutine 里执行，
// 绝不能在钩子线程上做文件 IO 或弹托盘气泡。
func SetRecoveryHandler(fn func(Recovery)) {
	if fn != nil {
		recoveryHandler.Store(fn)
	}
}

// Hook 的健康状态字段只由钩子线程读写，无需加锁。
type Hook struct {
	c         *counter.Counter
	kbdHook   uintptr
	mouseHook uintptr
	threadID  uint32
	kbdCB     uintptr
	mouseCB   uintptr

	lastTick    uint32
	tickInit    bool
	lastAlive   time.Time
	lastRecover time.Time
	gap         time.Duration
	recoveries  int
	suspects    int
	broken      bool // 上次重挂失败，当前无任何钩子，下个心跳无条件重试
}

func Paused() bool { return paused.Load() }

func SetPaused(v bool) bool {
	paused.Store(v)
	return v
}

func TogglePaused() bool {
	for {
		old := paused.Load()
		if paused.CompareAndSwap(old, !old) {
			return !old
		}
	}
}

// SetBelowNormalPriority 降低后台进程优先级。低层钩子回调在产生输入的线程
// 上下文中执行，不受本进程优先级影响，因此不会增加输入延迟。
func SetBelowNormalPriority() {
	p, _, _ := getProcess.Call()
	if p != 0 {
		setPriorityCls.Call(p, belowNormalPriorityClass)
	}
}

// Install 必须在即将运行 Run() 的同一个 goroutine 里调用（内部锁定 OS 线程）。
func Install(c *counter.Counter) (*Hook, error) {
	runtime.LockOSThread()
	active = &Hook{
		c:         c,
		gap:       reinstallMinGap,
		lastAlive: time.Now(),
	}
	active.kbdCB = syscall.NewCallback(keyboardProc)
	active.mouseCB = syscall.NewCallback(mouseProc)
	callNextAddr = callNext.Addr()

	tid, _, _ := getThreadID.Call()
	active.threadID = uint32(tid)

	kh, _, e := setHookEx.Call(whKeyboardLL, active.kbdCB, 0, 0)
	if kh == 0 {
		return nil, fmt.Errorf("安装键盘钩子失败: %v", e)
	}
	mh, _, e := setHookEx.Call(whMouseLL, active.mouseCB, 0, 0)
	if mh == 0 {
		unhookEx.Call(kh)
		return nil, fmt.Errorf("安装鼠标钩子失败: %v", e)
	}
	active.kbdHook, active.mouseHook = kh, mh
	return active, nil
}

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

// lastInputTick 只读地取系统最后一次输入事件的 tick（GetTickCount 基准）。
// 它来自系统输入队列，与本进程钩子无关，因此可以当"外部真值"用：
// tick 变了说明真的有人敲了键/动了鼠标，此时若钩子一次回调都没有，就是掉线。
// 只比较是否变化，不做差值运算，因此不受 49.7 天回绕与睡眠唤醒影响。
func lastInputTick() (uint32, bool) {
	li := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
	r, _, _ := getLastInputInfo.Call(uintptr(unsafe.Pointer(&li)))
	return li.dwTime, r != 0
}

type healthAction int

const (
	actionNone      healthAction = iota
	actionReinstall              // 疑似静默卸载，按冷却期重挂
	actionRetry                  // 上次重挂失败、当前无钩子，无条件重试
)

// shouldReinstall 抽成纯函数便于单测：系统有新输入、钩子零回调、
// 且已过冷却期，三个条件同时成立才重挂。
func shouldReinstall(inputChanged, hookAlive bool, elapsed, gap time.Duration) bool {
	return inputChanged && !hookAlive && elapsed >= gap
}

// decideAction 是心跳的全部判定逻辑，不触碰任何 Win32 调用，便于离线单测。
// first=首个心跳周期（只建基线）；ok=GetLastInputInfo 调用成功；
// changed=系统最后输入 tick 变化；alive=本周期收到过钩子回调。
func decideAction(first, ok, changed, alive, broken bool, elapsed, gap time.Duration) healthAction {
	if broken {
		return actionRetry
	}
	if !ok || first || !shouldReinstall(changed, alive, elapsed, gap) {
		return actionNone
	}
	return actionReinstall
}

// Heartbeat 只往钩子线程的消息队列投一条自定义消息；真正的检查与重挂
// 在 Run() 收到该消息后于同一 OS 线程执行。可由任意 goroutine 调用。
func (h *Hook) Heartbeat() {
	if h == nil || h.threadID == 0 {
		return
	}
	postThreadMsg.Call(uintptr(h.threadID), wmHeartbeat, 0, 0)
}

// checkHealth 只在钩子线程执行。
func (h *Hook) checkHealth() {
	now := time.Now()
	alive := seenFlag.Swap(0) != 0
	if alive {
		h.lastAlive = now
		h.gap = reinstallMinGap // 确认钩子活着，退避复位
	}

	cur, ok := lastInputTick()
	prev := h.lastTick
	h.lastTick = cur
	first := !h.tickInit
	if ok {
		h.tickInit = true // 首个周期只建立基线，避免启动瞬间误判
	}
	if ok && !first && cur != prev && !alive {
		h.suspects++
	}

	switch decideAction(first, ok, cur != prev, alive, h.broken, now.Sub(h.lastRecover), h.gap) {
	case actionRetry:
		// 上次重挂失败导致当前完全没有钩子：无条件重试，不等输入、不过冷却。
		// 否则一旦用户敲完一下就停下，tick 不再变化，钩子会永久丢失。
		if h.reinstall() {
			h.broken = false
			h.recordRecovery(now)
		}
	case actionReinstall:
		if h.reinstall() {
			h.recordRecovery(now)
		} else {
			h.broken = true
		}
	}
}

// reinstall 必须在钩子线程调用。失败时摘掉所有半装状态，交给下个心跳重试。
func (h *Hook) reinstall() bool {
	if h.kbdHook != 0 {
		unhookEx.Call(h.kbdHook)
	}
	if h.mouseHook != 0 {
		unhookEx.Call(h.mouseHook)
	}
	h.kbdHook, h.mouseHook = 0, 0

	kh, _, _ := setHookEx.Call(whKeyboardLL, h.kbdCB, 0, 0)
	if kh == 0 {
		return false
	}
	mh, _, _ := setHookEx.Call(whMouseLL, h.mouseCB, 0, 0)
	if mh == 0 {
		unhookEx.Call(kh)
		return false
	}
	h.kbdHook, h.mouseHook = kh, mh
	return true
}

func (h *Hook) recordRecovery(now time.Time) {
	h.recoveries++
	h.lastRecover = now
	if h.gap < reinstallMaxGap {
		h.gap *= 2
		if h.gap > reinstallMaxGap {
			h.gap = reinstallMaxGap
		}
	}
	gapSec := int(now.Sub(h.lastAlive).Seconds())
	if gapSec < 0 {
		gapSec = 0
	}
	rec := Recovery{
		At:     now,
		GapSec: gapSec,
		Total:  h.recoveries,
		Reason: "input_without_callback",
	}
	if v := recoveryHandler.Load(); v != nil {
		if fn, ok := v.(func(Recovery)); ok {
			r := rec
			go fn(r) // 通知/写日志一律离开钩子线程
		}
	}
}

// Stats 返回运行期健康计数（调试与自检用）。
func (h *Hook) Stats() (recoveries, suspects int, broken bool) {
	if h == nil {
		return 0, 0, false
	}
	return h.recoveries, h.suspects, h.broken
}

type msg struct {
	hwnd     uintptr
	message  uint32
	_        uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	ptX      int32
	ptY      int32
	lPrivate uint32
	_        uint32
}

// Run 阻塞运行消息循环，直到收到 WM_QUIT。
func (h *Hook) Run() {
	var m msg
	p := uintptr(unsafe.Pointer(&m))
	for {
		r, _, _ := getMessage.Call(p, 0, 0, 0)
		if r == 0 || r == ^uintptr(0) { // WM_QUIT 或错误
			return
		}
		if m.message == wmHeartbeat {
			h.checkHealth()
			continue // 线程消息没有 hwnd，不需要 Translate/Dispatch
		}
		translateMsg.Call(p)
		dispatchMsg.Call(p)
	}
}

func (h *Hook) Quit() {
	postThreadMsg.Call(uintptr(h.threadID), wmQuit, 0, 0)
}

func (h *Hook) Uninstall() {
	if h.kbdHook != 0 {
		unhookEx.Call(h.kbdHook)
		h.kbdHook = 0
	}
	if h.mouseHook != 0 {
		unhookEx.Call(h.mouseHook)
		h.mouseHook = 0
	}
}

// keyboardProc: KBDLLHOOKSTRUCT 的 vkCode 在 offset 0。
func keyboardProc(code, wparam, lparam uintptr) uintptr {
	seenFlag.Store(1) // 心跳存活标志：无条件、单条原子写
	if int32(code) >= 0 && lparam != 0 {
		vk := *(*uint32)(unsafe.Pointer(lparam))
		if vk < counter.KeySlots {
			switch wparam {
			case wmKeyDown, wmSysKeyDown:
				// 位图过滤长按连发：已按下则跳过计数
				if active.c.MarkDown(vk) && !paused.Load() {
					active.c.Add(int(vk))
				}
			case wmKeyUp, wmSysKeyUp:
				active.c.MarkUp(vk)
			}
		}
	}
	// SyscallN 的签名是 (trap, args...)，不需要参数个数；CallNextHookEx 的
	// 四个实参为 hhk(低层钩子忽略)、nCode、wParam、lParam，必须原样透传，
	// 否则会破坏全局钩子链，导致其他程序收到错位的消息。
	r, _, _ := syscall.SyscallN(callNextAddr, 0, code, wparam, lparam)
	return r
}

// mouseProc: WM_MOUSEMOVE 不在 switch 中，回调开销恒定为一个比较。
// 但它仍会置 seenFlag，否则"只移动鼠标"会被误判成钩子掉线。
// MSLLHOOKSTRUCT.mouseData 在 offset 8（POINT 占 8 字节）。
func mouseProc(code, wparam, lparam uintptr) uintptr {
	seenFlag.Store(1)
	if int32(code) >= 0 {
		slot := -1
		switch wparam {
		case wmLButtonDown:
			slot = counter.MouseLeft
		case wmRButtonDown:
			slot = counter.MouseRight
		case wmMButtonDown:
			slot = counter.MouseMiddle
		case wmXButtonDown:
			if lparam != 0 {
				md := *(*uint32)(unsafe.Pointer(lparam + 8))
				switch (md >> 16) & 0xFFFF {
				case 1:
					slot = counter.MouseX1
				case 2:
					slot = counter.MouseX2
				}
			}
		case wmMouseWheel, wmMouseHWheel:
			slot = counter.MouseWheel
		}
		if slot >= 0 && !paused.Load() {
			active.c.Add(slot)
		}
	}
	r, _, _ := syscall.SyscallN(callNextAddr, 0, code, wparam, lparam)
	return r
}
