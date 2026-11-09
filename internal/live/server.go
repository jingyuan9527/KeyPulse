// Package live 提供可选的本地看板服务：只绑定 127.0.0.1 的高位端口，
// 用 SSE 推送当天聚合快照与「最近按过哪些键」的 256-bit 位图。
//
// 边界（与项目隐私承诺一致）：
//   - 默认不启动；只有用户在托盘主动勾选「本地看板」才创建监听。
//   - 绝不绑定 0.0.0.0、绝不对外发包、不需要任何第三方依赖。
//   - 单键信息只以聚合位图形式出现：无顺序、无时间戳、无字符、无窗口信息，
//     且只在进程内存与 loopback 之间流动，**不落盘**。
//   - 每次启动生成一次性 token，并校验 Host / Origin，防 DNS rebinding 与跨站读取。
//   - 页面全部断开超过 30 秒自动关停并释放端口。
package live

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	portStart         = 39117
	portAttempts      = 64
	tokenBytes        = 32
	pulseInterval     = 200 * time.Millisecond
	snapshotInterval  = time.Second
	keepAliveInterval = 15 * time.Second
	defaultIdle       = 30 * time.Second
	defaultWatch      = 5 * time.Second
)

// Config 用回调注入数据来源，使本包不依赖项目内任何其他包，便于离线单测。
type Config struct {
	Page      func(token string) (string, error) // 完整 HTML（服务端直出，不写文件）
	Snapshot  func() ([]byte, error)             // 当天聚合 JSON，与重新生成报告同源
	TakePulse func() [8]uint32                   // 取走并清空键位脉冲位图
	IsLive    func() bool
	SetLive   func(bool)
	OnStop    func() // 自动关停后的回调，用于复位托盘勾选状态

	// 以下仅用于单测注入，生产为零值。
	Listen        func() (net.Listener, error)
	IdleTimeout   time.Duration
	WatchInterval time.Duration
}

type Server struct {
	cfg   Config
	token string
	host  string
	port  int
	ln    net.Listener
	srv   *http.Server

	conns    atomic.Int32
	lastConn atomic.Int64 // UnixNano
	stopCh   chan struct{}
	stopOnce sync.Once
	done     chan struct{}

	idle  time.Duration
	watch time.Duration
}

// listenLoopback 只绑 IPv4 loopback，端口占用时向后递增，绝不监听外网接口。
func listenLoopback() (net.Listener, error) {
	var last error
	for p := portStart; p < portStart+portAttempts; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
		if err == nil {
			return ln, nil
		}
		last = err
	}
	return nil, fmt.Errorf("127.0.0.1:%d~%d 均无法绑定: %w", portStart, portStart+portAttempts-1, last)
}

func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Start 创建监听并返回可直接交给系统默认浏览器打开的 URL。
// 所有网络 IO 都在独立 goroutine，调用方（托盘菜单线程）不会阻塞。
func Start(c Config) (*Server, error) {
	var ln net.Listener
	var err error
	if c.Listen != nil {
		ln, err = c.Listen()
	} else {
		ln, err = listenLoopback()
	}
	if err != nil {
		return nil, err
	}
	tok, err := newToken()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	port := 0
	if ta, ok := ln.Addr().(*net.TCPAddr); ok {
		port = ta.Port
	}
	s := &Server{
		cfg:    c,
		token:  tok,
		host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		port:   port,
		ln:     ln,
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
		idle:   defaultIdle,
		watch:  defaultWatch,
	}
	if c.IdleTimeout > 0 {
		s.idle = c.IdleTimeout
	}
	if c.WatchInterval > 0 {
		s.watch = c.WatchInterval
	}
	s.lastConn.Store(time.Now().UnixNano())

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/api/live", s.handleToggle)
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() { _ = s.srv.Serve(ln) }()
	go s.loop()
	return s, nil
}

func (s *Server) URL() string   { return "http://" + s.host + "/?t=" + s.token }
func (s *Server) Port() int     { return s.port }
func (s *Server) Token() string { return s.token }
func (s *Server) Addr() string  { return s.host }

// Close 幂等：强制断开所有 SSE 连接并释放端口。手动关闭不触发 OnStop，
// 由调用方自己复位状态；自动关停由 loop 负责回调。
func (s *Server) Close() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		if s.srv != nil {
			_ = s.srv.Close()
		}
	})
}

func secureHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy",
		"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; "+
			"connect-src 'self'; img-src 'none'; form-action 'none'; base-uri 'none'")
}

// authorized 同时校验一次性 token、Host 与 Origin。
// Host 必须是 127.0.0.1:<port>：恶意域名经 DNS rebinding 指向本机时
// 请求的 Host 仍是那个域名，会被这里拒掉。
func (s *Server) authorized(r *http.Request) bool {
	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("t")), []byte(s.token)) != 1 {
		return false
	}
	if r.Host != s.host {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" && o != "http://"+s.host {
		return false
	}
	return true
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	secureHeaders(w)
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	doc, err := s.cfg.Page(s.token)
	if err != nil {
		http.Error(w, "report unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(doc))
}

func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request) {
	secureHeaders(w)
	if r.Method != http.MethodPost || !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	on := r.URL.Query().Get("on") == "1"
	s.cfg.SetLive(on)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = fmt.Fprintf(w, `{"on":%t}`, on)
}

func writeEvent(w http.ResponseWriter, f http.Flusher, event, data string) error {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	f.Flush()
	return nil
}

func pulseEmpty(b [8]uint32) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	secureHeaders(w)
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	s.conns.Add(1)
	s.lastConn.Store(time.Now().UnixNano())
	defer func() {
		s.conns.Add(-1)
		s.lastConn.Store(time.Now().UnixNano())
	}()

	mode := s.cfg.IsLive()
	if writeEvent(w, f, "mode", fmt.Sprintf(`{"on":%t}`, mode)) != nil {
		return
	}
	lastSnap := ""
	if mode {
		if b, err := s.cfg.Snapshot(); err == nil && len(b) > 0 {
			lastSnap = string(b)
			if writeEvent(w, f, "snapshot", lastSnap) != nil {
				return
			}
		}
	}

	pulseTk := time.NewTicker(pulseInterval)
	defer pulseTk.Stop()
	snapTk := time.NewTicker(snapshotInterval)
	defer snapTk.Stop()
	keepTk := time.NewTicker(keepAliveInterval)
	defer keepTk.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.stopCh:
			return
		case <-pulseTk.C:
			// 即使实时关闭也要取走并丢弃，避免打开后闪出一堆积压的旧键。
			bits := s.cfg.TakePulse()
			cur := s.cfg.IsLive()
			if cur != mode {
				mode = cur
				if writeEvent(w, f, "mode", fmt.Sprintf(`{"on":%t}`, mode)) != nil {
					return
				}
			}
			if !mode || pulseEmpty(bits) {
				continue
			}
			data, err := json.Marshal(bits)
			if err != nil {
				continue
			}
			if writeEvent(w, f, "keys", string(data)) != nil {
				return
			}
		case <-snapTk.C:
			if !mode {
				continue
			}
			b, err := s.cfg.Snapshot()
			if err != nil || len(b) == 0 {
				continue
			}
			cur := string(b)
			if cur == lastSnap {
				continue
			}
			lastSnap = cur
			if writeEvent(w, f, "snapshot", cur) != nil {
				return
			}
		case <-keepTk.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			f.Flush()
		}
	}
}

// loop 做两件事：没有 SSE 连接时持续清空脉冲位图；连接全部断开超过
// idle 时间后自动关停，释放端口并通知调用方复位托盘状态。
func (s *Server) loop() {
	defer close(s.done)
	watchTk := time.NewTicker(s.watch)
	defer watchTk.Stop()
	drainTk := time.NewTicker(pulseInterval)
	defer drainTk.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-drainTk.C:
			if s.conns.Load() == 0 {
				s.cfg.TakePulse()
			}
		case <-watchTk.C:
			if s.conns.Load() != 0 {
				continue
			}
			last := time.Unix(0, s.lastConn.Load())
			if time.Since(last) < s.idle {
				continue
			}
			s.Close()
			if s.cfg.OnStop != nil {
				go s.cfg.OnStop()
			}
			return
		}
	}
}
