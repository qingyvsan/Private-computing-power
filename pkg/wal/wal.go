package wal

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EntryType WAL 条目类型
type EntryType byte

const (
	EntryTypeData   EntryType = 1 // 数据变更
	EntryTypeMeta   EntryType = 2 // 元数据变更
	EntryTypeCheck  EntryType = 3 // 检查点标记
	EntryTypeDelete EntryType = 4 // 删除操作
)

// Entry WAL 条目
type Entry struct {
	Type      EntryType `json:"type"`
	Timestamp int64     `json:"timestamp"`
	Key       string    `json:"key"`
	Data      []byte    `json:"data"`
	Checksum  uint32    `json:"checksum"`
}

// Writer WAL 写入器
type Writer struct {
	mu       sync.Mutex
	dir      string
	file     *os.File
	size     int64
	maxSize  int64 // 超过此大小自动轮转
	sequence uint64
}

// NewWriter 创建 WAL 写入器
func NewWriter(dir string, maxSize int64) (*Writer, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create WAL dir: %w", err)
	}

	w := &Writer{
		dir:     dir,
		maxSize: maxSize,
	}
	if err := w.openNewFile(); err != nil {
		return nil, err
	}
	return w, nil
}

// Write 写入一条 WAL 条目
func (w *Writer) Write(entry Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.size >= w.maxSize {
		if err := w.rotate(); err != nil {
			return err
		}
	}

	entry.Timestamp = time.Now().UnixNano()
	entry.Checksum = crc32(entry.Data)

	data := entry.Marshal()
	n, err := w.file.Write(data)
	if err != nil {
		return fmt.Errorf("write WAL entry: %w", err)
	}
	w.size += int64(n)
	return nil
}

// rotate 轮转 WAL 文件
func (w *Writer) rotate() error {
	if w.file != nil {
		w.file.Close()
	}
	// 重命名当前文件为归档
	oldPath := filepath.Join(w.dir, fmt.Sprintf("wal-%06d.log", w.sequence))
	os.Rename(w.file.Name(), oldPath)
	return w.openNewFile()
}

func (w *Writer) openNewFile() error {
	w.sequence++
	path := filepath.Join(w.dir, fmt.Sprintf("wal-%06d.log", w.sequence))
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create WAL file: %w", err)
	}
	w.file = f
	w.size = 0
	return nil
}

// Close 关闭 WAL 写入器
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Marshal 序列化 WAL 条目
func (e Entry) Marshal() []byte {
	keyLen := len(e.Key)
	dataLen := len(e.Data)
	buf := make([]byte, 1+8+2+keyLen+4+dataLen+4)
	offset := 0

	buf[offset] = byte(e.Type)
	offset++

	binary.BigEndian.PutUint64(buf[offset:], uint64(e.Timestamp))
	offset += 8

	binary.BigEndian.PutUint16(buf[offset:], uint16(keyLen))
	offset += 2

	copy(buf[offset:], e.Key)
	offset += keyLen

	binary.BigEndian.PutUint32(buf[offset:], e.Checksum)
	offset += 4

	copy(buf[offset:], e.Data)
	offset += dataLen

	binary.BigEndian.PutUint32(buf[offset:], crc32(buf[:offset]))
	offset += 4

	return buf[:offset]
}

// UnmarshalWAL 反序列化 WAL 条目
func UnmarshalWAL(data []byte) (*Entry, error) {
	if len(data) < 1+8+2+4+4 {
		return nil, fmt.Errorf("WAL entry too short")
	}

	offset := 0
	e := &Entry{}

	e.Type = EntryType(data[0])
	offset++

	e.Timestamp = int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	keyLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2

	if offset+keyLen > len(data) {
		return nil, fmt.Errorf("WAL entry key truncated")
	}
	e.Key = string(data[offset : offset+keyLen])
	offset += keyLen

	e.Checksum = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	dataLen := len(data) - offset - 4
	if dataLen < 0 {
		return nil, fmt.Errorf("WAL entry data truncated")
	}
	e.Data = make([]byte, dataLen)
	copy(e.Data, data[offset:offset+dataLen])
	offset += dataLen

	// 验证校验和
	storedCRC := binary.BigEndian.Uint32(data[offset:])
	calculatedCRC := crc32(data[:offset])
	if storedCRC != calculatedCRC {
		return nil, fmt.Errorf("WAL entry checksum mismatch")
	}

	return e, nil
}

// Reader WAL 读取器
type Reader struct {
	dir   string
	files []string
	pos   int
	file  *os.File
}

// NewReader 创建 WAL 读取器
func NewReader(dir string) (*Reader, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read WAL dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".log" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}

	return &Reader{
		dir:   dir,
		files: files,
	}, nil
}

// ReadAll 读取所有 WAL 条目
func (r *Reader) ReadAll() ([]*Entry, error) {
	var entries []*Entry
	for _, path := range r.files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read WAL file %s: %w", path, err)
		}
		offset := 0
		for offset < len(data) {
			entry, err := UnmarshalWAL(data[offset:])
			if err != nil {
				// 可能文件末尾有截断，忽略
				break
			}
			entries = append(entries, entry)
			// 计算实际消耗的字节数
			entryLen := 1 + 8 + 2 + len(entry.Key) + 4 + len(entry.Data) + 4
			offset += entryLen
		}
	}
	return entries, nil
}

// Close 关闭读取器
func (r *Reader) Close() error {
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

// crc32 简单的 CRC32 校验
func crc32(data []byte) uint32 {
	var crc uint32 = 0xFFFFFFFF
	for _, b := range data {
		crc ^= uint32(b) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}
	return crc ^ 0xFFFFFFFF
}

// SyncClient WAL 同步客户端（用于备节点拉取主节点 WAL）
type SyncClient struct {
	reader *Reader
	dir    string
}

// NewSyncClient 创建 WAL 同步客户端
func NewSyncClient(dir string) (*SyncClient, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create sync dir: %w", err)
	}
	return &SyncClient{dir: dir}, nil
}

// ProcessWALData 处理从主节点接收到的 WAL 数据
func (s *SyncClient) ProcessWALData(data []byte) (*Entry, error) {
	return UnmarshalWAL(data)
}

// Checkpointer 检查点管理器
type Checkpointer struct {
	dir       string
	interval  time.Duration
	lastCheck time.Time
}

// NewCheckpointer 创建检查点管理器
func NewCheckpointer(dir string, interval time.Duration) *Checkpointer {
	return &Checkpointer{
		dir:      dir,
		interval: interval,
	}
}

// ShouldCheckpoint 检查是否需要创建检查点
func (c *Checkpointer) ShouldCheckpoint() bool {
	return time.Since(c.lastCheck) > c.interval
}

// MarkCheckpoint 标记检查点
func (c *Checkpointer) MarkCheckpoint() error {
	c.lastCheck = time.Now()
	// 创建检查点标记文件
	path := filepath.Join(c.dir, fmt.Sprintf("checkpoint-%d", c.lastCheck.Unix()))
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create checkpoint marker: %w", err)
	}
	f.Close()
	return nil
}

// Cleanup 清理旧的 WAL 文件（检查点之前的）
func (c *Checkpointer) Cleanup(walDir string, keepAfter time.Time) error {
	entries, err := os.ReadDir(walDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".log" {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(keepAfter) {
				os.Remove(filepath.Join(walDir, e.Name()))
			}
		}
	}
	return nil
}