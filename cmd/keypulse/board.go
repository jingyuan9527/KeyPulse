package main

// 本地看板（可选、默认关闭）：托盘勾选后才启动 127.0.0.1 回环服务。
// 所有网络与文件 IO 都在独立 goroutine，绝不占用钩子消息循环线程。

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"keypulse/internal/counter"
	"keypulse/internal/live"
	"keypulse/internal/report"
	"keypulse/internal/store"
	"keypulse/internal/tray"
)

var (
	boardMode atomic.Bool // 默认关闭、重启不恢复
	boardMu   sync.Mutex
	boardSrv  *live.Server
	// liveSnap 缓存当天聚合 JSON，由主循环在计数变化后刷新；
	// SSE 只读这份缓存，不会与 Flush 抢 store 的锁。
	liveSnap atomic.Value
)

// refreshLiveSnapshot 生成与 live.js 逐字段同源的快照，只放内存不写文件。
func refreshLiveSnapshot(dir string, seq uint64) {
	b, err := report.LiveSnapshotJSON(dir, seq, time.Now())
	if err == nil && len(b) > 0 {
		liveSnap.Store(b)
	}
}

// boardPage 服务端直出首页，不写 report.html。
func boardPage(dir, token string) (string, error) {
	reportMu.Lock()
	defer reportMu.Unlock()
	days, err := store.ListDays(dir, report.ReportMaxDays)
	if err != nil {
		return "", err
	}
	if len(days) == 0 {
		return "", fmt.Errorf("暂无统计数据")
	}
	return report.RenderServer(dir, days, token)
}

func startBoard(dir string, st *store.Store, c *counter.Counter) {
	boardMu.Lock()
	defer boardMu.Unlock()
	if boardSrv != nil {
		// 已经开着：只把页面重新拉起来，不重复监听端口。
		tray.OpenURL(boardSrv.URL())
		return
	}

	// 先把内存计数落盘，再生成首帧：实时数字与历史报告同源，不会倒退。
	_, _ = st.Flush(true)
	refreshLiveSnapshot(dir, liveSeq.Add(1))
	liveMode.Store(true)

	srv, err := live.Start(live.Config{
		Page: func(token string) (string, error) { return boardPage(dir, token) },
		Snapshot: func() ([]byte, error) {
			v := liveSnap.Load()
			if v == nil {
				return nil, fmt.Errorf("快照尚未生成")
			}
			return v.([]byte), nil
		},
		TakePulse: func() [8]uint32 { return c.TakePulse() },
		IsLive:    func() bool { return liveMode.Load() },
		SetLive: func(on bool) {
			liveMode.Store(on)
			if on {
				go refreshLiveSnapshot(dir, liveSeq.Add(1))
			}
		},
		OnStop: func() {
			boardMu.Lock()
			boardSrv = nil
			boardMu.Unlock()
			boardMode.Store(false)
			liveMode.Store(false)
			tray.Notify("本地看板已自动停止", "所有页面关闭超过 30 秒，服务已停止并释放端口；重新勾选可再次打开。")
		},
	})
	if err != nil {
		boardMode.Store(false)
		liveMode.Store(false)
		tray.Notify("本地看板启动失败", "无法绑定 127.0.0.1 端口；统计与离线报告不受影响。")
		return
	}
	boardSrv = srv
	tray.OpenURL(srv.URL())
	tray.Notify("本地看板已启动",
		"仅绑定 127.0.0.1 回环，页面内可开关实时推送、键位随按键闪动；关闭全部页面 30 秒后自动停止。")
}

func stopBoard(dir string) {
	boardMu.Lock()
	srv := boardSrv
	boardSrv = nil
	boardMu.Unlock()
	if srv != nil {
		srv.Close()
	}
	boardMode.Store(false)
	liveMode.Store(false)
	_ = report.RemoveLive(dir)
	tray.Notify("本地看板已关闭", "已回到默认状态，统计与离线报告不受影响。")
}

// closeBoard 用于进程退出：只释放端口，不再弹气泡或改托盘状态。
func closeBoard() {
	boardMu.Lock()
	srv := boardSrv
	boardSrv = nil
	boardMu.Unlock()
	if srv != nil {
		srv.Close()
	}
}
