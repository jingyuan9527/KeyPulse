//go:build windows

package tray

import (
	"syscall"
	"unsafe"
)

// 程序化绘制托盘图标：32bpp DIB + CreateIconIndirect。
// 不依赖 .ico 文件、不依赖资源编译器，仍是单文件 exe；两态直接换配色。
//
// 设计：
//   - 圆角方形底：采集中深蓝，暂停中性灰
//   - 左上键帽 → 键盘；下方 3 根递增短柱 → 统计；右上状态圆点
//   - 尺寸取 SM_CXSMICON（100% DPI 通常为 16，高 DPI 可能 20/24/48），
//     按系统实际尺寸绘制而不是让 shell 把 32×32 硬缩下来。
//     小尺寸下键帽由 3 列减为 2 列，给状态点留出独立位置。
var (
	gdi32              = syscall.NewLazyDLL("gdi32.dll")
	getDC              = user32.NewProc("GetDC")
	releaseDC          = user32.NewProc("ReleaseDC")
	destroyIcon        = user32.NewProc("DestroyIcon")
	getSystemMetrics   = user32.NewProc("GetSystemMetrics")
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	createBitmap       = gdi32.NewProc("CreateBitmap")
	createIconIndirect = user32.NewProc("CreateIconIndirect") // 属于 user32，不是 gdi32
	deleteObject       = gdi32.NewProc("DeleteObject")

	currentIcon uintptr
)

const (
	smCxSmIcon = 49
	iconMin    = 16
	iconMax    = 48
)

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type iconInfo struct {
	fIcon    uint32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

// bgra 返回可直接写入 32bpp DIB 的像素值（小端内存布局 B,G,R,A）。
func bgra(r, g, b, a uint8) uint32 {
	return uint32(b) | uint32(g)<<8 | uint32(r)<<16 | uint32(a)<<24
}

// iconSize 取托盘小图标的系统尺寸，异常值统一退回 16。
func iconSize() int {
	n, _, _ := getSystemMetrics.Call(smCxSmIcon)
	s := int(n)
	if s < iconMin || s > iconMax {
		return iconMin
	}
	return s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type canvas struct {
	size int
	px   []uint32
}

func (c *canvas) set(x, y int, col uint32) {
	if x >= 0 && y >= 0 && x < c.size && y < c.size {
		c.px[y*c.size+x] = col
	}
}

func (c *canvas) rect(x0, y0, x1, y1 int, col uint32) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c.set(x, y, col)
		}
	}
}

func (c *canvas) roundRect(x0, y0, x1, y1, r int, col uint32) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			dx, dy := 0, 0
			if x < x0+r {
				dx = x0 + r - x
			} else if x > x1-1-r {
				dx = x - (x1 - 1 - r)
			}
			if y < y0+r {
				dy = y0 + r - y
			} else if y > y1-1-r {
				dy = y - (y1 - 1 - r)
			}
			if dx*dx+dy*dy <= r*r {
				c.set(x, y, col)
			}
		}
	}
}

func (c *canvas) circle(cx, cy, r int, col uint32) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				c.set(x, y, col)
			}
		}
	}
}

// drawIcon 用 32×32 基准坐标按比例映射到目标尺寸。size=32 时 S(v)==v，
// 像素与初版完全一致；小尺寸用最小像素宽度防止元素粘成一片。
func drawIcon(paused bool, size int) *canvas {
	if size < iconMin {
		size = iconMin
	}
	if size > iconMax {
		size = iconMax
	}
	S := func(v int) int { return (v*size + 16) / 32 }

	c := &canvas{size: size, px: make([]uint32, size*size)}
	for i := range c.px {
		c.px[i] = bgra(0, 0, 0, 0) // 圆角外透明
	}

	bg := bgra(24, 95, 165, 255) // #185FA5
	cap := bgra(255, 255, 255, 255)
	bar := bgra(181, 212, 244, 255)
	dot := bgra(29, 158, 117, 255) // #1D9E75
	if paused {
		bg = bgra(95, 94, 90, 255)     // #5F5E5A
		cap = bgra(211, 209, 199, 255) // #D3D1C7
		bar = bgra(239, 159, 39, 255)  // #EF9F27
		dot = bgra(239, 159, 39, 255)
	}

	c.roundRect(S(1), S(1), S(31), S(31), maxInt(2, S(7)), bg)

	// 键帽：>=24 保持 3×2；更小尺寸减为 2×2，避免与右上状态点抢位置。
	capsX := []int{S(5), S(12), S(19)}
	capW := maxInt(2, S(5))
	if size < 24 {
		capsX = capsX[:2]
		capW = 2
	}
	for _, y := range []int{S(6), S(13)} {
		for _, x := range capsX {
			c.rect(x, y, x+capW, y+capW, cap)
		}
	}

	// 3 根递增统计柱
	barW := maxInt(2, S(4))
	bottom := S(27)
	for i, x := range []int{S(7), S(14), S(21)} {
		h := []int{S(3), S(6), S(8)}[i]
		c.rect(x, bottom-h, x+barW, bottom, bar)
	}
	// 状态圆点
	c.circle(S(27), S(6), maxInt(2, S(3)), dot)
	return c
}

// makeIcon 返回 0 表示 GDI 路径不可用，调用方会退回系统默认图标。
// 任何 Win32 过程解析失败（LazyProc.Call 会 panic）都在此兜住，
// 绝不让图标绘制把整个托盘 / 进程带崩。
func makeIcon(paused bool, size int) (icon uintptr) {
	if size < iconMin || size > iconMax {
		size = iconMin
	}
	var hColor, hMask uintptr
	defer func() {
		// 无论正常返回还是某个 Win32 过程解析失败，都释放已创建的 GDI 对象，
		// 避免暂停/恢复反复切图时累积句柄泄漏。
		if recover() != nil {
			icon = 0
		}
		if hColor != 0 {
			deleteObject.Call(hColor)
		}
		if hMask != 0 {
			deleteObject.Call(hMask)
		}
	}()

	hdc, _, _ := getDC.Call(0)
	if hdc == 0 {
		return 0
	}
	defer releaseDC.Call(0, hdc)

	bih := bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		biWidth:       int32(size),
		biHeight:      -int32(size), // 负值 = top-down
		biPlanes:      1,
		biBitCount:    32,
		biCompression: 0, // BI_RGB
	}
	var bits uintptr
	hColor, _, _ = createDIBSection.Call(
		hdc, uintptr(unsafe.Pointer(&bih)), 0,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hColor == 0 || bits == 0 {
		return 0
	}

	src := drawIcon(paused, size)
	dst := unsafe.Slice((*uint32)(unsafe.Pointer(bits)), size*size)
	copy(dst, src.px)

	// 32bpp 带 alpha 的图标 mask 内容被忽略，但 ICONINFO 仍要求有效 HBITMAP。
	// 单色位图每行按 32 位对齐；传全 0 缓冲而非 NULL，避免未初始化掩码位。
	rowBytes := ((size + 31) / 32) * 4
	maskBits := make([]byte, rowBytes*size)
	hMask, _, _ = createBitmap.Call(uintptr(size), uintptr(size), 1, 1,
		uintptr(unsafe.Pointer(&maskBits[0])))
	if hMask == 0 {
		return 0
	}

	ii := iconInfo{fIcon: 1, hbmMask: hMask, hbmColor: hColor}
	hIcon, _, _ := createIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	return hIcon
}

// updateIcon 重建并切换托盘图标，返回是否成功（失败时调用方仍需刷新 tip）。
func updateIcon(paused bool) bool {
	h := makeIcon(paused, iconSize())
	if h == 0 {
		return false
	}
	old := currentIcon
	currentIcon = h
	nid.hIcon = h
	shellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	if old != 0 {
		destroyIcon.Call(old)
	}
	return true
}
