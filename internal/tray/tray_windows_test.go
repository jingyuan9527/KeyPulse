//go:build windows

package tray

import (
	"syscall"
	"testing"
	"unsafe"
)

// 这些测试只验证 Win32 过程解析、结构体 ABI 布局和图标绘制，
// 不创建托盘、不挂任何钩子、不读取用户真实输入。

func TestWin32ProcsResolvable(t *testing.T) {
	procs := []struct {
		name string
		p    *syscall.LazyProc
	}{
		{"GetDC", getDC},
		{"ReleaseDC", releaseDC},
		{"DestroyIcon", destroyIcon},
		{"CreateDIBSection", createDIBSection},
		{"CreateBitmap", createBitmap},
		{"CreateIconIndirect", createIconIndirect},
		{"GetSystemMetrics", getSystemMetrics},
		{"DeleteObject", deleteObject},
		{"Shell_NotifyIconW", shellNotifyIcon},
		{"RegisterClassExW", registerClassEx},
		{"CreateWindowExW", createWindowEx},
		{"LoadIconW", loadIcon},
		{"TrackPopupMenu", trackPopupMenu},
		{"ShellExecuteW", shellExecute},
	}
	for _, x := range procs {
		if err := x.p.Find(); err != nil {
			t.Errorf("%s 解析失败: %v", x.name, err)
		}
	}
}

func TestWin32StructLayout(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"NOTIFYICONDATAW", unsafe.Sizeof(notifyIconDataW{}), 976},
		{"WNDCLASSEXW", unsafe.Sizeof(wndClassExW{}), 80},
		{"ICONINFO", unsafe.Sizeof(iconInfo{}), 32},
		{"BITMAPINFOHEADER", unsafe.Sizeof(bitmapInfoHeader{}), 40},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s 大小 = %d，Win32 x64 期望 %d", c.name, c.got, c.want)
		}
	}
	// NOTIFYICONDATAW 的关键字段偏移必须与 C ABI 一致
	var n notifyIconDataW
	base := uintptr(unsafe.Pointer(&n))
	if off := uintptr(unsafe.Pointer(&n.hWnd)) - base; off != 8 {
		t.Errorf("hWnd 偏移 = %d, 期望 8", off)
	}
	if off := uintptr(unsafe.Pointer(&n.hIcon)) - base; off != 32 {
		t.Errorf("hIcon 偏移 = %d, 期望 32", off)
	}
	if off := uintptr(unsafe.Pointer(&n.szTip)) - base; off != 40 {
		t.Errorf("szTip 偏移 = %d, 期望 40", off)
	}
}

func TestMakeIconBothStates(t *testing.T) {
	// 托盘尺寸随 DPI 变化，逐个尺寸实机走一遍 GDI 路径，
	// 能抓到 DIB / mask 缓冲按 32 硬编码时在小尺寸下的越界。
	for _, size := range []int{16, 20, 24, 32, 48} {
		for _, paused := range []bool{false, true} {
			h := makeIcon(paused, size)
			if h == 0 {
				t.Fatalf("makeIcon(size=%d, paused=%v) 返回空句柄", size, paused)
			}
			destroyIcon.Call(h)
		}
	}
}

func TestDrawIconAllSizes(t *testing.T) {
	for _, size := range []int{16, 20, 24, 32, 48} {
		a, p := drawIcon(false, size), drawIcon(true, size)
		if len(a.px) != size*size || len(p.px) != size*size {
			t.Errorf("size=%d 像素缓冲大小错误: %d / %d", size, len(a.px), len(p.px))
			continue
		}
		if a.px[0] != 0 || p.px[0] != 0 {
			t.Errorf("size=%d 圆角外未保持透明", size)
		}
		if center := a.px[(size/2)*size+size/2]; center>>24 == 0 {
			t.Errorf("size=%d 中心像素未填充底色", size)
		}
		diff := 0
		for i := range a.px {
			if a.px[i] != p.px[i] {
				diff++
			}
		}
		if diff == 0 {
			t.Errorf("size=%d 两态像素完全相同，状态色未生效", size)
		}
	}
}

func TestIconSizeWithinBounds(t *testing.T) {
	if s := iconSize(); s < iconMin || s > iconMax {
		t.Errorf("iconSize()=%d 超出 [%d,%d]", s, iconMin, iconMax)
	}
}

func TestIconStatesDiffer(t *testing.T) {
	active := drawIcon(false, 32)
	paused := drawIcon(true, 32)
	if len(active.px) != 32*32 || len(paused.px) != 32*32 {
		t.Fatal("图标像素缓冲大小错误")
	}
	diff := 0
	for i := range active.px {
		if active.px[i] != paused.px[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("采集中与暂停两态像素完全相同，状态色未生效")
	}
	// 圆角外必须透明，圆角内必须不透明
	if active.px[0] != 0 {
		t.Error("左上角应透明")
	}
	if active.px[16*32+16]>>24 == 0 {
		t.Error("中心像素应不透明")
	}
}

// NewCallback 对参数大小有严格要求：签名不合法时会直接 panic。
// wndProc 的 msg 是 uintptr（避免 x64 回调的取值歧义）。
func TestWndProcCallbackABI(t *testing.T) {
	if cb := syscall.NewCallback(wndProc); cb == 0 {
		t.Fatal("wndProc 回调编译返回空指针")
	}
}

// 气泡文案来自程序自身，但仍必须保证超长文本不越界且以 \0 收尾，
// 否则 Shell_NotifyIconW 会读到缓冲区外的内存。
func TestSetUTF16TruncatesSafely(t *testing.T) {
	for _, s := range []string{"采集已恢复", "这是一段非常长的中文提示文案用来验证缓冲区截断是否安全不会读越界"} {
		dst := make([]uint16, 8)
		setUTF16(dst, s)
		if dst[0] == 0 {
			t.Errorf("内容为空: %q", s)
		}
		if dst[len(dst)-1] != 0 {
			t.Errorf("缓冲区末尾未以 0 结尾: %q", s)
		}
	}
}

func TestNotifyFlags(t *testing.T) {
	if nifInfo != 0x10 || niifInfo != 0x1 || niifNosound != 0x10 {
		t.Fatal("气泡通知的标志常量错误")
	}
}
