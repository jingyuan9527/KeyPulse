// Package counter 保存进程内的实时计数与位图状态。
//
// 设计约束：Windows 低层钩子回调运行在消息循环线程上，必须做到
// 零内存分配、零时间函数调用、零锁、零 IO。因此回调路径只允许：
//   - 一次 atomic.AddUint32（计数槽）
//   - 一次 atomic.OrUint64（脏位图）
//
// 时间相关的处理（小时归属、分钟活跃位）全部交给每秒一次的 flush goroutine。
package counter

import (
	"math/bits"
	"sync/atomic"
)

const (
	KeySlots    = 256 // VK 0..255
	MouseSlots  = 8   // 左/右/中/X1/X2/滚轮/合计/预留
	HourSlots   = 48  // 24 小时 × (按键数, 点击数)
	SlotCount   = KeySlots + MouseSlots + HourSlots
	DirtyWords  = (SlotCount + 63) / 64 // 5
	StateWords  = KeySlots / 64         // 4
	MinuteWords = (1440 + 63) / 64      // 23
)

// 鼠标槽索引（在总槽数组中的绝对位置）
const (
	MouseLeft = KeySlots + iota
	MouseRight
	MouseMiddle
	MouseX1
	MouseX2
	MouseWheel
	MouseTotal
	MouseReserved
)

// HourBase + hour*2 是按键数，+1 是点击数
const HourBase = KeySlots + MouseSlots

type Counter struct {
	slots   [SlotCount]uint32
	dirty   [DirtyWords]uint64
	keyDown [StateWords]uint64 // 256 位：当前处于按下状态的键
	minutes [MinuteWords]uint64

	minuteDirty atomic.Bool
}

func New() *Counter { return &Counter{} }

// Add 是钩子回调唯一会调用的计数函数。
func (c *Counter) Add(slot int) {
	atomic.AddUint32(&c.slots[slot], 1)
	atomic.OrUint64(&c.dirty[slot>>6], uint64(1)<<uint(slot&63))
}

// AddN 供 flush goroutine 与启动恢复使用，不出现在回调路径。
func (c *Counter) AddN(slot int, n uint32) {
	if n == 0 {
		return
	}
	atomic.AddUint32(&c.slots[slot], n)
	atomic.OrUint64(&c.dirty[slot>>6], uint64(1)<<uint(slot&63))
}

// MarkDown 用按下位图过滤长按连发：已置位说明是 auto-repeat，返回 false。
func (c *Counter) MarkDown(vk uint32) bool {
	if vk >= KeySlots {
		return false
	}
	w, b := vk>>6, uint64(1)<<uint(vk&63)
	if atomic.LoadUint64(&c.keyDown[w])&b != 0 {
		return false
	}
	atomic.OrUint64(&c.keyDown[w], b)
	return true
}

func (c *Counter) MarkUp(vk uint32) {
	if vk >= KeySlots {
		return
	}
	w := vk >> 6
	mask := ^(uint64(1) << uint(vk&63))
	atomic.AndUint64(&c.keyDown[w], mask)
}

func (c *Counter) Slot(i int) uint32 { return atomic.LoadUint32(&c.slots[i]) }

// TakeDirty 取出并清空脏槽索引。
// 竞态说明：若在 Swap 之后、读 slots 之前又发生 Add，该槽会被重新标脏，
// 下次 flush 再写一次相同的值——写的是累计值，幂等，不会错。
func (c *Counter) TakeDirty() []int {
	out := make([]int, 0, 32)
	for w := range c.dirty {
		v := atomic.SwapUint64(&c.dirty[w], 0)
		for v != 0 {
			i := bits.TrailingZeros64(v)
			if idx := w*64 + i; idx < SlotCount {
				out = append(out, idx)
			}
			v &= v - 1
		}
	}
	return out
}

func (c *Counter) ClearDirty() {
	for w := range c.dirty {
		atomic.StoreUint64(&c.dirty[w], 0)
	}
}

func (c *Counter) SetMinute(hour, minute int) {
	idx := hour*60 + minute
	if idx < 0 || idx >= 1440 {
		return
	}
	w, b := idx>>6, uint64(1)<<uint(idx&63)
	if atomic.LoadUint64(&c.minutes[w])&b == 0 {
		atomic.OrUint64(&c.minutes[w], b)
		c.minuteDirty.Store(true)
	}
}

func (c *Counter) TakeMinuteDirty() bool { return c.minuteDirty.Swap(false) }

func (c *Counter) MinuteBitmap() [180]byte {
	var out [180]byte
	for w := range c.minutes {
		v := atomic.LoadUint64(&c.minutes[w])
		for i := 0; i < 8; i++ {
			if off := w*8 + i; off < len(out) {
				out[off] = byte(v >> uint(i*8))
			}
		}
	}
	return out
}

func (c *Counter) LoadMinuteBitmap(b [180]byte) {
	for i, v := range b {
		if v == 0 {
			continue
		}
		w, shift := i/8, uint(i%8*8)
		atomic.OrUint64(&c.minutes[w], uint64(v)<<shift)
	}
	c.minuteDirty.Store(false)
}

// ResetForNewDay 清零累计值，但保留 keyDown 位图：
// 跨零点时仍被按住的键不能算成新的一次按下。
func (c *Counter) ResetForNewDay() {
	for i := range c.slots {
		atomic.StoreUint32(&c.slots[i], 0)
	}
	for i := range c.dirty {
		atomic.StoreUint64(&c.dirty[i], 0)
	}
	for i := range c.minutes {
		atomic.StoreUint64(&c.minutes[i], 0)
	}
	c.minuteDirty.Store(false)
}
