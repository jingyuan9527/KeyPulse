// Package store 实现按天的固定长度二进制快照。
//
// 文件布局（1536 字节，固定）：
//
//	0    magic[4]="KP01" | day uint32 | version uint32 | reserved[20]
//	32   keyCounts   [256]uint32   (1024 B)
//	1056 mouseCounts [8]uint32     (32 B)
//	1088 hourPairs   [48]uint32    (192 B)
//	1280 minuteActive bitmap       (180 B)
//	1460 reserved                 (76 B)
//
// 只写"脏槽"：flush 时按脏位图把变化的 4 字节累计值 WriteAt 回原位置。
// 没有 SQL 引擎、没有 WAL、没有索引、没有 VACUUM，一天 1.5 KB。
package store

import (
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"keypulse/internal/counter"
)

const (
	FileSize   = 1536
	OffKeys    = 32
	OffMouse   = OffKeys + counter.KeySlots*4
	OffHours   = OffMouse + counter.MouseSlots*4
	OffMinutes = OffHours + counter.HourSlots*4
	Magic      = "KP01"
	IdleTicks  = 30 // 连续 30 秒无输入后彻底停止写盘
)

type DayData struct {
	Stamp   string // YYYYMMDD
	Day     uint32
	Keys    [counter.KeySlots]uint32
	Mouse   [counter.MouseSlots]uint32
	Hours   [counter.HourSlots]uint32
	Minutes [180]byte
}

func (d DayData) TotalKeys() uint32 {
	var n uint32
	for _, v := range d.Keys {
		n += v
	}
	return n
}

func (d DayData) TotalClicks() uint32 {
	var n uint32
	for i := 0; i < 5; i++ { // 左/右/中/X1/X2，滚轮不计入点击
		n += d.Mouse[i]
	}
	return n
}

func (d DayData) Wheel() uint32 { return d.Mouse[counter.MouseWheel-counter.KeySlots] }

func (d DayData) ActiveMinutes() int {
	n := 0
	for _, b := range d.Minutes {
		for b != 0 {
			n += int(b & 1)
			b >>= 1
		}
	}
	return n
}

func (d DayData) Date() string {
	if len(d.Stamp) != 8 {
		return d.Stamp
	}
	return d.Stamp[:4] + "-" + d.Stamp[4:6] + "-" + d.Stamp[6:8]
}

type Store struct {
	mu        sync.Mutex
	dir       string
	c         *counter.Counter
	file      *os.File
	stamp     string
	day       uint32
	last      [counter.SlotCount]uint32
	deltaBuf  [counter.SlotCount]uint32
	idleTicks int
}

func dayNumber(t time.Time) uint32 {
	y, m, d := t.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	return uint32(midnight.Unix() / 86400)
}

func Open(dir string, c *counter.Counter) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, c: c}
	if err := s.openDay(time.Now()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) openDay(t time.Time) error {
	stamp := t.Format("20060102")
	day := dayNumber(t)
	path := filepath.Join(s.dir, stamp+".bin")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	if fi.Size() < FileSize {
		buf := make([]byte, FileSize)
		copy(buf[0:4], Magic)
		binary.LittleEndian.PutUint32(buf[4:8], day)
		binary.LittleEndian.PutUint32(buf[8:12], 1)
		if _, err := f.WriteAt(buf, 0); err != nil {
			f.Close()
			return err
		}
	} else {
		data, err := ReadDayFile(path)
		if err != nil {
			f.Close()
			return err
		}
		for i, v := range data.Keys {
			s.c.AddN(i, v)
		}
		for i, v := range data.Mouse {
			s.c.AddN(counter.KeySlots+i, v)
		}
		for i, v := range data.Hours {
			s.c.AddN(counter.HourBase+i, v)
		}
		s.c.LoadMinuteBitmap(data.Minutes)
		s.c.ClearDirty()
	}
	if s.file != nil {
		s.file.Close()
	}
	s.file, s.stamp, s.day = f, stamp, day
	for i := range s.last {
		s.last[i] = s.c.Slot(i)
	}
	s.idleTicks = 0
	return nil
}

func slotOffset(slot int) int64 {
	switch {
	case slot < counter.KeySlots:
		return int64(OffKeys + slot*4)
	case slot < counter.KeySlots+counter.MouseSlots:
		return int64(OffMouse + (slot-counter.KeySlots)*4)
	default:
		return int64(OffHours + (slot-counter.HourBase)*4)
	}
}

// Flush 把内存增量结算到小时/分钟维度，再只写脏槽。
// force=true 用于退出、暂停、跨天等必须落盘的场景。
func (s *Store) Flush(force bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if now.Format("20060102") != s.stamp {
		s.c.ResetForNewDay()
		for i := range s.last {
			s.last[i] = 0
		}
		if err := s.openDay(now); err != nil {
			return false, err
		}
	}

	changed := false
	for i := range s.deltaBuf {
		v := s.c.Slot(i)
		if v != s.last[i] {
			s.deltaBuf[i] = v - s.last[i]
			changed = true
		} else {
			s.deltaBuf[i] = 0
		}
	}

	if changed {
		var keyDelta, clickDelta uint32
		for i := 0; i < counter.KeySlots; i++ {
			keyDelta += s.deltaBuf[i]
		}
		for i := 0; i < 5; i++ {
			clickDelta += s.deltaBuf[counter.KeySlots+i]
		}
		h := now.Hour()
		s.c.AddN(counter.HourBase+h*2, keyDelta)
		s.c.AddN(counter.HourBase+h*2+1, clickDelta)
		s.c.SetMinute(h, now.Minute())
		s.idleTicks = 0
	} else {
		s.idleTicks++
	}

	dirty := s.c.TakeDirty()
	minutesDirty := s.c.TakeMinuteDirty()
	if !force && len(dirty) == 0 && !minutesDirty && s.idleTicks > IdleTicks {
		return false, nil
	}

	if len(dirty) > 0 {
		sort.Ints(dirty)
		for i := 0; i < len(dirty); {
			j := i + 1
			for j < len(dirty) && dirty[j] == dirty[j-1]+1 {
				j++
			}
			buf := make([]byte, (j-i)*4)
			for k := i; k < j; k++ {
				binary.LittleEndian.PutUint32(buf[(k-i)*4:], s.c.Slot(dirty[k]))
			}
			if _, err := s.file.WriteAt(buf, slotOffset(dirty[i])); err != nil {
				return false, err
			}
			i = j
		}
	}
	if minutesDirty {
		mb := s.c.MinuteBitmap()
		if _, err := s.file.WriteAt(mb[:], OffMinutes); err != nil {
			return false, err
		}
	}
	for i := range s.last {
		s.last[i] = s.c.Slot(i)
	}
	return len(dirty) > 0 || minutesDirty, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

func (s *Store) Dir() string { return s.dir }

func ReadDayFile(path string) (DayData, error) {
	var d DayData
	f, err := os.Open(path)
	if err != nil {
		return d, err
	}
	defer f.Close()
	buf := make([]byte, FileSize)
	if _, err := io.ReadFull(f, buf); err != nil {
		return d, err
	}
	if string(buf[0:4]) != Magic {
		return d, fmt.Errorf("非法数据文件: %s", path)
	}
	base := filepath.Base(path)
	d.Stamp = strings.TrimSuffix(base, filepath.Ext(base))
	d.Day = binary.LittleEndian.Uint32(buf[4:8])
	for i := range d.Keys {
		d.Keys[i] = binary.LittleEndian.Uint32(buf[OffKeys+i*4:])
	}
	for i := range d.Mouse {
		d.Mouse[i] = binary.LittleEndian.Uint32(buf[OffMouse+i*4:])
	}
	for i := range d.Hours {
		d.Hours[i] = binary.LittleEndian.Uint32(buf[OffHours+i*4:])
	}
	copy(d.Minutes[:], buf[OffMinutes:OffMinutes+180])
	return d, nil
}

func WriteDayFile(dir string, d DayData) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	buf := make([]byte, FileSize)
	copy(buf[0:4], Magic)
	binary.LittleEndian.PutUint32(buf[4:8], d.Day)
	binary.LittleEndian.PutUint32(buf[8:12], 1)
	for i, v := range d.Keys {
		binary.LittleEndian.PutUint32(buf[OffKeys+i*4:], v)
	}
	for i, v := range d.Mouse {
		binary.LittleEndian.PutUint32(buf[OffMouse+i*4:], v)
	}
	for i, v := range d.Hours {
		binary.LittleEndian.PutUint32(buf[OffHours+i*4:], v)
	}
	copy(buf[OffMinutes:], d.Minutes[:])
	return os.WriteFile(filepath.Join(dir, d.Stamp+".bin"), buf, 0o644)
}

// ListDays 返回最近 limit 天的数据，按日期升序。
func ListDays(dir string, limit int) ([]DayData, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var stamps []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".bin") || len(n) != 12 {
			continue
		}
		if _, err := time.Parse("20060102", n[:8]); err == nil {
			stamps = append(stamps, n[:8])
		}
	}
	sort.Strings(stamps)
	if limit > 0 && len(stamps) > limit {
		stamps = stamps[len(stamps)-limit:]
	}
	out := make([]DayData, 0, len(stamps))
	for _, st := range stamps {
		d, err := ReadDayFile(filepath.Join(dir, st+".bin"))
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func Cleanup(dir string, keepDays int) (int, error) {
	if keepDays <= 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("20060102")
	n := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".bin") || len(name) != 12 {
			continue
		}
		if name[:8] < cutoff {
			if err := os.Remove(filepath.Join(dir, name)); err == nil {
				n++
			}
		}
	}
	return n, nil
}

func ExportCSV(w io.Writer, days []DayData) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	head := []string{"日期", "按键次数", "鼠标点击", "滚轮格数", "活跃分钟"}
	for h := 0; h < 24; h++ {
		head = append(head, fmt.Sprintf("%02d时按键", h), fmt.Sprintf("%02d时点击", h))
	}
	if err := cw.Write(head); err != nil {
		return err
	}
	for _, d := range days {
		row := []string{
			d.Date(),
			strconv.FormatUint(uint64(d.TotalKeys()), 10),
			strconv.FormatUint(uint64(d.TotalClicks()), 10),
			strconv.FormatUint(uint64(d.Wheel()), 10),
			strconv.Itoa(d.ActiveMinutes()),
		}
		for h := 0; h < 24; h++ {
			row = append(row,
				strconv.FormatUint(uint64(d.Hours[h*2]), 10),
				strconv.FormatUint(uint64(d.Hours[h*2+1]), 10))
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}
