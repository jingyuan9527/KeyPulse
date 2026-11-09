package live

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type testState struct {
	mu     sync.Mutex
	live   bool
	pulse  [8]uint32
	onStop chan struct{}
}

func newTestServer(t *testing.T, st *testState, idle, watch time.Duration) *Server {
	t.Helper()
	s, err := Start(Config{
		Page: func(token string) (string, error) {
			if token == "" {
				return "", fmt.Errorf("token 未传给页面生成回调")
			}
			return "<html><body>KeyPulse 本地看板</body></html>", nil
		},
		Snapshot: func() ([]byte, error) {
			return []byte(`{"seq":1,"d":{"s":"20261109","k":10}}`), nil
		},
		TakePulse: func() [8]uint32 {
			st.mu.Lock()
			defer st.mu.Unlock()
			p := st.pulse
			st.pulse = [8]uint32{}
			return p
		},
		IsLive:  func() bool { st.mu.Lock(); defer st.mu.Unlock(); return st.live },
		SetLive: func(on bool) { st.mu.Lock(); st.live = on; st.mu.Unlock() },
		OnStop: func() {
			select {
			case st.onStop <- struct{}{}:
			default:
			}
		},
		Listen:        func() (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") },
		IdleTimeout:   idle,
		WatchInterval: watch,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func freshState(live bool) *testState {
	return &testState{live: live, onStop: make(chan struct{}, 1)}
}

func TestAuthorization(t *testing.T) {
	s := newTestServer(t, freshState(true), 5*time.Second, 20*time.Millisecond)
	client := &http.Client{Timeout: 3 * time.Second}

	ok := func(resp *http.Response, err error) {
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("期望 200，实际 %d", resp.StatusCode)
		}
	}
	denied := func(resp *http.Response, err error) {
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("期望 403，实际 %d", resp.StatusCode)
		}
	}

	ok(client.Get(s.URL()))
	denied(client.Get("http://" + s.Addr() + "/"))
	denied(client.Get("http://" + s.Addr() + "/?t=wrong-token"))

	// Host 伪造：DNS rebinding / 恶意域名指向本机时必须拒绝。
	req, _ := http.NewRequest("GET", s.URL(), nil)
	req.Host = "evil.example"
	denied(client.Do(req))

	// 跨站 Origin 拒绝，同源 Origin 放行。
	req2, _ := http.NewRequest("GET", s.URL(), nil)
	req2.Header.Set("Origin", "https://evil.example")
	denied(client.Do(req2))
	req3, _ := http.NewRequest("GET", s.URL(), nil)
	req3.Header.Set("Origin", "http://"+s.Addr())
	ok(client.Do(req3))

	resp, err := client.Get("http://" + s.Addr() + "/favicon.ico?t=" + s.Token())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("未知路径应 404，实际 %d", resp.StatusCode)
	}
	if len(s.Token()) != tokenBytes*2 {
		t.Errorf("token 长度异常: %d", len(s.Token()))
	}
}

func TestToggle(t *testing.T) {
	st := freshState(true)
	s := newTestServer(t, st, 5*time.Second, 20*time.Millisecond)
	client := &http.Client{Timeout: 3 * time.Second}

	if resp, err := client.Get("http://" + s.Addr() + "/api/live?t=" + s.Token() + "&on=0"); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Error("GET 不应允许切换实时模式")
		}
	}

	for _, tc := range []struct {
		on   string
		want bool
	}{{"0", false}, {"1", true}} {
		url := "http://" + s.Addr() + "/api/live?t=" + s.Token() + "&on=" + tc.on
		resp, err := client.Post(url, "application/json", strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("POST on=%s 返回 %d", tc.on, resp.StatusCode)
		}
		st.mu.Lock()
		got := st.live
		st.mu.Unlock()
		if got != tc.want {
			t.Errorf("POST on=%s 后状态为 %v，期望 %v", tc.on, got, tc.want)
		}
	}
}

func readFrame(r *bufio.Reader) (string, string, error) {
	event, data := "", ""
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return event, data, nil
		}
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func waitFrame(t *testing.T, r *bufio.Reader, want string) string {
	t.Helper()
	for i := 0; i < 40; i++ {
		event, data, err := readFrame(r)
		if err != nil {
			t.Fatalf("读取 SSE 帧失败（等待 %s）: %v", want, err)
		}
		if event == want {
			return data
		}
	}
	t.Fatalf("40 帧内未收到 %s 事件", want)
	return ""
}

func TestSSEStreamAndKeyFlash(t *testing.T) {
	st := freshState(true)
	s := newTestServer(t, st, 10*time.Second, 20*time.Millisecond)
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Get("http://" + s.Addr() + "/events?t=" + s.Token())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE 连接返回 %d", resp.StatusCode)
	}
	r := bufio.NewReader(resp.Body)

	if data := waitFrame(t, r, "mode"); data != `{"on":true}` {
		t.Errorf("首帧 mode 异常: %s", data)
	}
	if data := waitFrame(t, r, "snapshot"); !strings.Contains(data, `"seq":1`) {
		t.Errorf("snapshot 帧异常: %s", data)
	}

	// 只推聚合位图：VK 0x41('A') 与 0x0D(Enter)。
	st.mu.Lock()
	st.pulse = [8]uint32{}
	st.pulse[0x41>>5] |= 1 << (0x41 & 31)
	st.pulse[0x0D>>5] |= 1 << (0x0D & 31)
	st.mu.Unlock()

	data := waitFrame(t, r, "keys")
	var words [8]uint32
	if err := json.Unmarshal([]byte(data), &words); err != nil {
		t.Fatalf("keys 不是 8 个 uint32: %v / %s", err, data)
	}
	if (words[0x41>>5]>>(0x41&31))&1 != 1 || (words[0x0D>>5]>>(0x0D&31))&1 != 1 {
		t.Errorf("键位位图缺位: %v", words)
	}
	// 只有位图，没有顺序 / 时间戳 / 字符 / 序列字段。
	for _, forbidden := range []string{"time", "char", "text", "window", "order", "sequence"} {
		if strings.Contains(data, forbidden) {
			t.Errorf("keys 帧含越界字段 %s: %s", forbidden, data)
		}
	}

	url := "http://" + s.Addr() + "/api/live?t=" + s.Token() + "&on=0"
	r2, err := client.Post(url, "application/json", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if data := waitFrame(t, r, "mode"); data != `{"on":false}` {
		t.Errorf("页面开关后未广播 mode: %s", data)
	}
}

func TestIdleShutdownReleasesPort(t *testing.T) {
	st := freshState(true)
	s := newTestServer(t, st, 60*time.Millisecond, 10*time.Millisecond)
	select {
	case <-st.onStop:
	case <-time.After(2 * time.Second):
		t.Fatal("无连接超时后未触发 OnStop")
	}
	// 端口必须真正释放，而不是留着一个半死的监听。
	deadline := time.Now().Add(time.Second)
	for {
		ln, err := net.Listen("tcp", s.Addr())
		if err == nil {
			_ = ln.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("关停后端口 %s 仍被占用: %v", s.Addr(), err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPortFallback(t *testing.T) {
	a, err := listenLoopback()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := listenLoopback()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	pa := a.Addr().(*net.TCPAddr).Port
	pb := b.Addr().(*net.TCPAddr).Port
	if pa < portStart || pa >= portStart+portAttempts || pb != pa+1 {
		t.Errorf("端口递增回退异常: %d -> %d", pa, pb)
	}
	if a.Addr().(*net.TCPAddr).IP.String() != "127.0.0.1" {
		t.Error("监听地址不是 127.0.0.1")
	}
}

func TestNoListenerLeakOnBadConfig(t *testing.T) {
	// Listen 失败时不应留下半初始化的 server 或 token。
	_, err := Start(Config{
		Listen: func() (net.Listener, error) { return nil, fmt.Errorf("模拟占用") },
	})
	if err == nil {
		t.Fatal("监听失败应返回错误")
	}
}
