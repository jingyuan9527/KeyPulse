package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// 时间跳跃判定必须只对睡眠/休眠/时间调整这类真实跳变触发，
// 正常 ticker 抖动与跨零点的 1 秒推进都不能误判。
func TestTimeJump(t *testing.T) {
	const base = int64(1_700_000_000)
	cases := []struct {
		name      string
		prev, now int64
		want      bool
	}{
		{"正常 1 秒", base, base + 1, false},
		{"卡顿 8 秒", base, base + 8, false},
		{"阈值边界 60 秒", base, base + 60, false},
		{"刚过阈值 61 秒", base, base + 61, true},
		{"睡眠 10 分钟", base, base + 600, true},
		{"系统时间回拨", base, base - 3600, true},
		{"跨零点正常推进", base, base + 1, false},
	}
	for _, c := range cases {
		if got := timeJump(c.prev, c.now); got != c.want {
			t.Errorf("%s: timeJump=%v, want %v", c.name, got, c.want)
		}
	}
}

// 启动期消息在 GUI 版必须可见。用可替换的间接层验证通道选择，
// 不真的弹窗，也不依赖测试进程所在的终端环境。
type notifyCapture struct {
	printCalls  int
	printed     string
	dialogCalls int
	title, text string
}

func stubNotify(t *testing.T, console, stdout uintptr) *notifyCapture {
	t.Helper()
	oc, oh, od, op := consoleWindow, stdoutHandle, showDialog, printLine
	t.Cleanup(func() { consoleWindow, stdoutHandle, showDialog, printLine = oc, oh, od, op })
	c := &notifyCapture{}
	consoleWindow = func() uintptr { return console }
	stdoutHandle = func() uintptr { return stdout }
	printLine = func(s string) { c.printCalls++; c.printed = s }
	showDialog = func(title, text string) { c.dialogCalls++; c.title, c.text = title, text }
	return c
}

func TestNotifyPrintsWhenConsoleAttached(t *testing.T) {
	c := stubNotify(t, 0x1234, 0)
	notify("标题", "正文")
	if c.printCalls != 1 || c.dialogCalls != 0 {
		t.Fatalf("有控制台时应只打印：print=%d dialog=%d", c.printCalls, c.dialogCalls)
	}
	if !strings.Contains(c.printed, "标题") || !strings.Contains(c.printed, "正文") {
		t.Errorf("打印内容缺少标题或正文: %q", c.printed)
	}
}

// MSYS / mintty 没有 Windows 控制台窗口，但 stdout 是有效管道句柄，
// CLI 版在这里也必须打印，不能误判成「无输出通道」去弹框阻塞。
func TestNotifyPrintsWhenOnlyStdoutIsValid(t *testing.T) {
	c := stubNotify(t, 0, 712)
	notify("标题", "正文")
	if c.printCalls != 1 || c.dialogCalls != 0 {
		t.Fatalf("stdout 有效时应只打印：print=%d dialog=%d", c.printCalls, c.dialogCalls)
	}
}

func TestNotifyShowsDialogWithoutConsole(t *testing.T) {
	c := stubNotify(t, 0, 0)
	notify("KeyPulse 已在运行", "请在托盘右键退出后重新启动")
	if c.dialogCalls != 1 || c.printCalls != 0 {
		t.Fatalf("无控制台且无 stdout 时应只弹窗：print=%d dialog=%d", c.printCalls, c.dialogCalls)
	}
	if c.title != "KeyPulse 已在运行" || !strings.Contains(c.text, "托盘右键退出") {
		t.Errorf("弹窗内容错误: title=%q text=%q", c.title, c.text)
	}
}

func TestNotifyShowsDialogOnInvalidStdout(t *testing.T) {
	c := stubNotify(t, 0, invalidHandle)
	notify("标题", "正文")
	if c.dialogCalls != 1 || c.printCalls != 0 {
		t.Fatalf("stdout 为 INVALID_HANDLE_VALUE 时应弹窗：print=%d dialog=%d", c.printCalls, c.dialogCalls)
	}
}

// 拼错 DLL 导出名只会在运行时暴露，且 GUI 版看不见任何错误，故显式校验解析。
func TestNotifyWindowsProcsResolvable(t *testing.T) {
	for _, p := range []struct {
		name string
		proc *syscall.LazyProc
	}{
		{"CreateMutexW", procCreateMutexW},
		{"GetConsoleWindow", procGetConsoleWindow},
		{"GetStdHandle", procGetStdHandle},
		{"MessageBoxW", procMessageBoxW},
	} {
		if err := p.proc.Find(); err != nil {
			t.Errorf("%s 无法解析: %v", p.name, err)
		}
	}
}

// 欢迎气泡按数据目录判定「首次」：换 --dir 的演示目录必须能再次触发，
// 文案不得出现键盘 / 监控等敏感词（与隐私叙事一致）。
func TestWelcomeOnce(t *testing.T) {
	dir := t.TempDir()
	var calls [][]string
	orig := trayNotify
	t.Cleanup(func() { trayNotify = orig })
	trayNotify = func(title, text string) { calls = append(calls, []string{title, text}) }

	welcomeOnce(dir)
	if len(calls) != 1 {
		t.Fatalf("首次启动应弹一次气泡，实际 %d 次", len(calls))
	}
	if calls[0][0] != "KeyPulse 已开始统计" || !strings.Contains(calls[0][1], "双击托盘图标查看报告") {
		t.Errorf("欢迎气泡文案错误: %q / %q", calls[0][0], calls[0][1])
	}
	for _, s := range []string{"键盘", "监控", "记录你"} {
		if strings.Contains(calls[0][0]+calls[0][1], s) {
			t.Errorf("欢迎气泡出现敏感词 %q: %q / %q", s, calls[0][0], calls[0][1])
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".welcomed")); err != nil {
		t.Fatalf("标记文件未写入: %v", err)
	}

	welcomeOnce(dir)
	if len(calls) != 1 {
		t.Fatalf("同目录第二次启动不应再弹，实际 %d 次", len(calls))
	}

	os.Remove(filepath.Join(dir, ".welcomed"))
	welcomeOnce(dir)
	if len(calls) != 2 {
		t.Fatalf("删除标记后应再次弹出，实际 %d 次", len(calls))
	}
	if !strings.Contains(calls[1][1], dir) {
		t.Errorf("气泡未携带数据目录: %q", calls[1][1])
	}
}
