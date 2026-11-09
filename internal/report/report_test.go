package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"keypulse/internal/store"
)

func mkDay(stamp string, keys, clicks uint32) store.DayData {
	d := store.DayData{Stamp: stamp}
	d.Keys[0x41] = keys // 'A'
	d.Mouse[0] = clicks
	for h := 9; h < 18; h++ {
		d.Hours[h*2] = keys / 9
		d.Hours[h*2+1] = clicks / 9
		for m := 0; m < 60; m++ {
			idx := h*60 + m
			d.Minutes[idx/8] |= 1 << uint(idx%8)
		}
	}
	return d
}

func decode(t *testing.T, b []byte) dataset {
	t.Helper()
	var ds dataset
	if err := json.Unmarshal(b, &ds); err != nil {
		t.Fatalf("数据集不是合法 JSON: %v", err)
	}
	return ds
}

func TestWeekStartIsMonday(t *testing.T) {
	cases := []struct{ in, want string }{
		{"20261102", "20261102"}, // 周一
		{"20261108", "20261102"}, // 周日归到本周一
		{"20261109", "20261109"}, // 下周一
	}
	for _, c := range cases {
		got, err := time.ParseInLocation("20060102", c.in, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		if s := weekStart(got).Format("20060102"); s != c.want {
			t.Errorf("weekStart(%s)=%s, want %s", c.in, s, c.want)
		}
	}
}

func TestDatasetWeeksAndCompare(t *testing.T) {
	raw := []store.DayData{
		mkDay("20261102", 1000, 100), // 第 1 周周一
		mkDay("20261103", 2000, 200), // 第 1 周周二
		mkDay("20261109", 4000, 400), // 第 2 周周一
		mkDay("20261115", 500, 50),   // 第 2 周周日：让最新一周成为完整周
	}
	b, err := BuildDataset("D:/data", raw, time.Date(2026, 11, 15, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "<") || strings.Contains(string(b), ">") {
		t.Fatal("数据集里出现未转义的尖括号，可能提前闭合 <script>")
	}
	ds := decode(t, b)
	if len(ds.Days) != 4 || len(ds.Weeks) != 2 {
		t.Fatalf("days=%d weeks=%d, want 4 / 2", len(ds.Days), len(ds.Weeks))
	}
	w1, w2 := ds.Weeks[0], ds.Weeks[1]
	if w1.Start != "20261102" || w2.Start != "20261109" {
		t.Fatalf("周起点错误: %s / %s", w1.Start, w2.Start)
	}
	// 缺失日期按 0 计入、日均固定除以 7、有记录天数单列
	if w1.Keys != 3000 || w1.RecordDays != 2 || w1.DailyAvg != 3000/7 {
		t.Errorf("第 1 周聚合错误: keys=%d record=%d avg=%d", w1.Keys, w1.RecordDays, w1.DailyAvg)
	}
	if !w1.Days[0].Has || !w1.Days[1].Has || w1.Days[2].Has || w1.Days[6].Has {
		t.Error("周内日期存在性判断错误")
	}
	if w1.Cmp == nil || w1.Cmp.Has {
		t.Error("第 1 周不应有可比的上一周")
	}
	if w2.Cmp == nil || !w2.Cmp.Has {
		t.Fatal("第 2 周应能与第 1 周对比")
	}
	if w2.Partial || w2.Elapsed != 7 {
		t.Errorf("最新周以周日结尾，应为完整周: partial=%v elapsed=%d", w2.Partial, w2.Elapsed)
	}
	if w2.Cmp.Partial || w2.Cmp.SameDays != 7 {
		t.Errorf("完整周不应启用同期口径: %+v", w2.Cmp)
	}
	if w2.Cmp.Keys.Prev != 3000 || w2.Cmp.Keys.Cur != 4500 || w2.Cmp.Keys.Pct != 50 {
		t.Errorf("环比错误: %+v", w2.Cmp.Keys)
	}
	if !strings.Contains(w2.Cmp.Verdict, "多敲 50%") || strings.Contains(w2.Cmp.Verdict, "同期") {
		t.Errorf("完整周结论口径错误: %s", w2.Cmp.Verdict)
	}
	if w2.Cmp.PrevDays[0] != 1000 || w2.Cmp.PrevDays[1] != 2000 || w2.Cmp.PrevDays[6] != 0 {
		t.Errorf("上一周逐日数组错误: %v", w2.Cmp.PrevDays)
	}
	// 历史日的速写不能把非相邻记录日说成"昨天"
	if !strings.Contains(strings.Join(ds.Days[2].Sum.Quips, "|"), "上一个记录日") {
		t.Errorf("跨空白日的对比文案仍写成昨天: %v", ds.Days[2].Sum.Quips)
	}
	if ds.Overview.TotalKeys != 7500 || ds.Overview.Days != 4 {
		t.Errorf("总览累计错误: %+v", ds.Overview)
	}
	if len(ds.Overview.Calendar) == 0 || len(ds.Overview.Trend) != 4 {
		t.Errorf("总览日历/趋势为空: cal=%d trend=%d", len(ds.Overview.Calendar), len(ds.Overview.Trend))
	}
}

// 本周没过完时，不能拿 5 天总量和上一完整周的 7 天比，否则必然误判为"变懒了"。
func TestPartialWeekUsesSamePeriod(t *testing.T) {
	raw := []store.DayData{
		mkDay("20261102", 1000, 100), // 上一周周一
		mkDay("20261103", 2000, 200), // 上一周周二
		mkDay("20261109", 4000, 400), // 本周周一（最新数据日）
	}
	b, err := BuildDataset("D:/data", raw, time.Date(2026, 11, 9, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	ds := decode(t, b)
	w := ds.Weeks[len(ds.Weeks)-1]
	if !w.Partial || w.Elapsed != 1 || w.ElapsedLabel != "周一" {
		t.Fatalf("未识别出进行中的周: %+v", w)
	}
	cmp := w.Cmp
	if cmp == nil || !cmp.Has || !cmp.Partial || cmp.SameDays != 1 {
		t.Fatalf("同期对比未启用: %+v", cmp)
	}
	// 总量口径：4000 vs 3000 = +33%；同期口径：4000 vs 1000 = +300%
	if cmp.Keys.Pct != 33 {
		t.Errorf("总量环比错误: %+v", cmp.Keys)
	}
	if cmp.SameKeys.Cur != 4000 || cmp.SameKeys.Prev != 1000 || cmp.SameKeys.Pct != 300 {
		t.Errorf("同期聚合错误: %+v", cmp.SameKeys)
	}
	if cmp.Note == "" || !strings.Contains(cmp.Note, "本周尚未结束") {
		t.Errorf("缺少口径提示: %q", cmp.Note)
	}
	if !strings.Contains(cmp.Verdict, "同期") || !strings.Contains(cmp.Verdict, "多敲 300%") {
		t.Errorf("结论未使用同期口径: %s", cmp.Verdict)
	}
	if strings.Contains(cmp.Verdict, "少敲") {
		t.Errorf("未完成周不应得出下降结论: %s", cmp.Verdict)
	}
	if !strings.Contains(strings.Join(w.Quips, "|"), "本周进行中") {
		t.Errorf("周速写未提示进行中: %v", w.Quips)
	}
}

func TestDatasetEmpty(t *testing.T) {
	if _, err := BuildDataset("x", nil, time.Now()); err == nil {
		t.Fatal("空数据应返回错误")
	}
}

// 钩子自动恢复会写 health.log，报告必须如实标注数据缺口；
// 同时要忽略格式非法/被外部篡改的行，且不能让内容逃逸出 <script>。
func TestHealthLogInjection(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().Format("2006-01-02")
	log := today + " 09:12:03 RECOVER gap=7s total=1 reason=confirmed_silence\n" +
		today + " 10:44:20 RECOVER gap=132s total=2 reason=confirmed_silence\n" +
		today + " 08:00:00 RECOVER gap=1s total=0 reason=input_without_callback\n" +
		"这不是一条合法记录\n" +
		"</script><script>alert(1)</script> RECOVER gap=1s total=3 reason=confirmed_silence\n"
	if err := os.WriteFile(filepath.Join(dir, "health.log"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []store.DayData{mkDay("20261102", 1000, 100)}
	b, err := BuildDataset(dir, raw, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ds := decode(t, b)
	if ds.Health.Total != 2 || ds.Health.Today != 2 {
		t.Errorf("健康日志解析错误: %+v", ds.Health)
	}
	for _, want := range []string{"近似值", "10:44", "2 分钟", "今日 2 次"} {
		if !strings.Contains(ds.Health.Note, want) {
			t.Errorf("提示缺少 %q: %s", want, ds.Health.Note)
		}
	}
	if strings.Contains(string(b), "<script>") {
		t.Error("篡改内容进入了内嵌 JSON")
	}
	p, err := Render(dir, raw)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `id="health"`) || !strings.Contains(string(doc), "采集提示") {
		t.Error("页面缺少健康提示容器")
	}
	// 无日志目录不应产生任何提示
	dir2 := t.TempDir()
	b2, err := BuildDataset(dir2, raw, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if h := decode(t, b2).Health; h.Total != 0 || h.Note != "" {
		t.Errorf("无日志时不应有健康提示: %+v", h)
	}
	// 只有旧版误报日志时同样不应产生提示（用户升级后历史 health.log 保留）
	dir3 := t.TempDir()
	legacy := today + " 09:00:00 RECOVER gap=0s total=1 reason=input_without_callback\n" +
		today + " 09:00:02 RECOVER gap=1s total=2 reason=input_without_callback\n"
	if err := os.WriteFile(filepath.Join(dir3, "health.log"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	b3, err := BuildDataset(dir3, raw, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if h := decode(t, b3).Health; h.Total != 0 || h.Note != "" {
		t.Errorf("旧版误报日志不应再挂采集提示: %+v", h)
	}
}

func TestRenderEmbedsDataAtomically(t *testing.T) {
	dir := t.TempDir()
	raw := []store.DayData{mkDay("20261102", 1000, 100), mkDay("20261103", 1200, 120)}
	p, err := Render(dir, raw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	if !strings.Contains(doc, "window.KP_DATA=") {
		t.Error("数据未注入页面")
	}
	if strings.Contains(doc, dataPlaceholder) {
		t.Error("占位符未被替换")
	}
	if strings.Count(doc, "</script>") != 2 {
		t.Errorf("script 标签数量异常: %d", strings.Count(doc, "</script>"))
	}
	if !strings.Contains(doc, "周报 · 对比") || !strings.Contains(doc, "总览") || !strings.Contains(doc, "日历热力") {
		t.Error("三个视图的入口或容器缺失")
	}
	if !strings.Contains(doc, `http-equiv="refresh" content="60"`) {
		t.Error("自动刷新 meta 缺失")
	}
	if filepath.Clean(p) != filepath.Clean(filepath.Join(dir, "report.html")) {
		t.Errorf("报告路径错误: %s", p)
	}
	// 第二次渲染覆盖同一文件，不残留半截内容
	if _, err := Render(dir, raw); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(p)
	if len(b2) != len(b) {
		t.Logf("两次渲染长度不同（生成时间长度一致时通常相同）: %d / %d", len(b), len(b2))
	}
}
