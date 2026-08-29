package wal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func TestWriter_WriteAndRead(t *testing.T) {
	dir := newTestDir(t)
	w, err := NewWriter(dir, 1024*1024)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	seq, err := w.Write(Entry{
		Type: EntryTypeData,
		Key:  "test:key1",
		Data: []byte("hello"),
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if seq != 1 {
		t.Fatalf("expected sequence 1, got %d", seq)
	}

	seq2, err := w.Write(Entry{
		Type: EntryTypeDelete,
		Key:  "test:key2",
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if seq2 != 2 {
		t.Fatalf("expected sequence 2, got %d", seq2)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read all
	r, err := NewReader(dir)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Key != "test:key1" || string(entries[0].Data) != "hello" {
		t.Fatalf("unexpected entry 0: %+v", entries[0])
	}
	if entries[1].Key != "test:key2" || entries[1].Type != EntryTypeDelete {
		t.Fatalf("unexpected entry 1: %+v", entries[1])
	}
}

func TestWriter_LastSequence(t *testing.T) {
	dir := newTestDir(t)
	w, err := NewWriter(dir, 1024*1024)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	if w.LastSequence() != 0 {
		t.Fatalf("expected initial sequence 0, got %d", w.LastSequence())
	}

	w.Write(Entry{Type: EntryTypeData, Key: "k1"})
	if w.LastSequence() != 1 {
		t.Fatalf("expected sequence 1, got %d", w.LastSequence())
	}

	w.Write(Entry{Type: EntryTypeData, Key: "k2"})
	if w.LastSequence() != 2 {
		t.Fatalf("expected sequence 2, got %d", w.LastSequence())
	}
}

func TestWriter_Rotate(t *testing.T) {
	dir := newTestDir(t)
	// Use a small max size to force rotation
	w, err := NewWriter(dir, 50)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	// Write enough entries to trigger rotation
	for i := 0; i < 20; i++ {
		key := string(rune('a' + i))
		_, err := w.Write(Entry{Type: EntryTypeData, Key: key, Data: make([]byte, 10)})
		if err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	// Verify multiple files exist
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	logFiles := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".log" {
			logFiles++
		}
	}
	if logFiles < 2 {
		t.Fatalf("expected at least 2 log files after rotation, got %d", logFiles)
	}

	// Read all should return all entries
	r, err := NewReader(dir)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	all, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(all) != 20 {
		t.Fatalf("expected 20 entries, got %d", len(all))
	}

	// Verify sequences are monotonically increasing
	for i, e := range all {
		if e.Sequence != uint64(i+1) {
			t.Fatalf("entry %d: expected sequence %d, got %d", i, i+1, e.Sequence)
		}
	}
}

func TestReadFrom(t *testing.T) {
	dir := newTestDir(t)
	w, err := NewWriter(dir, 1024*1024)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		w.Write(Entry{Type: EntryTypeData, Key: key})
	}
	w.Close()

	r, err := NewReader(dir)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	// ReadFrom sequence 3
	entries, err := r.ReadFrom(3)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (seq 3,4,5), got %d", len(entries))
	}
	if entries[0].Sequence != 3 {
		t.Fatalf("first entry should have sequence 3, got %d", entries[0].Sequence)
	}

	// ReadFrom 0 should return all
	all, err := r.ReadFrom(0)
	if err != nil {
		t.Fatalf("ReadFrom(0): %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(all))
	}
}

func TestUnmarshalWAL_Invalid(t *testing.T) {
	// Too short
	_, err := UnmarshalWAL([]byte{0, 0, 0})
	if err == nil {
		t.Fatal("expected error for short data")
	}

	// Invalid CRC
	_, err = UnmarshalWAL([]byte{
		1,                                     // Type
		0, 0, 0, 0, 0, 0, 0, 0,               // Timestamp
		0, 0, 0, 0, 0, 0, 0, 0,               // Sequence
		0, 1,                                  // KeyLen=1
		'x',                                   // Key
		0, 0, 0, 0,                            // Checksum
		0, 0, 0, 0, 0, 0, 0, 0,               // Data (8 bytes)
		0, 0, 0, 0,                            // TrailingCRC (wrong)
	})
	if err == nil {
		t.Fatal("expected error for invalid CRC")
	}
}

func TestCompressDecompress(t *testing.T) {
	original := []byte("hello world, this is a test of zstd compression in WAL")

	compressed, err := Compress(original)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if len(compressed) == 0 {
		t.Fatal("compressed data should not be empty")
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if string(decompressed) != string(original) {
		t.Fatalf("decompressed data mismatch: got %s", string(decompressed))
	}
}

func TestCompressDecompress_Empty(t *testing.T) {
	compressed, err := Compress(nil)
	if err != nil {
		t.Fatalf("Compress nil: %v", err)
	}
	if compressed != nil {
		t.Fatal("expected nil for empty input")
	}

	decompressed, err := Decompress(nil)
	if err != nil {
		t.Fatalf("Decompress nil: %v", err)
	}
	if decompressed != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestCheckpointer(t *testing.T) {
	dir := newTestDir(t)
	cp := NewCheckpointer(dir, time.Second)

	if cp.ShouldCheckpoint() {
		t.Fatal("should not need checkpoint immediately")
	}

	if err := cp.MarkCheckpoint(); err != nil {
		t.Fatalf("MarkCheckpoint: %v", err)
	}

	// Verify checkpoint marker file exists
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 10 && e.Name()[:10] == "checkpoint" {
			found = true
		}
	}
	if !found {
		t.Fatal("checkpoint marker file not found")
	}
}

func TestConcurrentWrites(t *testing.T) {
	dir := newTestDir(t)
	w, err := NewWriter(dir, 1024*1024)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	const goroutines = 10
	const writesPerGoroutine = 100

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < writesPerGoroutine; j++ {
				key := fmt.Sprintf("goroutine-%d-write-%d", id, j)
				w.Write(Entry{
					Type: EntryTypeData,
					Key:  key,
					Data: []byte(key),
				})
			}
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Verify all entries were written
	r, err := NewReader(dir)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	all, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	expected := goroutines * writesPerGoroutine
	if len(all) != expected {
		t.Fatalf("expected %d entries, got %d", expected, len(all))
	}

	// Verify sequences are unique and monotonically increasing
	seen := make(map[uint64]bool)
	for _, e := range all {
		if seen[e.Sequence] {
			t.Fatalf("duplicate sequence %d", e.Sequence)
		}
		seen[e.Sequence] = true
	}
}