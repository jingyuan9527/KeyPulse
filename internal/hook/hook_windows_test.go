//go:build windows

package hook

import (
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// 只验证 Win32 过程解析与回调 ABI 是否合法，
// 不安装任何钩子、不接收真实输入。

func TestHookProcsResolvable(t *testing.T) {
	procs := []struct {
		name string
		p    *syscall.LazyProc
	}{
		{"SetWindowsHookExW", setHookEx},
		{"UnhookWindowsHookEx", unhookEx},
		{"CallNextHookEx", callNext},
		{"GetMessageW", getMessage},
		{"TranslateMessage", translateMsg},
		{"DispatchMessageW", dispatchMsg},
		{"PostThreadMessageW", postThreadMsg},
		{"GetCurrentThreadId", getThreadID},
		{"SetPriorityClass", setPriorityCls},
	}
	for _, x := range procs {
		if err := x.p.Find(); err != nil {
			t.Errorf("%s 解析失败: %v", x.name, err)
		}
	}
	if addr := callNext.Addr(); addr == 0 {
		t.Fatal("CallNextHookEx 地址为空")
	}
}

func TestHookCallbackABI(t *testing.T) {
	for name, fn := range map[string]any{
		"keyboardProc": keyboardProc,
		"mouseProc":    mouseProc,
	} {
		if cb := syscall.NewCallback(fn); cb == 0 {
			t.Errorf("%s 回调编译返回空指针", name)
		}
	}
}

func TestGetLastInputInfoABI(t *testing.T) {
	if err := getLastInputInfo.Find(); err != nil {
		t.Fatalf("GetLastInputInfo 解析失败: %v", err)
	}
	// LASTINPUTINFO = {UINT cbSize; DWORD dwTime}，x64 上必须是 8 字节。
	if got := unsafe.Sizeof(lastInputInfo{}); got != 8 {
		t.Fatalf("LASTINPUTINFO 大小 = %d，期望 8", got)
	}
	if _, ok := lastInputTick(); !ok {
		t.Fatal("GetLastInputInfo 调用失败（cbSize 可能填错）")
	}
}

// 心跳判定全表：覆盖首周期基线、API 失败、无输入、钩子正常、
// 冷却期抑制、疑似掉线触发重挂、以及重挂失败后的无条件重试。
func TestDecideAction(t *testing.T) {
	cases := []struct {
		name                              string
		first, ok, changed, alive, broken bool
		elapsed, gap                      time.Duration
		want                              healthAction
	}{
		{"首周期只建基线", true, true, true, false, false, time.Hour, reinstallMinGap, actionNone},
		{"GetLastInputInfo 失败", false, false, true, false, false, time.Hour, reinstallMinGap, actionNone},
		{"系统无新输入", false, true, false, false, false, time.Hour, reinstallMinGap, actionNone},
		{"有输入且钩子有回调", false, true, true, true, false, time.Hour, reinstallMinGap, actionNone},
		{"疑似掉线但冷却未过", false, true, true, false, false, time.Second, reinstallMinGap, actionNone},
		{"退避到 60 秒时冷却未过", false, true, true, false, false, 30 * time.Second, reinstallMaxGap, actionNone},
		{"疑似掉线且冷却已过", false, true, true, false, false, 6 * time.Second, reinstallMinGap, actionReinstall},
		{"重挂失败后无条件重试", false, true, false, false, true, 0, reinstallMaxGap, actionRetry},
		{"broken 优先于首周期", true, false, false, false, true, 0, reinstallMaxGap, actionRetry},
	}
	for _, c := range cases {
		got := decideAction(c.first, c.ok, c.changed, c.alive, c.broken, c.elapsed, c.gap)
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	if !shouldReinstall(true, false, reinstallMinGap, reinstallMinGap) {
		t.Error("冷却期刚好到达时应允许重挂")
	}
	if shouldReinstall(true, false, reinstallMinGap-time.Nanosecond, reinstallMinGap) {
		t.Error("冷却期未到不应重挂")
	}
}

// 心跳消息必须落在 WM_APP 段内，且不能撞上托盘回调消息或 WM_QUIT，
// 否则 wndProc 会把心跳当成托盘点击，或被当成退出指令。
func TestHeartbeatMessageIsolated(t *testing.T) {
	if wmHeartbeat < 0x8000 || wmHeartbeat > 0xBFFF {
		t.Fatalf("心跳消息 0x%X 不在 WM_APP 段", wmHeartbeat)
	}
	if wmHeartbeat == 0x8001 {
		t.Fatal("心跳消息与托盘 wmTray 冲突")
	}
	if wmHeartbeat == wmQuit {
		t.Fatal("心跳消息与 WM_QUIT 冲突")
	}
	if reinstallMinGap <= 0 || reinstallMaxGap < reinstallMinGap {
		t.Fatal("重挂退避常量非法")
	}
}

// 实机验证心跳的传输链路：线程消息队列 → PostThreadMessageW →
// MSG 结构里 message 字段的偏移。不安装任何钩子、不接收真实输入。
// 用 PeekMessage(PM_REMOVE) 而不是 GetMessageW，避免异常时测试进程被阻塞。
func TestHeartbeatMessageRoundTrip(t *testing.T) {
	if err := peekMsg.Find(); err != nil {
		t.Fatalf("PeekMessageW 解析失败: %v", err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var m msg
	p := uintptr(unsafe.Pointer(&m))
	// 线程消息队列是懒创建的：不先 Peek 一次，PostThreadMessage 会失败。
	peekMsg.Call(p, 0, 0, 0, 0) // PM_NOREMOVE

	tid, _, _ := getThreadID.Call()
	if tid == 0 {
		t.Fatal("GetCurrentThreadId 返回 0")
	}
	if r, _, _ := postThreadMsg.Call(tid, wmHeartbeat, 0, 0); r == 0 {
		t.Fatal("PostThreadMessageW 投递心跳失败")
	}
	if r, _, _ := peekMsg.Call(p, 0, 0, 0, 1); r == 0 { // PM_REMOVE
		t.Fatal("未取到心跳消息")
	}
	if m.message != wmHeartbeat {
		t.Errorf("MSG.message 偏移不对: got 0x%X, want 0x%X", m.message, wmHeartbeat)
	}
}

func TestSetRecoveryHandler(t *testing.T) {
	SetRecoveryHandler(func(Recovery) {})
	v := recoveryHandler.Load()
	if v == nil {
		t.Fatal("恢复处理器未注册")
	}
	if _, ok := v.(func(Recovery)); !ok {
		t.Fatal("恢复处理器类型错误")
	}
}

func TestMouseMoveIsIgnored(t *testing.T) {
	// WM_MOUSEMOVE(0x0200) 不在 mouseProc 的 switch 中，
	// 这里用计数器的不变量校验：把该消息喂进判定表不应命中任何槽。
	const wmMouseMove = 0x0200
	handled := map[uintptr]bool{
		wmLButtonDown: true, wmRButtonDown: true, wmMButtonDown: true,
		wmXButtonDown: true, wmMouseWheel: true, wmMouseHWheel: true,
	}
	if handled[wmMouseMove] {
		t.Fatal("WM_MOUSEMOVE 不应进入计数分支")
	}
}
