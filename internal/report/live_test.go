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

func readLive(t *testing.T, dir string) livePayload {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, LiveFileName))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimSpace(string(b))
	if !strings.HasPrefix(s, "window.KP_LIVE=") || !strings.HasSuffix(s, ";") {
		t.Fatalf("live.js 不是合法的 JS 赋值语句: %q", s[:min(len(s), 80)])
	}
	js := strings.TrimSuffix(strings.TrimPrefix(s, "window.KP_LIVE="), ";")
	var lp livePayload
	if err := json.Unmarshal([]byte(js), &lp); err != nil {
		t.Fatalf("live.js 内嵌 JSON 非法: %v", err)
	}
	return lp
}

func TestWriteLiveMatchesReportToday(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, time.Local)
	prev := mkDay("20261105", 900, 90)
	today := mkDay("20261106", 1234, 234)
	for _, d := range []store.DayData{prev, today} {
		if err := store.WriteDayFile(dir, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteLive(dir, 7, now); err != nil {
		t.Fatal(err)
	}
	lp := readLive(t, dir)
	if lp.Seq != 7 {
		t.Errorf("seq=%d, want 7", lp.Seq)
	}
	if lp.Day.Stamp != "20261106" {
		t.Errorf("实时快照日期=%s, want 20261106", lp.Day.Stamp)
	}
	if lp.Day.Keys != 1234 || lp.Day.Clicks != 234 {
		t.Errorf("实时计数=%d/%d, want 1234/234", lp.Day.Keys, lp.Day.Clicks)
	}
	// 与重新生成报告用的是同一套聚合逻辑，口径必须逐字段一致，
	// 否则用户会看到实时数字和刷新后的历史数字对不上。
	b, err := BuildDataset(dir, []store.DayData{prev, today}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := decode(t, b).Days[1]
	if lp.Day.Keys != want.Keys || lp.Day.Clicks != want.Clicks ||
		lp.Day.Wheel != want.Wheel || lp.Day.Active != want.Active ||
		lp.Day.HourK != want.HourK || lp.Day.Sum.Title != want.Sum.Title {
		t.Error("实时快照与报告的今日口径不一致")
	}
	if strings.Contains(string(mustRead(t, filepath.Join(dir, LiveFileName))), "</script>") {
		t.Error("实时快照未转义脚本闭合标签")
	}
}

func TestWriteLiveSeqAndRemove(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, time.Local)
	if err := store.WriteDayFile(dir, mkDay("20261106", 10, 2)); err != nil {
		t.Fatal(err)
	}
	for _, seq := range []uint64{1, 2, 3} {
		if err := WriteLive(dir, seq, now); err != nil {
			t.Fatal(err)
		}
		if got := readLive(t, dir).Seq; got != seq {
			t.Errorf("seq=%d, want %d", got, seq)
		}
	}
	if err := RemoveLive(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, LiveFileName)); !os.IsNotExist(err) {
		t.Error("关闭实时模式后通道文件仍在")
	}
	// 重复关闭不应报错（切换开关可能连点）
	if err := RemoveLive(dir); err != nil {
		t.Errorf("重复清理报错: %v", err)
	}
}

func TestWriteLiveMissingDayFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, time.Local)
	if err := WriteLive(dir, 1, now); err == nil {
		t.Error("当天无 .bin 时应返回错误而不是写空快照")
	}
}

func TestRenderWithLiveModeFlag(t *testing.T) {
	dir := t.TempDir()
	raw := []store.DayData{mkDay("20261106", 100, 10)}
	p, err := Render(dir, raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(mustRead(t, p))
	if !strings.Contains(doc, "window.KP_LIVE_MODE=0;") {
		t.Error("普通模式页面未注入关闭标志")
	}
	if strings.Contains(doc, "window.KP_LIVE_MODE=1;") {
		t.Error("普通模式误开启实时轮询")
	}
	p2, err := RenderWithLive(dir, raw, true)
	if err != nil {
		t.Fatal(err)
	}
	doc2 := string(mustRead(t, p2))
	if !strings.Contains(doc2, "window.KP_LIVE_MODE=1;") {
		t.Error("实时模式页面未注入开启标志")
	}
	if strings.Contains(doc2, livePlaceholder) {
		t.Error("实时模式占位符未被替换")
	}
	// 60 秒兜底刷新在两个模式下都保留：周报 / 总览靠它更新聚合视图
	if !strings.Contains(doc2, `http-equiv="refresh" content="60"`) {
		t.Error("实时模式丢掉了 60 秒兜底刷新")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
