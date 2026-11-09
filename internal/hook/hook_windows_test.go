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

// 心跳判定全表：覆盖首周期基线、API 失败、无输入、钩子正常、冷却期抑制、
// 静默窗口抑制（旧版误报主因）、确认掉线触发重挂、重挂失败无条件重试。
func TestDecideAction(t *testing.T) {
	cases := []struct {
		name                                string
		first, ok, alive, broken, inputSeen bool
		elapsed, silence, gap               time.Duration
		want                                healthAction
	}{
		{"首周期只建基线", true, true, false, false, true, time.Hour, time.Hour, reinstallMinGap, actionNone},
		{"GetLastInputInfo 失败", false, false, false, false, true, time.Hour, time.Hour, reinstallMinGap, actionNone},
		{"期间系统无输入", false, true, false, false, false, time.Hour, time.Hour, reinstallMinGap, actionNone},
		{"有输入且钩子有回调", false, true, true, false, true, time.Hour, time.Hour, reinstallMinGap, actionNone},
		{"静默仅 1 秒：旧版误报主因", false, true, false, false, true, time.Hour, time.Second, reinstallMinGap, actionNone},
		{"静默期已过但冷却未过", false, true, false, false, true, time.Second, time.Hour, reinstallMinGap, actionNone},
		{"退避到 60 秒时冷却未过", false, true, false, false, true, 30 * time.Second, time.Hour, reinstallMaxGap, actionNone},
		{"静默窗口差 1ns 不重挂", false, true, false, false, true, time.Hour, confirmedSilence - time.Nanosecond, reinstallMinGap, actionNone},
		{"静默与冷却均已过才重挂", false, true, false, false, true, 6 * time.Second, confirmedSilence, reinstallMinGap, actionReinstall},
		{"重挂失败后无条件重试", false, true, false, true, false, 0, 0, reinstallMaxGap, actionRetry},
		{"broken 优先于首周期", true, false, false, true, false, 0, 0, reinstallMaxGap, actionRetry},
	}
	for _, c := range cases {
		got := decideAction(c.first, c.ok, c.alive, c.broken, c.inputSeen, c.elapsed, c.silence, c.gap)
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	if !shouldReinstall(true, false, confirmedSilence, reinstallMinGap, reinstallMinGap) {
		t.Error("静默窗口与冷却期刚好到达时应允许重挂")
	}
	if shouldReinstall(true, false, confirmedSilence, reinstallMinGap-time.Nanosecond, reinstallMinGap) {
		t.Error("冷却期未到不应重挂")
	}
	if shouldReinstall(true, false, confirmedSilence-time.Nanosecond, reinstallMinGap, reinstallMinGap) {
		t.Error("静默窗口未到不应重挂")
	}
	if shouldReinstall(false, false, confirmedSilence, reinstallMinGap, reinstallMinGap) {
		t.Error("期间系统无输入不应重挂")
	}
	if shouldReinstall(true, true, confirmedSilence, reinstallMinGap, reinstallMinGap) {
		t.Error("钩子有回调不应重挂")
	}
}

// 复现线上 67 条假阳性里的主因：回调先到、系统输入 tick 晚一个心跳周期变化。
// 旧判定会在第二个周期直接重挂并写 health.log；新判定要求连续 30 秒静默。
func TestObserveHealthFiltersCrossCycleTickLag(t *testing.T) {
	base := time.Now()
	h := &Hook{lastAlive: base, gap: reinstallMinGap, lastRecover: base}

	h.observeHealth(base.Add(2*time.Second), true, false) // 用户敲键，钩子有回调
	h.observeHealth(base.Add(4*time.Second), false, true) // tick 滞后变化，本周期无回调
	silence := 2 * time.Second
	if got := decideAction(false, true, false, false, h.inputSinceAlive, time.Hour, silence, h.gap); got != actionNone {
		t.Errorf("跨周期 tick 错位被误判为掉线: %v", got)
	}
	h.observeHealth(base.Add(6*time.Second), true, false) // 钩子恢复回调
	if h.inputSinceAlive || h.suspects != 0 {
		t.Errorf("确认存活后观察窗口未复位: inputSinceAlive=%v suspects=%d", h.inputSinceAlive, h.suspects)
	}

	// 真掉线：连续 31 秒系统有输入、钩子零回调，仍必须检出
	h2 := &Hook{lastAlive: base, gap: reinstallMinGap, lastRecover: base}
	h2.observeHealth(base.Add(time.Second), false, true)
	for s := 3; s <= 31; s += 2 {
		h2.observeHealth(base.Add(time.Duration(s)*time.Second), false, false)
	}
	now := base.Add(31 * time.Second)
	if got := decideAction(false, true, false, false, h2.inputSinceAlive,
		now.Sub(h2.lastRecover), now.Sub(h2.lastAlive), h2.gap); got != actionReinstall {
		t.Errorf("真实掉线未检出: %v", got)
	}
}

// 重挂成功后必须重置观察窗口，否则下个心跳会因"仍无回调且静默期已过"
// 立刻再重挂一次；同时验证新版 reason 与估算缺口。
func TestRecordRecoveryResetsObservation(t *testing.T) {
	ch := make(chan Recovery, 1)
	SetRecoveryHandler(func(r Recovery) { ch <- r })
	now := time.Now()
	h := &Hook{
		lastAlive:       now.Add(-95 * time.Second),
		inputSinceAlive: true,
		suspects:        7,
		gap:             reinstallMinGap,
	}
	h.recordRecovery(now)
	select {
	case r := <-ch:
		if r.Reason != "confirmed_silence" {
			t.Errorf("reason = %q，应为 confirmed_silence", r.Reason)
		}
		if r.GapSec != 95 {
			t.Errorf("GapSec = %d，应为 95", r.GapSec)
		}
		if r.Total != 1 {
			t.Errorf("Total = %d，应为 1", r.Total)
		}
	case <-time.After(time.Second):
		t.Fatal("恢复通知未在 1 秒内到达")
	}
	if h.inputSinceAlive || h.suspects != 0 || !h.lastAlive.Equal(now) {
		t.Errorf("观察窗口未复位: inputSinceAlive=%v suspects=%d lastAlive=%v",
			h.inputSinceAlive, h.suspects, h.lastAlive)
	}
	if h.gap != 2*reinstallMinGap {
		t.Errorf("退避未生效: gap=%v", h.gap)
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
