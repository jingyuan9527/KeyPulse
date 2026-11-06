package report

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

const maxCalendarWeeks = 53 // 日历热力最多回溯一年，避免孤立旧数据撑出巨大空白

func buildOverview(days []dayPayload, lookup map[string]dayPayload) overviewPayload {
	ov := overviewPayload{TrendUnit: "day", Quips: []string{}}
	if len(days) == 0 {
		return ov
	}
	first, err1 := parseStamp(days[0].Stamp)
	last, err2 := parseStamp(days[len(days)-1].Stamp)
	if err1 != nil || err2 != nil {
		return ov
	}

	var maxKeys uint32
	var weekend uint64
	for _, d := range days {
		ov.TotalKeys += uint64(d.Keys)
		ov.TotalClicks += uint64(d.Clicks)
		ov.TotalWheel += uint64(d.Wheel)
		ov.Active += d.Active
		if d.Keys > maxKeys {
			maxKeys, ov.BestDate, ov.BestKeys = d.Keys, d.Date, d.Keys
		}
		t, err := parseStamp(d.Stamp)
		if err == nil && weekdayIndex(t) >= 6 {
			weekend += uint64(d.Keys)
		}
	}
	ov.Days = len(days)
	ov.ActiveHours = ov.Active / 60
	if ov.TotalKeys > 0 {
		ov.WeekendPct = int(weekend * 100 / ov.TotalKeys)
	}

	// 最长连续有输入天数：按自然日推进，缺文件的日期中断连续。
	var run, best int
	for t := first; !t.After(last); t = t.AddDate(0, 0, 1) {
		d, ok := lookup[t.Format("20060102")]
		if ok && d.Keys+d.Clicks > 0 {
			run++
			if run > best {
				best = run
			}
		} else {
			run = 0
		}
	}
	ov.MaxStreak = best

	// 日历：从最早数据所在周的周一到最新数据所在周的周日，最多 53 周。
	calStart := weekStart(first)
	minStart := weekStart(last).AddDate(0, 0, -7*(maxCalendarWeeks-1))
	if calStart.Before(minStart) {
		calStart = minStart
	}
	calEnd := weekStart(last).AddDate(0, 0, 6)
	for t := calStart; !t.After(calEnd); t = t.AddDate(0, 0, 1) {
		stamp := t.Format("20060102")
		c := calCell{
			Stamp: stamp,
			Month: int(t.Month()),
			Day:   t.Day(),
			WD:    weekdayIndex(t),
		}
		if d, ok := lookup[stamp]; ok {
			c.Has, c.Keys = true, d.Keys
			if d.Keys > 0 && maxKeys > 0 {
				lv := int(math.Ceil(float64(d.Keys) * 5 / float64(maxKeys)))
				if lv < 1 {
					lv = 1
				}
				if lv > 5 {
					lv = 5
				}
				c.Level = lv
			}
		}
		ov.Calendar = append(ov.Calendar, c)
	}

	ov.Trend = buildTrend(days)
	ov.Quips = overviewQuips(ov)
	return ov
}

// buildTrend：60 天内按天展示；更长的跨度按自然周聚合，避免上百根细柱。
func buildTrend(days []dayPayload) []trendPoint {
	if len(days) <= 60 {
		out := make([]trendPoint, 0, len(days))
		for _, d := range days {
			out = append(out, trendPoint{
				Label:  d.Stamp[4:6] + "/" + d.Stamp[6:8],
				Stamp:  d.Stamp,
				Keys:   d.Keys,
				Clicks: d.Clicks,
			})
		}
		return out
	}
	type agg struct {
		start  time.Time
		keys   uint32
		clicks uint32
	}
	var out []trendPoint
	var cur *agg
	flush := func() {
		if cur == nil {
			return
		}
		out = append(out, trendPoint{
			Label:  shortDate(cur.start),
			Stamp:  cur.start.Format("20060102"),
			Keys:   cur.keys,
			Clicks: cur.clicks,
		})
	}
	for _, d := range days {
		t, err := parseStamp(d.Stamp)
		if err != nil {
			continue
		}
		ws := weekStart(t)
		if cur == nil || !ws.Equal(cur.start) {
			flush()
			cur = &agg{start: ws}
		}
		cur.keys += d.Keys
		cur.clicks += d.Clicks
	}
	flush()
	return out
}

func overviewQuips(ov overviewPayload) []string {
	q := []string{}
	if ov.Days == 0 {
		return q
	}
	q = append(q, fmt.Sprintf("共记录 %d 天，累计 %s 次按键、%s 次鼠标点击。",
		ov.Days, comma64(ov.TotalKeys), comma64(ov.TotalClicks)))
	if ov.BestDate != "" {
		q = append(q, fmt.Sprintf("最高单日 %s，%s 次按键。", ov.BestDate, comma(ov.BestKeys)))
	}
	if ov.MaxStreak >= 2 {
		q = append(q, fmt.Sprintf("最长连续有输入 %d 天。", ov.MaxStreak))
	}
	if ov.TotalKeys > 0 {
		q = append(q, fmt.Sprintf("日均 %s 次按键，周末占全期输入 %d%%。",
			comma64(ov.TotalKeys/uint64(ov.Days)), ov.WeekendPct))
	}
	q = append(q, fmt.Sprintf("累计活跃 %.1f 小时，滚轮 %s 格。",
		float64(ov.Active)/60, comma64(ov.TotalWheel)))
	return q
}

func comma64(n uint64) string {
	s := strconv.FormatUint(n, 10)
	if len(s) <= 3 {
		return s
	}
	out := ""
	for len(s) > 3 {
		out = "," + s[len(s)-3:] + out
		s = s[:len(s)-3]
	}
	return s + out
}
