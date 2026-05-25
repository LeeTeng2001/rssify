package cache

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNewCreatesCacheDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "cache")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Stat() error = %v, want IsNotExist", err)
	}

	if _, err := New(dir); err != nil {
		t.Fatalf("New() error = %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("New() created non-directory at %q", dir)
	}
}

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
	info, err := os.Stat(filepath.Join(dir, "hn.xml"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("disk file mode = %v, want %v", mode, os.FileMode(0o600))
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

func TestLoadExistingSkipsMissingIDs(t *testing.T) {
	t.Parallel()

	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := c.LoadExisting([]string{"missing"}); err != nil {
		t.Fatalf("LoadExisting() error = %v", err)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("Get() ok = true for missing id, want false")
	}
}

func TestLoadExistingReturnsReadErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "hn.xml"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	c, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = c.LoadExisting([]string{"hn"})
	if err == nil {
		t.Fatal("LoadExisting() error = nil, want error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadExisting() error = %v, want non-IsNotExist error", err)
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
