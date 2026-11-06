package main

import "testing"

// 时间跳跃判定必须只对睡眠/休眠/时间调整这类真实跳变触发，
// 正常 ticker 抖动与跨零点的 1 秒推进都不能误判。
func TestTimeJump(t *testing.T) {
	const base = int64(1_700_000_000)
	cases := []struct {
		name       string
		prev, now  int64
		want       bool
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
