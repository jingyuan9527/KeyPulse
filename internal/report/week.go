package report

import (
	"fmt"
	"math"
	"time"
)

// 自然周：周一为起点（ISO 8601）。缺失日期按 0 计入，日均固定除以 7，
// 不因缺数据虚高；同时单独给出「有记录天数」以免误读。

const maxWeeks = 60 // 页面最多承载约 14 个月的周报，避免极端历史把 HTML 撑大

func buildWeeks(days []dayPayload, lookup map[string]dayPayload) []weekPayload {
	if len(days) == 0 {
		return nil
	}
	first, err1 := parseStamp(days[0].Stamp)
	last, err2 := parseStamp(days[len(days)-1].Stamp)
	if err1 != nil || err2 != nil {
		return nil
	}
	start := weekStart(first)
	end := weekStart(last)

	// 从最新一周倒推，最多保留 maxWeeks 周。
	var starts []time.Time
	for t := end; !t.Before(start); t = t.AddDate(0, 0, -7) {
		starts = append(starts, t)
		if len(starts) >= maxWeeks {
			break
		}
	}
	for i, j := 0, len(starts)-1; i < j; i, j = i+1, j-1 {
		starts[i], starts[j] = starts[j], starts[i]
	}

	weeks := make([]weekPayload, 0, len(starts))
	for _, ws := range starts {
		weeks = append(weeks, buildOneWeek(ws, lookup, last))
	}
	for i := range weeks {
		var prev *weekPayload
		if i > 0 {
			prev = &weeks[i-1]
		}
		weeks[i].Cmp = buildWeekCompare(weeks[i], prev)
	}
	return weeks
}

func buildOneWeek(ws time.Time, lookup map[string]dayPayload, lastDay time.Time) weekPayload {
	y, isoWeek := ws.ISOWeek()
	we := ws.AddDate(0, 0, 6)
	w := weekPayload{
		Start: ws.Format("20060102"),
		End:   we.Format("20060102"),
		Label: fmt.Sprintf("%d 第 %d 周 · %s–%s", y, isoWeek, shortDate(ws), shortDate(we)),
		Quips: []string{},
	}
	// 含最新数据的一周若还没到周日，只统计已过去的天数。
	w.Elapsed = 7
	if we.After(lastDay) {
		w.Partial = true
		w.Elapsed = weekdayIndex(lastDay)
		if w.Elapsed < 1 {
			w.Elapsed = 1
		}
	}
	if w.Elapsed == 1 {
		w.ElapsedLabel = weekdayLabel[0]
	} else {
		w.ElapsedLabel = weekdayLabel[0] + "–" + weekdayLabel[w.Elapsed-1]
	}
	keySum := make(map[uint32]uint32, 64)
	peak, quiet := uint32(0), uint32(math.MaxUint32)
	var weekend, streak, run uint32

	for i := 0; i < 7; i++ {
		t := ws.AddDate(0, 0, i)
		stamp := t.Format("20060102")
		b := dayBrief{
			Stamp: stamp,
			Date:  t.Format("2006-01-02"),
			Label: weekdayLabel[i],
		}
		if d, ok := lookup[stamp]; ok {
			b.Has = true
			b.Keys, b.Clicks, b.Active = d.Keys, d.Clicks, d.Active
			w.Keys += d.Keys
			w.Clicks += d.Clicks
			w.Wheel += d.Wheel
			w.Active += d.Active
			for h := 0; h < 24; h++ {
				w.HourK[h] += d.HourK[h]
				w.HourC[h] += d.HourC[h]
			}
			for _, kv := range d.KeyUse {
				keySum[kv[0]] += kv[1]
			}
			if d.Keys+d.Clicks > 0 {
				w.RecordDays++
				run++
				if run > streak {
					streak = run
				}
			} else {
				run = 0
			}
			if d.Keys > peak {
				peak, w.PeakDate = d.Keys, d.Date
			}
			if d.Keys < quiet {
				quiet, w.QuietDate = d.Keys, d.Date
			}
			if i >= 5 {
				weekend += d.Keys
			}
		} else {
			run = 0
		}
		w.Days[i] = b
	}
	w.Streak = int(streak)
	w.DailyAvg = w.Keys / 7
	w.Top = topFrom(keySum, 10)
	if w.Keys > 0 {
		w.WeekendPct = int(uint64(weekend) * 100 / uint64(w.Keys))
	}
	if quiet == math.MaxUint32 {
		w.QuietDate = ""
	}
	w.Title = weekTitle(w)
	w.Quips = weekQuips(w)
	return w
}

func weekTitle(w weekPayload) string {
	switch {
	case w.RecordDays == 0:
		return "空白的一周"
	case w.RecordDays <= 2:
		return "安静的一周"
	case w.Keys > 0 && w.PeakDate != w.QuietDate && maxDayKeys(w) >= 3*minDayKeys(w) && minDayKeys(w) > 0:
		return "起伏很大的一周"
	case w.RecordDays >= 6 && w.Keys >= 35000:
		return "满负荷的一周"
	case w.RecordDays >= 5:
		return "稳定输出的一周"
	default:
		return "零散的一周"
	}
}

func maxDayKeys(w weekPayload) uint32 {
	var n uint32
	for _, d := range w.Days {
		if d.Has && d.Keys > n {
			n = d.Keys
		}
	}
	return n
}

func minDayKeys(w weekPayload) uint32 {
	var n uint32 = math.MaxUint32
	for _, d := range w.Days {
		if d.Has && d.Keys < n {
			n = d.Keys
		}
	}
	if n == math.MaxUint32 {
		return 0
	}
	return n
}

// weekPeakWindow 对 7 天叠加的小时分布取 3 小时环形滑窗。
func weekPeakWindow(w weekPayload) (int, int, uint32) {
	var best, total uint32
	start := 0
	for h := 0; h < 24; h++ {
		total += w.HourK[h] + w.HourC[h]
	}
	if total == 0 {
		return 9, 12, 0
	}
	for s := 0; s < 24; s++ {
		var n uint32
		for k := 0; k < 3; k++ {
			i := (s + k) % 24
			n += w.HourK[i] + w.HourC[i]
		}
		if n > best {
			best, start = n, s
		}
	}
	pct := uint32(0)
	if total > 0 {
		pct = best * 100 / total
	}
	return start, (start + 3) % 24, pct
}

func weekQuips(w weekPayload) []string {
	q := []string{}
	if w.RecordDays == 0 {
		return []string{"这一周没有任何输入记录，可能是暂停了采集或没开机。"}
	}
	if w.Partial {
		q = append(q, fmt.Sprintf("本周进行中，目前只统计到%s（%d/7 天）。", w.ElapsedLabel, w.Elapsed))
	}
	q = append(q, fmt.Sprintf("7 天里有 %d 天有输入记录，最长连续 %d 天。", w.RecordDays, w.Streak))
	if w.PeakDate != "" {
		q = append(q, fmt.Sprintf("最忙是 %s（%s 次按键）", w.PeakDate, comma(maxDayKeys(w))))
		if w.QuietDate != "" && w.QuietDate != w.PeakDate {
			q = append(q, fmt.Sprintf("最闲是 %s（%s 次）", w.QuietDate, comma(minDayKeys(w))))
		}
	}
	q = append(q, fmt.Sprintf("日均 %s 次按键，活跃合计 %.1f 小时。", comma(w.DailyAvg), float64(w.Active)/60))
	if w.WeekendPct >= 30 {
		q = append(q, fmt.Sprintf("周末贡献了 %d%% 的按键，休息日也没闲着。", w.WeekendPct))
	} else if w.WeekendPct <= 8 && w.RecordDays >= 4 {
		q = append(q, "输入几乎全在工作日，周末基本离线。")
	}
	s, e, pct := weekPeakWindow(w)
	q = append(q, fmt.Sprintf("一周里最密集的是 %02d:00–%02d:00，占全周输入 %d%%。", s, e, pct))
	if len(w.Top) > 0 {
		q = append(q, fmt.Sprintf("本周最常用的键是 %s（%s 次）。", KeyName(w.Top[0][0]), comma(w.Top[0][1])))
	}
	return q
}

func buildWeekCompare(cur weekPayload, prev *weekPayload) *weekCompare {
	c := &weekCompare{
		Keys:     metric{Cur: cur.Keys},
		Clicks:   metric{Cur: cur.Clicks},
		Wheel:    metric{Cur: cur.Wheel},
		Active:   metric{Cur: uint32(cur.Active)},
		Daily:    metric{Cur: cur.DailyAvg},
		SameDays: cur.Elapsed,
		Partial:  cur.Partial,
	}
	if prev == nil {
		c.Label = "无上一周数据"
		c.Verdict = "上一周没有数据，暂时无法做周对比。累计两周记录后，这里会自动给出环比结论。"
		return c
	}
	c.Has = true
	c.Label = prev.Label
	c.Keys.Prev, c.Keys.Pct = prev.Keys, pctDelta(cur.Keys, prev.Keys)
	c.Clicks.Prev, c.Clicks.Pct = prev.Clicks, pctDelta(cur.Clicks, prev.Clicks)
	c.Wheel.Prev, c.Wheel.Pct = prev.Wheel, pctDelta(cur.Wheel, prev.Wheel)
	c.Active.Prev, c.Active.Pct = uint32(prev.Active), pctDelta(uint32(cur.Active), uint32(prev.Active))
	c.Daily.Prev, c.Daily.Pct = prev.DailyAvg, pctDelta(cur.DailyAvg, prev.DailyAvg)
	for i, d := range prev.Days {
		c.PrevDays[i] = d.Keys
	}

	// 同期口径：只取双方都已过去的前 N 天，剔除尚未到来的日期。
	n := cur.Elapsed
	var sameCur, samePrev uint32
	for i := 0; i < n; i++ {
		sameCur += cur.Days[i].Keys
		samePrev += prev.Days[i].Keys
	}
	c.SameKeys = metric{Cur: sameCur, Prev: samePrev, Pct: pctDelta(sameCur, samePrev)}
	if cur.Partial {
		c.Note = fmt.Sprintf("本周尚未结束（已过 %d/7 天，%s）。总量对比会系统性偏低，上方结论已改用同期口径；下表仍保留 7 天总量供参考。",
			n, cur.ElapsedLabel)
	}

	// 变化贡献最大的一天也只在同期范围内找，避免把"还没到的周六/周日"当成回落。
	bestIdx, bestDelta := -1, int64(0)
	for i := 0; i < n; i++ {
		d := int64(cur.Days[i].Keys) - int64(prev.Days[i].Keys)
		if absInt64(d) > absInt64(bestDelta) {
			bestIdx, bestDelta = i, d
		}
	}
	day := "各天变化比较均匀"
	if bestIdx >= 0 && bestDelta != 0 {
		word := "多"
		if bestDelta < 0 {
			word = "少"
		}
		day = weekdayLabel[bestIdx] + "（" + word + " " + comma(uint32(absInt64(bestDelta))) + " 次"
		if !cur.Days[bestIdx].Has {
			day += "，当天无记录"
		}
		day += "）"
	}

	if cur.Partial {
		p := c.SameKeys.Pct
		switch {
		case samePrev == 0:
			c.Verdict = "同期（" + cur.ElapsedLabel + "）上一周没有输入记录，环比无法计算。"
		case p >= 5:
			c.Verdict = fmt.Sprintf("同期（%s）比上周多敲 %d%%，%s → %s 次；增量主要来自%s。",
				cur.ElapsedLabel, p, comma(samePrev), comma(sameCur), day)
		case p <= -5:
			c.Verdict = fmt.Sprintf("同期（%s）比上周少敲 %d%%，%s → %s 次；主要回落在%s。",
				cur.ElapsedLabel, -p, comma(samePrev), comma(sameCur), day)
		default:
			c.Verdict = fmt.Sprintf("同期（%s）与上周基本持平（%s → %s 次，%d%%），节奏稳定。",
				cur.ElapsedLabel, comma(samePrev), comma(sameCur), p)
		}
		c.Verdict += " 本周结束后会自动切回完整周口径。"
		return c
	}

	p := c.Keys.Pct
	switch {
	case prev.Keys == 0:
		c.Verdict = "上一周按键为 0，环比无法计算；本周共 " + comma(cur.Keys) + " 次按键。"
	case p >= 5:
		c.Verdict = fmt.Sprintf("本周比上周多敲 %d%%，从 %s 次升到 %s 次；增量主要来自%s。",
			p, comma(prev.Keys), comma(cur.Keys), day)
	case p <= -5:
		c.Verdict = fmt.Sprintf("本周比上周少敲 %d%%，从 %s 次降到 %s 次；主要回落在%s。",
			-p, comma(prev.Keys), comma(cur.Keys), day)
	default:
		c.Verdict = fmt.Sprintf("本周与上周基本持平（%s 次 → %s 次，%d%%），节奏稳定。",
			comma(prev.Keys), comma(cur.Keys), p)
	}
	if cur.RecordDays != prev.RecordDays {
		c.Verdict += fmt.Sprintf(" 有记录天数 %d → %d 天，日均仍按 7 天计算。",
			prev.RecordDays, cur.RecordDays)
	}
	return c
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
