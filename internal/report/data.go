package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"keypulse/internal/store"
	"keypulse/internal/version"
)

// 本文件把按天二进制快照转换成一次性注入 HTML 的紧凑 JSON 数据集。
// 只包含聚合计数，不含任何原始事件序列；页面里的单日/周/总览切换全部在
// 本地 JS 完成，不产生额外磁盘 IO、不联网、不暴露隐私。

type metric struct {
	Cur  uint32 `json:"c"`
	Prev uint32 `json:"p"`
	Pct  int    `json:"d"`
}

func pctDelta(cur, prev uint32) int {
	if prev == 0 {
		return 0
	}
	return int((int64(cur) - int64(prev)) * 100 / int64(prev))
}

type summaryLite struct {
	Title   string   `json:"t"`
	Equiv   string   `json:"e"`
	Peak    string   `json:"p"`
	Persona string   `json:"pe"`
	Quips   []string `json:"q"`
}

type dayPayload struct {
	Stamp  string      `json:"s"`
	Date   string      `json:"d"`
	WD     int         `json:"w"` // 1=周一 … 7=周日
	Keys   uint32      `json:"k"`
	Clicks uint32      `json:"c"`
	Wheel  uint32      `json:"r"`
	Active int         `json:"a"`
	HourK  [24]uint32  `json:"hk"`
	HourC  [24]uint32  `json:"hc"`
	KeyUse [][2]uint32 `json:"e"` // 非零键 [vk,count]，含键盘热力所需的全部键
	Sum    summaryLite `json:"su"`
}

type dayBrief struct {
	Stamp  string `json:"s"`
	Date   string `json:"d"`
	Label  string `json:"lb"`
	Has    bool   `json:"h"`
	Keys   uint32 `json:"k"`
	Clicks uint32 `json:"c"`
	Active int    `json:"a"`
}

type weekCompare struct {
	Has      bool      `json:"ok"`
	Label    string    `json:"lb"`
	Keys     metric    `json:"k"`
	Clicks   metric    `json:"c"`
	Wheel    metric    `json:"r"`
	Active   metric    `json:"a"`
	Daily    metric    `json:"da"`
	PrevDays [7]uint32 `json:"pd"` // 上一周 7 天按键，用于双色柱
	// 未完成周按「同期」比较：只取双方都已过去的前 N 天，避免总量被系统性低估。
	SameDays int    `json:"sd"`
	SameKeys metric `json:"sk"`
	Partial  bool   `json:"part"`
	Note     string `json:"nt"`
	Verdict  string `json:"v"`
}

type weekPayload struct {
	Start      string      `json:"s"`
	End        string      `json:"e"`
	Label      string      `json:"lb"`
	Keys       uint32      `json:"k"`
	Clicks     uint32      `json:"c"`
	Wheel      uint32      `json:"r"`
	Active     int         `json:"a"`
	RecordDays int         `json:"rd"`
	DailyAvg   uint32      `json:"da"`
	Days       [7]dayBrief `json:"d"`
	HourK      [24]uint32  `json:"hk"`
	HourC      [24]uint32  `json:"hc"`
	Top        [][2]uint32 `json:"top"`
	PeakDate   string      `json:"pd"`
	QuietDate  string      `json:"qd"`
	Streak     int         `json:"st"`
	WeekendPct int         `json:"wp"`
	// 本周（含最新数据那一周）尚未过完时，Elapsed < 7，Partial=true。
	Elapsed      int          `json:"el"`
	Partial      bool         `json:"part"`
	ElapsedLabel string       `json:"pl"`
	Title        string       `json:"ti"`
	Quips        []string     `json:"q"`
	Cmp          *weekCompare `json:"cmp"`
}

type calCell struct {
	Stamp string `json:"s"`
	Keys  uint32 `json:"k"`
	Has   bool   `json:"h"`
	Level int    `json:"lv"`
	Month int    `json:"m"`
	Day   int    `json:"dd"`
	WD    int    `json:"w"`
}

type trendPoint struct {
	Label  string `json:"l"`
	Stamp  string `json:"s"`
	Keys   uint32 `json:"k"`
	Clicks uint32 `json:"c"`
}

type overviewPayload struct {
	TotalKeys   uint64       `json:"k"`
	TotalClicks uint64       `json:"c"`
	TotalWheel  uint64       `json:"r"`
	Active      int          `json:"a"` // 累计活跃分钟
	ActiveHours int          `json:"ah"`
	Days        int          `json:"n"`
	BestDate    string       `json:"bd"`
	BestKeys    uint32       `json:"bk"`
	MaxStreak   int          `json:"ms"`
	WeekendPct  int          `json:"wp"`
	Calendar    []calCell    `json:"cal"`
	Trend       []trendPoint `json:"tr"`
	TrendUnit   string       `json:"tu"` // "day" / "week"
	Quips       []string     `json:"q"`
}

// healthPayload 来自 health.log：钩子连续静默且期间系统有输入、确认中断后
// 自动重挂的记录。只含时间与估算缺口，不含任何按键内容。中断窗口内的输入
// 确实会漏计，所以必须在页面上如实标注，不能把近似值伪装成精确统计。
// 旧版把单周期的输入/回调时序错位也记为恢复，那些行由 readHealthLog 忽略。
type healthPayload struct {
	Total int    `json:"n"`
	Today int    `json:"t"`
	Last  string `json:"l"`
	Note  string `json:"nt"`
}

const healthLogMaxBytes = 64 * 1024

// 只有经过静默窗口确认的恢复才计入报告。旧版 reason=input_without_callback
// 绝大多数是"1 秒前钩子还在回调"的误报，升级后不应继续挂着采集提示；
// 日志本身保留不删，历史数据可追溯。
const healthReasonConfirmed = "confirmed_silence"

func gapLabel(sec int) string {
	switch {
	case sec < 60:
		return fmt.Sprintf("%d 秒", sec)
	case sec < 3600:
		return fmt.Sprintf("%d 分钟", sec/60)
	default:
		return fmt.Sprintf("%d 小时", sec/3600)
	}
}

// readHealthLog 读取健康日志。只信任结构合法的行，并限制读取尾部字节，
// 避免异常增长或被外部篡改的文件把报告撑大。内容一律走 JSON 转义 + esc。
func readHealthLog(dir string, now time.Time) healthPayload {
	b, err := os.ReadFile(filepath.Join(dir, "health.log"))
	if err != nil {
		return healthPayload{}
	}
	truncated := len(b) >= healthLogMaxBytes
	if truncated {
		b = b[len(b)-healthLogMaxBytes:]
	}
	lines := strings.Split(string(b), "\n")
	datePrefix := now.Format("2006-01-02")
	total, todayCount, maxGap := 0, 0, 0
	last := ""
	for i, ln := range lines {
		ln = strings.TrimSpace(ln)
		if len(ln) < 19 || !strings.Contains(ln, " RECOVER ") {
			continue
		}
		if !strings.Contains(ln, "reason="+healthReasonConfirmed) {
			continue // 旧版误报记录，不升级提示条
		}
		ts := ln[:19]
		if _, err := time.ParseInLocation("2006-01-02 15:04:05", ts, time.Local); err != nil {
			continue
		}
		if truncated && i == 0 {
			continue // 尾部截断的第一行可能不完整
		}
		total++
		last = ts
		if strings.HasPrefix(ts, datePrefix) {
			todayCount++
		}
		if j := strings.Index(ln, "gap="); j >= 0 {
			var g int
			if _, err := fmt.Sscanf(ln[j+4:], "%d", &g); err == nil && g > maxGap {
				maxGap = g
			}
		}
	}
	if total == 0 {
		return healthPayload{}
	}
	h := healthPayload{Total: total, Today: todayCount, Last: last}
	prefix := "检测到"
	if truncated {
		prefix = "最近记录到" // 日志超过读取上限，只统计尾部
	}
	h.Note = fmt.Sprintf("%s %d 次采集中断并自动重挂", prefix, total)
	if todayCount > 0 {
		h.Note += fmt.Sprintf("（今日 %d 次）", todayCount)
	}
	h.Note += fmt.Sprintf("，最近一次 %s，最长缺口约 %s。多为系统静默卸载钩子或长时间停留在高完整性窗口；缺口内的输入未被计入，该时段统计为近似值。",
		last[11:16], gapLabel(maxGap))
	return h
}

type dataset struct {
	Generated string            `json:"gen"`
	Version   string            `json:"ver"` // 生成该报告的程序构建信息，便于追溯
	DataDir   string            `json:"dir"`
	Truncated bool              `json:"trunc"` // 历史超过上限，仅内嵌最近一段
	MaxDays   int               `json:"maxd"`
	Days      []dayPayload      `json:"days"`
	Weeks     []weekPayload     `json:"weeks"`
	Overview  overviewPayload   `json:"ov"`
	Health    healthPayload     `json:"health"`
	Layout    [][]uint32        `json:"layout"`
	Names     map[string]string `json:"names"`
	Note      string            `json:"note"`
}

var weekdayLabel = [7]string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}

func parseStamp(stamp string) (time.Time, error) {
	return time.ParseInLocation("20060102", stamp, time.Local)
}

// weekStart 返回该日期所在自然周的周一零点（ISO：周一为一周起点）。
func weekStart(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	y, m, d := t.Date()
	return time.Date(y, m, d-wd+1, 0, 0, 0, 0, t.Location())
}

func weekdayIndex(t time.Time) int {
	wd := int(t.Weekday())
	if wd == 0 {
		return 7
	}
	return wd
}

func shortDate(t time.Time) string { return t.Format("01/02") }

func topFrom(m map[uint32]uint32, n int) [][2]uint32 {
	type kv struct {
		vk uint32
		n  uint32
	}
	pairs := make([]kv, 0, len(m))
	for vk, c := range m {
		if c > 0 {
			pairs = append(pairs, kv{vk, c})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].vk < pairs[j].vk
	})
	if len(pairs) > n {
		pairs = pairs[:n]
	}
	out := make([][2]uint32, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, [2]uint32{p.vk, p.n})
	}
	return out
}

func buildNames() map[string]string {
	m := make(map[string]string, len(vkNames))
	for vk, name := range vkNames {
		m[strconv.FormatUint(uint64(vk), 10)] = name
	}
	return m
}

// BuildDataset 把全部历史数据序列化成可安全内嵌 <script> 的 JSON。
func BuildDataset(dir string, raw []store.DayData, generated time.Time) ([]byte, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("没有可统计的数据")
	}
	days := make([]dayPayload, 0, len(raw))
	lookup := make(map[string]dayPayload, len(raw))
	for i, d := range raw {
		var prev *store.DayData
		if i > 0 {
			prev = &raw[i-1]
		}
		p := toDayPayload(d, prev)
		days = append(days, p)
		lookup[p.Stamp] = p
	}
	weeks := buildWeeks(days, lookup)
	ov := buildOverview(days, lookup)

	ds := dataset{
		Generated: generated.Format("2006-01-02 15:04:05"),
		Version:   version.String(),
		DataDir:   dir,
		Truncated: len(raw) >= ReportMaxDays,
		MaxDays:   ReportMaxDays,
		Days:      days,
		Weeks:     weeks,
		Overview:  ov,
		Health:    readHealthLog(dir, generated),
		Layout:    layoutCodes(),
		Names:     buildNames(),
		Note:      "趣味换算：按 1 次按键 ≈ 1 个字符估算，含删除、快捷键与中文输入过程，与实际产出字数无关；作品字数为常见版本粗略量级。",
	}
	b, err := json.Marshal(ds)
	if err != nil {
		return nil, err
	}
	// 数据集里含日期、路径与固定文案，仍统一转义，杜绝 </script> 提前闭合
	// 以及 HTML 解析器把 & / < 当标记处理的可能。
	s := string(b)
	s = strings.ReplaceAll(s, "<", "\\u003c")
	s = strings.ReplaceAll(s, ">", "\\u003e")
	s = strings.ReplaceAll(s, "&", "\\u0026")
	return []byte(s), nil
}

func layoutCodes() [][]uint32 {
	out := make([][]uint32, 0, len(keyLayout))
	for _, row := range keyLayout {
		r := make([]uint32, len(row))
		copy(r, row)
		out = append(out, r)
	}
	return out
}

func toDayPayload(d store.DayData, prev *store.DayData) dayPayload {
	t, err := parseStamp(d.Stamp)
	if err != nil {
		t = time.Now()
	}
	p := dayPayload{
		Stamp:  d.Stamp,
		Date:   d.Date(),
		WD:     weekdayIndex(t),
		Keys:   d.TotalKeys(),
		Clicks: d.TotalClicks(),
		Wheel:  d.Wheel(),
		Active: d.ActiveMinutes(),
		Sum:    summaryLite{Quips: []string{}},
	}
	for h := 0; h < 24; h++ {
		p.HourK[h] = d.Hours[h*2]
		p.HourC[h] = d.Hours[h*2+1]
	}
	for vk, n := range d.Keys {
		if n > 0 {
			p.KeyUse = append(p.KeyUse, [2]uint32{uint32(vk), n})
		}
	}
	s := Build(d, prev)
	p.Sum = summaryLite{
		Title:   s.Title,
		Equiv:   s.Equiv,
		Peak:    s.Peak,
		Persona: s.Persona,
		Quips:   s.Quips,
	}
	if p.Sum.Quips == nil {
		p.Sum.Quips = []string{}
	}
	return p
}
