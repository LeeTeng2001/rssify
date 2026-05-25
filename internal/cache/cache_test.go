package cache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPutGetAndDiskWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := c.Put("hn", []byte("<rss/>")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, ok := c.Get("hn")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if string(got) != "<rss/>" {
		t.Fatalf("Get() = %q, want %q", got, "<rss/>")
	}
	got[0] = 'x'
	again, ok := c.Get("hn")
	if !ok {
		t.Fatal("Get() after mutation ok = false, want true")
	}
	if string(again) != "<rss/>" {
		t.Fatalf("Get() returned mutable data = %q, want %q", again, "<rss/>")
	}

	disk, err := os.ReadFile(filepath.Join(dir, "hn.xml"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(disk) != "<rss/>" {
		t.Fatalf("disk file = %q, want %q", disk, "<rss/>")
	}
}

func TestLoadExisting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hn.xml"), []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	c, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := c.LoadExisting([]string{"hn"}); err != nil {
		t.Fatalf("LoadExisting() error = %v", err)
	}

	got, ok := c.Get("hn")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if string(got) != "old" {
		t.Fatalf("Get() = %q, want %q", got, "old")
	}
}

func TestConcurrentGetPut(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Put("hn", []byte("x")); err != nil {
				t.Errorf("Put() error = %v", err)
			}
			_, _ = c.Get("hn")
		}()
	}
	wg.Wait()
}
