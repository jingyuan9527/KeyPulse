package report

// 实时模式的数据通道：进程把当天的聚合计数写成一个极小的 JS 文件，
// 报告页用动态 <script> 每秒读取并局部更新今日视图。
// 全程是 file:// 本地文件，不起服务、不监听端口、不发起网络请求。
// 与 Render 一样只输出聚合计数，不含按键序列、字符文本或窗口信息。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"keypulse/internal/store"
)

// LiveFileName 是实时通道文件名，与 report.html 放在同一数据目录。
const LiveFileName = "live.js"

var liveMu sync.Mutex

type livePayload struct {
	Seq uint64     `json:"seq"`
	Gen string     `json:"gen"`
	Day dayPayload `json:"d"`
}

// WriteLive 读当天（以及前一日）的 .bin，生成今日快照并一次写入 live.js。
// 复用 toDayPayload，保证实时视图与重新生成报告的口径完全一致：
// 卡片、趣味总结、Top 10、热力图、小时分布、趋势柱都来自同一份聚合数据。
// 调用方须先完成一次 Flush，确保读到的是已落盘的值，避免实时数字在
// 崩溃后出现「倒退」。
func WriteLive(dir string, seq uint64, now time.Time) error {
	stamp := now.Format("20060102")
	today, err := store.ReadDayFile(filepath.Join(dir, stamp+".bin"))
	if err != nil {
		return err
	}
	var prev *store.DayData
	if y, e := store.ReadDayFile(filepath.Join(dir, now.AddDate(0, 0, -1).Format("20060102")+".bin")); e == nil {
		prev = &y
	}
	p := livePayload{
		Seq: seq,
		Gen: now.Format("2006-01-02 15:04:05"),
		Day: toDayPayload(today, prev),
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	// json.Marshal 默认转义 < > & 与 U+2028/U+2029，可直接作为 JS 字面量。
	doc := "window.KP_LIVE=" + string(b) + ";\n"
	liveMu.Lock()
	defer liveMu.Unlock()
	return os.WriteFile(filepath.Join(dir, LiveFileName), []byte(doc), 0o644)
}

// RemoveLive 在关闭实时模式时清掉通道文件，页面回落到 60 秒整页刷新。
func RemoveLive(dir string) error {
	liveMu.Lock()
	defer liveMu.Unlock()
	err := os.Remove(filepath.Join(dir, LiveFileName))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
