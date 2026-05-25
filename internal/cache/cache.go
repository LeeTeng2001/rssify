package cache

import (
	"os"
	"path/filepath"
	"sync"
)

type Cache struct {
	dir  string
	mu   sync.RWMutex
	data map[string][]byte
}

func New(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Cache{
		dir:  dir,
		data: make(map[string][]byte),
	}, nil
}

func (c *Cache) Get(id string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	xml, ok := c.data[id]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), xml...), true
}

func (c *Cache) Put(id string, xml []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	path := filepath.Join(c.dir, id+".xml")
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, xml, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	c.data[id] = append([]byte(nil), xml...)
	return nil
}

func (c *Cache) LoadExisting(ids []string) error {
	for _, id := range ids {
		xml, err := os.ReadFile(filepath.Join(c.dir, id+".xml"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}

		c.mu.Lock()
		c.data[id] = append([]byte(nil), xml...)
		c.mu.Unlock()
	}
	return nil
}
