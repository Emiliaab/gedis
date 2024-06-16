package cache

import (
	"container/list"
	"encoding/json"
	lru_k "github.com/Emiliaab/gedis/lru-k"
	"io"
	"sync"
)

/*
*
cache代理，封装lru kv并提供并发控制
*/
type Cache struct {
	mutex sync.Mutex
	lru   lru_k.Cache
}

const (
	maxitems = 10
)

func (c *Cache) Add(key string, value []byte) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru == nil {
		OnEliminateKeys := make([]string, 0)
		OnEliminateFun := func(key string, value any) {
			OnEliminateKeys = append(OnEliminateKeys, key)
		}
		c.lru = lru_k.NewCache(2, int64(maxitems*4), lru_k.WithOnEliminate(OnEliminateFun))
	}
	c.lru.Set(key, &gvalue{bytes: value})
}

func (c *Cache) Get(key string) (value []byte, ok bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru == nil {
		return nil, false
	}
	gv, ok := c.lru.Get(key)
	value = gv.(*gvalue).GetBytes()
	return
}

func (c *Cache) Remove(key string) (ok bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru == nil {
		ok = false
	}

	return c.Remove(key)
}

func (c *Cache) Marshal() ([]byte, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	dataBytes, err := json.Marshal(c.lru.GetData())
	return dataBytes, err
}

func (c *Cache) UnMarshal(serialized io.ReadCloser) error {
	//var newData map[string]string
	var newData map[string]*list.Element
	if err := json.NewDecoder(serialized).Decode(&newData); err != nil {
		return err
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lru.SetData(newData)
	return nil
}

type gvalue struct {
	bytes []byte
}

func (g *gvalue) Len() int {
	return len(g.bytes)
}

func (g *gvalue) GetBytes() []byte {
	return g.bytes
}
