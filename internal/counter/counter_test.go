package counter

import "testing"

// 键位脉冲位图只有聚合 bit：不记录顺序、次数、字符或时间。
// 校验位边界（0 / 31 / 32 / 63 / 64 / 255）与「取走即清空」。
func TestPulseBitmap(t *testing.T) {
	c := New()
	for _, vk := range []uint32{0, 31, 32, 63, 64, 255} {
		c.MarkPulse(vk)
	}
	// 同一键重复置位不应产生任何额外信息
	c.MarkPulse(64)
	// 越界 VK 必须被忽略，不能越界写数组
	c.MarkPulse(256)
	c.MarkPulse(9999)

	got := c.TakePulse()
	bit := func(vk uint32) bool {
		return (got[vk>>5]>>(vk&31))&1 == 1
	}
	for _, vk := range []uint32{0, 31, 32, 63, 64, 255} {
		if !bit(vk) {
			t.Errorf("VK %d 的脉冲位缺失", vk)
		}
	}
	for _, vk := range []uint32{1, 30, 33, 62, 65, 254} {
		if bit(vk) {
			t.Errorf("VK %d 不该被置位", vk)
		}
	}

	empty := c.TakePulse()
	for i, v := range empty {
		if v != 0 {
			t.Errorf("取走后第 %d 个 uint32 仍为 %d，未清空", i, v)
		}
	}
}

// 脉冲位图是纯内存状态：跨天重置必须清掉，且绝不进入 .bin 快照字段。
func TestPulseResetForNewDay(t *testing.T) {
	c := New()
	c.MarkPulse(0x41)
	c.ResetForNewDay()
	for i, v := range c.TakePulse() {
		if v != 0 {
			t.Fatalf("跨天重置后脉冲位图未清空，第 %d 个 uint32 = %d", i, v)
		}
	}
}
