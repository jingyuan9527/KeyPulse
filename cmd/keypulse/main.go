package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"keypulse/internal/counter"
	"keypulse/internal/hook"
	"keypulse/internal/report"
	"keypulse/internal/store"
	"keypulse/internal/tray"
	"keypulse/internal/version"
)

var singletonMutex uintptr

// GUI 版由 -H=windowsgui 编译，进程没有控制台，fmt.Println 的输出会被直接
// 丢弃：启动期错误与单实例冲突若只打印，用户双击后看到的就是「毫无反应」。
// 这里在运行时判断 stdout 是否可见（控制台窗口或有效标准输出句柄）——
// CLI 版照旧打印，GUI 版弹系统消息框。两份 exe 仍是同一份源码，构建脚本无需改动。
const msgBoxFlags = 0x00000040 | 0x00010000 | 0x00040000 // MB_ICONINFORMATION | MB_SETFOREGROUND | MB_TOPMOST

var (
	k32 = syscall.NewLazyDLL("kernel32.dll")
	u32 = syscall.NewLazyDLL("user32.dll")

	procCreateMutexW     = k32.NewProc("CreateMutexW")
	procGetConsoleWindow = k32.NewProc("GetConsoleWindow")
	procGetStdHandle     = k32.NewProc("GetStdHandle")
	procMessageBoxW      = u32.NewProc("MessageBoxW")

	stdOutputHandle = ^uintptr(10) // DWORD(STD_OUTPUT_HANDLE) = -11
	invalidHandle   = ^uintptr(0)  // INVALID_HANDLE_VALUE

	// 四个间接层供单测替换，避免测试真的弹窗或依赖终端环境。
	consoleWindow = func() uintptr {
		h, _, _ := procGetConsoleWindow.Call()
		return h
	}
	stdoutHandle = func() uintptr {
		h, _, _ := procGetStdHandle.Call(stdOutputHandle)
		return h
	}
	showDialog = func(title, text string) {
		caption := syscall.StringToUTF16(title)
		body := syscall.StringToUTF16(text)
		// MessageBoxW(hWnd, lpText, lpCaption, uType)：正文在前、标题在后。
		procMessageBoxW.Call(0,
			uintptr(unsafe.Pointer(&body[0])),
			uintptr(unsafe.Pointer(&caption[0])),
			msgBoxFlags)
	}
	printLine = func(s string) { fmt.Println(s) }
)

// outputVisible 判断 stdout 能否真正被看见。不能只靠 GetConsoleWindow：
// MSYS / mintty 这类终端没有 Windows 控制台窗口（返回 0），但 stdout 是有效
// 管道句柄，CLI 版仍应照旧打印。只有两者都拿不到时（GUI 版由资源管理器
// 双击启动），才需要弹系统消息框。
func outputVisible() bool {
	if consoleWindow() != 0 {
		return true
	}
	h := stdoutHandle()
	return h != 0 && h != invalidHandle
}

// notify 把用户必须看见的启动期消息送到当前进程真正可见的通道。
func notify(title, text string) {
	if outputVisible() {
		printLine(title + "：" + text)
		return
	}
	showDialog(title, text)
}

const (
	reportKeepAlive = 10 * time.Minute // 打开报告后的自动更新保活窗口
	reportRegenGap  = 30 * time.Second // 保活期内最快重生成间隔
	timeJumpSeconds = 60               // 墙上时间跳跃阈值，用于识别睡眠/休眠/时间调整
)

// timeJump 判断两次 tick 之间是否发生了非正常的墙上时间跳变。
// 用墙上时间而不是 time.Since：Go 的 monotonic 时钟在系统睡眠/虚拟机
// 暂停期间可能同样停走，只有墙上时间一定会跳。60 秒阈值远大于正常
// ticker 抖动，又远小于一次真实睡眠。
func timeJump(prev, now int64) bool {
	d := now - prev
	return d > timeJumpSeconds || d < -timeJumpSeconds
}

var (
	reportMu        sync.Mutex
	reportActive    atomic.Int64 // 保活截止时刻（UnixNano）
	lastReportGen   atomic.Int64 // 上次生成时刻（UnixNano）
	lastHealthNotif atomic.Int64 // 上次弹恢复气泡的时刻（UnixNano）
	liveMode        atomic.Bool  // 离线报告的实时模式：默认关闭、重启不恢复，写 live.js
	boardLive       atomic.Bool  // 看板 SSE 推送开关，与离线报告的实时模式互不影响
	liveSeq         atomic.Uint64
)

// appendHealthLog 只在确认采集中断并恢复时写一行，不含任何按键内容。
// 报告生成时读取它，用来如实标注数据缺口；旧版误报记录由报告端忽略。
func appendHealthLog(dir string, r hook.Recovery) {
	p := filepath.Join(dir, "health.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s RECOVER gap=%ds total=%d reason=%s\n",
		r.At.Format("2006-01-02 15:04:05"), r.GapSec, r.Total, r.Reason)
}

func humanGap(sec int) string {
	switch {
	case sec < 60:
		return fmt.Sprintf("%d 秒", sec)
	case sec < 3600:
		return fmt.Sprintf("%d 分钟", sec/60)
	default:
		return fmt.Sprintf("%d 小时", sec/3600)
	}
}

func defaultDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	if base == "" {
		base = "."
	}
	return filepath.Join(base, "KeyPulse")
}

func acquireSingleton() bool {
	name := syscall.StringToUTF16(`Local\KeyPulseSingleton`)
	h, _, e := procCreateMutexW.Call(0, 1, uintptr(unsafe.Pointer(&name[0])))
	if h == 0 {
		return false
	}
	if errno, ok := e.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
		return false
	}
	singletonMutex = h // 持有句柄，进程退出才释放
	return true
}

// writeReport 生成一次报告快照，并发调用由 reportMu 串行化。
func writeReport(dir string) (string, error) {
	reportMu.Lock()
	defer reportMu.Unlock()
	// 页面内嵌最近 ReportMaxDays 天（默认保留策略为 90 天，实际远低于上限）；
	// 单日 / 周报 / 总览切换全部在浏览器本地完成，不产生额外磁盘 IO。
	days, err := store.ListDays(dir, report.ReportMaxDays)
	if err != nil {
		return "", err
	}
	if len(days) == 0 {
		return "", fmt.Errorf("暂无统计数据")
	}
	p, err := report.RenderWithLive(dir, days, liveMode.Load())
	if err == nil {
		lastReportGen.Store(time.Now().UnixNano())
	}
	return p, err
}

// makeReport: open=true 表示用户主动打开，会续保活窗口并弹出浏览器。
func makeReport(dir string, open bool) {
	p, err := writeReport(dir)
	if err != nil {
		fmt.Println("生成报告失败:", err)
		return
	}
	if open {
		reportActive.Store(time.Now().Add(reportKeepAlive).UnixNano())
		tray.OpenFile(p)
	} else {
		fmt.Println("报告已生成:", p)
	}
}

// backgroundReport 在保活窗口内、且距上次生成 ≥30 秒时后台重写快照。
// 不弹浏览器、不延长保活窗口；窗口过期后彻底停止写盘，回到零额外 IO。
func backgroundReport(dir string) {
	if time.Now().UnixNano() > reportActive.Load() {
		return
	}
	if time.Since(time.Unix(0, lastReportGen.Load())) < reportRegenGap {
		return
	}
	_, _ = writeReport(dir)
}

func exportCSV(dir string) {
	// CSV 导出不受报告上限约束，包含磁盘上的全部历史。
	days, err := store.ListDays(dir, 0)
	if err != nil || len(days) == 0 {
		return
	}
	p := filepath.Join(dir, "export.csv")
	f, err := os.Create(p)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write([]byte{0xEF, 0xBB, 0xBF}) // Excel 识别 UTF-8
	if err := store.ExportCSV(f, days); err != nil {
		return
	}
	tray.OpenFile(dir)
}

func weighted(r *rand.Rand, keys []uint32, w []int) uint32 {
	total := 0
	for _, x := range w {
		total += x
	}
	n := r.Intn(total)
	for i, x := range w {
		n -= x
		if n < 0 {
			return keys[i]
		}
	}
	return keys[0]
}

func weekMonday(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	y, m, d := t.Date()
	return time.Date(y, m, d-wd+1, 0, 0, 0, 0, t.Location())
}

// runSelfTest 只写模拟数据并生成报告，不挂钩子、不建托盘、不监控真实输入。
// 生成跨 5 个自然周的 35 天数据：周末量偏低、每周递增，便于验收周报与环比。
func runSelfTest(dir string) {
	d := filepath.Join(dir, "_selftest")
	r := rand.New(rand.NewSource(20261106))
	keys := []uint32{0x20, 0x41, 0x53, 0x44, 0x45, 0x49, 0x4F, 0x4E, 0x54, 0x52, 0x08, 0x0D, 0xA2, 0x10, 0xBC, 0xBE, 0x51, 0x57, 0x1B}
	weights := []int{18, 9, 8, 7, 11, 8, 7, 7, 8, 7, 4, 5, 3, 3, 2, 2, 2, 2, 1}
	now := time.Now()
	thisWeek := weekMonday(now)

	for i := 34; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		wd := int(day.Weekday())
		if wd == 0 {
			wd = 7
		}
		weeksAgo := int(thisWeek.Sub(weekMonday(day)).Hours() / (24 * 7))
		band := 1.0 + float64(4-weeksAgo)*0.12
		if wd >= 6 {
			band *= 0.45 // 周末明显少一些
		}
		dd := store.DayData{
			Stamp: day.Format("20060102"),
			Day:   uint32(day.Unix() / 86400),
		}
		for h := 7; h < 23; h++ {
			n := int(float64(120+r.Intn(700)) * band)
			for k := 0; k < n; k++ {
				dd.Keys[weighted(r, keys, weights)]++
			}
			dd.Hours[h*2] = uint32(n)
			clicks := int(float64(20+r.Intn(110)) * band)
			dd.Mouse[0] += uint32(clicks)
			dd.Mouse[5] += uint32(r.Intn(260))
			dd.Hours[h*2+1] = uint32(clicks)
			for m := 0; m < 60; m++ {
				if r.Intn(4) > 0 {
					idx := h*60 + m
					dd.Minutes[idx/8] |= 1 << uint(idx%8)
				}
			}
		}
		if err := store.WriteDayFile(d, dd); err != nil {
			fmt.Println("生成模拟数据失败:", err)
			return
		}
	}
	makeReport(d, false)
	fmt.Println("自检完成。模拟数据目录:", d)
	fmt.Println("用浏览器打开该目录下的 report.html 可查看单日 / 周报对比 / 总览三个视图。")
}

func main() {
	selfTest := flag.Bool("selftest", false, "生成模拟数据并输出报告（不挂钩子、不监控输入）")
	reportOnly := flag.Bool("report-only", false, "仅根据已有数据重新生成 HTML 报告")
	showVersion := flag.Bool("version", false, "输出版本与构建信息")
	dir := flag.String("dir", defaultDir(), "数据目录")
	keep := flag.Int("keep-days", 90, "数据保留天数，0 表示不清理")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}
	if *selfTest {
		runSelfTest(*dir)
		return
	}
	if *reportOnly {
		makeReport(*dir, false)
		return
	}

	if !acquireSingleton() {
		notify("KeyPulse 已在运行（单实例保护）",
			"程序只允许一个实例。请在任务栏托盘（可能被折叠在「^ 显示隐藏图标」里）右键选择「退出」，然后重新启动。")
		return
	}
	hook.SetBelowNormalPriority()

	c := counter.New()
	st, err := store.Open(*dir, c)
	if err != nil {
		notify("打开数据目录失败", err.Error())
		return
	}
	store.Cleanup(*dir, *keep)

	// 恢复通知由 hook 包丢到独立 goroutine 执行：文件 IO 与托盘气泡
	// 绝不能占用钩子的消息循环线程，否则会反过来诱发下一次钩子超时。
	hook.SetRecoveryHandler(func(r hook.Recovery) {
		appendHealthLog(*dir, r)
		now := r.At.UnixNano()
		for {
			last := lastHealthNotif.Load()
			if now-last < int64(5*time.Minute) {
				return // 5 分钟内最多提醒一次，避免锁屏场景反复打扰
			}
			if lastHealthNotif.CompareAndSwap(last, now) {
				break
			}
		}
		tray.Notify("KeyPulse 采集已恢复",
			"检测到连续 "+humanGap(r.GapSec)+"的输入无法采集，已自动重挂；该时段统计为近似值。")
	})

	h, err := hook.Install(c)
	if err != nil {
		notify("采集钩子安装失败", err.Error())
		st.Close()
		return
	}

	if err := tray.New(tray.Config{
		OnOpen: func() { makeReport(*dir, true) },
		OnToggle: func() bool {
			p := hook.TogglePaused()
			go st.Flush(true)
			return p
		},
		OnExport: func() { exportCSV(*dir) },
		Paused:   hook.Paused,
		Live:     func() bool { return liveMode.Load() },
		OnLiveToggle: func() bool {
			if boardMode.Load() {
				// 看板开启时菜单已隐藏本项；若因状态切换的时序竞态仍被调用，
				// 只切 SSE 推送开关，绝不碰离线报告的实时模式或落盘文件。
				on := !boardLive.Load()
				boardLive.Store(on)
				if on {
					go refreshLiveSnapshot(*dir, liveSeq.Add(1))
					tray.Notify("实时推送已开启", "仅影响本地看板的数据推送；关闭托盘「本地看板」才会释放端口。")
				} else {
					tray.Notify("实时推送已暂停", "看板页面仍在，但不再推送数据；关闭托盘「本地看板」才会释放端口。")
				}
				return on
			}
			on := !liveMode.Load()
			liveMode.Store(on)
			// 托盘菜单跑在钩子的消息循环线程上，读快照 / 写 HTML 一律
			// 丢到 goroutine，绝不能同步做文件 IO 阻塞输入。
			go func() {
				_, _ = st.Flush(true) // 先把内存计数落盘，实时值不会「倒退」
				if on {
					_ = report.WriteLive(*dir, liveSeq.Add(1), time.Now())
					reportActive.Store(time.Now().Add(reportKeepAlive).UnixNano())
				} else {
					_ = report.RemoveLive(*dir)
				}
				_, _ = writeReport(*dir) // 重写页面以注入 / 撤掉实时模式标志
				if on {
					tray.Notify("实时模式已开启", "报告每秒更新今日数据；已打开的页面会在 60 秒内自动切换。会增加少量磁盘写入，关闭即恢复空闲零 IO。")
				} else {
					tray.Notify("实时模式已关闭", "报告回到 60 秒自动刷新，空闲时不再额外写盘。")
				}
			}()
			return on
		},
		Board: func() bool { return boardMode.Load() },
		OnBoardToggle: func() bool {
			on := !boardMode.Load()
			boardMode.Store(on)
			// 监听端口、Flush、渲染页面全部异步，菜单线程只做一次原子写。
			go func() {
				if on {
					startBoard(*dir, st, c)
				} else {
					stopBoard(*dir)
				}
			}()
			return on
		},
	}); err != nil {
		notify("托盘初始化失败", err.Error())
		h.Uninstall()
		st.Close()
		return
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		hb := time.NewTicker(2 * time.Second) // 钩子心跳：检测静默卸载并自动重挂
		defer hb.Stop()
		lastWall := time.Now().Unix()
		for {
			select {
			case <-tk.C:
				nowWall := time.Now().Unix()
				// 睡眠/休眠/VM 暂停/系统时间调整：先强制结算到当前自然日，
				// 并立刻做一次钩子自检（唤醒是钩子最容易失效的时刻）。
				force := timeJump(lastWall, nowWall)
				lastWall = nowWall
				if force {
					h.Heartbeat()
				}
				changed, _ := st.Flush(force)
				// 看板快照只进内存由 SSE 推送，不落盘。两条实时通道彼此独立：
				// 用户若同时开着离线报告的实时模式，live.js 仍要继续写，
				// 否则那个已打开的页面会因读不到新帧而挂「数据未更新」提示。
				if changed || force {
					if boardMode.Load() && boardLive.Load() {
						refreshLiveSnapshot(*dir, liveSeq.Add(1))
					}
					if liveMode.Load() {
						_ = report.WriteLive(*dir, liveSeq.Add(1), time.Now())
					}
				}
				if changed && !boardMode.Load() {
					backgroundReport(*dir)
				}
			case <-hb.C:
				h.Heartbeat()
			case <-stop:
				st.Flush(true)
				return
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		h.Quit()
	}()

	fmt.Println("KeyPulse 已启动。双击托盘图标查看统计，右键可暂停 / 导出 / 退出；勾选「本地看板」才会监听 127.0.0.1 回环端口。")
	h.Run() // 阻塞直到 WM_QUIT

	closeBoard() // 显式释放端口，不等进程退出时的隐式清理
	close(stop)
	wg.Wait()
	tray.Remove()
	h.Uninstall()
	st.Close()
}
