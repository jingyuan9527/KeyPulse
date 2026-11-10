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

// 离线报告的两个动作收进一个子菜单，顶层不再并排出现两个「实时」勾选
// 项；命令 ID 全部保留，TrackPopupMenu 仍会返回子菜单里被点中的那个 ID。
func TestMenuItemsGroupReportActionsInSubmenu(t *testing.T) {
	base := menuItems(false, false, false)
	if got := len(base); got != 6 {
		t.Fatalf("顶层菜单项数 = %d，期望 6", got)
	}
	var group *menuEntry
	for i := range base {
		if len(base[i].children) > 0 {
			group = &base[i]
			break
		}
	}
	if group == nil {
		t.Fatal("缺少「统计报告」子菜单")
	}
	if group.flags&mfPopup == 0 || group.text != "统计报告" {
		t.Errorf("子菜单容器标志或文案错误: flags=%#x text=%q", group.flags, group.text)
	}
	if mfPopup != 0x10 {
		t.Errorf("MF_POPUP 常量错误: %#x", mfPopup)
	}
	// 两个报告动作必须在子菜单里，顶层不得再平铺。
	for _, id := range []uintptr{idOpen, idLive} {
		if hasMenuID(base, id) {
			t.Errorf("顶层不应直接出现 id=%d", id)
		}
		if !hasMenuID(group.children, id) {
			t.Errorf("子菜单缺少 id=%d", id)
		}
	}
	if got := menuText(group.children, idOpen); got != "打开（离线快照）" {
		t.Errorf("打开报告菜单项未与实时看板区分: %q", got)
	}
	if got := menuText(group.children, idLive); got != "实时刷新" {
		t.Errorf("实时菜单项未收敛为「实时刷新」: %q", got)
	}
	// 看板开启时子菜单里也不再摆重复的实时开关，但打开报告仍可达。
	withBoard := menuItems(false, false, true)
	if hasMenuID(flatten(withBoard), idLive) {
		t.Error("看板开启时不应再显示「实时刷新」")
	}
	if !hasMenuID(flatten(withBoard), idOpen) {
		t.Error("看板开启时仍应能打开离线报告")
	}
	for _, id := range []uintptr{idBoard, idPause, idExport, idQuit} {
		if !hasMenuID(withBoard, id) {
			t.Errorf("顶层菜单缺少 id=%d", id)
		}
	}
	if !menuChecked(withBoard, idBoard) {
		t.Error("看板开启时「本地看板」未显示勾选态")
	}
	if !menuChecked(flatten(menuItems(false, true, false)), idLive) {
		t.Error("静态实时模式开启时子菜单项未显示勾选态")
	}
	// 暂停态文案与 mfChecked 组合不随层级改动退化。
	paused := menuItems(true, false, false)
	if got := menuText(paused, idPause); got != "恢复采集" {
		t.Errorf("暂停态菜单文案错误: %q", got)
	}
}

// MF_POPUP 的 ABI 陷阱：AppendMenuW 第三参必须传子菜单句柄而非命令 ID。
// 这里建真实 Win32 菜单再回读，避免纯函数测试漏掉句柄传错。
func TestBuildMenuSubmenuABI(t *testing.T) {
	menu := buildMenu(menuItems(false, false, false))
	if menu == 0 {
		t.Fatal("创建 popup 菜单失败")
	}
	defer destroyMenu.Call(menu)

	sub, _, _ := getSubMenu.Call(menu, 0)
	if sub == 0 {
		t.Fatal("顶层第 0 项不是子菜单（MF_POPUP 未生效）")
	}
	if count, _, _ := getMenuItemCount.Call(sub); count != 2 {
		t.Fatalf("子菜单项数 = %d，期望 2", count)
	}
	for i, want := range []uintptr{idOpen, idLive} {
		got, _, _ := getMenuItemID.Call(sub, uintptr(i))
		if got != want {
			t.Errorf("子菜单第 %d 项 ID = %d，期望 %d", i, got, want)
		}
	}
	for i, want := range map[int]uintptr{1: idPause, 2: idBoard, 3: idExport, 5: idQuit} {
		got, _, _ := getMenuItemID.Call(menu, uintptr(i))
		if got != want {
			t.Errorf("顶层第 %d 项 ID = %d，期望 %d", i, got, want)
		}
	}

	// 看板开启时子菜单只剩「打开（离线快照）」，实时项确实被摘掉。
	menu2 := buildMenu(menuItems(false, false, true))
	if menu2 == 0 {
		t.Fatal("创建第二份 popup 菜单失败")
	}
	defer destroyMenu.Call(menu2)
	sub2, _, _ := getSubMenu.Call(menu2, 0)
	if sub2 == 0 {
		t.Fatal("看板开启时顶层第 0 项不是子菜单")
	}
	if count, _, _ := getMenuItemCount.Call(sub2); count != 1 {
		t.Fatalf("看板开启时子菜单项数 = %d，期望 1", count)
	}
	if got, _, _ := getMenuItemID.Call(sub2, 0); got != idOpen {
		t.Errorf("看板开启时子菜单首项 ID = %d，期望 %d", got, idOpen)
	}
}

// flatten 把顶层与子菜单展开成一层，供按命令 ID 查找断言使用。
func flatten(items []menuEntry) []menuEntry {
	var out []menuEntry
	for _, it := range items {
		out = append(out, it)
		if len(it.children) > 0 {
			out = append(out, flatten(it.children)...)
		}
	}
	return out
}

func hasMenuID(items []menuEntry, id uintptr) bool {
	for _, it := range items {
		if it.id == id {
			return true
		}
	}
	return false
}

func menuChecked(items []menuEntry, id uintptr) bool {
	for _, it := range items {
		if it.id == id {
			return it.flags&mfChecked != 0
		}
	}
	return false
}

func menuText(items []menuEntry, id uintptr) string {
	for _, it := range items {
		if it.id == id {
			return it.text
		}
	}
	return ""
}
