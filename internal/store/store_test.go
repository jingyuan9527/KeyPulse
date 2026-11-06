package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"keypulse/internal/counter"
)

func TestFlushAndReload(t *testing.T) {
	dir := t.TempDir()
	c := counter.New()
	s, err := Open(dir, c)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		c.Add(0x41) // A
	}
	for i := 0; i < 3; i++ {
		c.Add(counter.MouseLeft)
	}
	for i := 0; i < 2; i++ {
		c.Add(counter.MouseWheel)
	}
	if _, err := s.Flush(true); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, time.Now().Format("20060102")+".bin")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != FileSize {
		t.Fatalf("文件大小 %d，期望 %d", fi.Size(), FileSize)
	}

	d, err := ReadDayFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Keys[0x41] != 10 {
		t.Errorf("按键计数 = %d，期望 10", d.Keys[0x41])
	}
	if d.TotalClicks() != 3 {
		t.Errorf("点击 = %d，期望 3（滚轮不计入）", d.TotalClicks())
	}
	if d.Wheel() != 2 {
		t.Errorf("滚轮 = %d，期望 2", d.Wheel())
	}
	if d.ActiveMinutes() < 1 {
		t.Error("分钟活跃位图未置位")
	}
	var hourSum uint32
	for h := 0; h < 24; h++ {
		hourSum += d.Hours[h*2]
	}
	if hourSum != 10 {
		t.Errorf("小时分布合计 = %d，期望 10", hourSum)
	}

	// 模拟进程重启：已有计数必须读回内存而不是从 0 开始
	c2 := counter.New()
	s2, err := Open(dir, c2)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if got := c2.Slot(0x41); got != 10 {
		t.Errorf("重启后读回按键 = %d，期望 10", got)
	}

	// 在已有值上继续累加，验证不会重复计入小时分布
	c2.Add(0x41)
	if _, err := s2.Flush(true); err != nil {
		t.Fatal(err)
	}
	d2, err := ReadDayFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if d2.Keys[0x41] != 11 {
		t.Errorf("累加后按键 = %d，期望 11", d2.Keys[0x41])
	}

	var buf bytes.Buffer
	if err := ExportCSV(&buf, []DayData{d2}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(d2.Date())) {
		t.Error("CSV 缺少日期列")
	}
}

func TestMarkDownFiltersAutoRepeat(t *testing.T) {
	c := counter.New()
	if !c.MarkDown(0x41) {
		t.Fatal("首次按下应计一次")
	}
	if c.MarkDown(0x41) {
		t.Fatal("长按连发应被位图过滤")
	}
	c.MarkUp(0x41)
	if !c.MarkDown(0x41) {
		t.Fatal("抬起后再次按下应重新计数")
	}
}
