package report

import (
	"fmt"
	"math"

	"keypulse/internal/store"
)

// Summary 是「今日速写」的数据驱动文案。所有称号、等效对照、时段人设
// 都由当天计数触发，不写死鸡汤句子。
type Summary struct {
	Title   string
	Equiv   string
	Peak    string
	Persona string
	Quips   []string
	Note    string
}

// 作品字数为常见中译本/版本的粗略量级，仅用于趣味对照，不是精确统计。
var referenceWorks = []struct {
	chars uint32
	name  string
}{
	{20000, "《小王子》"},
	{60000, "《老人与海》"},
	{250000, "《围城》"},
	{800000, "《红楼梦》"},
}

func equivalence(keys uint32) string {
	if keys < 300 {
		return "≈ 相当于敲了一条长朋友圈"
	}
	if keys < 2000 {
		return fmt.Sprintf("≈ 一篇短文的量，%s 的 %.0f%%", referenceWorks[0].name,
			float64(keys)*100/float64(referenceWorks[0].chars))
	}
	best := referenceWorks[0]
	bestDiff := math.MaxFloat64
	for _, w := range referenceWorks {
		d := math.Abs(math.Log(float64(keys) / float64(w.chars)))
		if d < bestDiff {
			bestDiff, best = d, w
		}
	}
	ratio := float64(keys) / float64(best.chars)
	if ratio >= 1 {
		if ratio < 1.15 {
			return fmt.Sprintf("≈ 差不多敲完了一部%s", best.name)
		}
		return fmt.Sprintf("≈ 够写 %.1f 部%s", ratio, best.name)
	}
	return fmt.Sprintf("≈ 敲出了%s的 %.0f%%", best.name, ratio*100)
}

// peakWindow 用 3 小时环形滑动窗口找输入最密集的时段。
func peakWindow(d store.DayData) (start, end int, sum, total uint32) {
	for h := 0; h < 24; h++ {
		total += d.Hours[h*2] + d.Hours[h*2+1]
	}
	if total == 0 {
		return 0, 3, 0, 0
	}
	var best uint32
	for s := 0; s < 24; s++ {
		var n uint32
		for k := 0; k < 3; k++ {
			i := (s + k) % 24
			n += d.Hours[i*2] + d.Hours[i*2+1]
		}
		if n > best {
			best, start = n, s
		}
	}
	return start, (start + 3) % 24, best, total
}

func personaOf(start int, keys uint32, activeMinutes int, pct uint32) (string, string) {
	var slot string
	switch {
	case start >= 6 && start < 10:
		slot = "早鸟"
	case start >= 10 && start < 13:
		slot = "午前冲刺"
	case start >= 13 && start < 18:
		slot = "下午档"
	case start >= 18 && start < 23:
		slot = "夜猫子"
	default:
		slot = "凌晨档"
	}

	var level string
	switch {
	case keys >= 12000 && activeMinutes >= 420:
		level = "敬业"
	case keys >= 12000 || activeMinutes >= 480:
		level = "硬核"
	case keys >= 6000 || activeMinutes >= 240:
		level = "活跃"
	default:
		level = "轻量"
	}

	var desc string
	switch {
	case pct >= 55:
		desc = "注意力高度集中，三小时里干掉了大半输出"
	case pct >= 40:
		desc = "主要产出集中在这一段，节奏清楚"
	case pct >= 25:
		desc = "全天分布比较均匀，没有明显的空档"
	default:
		desc = "输出铺得很散，没有明显的高峰"
	}
	return level + "的" + slot, desc
}

func dominantKey(d store.DayData) (uint32, uint32) {
	var vk, n uint32
	for i, v := range d.Keys {
		if v > n {
			vk, n = uint32(i), v
		}
	}
	return vk, n
}

func buildQuips(d store.DayData, prev *store.DayData, keys, clicks uint32, prevLabel string) []string {
	if keys == 0 {
		keys = 1
	}
	var q []string

	backspace := d.Keys[0x08]
	ctrl := d.Keys[0x11] + d.Keys[0xA2] + d.Keys[0xA3]
	space := d.Keys[0x20]
	enter := d.Keys[0x0D]
	switch {
	case backspace*100/keys >= 7:
		q = append(q, fmt.Sprintf("Backspace 占 %.1f%%，边敲边改的斟酌派。", float64(backspace)*100/float64(keys)))
	case ctrl*100/keys >= 6:
		q = append(q, fmt.Sprintf("Ctrl 按了 %s 次，快捷键流。", comma(ctrl)))
	case space*100/keys >= 12:
		q = append(q, fmt.Sprintf("空格占 %.1f%%，长文或代码量扎实。", float64(space)*100/float64(keys)))
	case enter*100/keys >= 5:
		q = append(q, fmt.Sprintf("回车 %s 次，一行一个想法。", comma(enter)))
	default:
		vk, n := dominantKey(d)
		q = append(q, fmt.Sprintf("最常用的是 %s，占全天 %.1f%%。", KeyName(vk), float64(n)*100/float64(keys)))
	}

	if clicks > 0 {
		r := float64(keys) / float64(clicks)
		switch {
		case r >= 4:
			q = append(q, fmt.Sprintf("键鼠比 %.1f : 1，纯键盘派。", r))
		case r <= 1.2:
			q = append(q, fmt.Sprintf("键鼠比 %.1f : 1，今天偏图形界面操作。", r))
		default:
			q = append(q, fmt.Sprintf("键鼠比 %.1f : 1，两手都没闲着。", r))
		}
	}

	if prev != nil {
		if pk := prev.TotalKeys(); pk > 0 {
			delta := (float64(keys) - float64(pk)) / float64(pk) * 100
			if delta >= 0 {
				q = append(q, fmt.Sprintf("%s多敲了 %.0f%%。", prevLabel, delta))
			} else {
				q = append(q, fmt.Sprintf("%s少敲了 %.0f%%，节奏放缓。", prevLabel, -delta))
			}
		}
	}

	var night uint32
	for h := 23; h < 24; h++ {
		night += d.Hours[h*2]
	}
	for h := 0; h < 5; h++ {
		night += d.Hours[h*2]
	}
	if night*100/keys >= 18 {
		q = append(q, fmt.Sprintf("有 %.0f%% 的输入发生在 23:00–05:00，别熬太狠。", float64(night)*100/float64(keys)))
	}
	if d.Wheel() >= 2500 {
		q = append(q, fmt.Sprintf("滚轮滚了 %s 格，今天读的比写的多。", comma(d.Wheel())))
	}
	return q
}

// Build 生成今日速写。prev 为昨天数据，可为 nil。
func Build(today store.DayData, prev *store.DayData) Summary {
	keys := today.TotalKeys()
	clicks := today.TotalClicks()
	start, end, sum, total := peakWindow(today)
	var pct uint32
	if total > 0 {
		pct = sum * 100 / total
	}
	title, desc := personaOf(start, keys, today.ActiveMinutes(), pct)

	// 历史回看时 prev 可能不是自然昨天（中间有空白日），文案不能一律说"昨天"。
	prevLabel := "比昨天"
	if prev != nil {
		if t, err := parseStamp(today.Stamp); err == nil {
			if t.AddDate(0, 0, -1).Format("20060102") != prev.Stamp {
				prevLabel = "比上一个记录日（" + prev.Date() + "）"
			}
		}
	}

	return Summary{
		Title:   title,
		Equiv:   fmt.Sprintf("%s 次按键，%s", comma(keys), equivalence(keys)),
		Peak:    fmt.Sprintf("最密集时段 %02d:00–%02d:00，占全天 %d%%", start, end, pct),
		Persona: desc,
		Quips:   buildQuips(today, prev, keys, clicks, prevLabel),
		Note:    "趣味换算：按 1 次按键 ≈ 1 个字符估算，含删除、快捷键与中文输入过程，与实际产出字数无关；作品字数为常见版本粗略量级。",
	}
}
