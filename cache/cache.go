package cache

import (
	lru_k "github.com/Emiliaab/gedis/lru-k"
	"io"
	"sync"
)

/*
*
cache代理，封装lru kv并提供并发控制
*/
type cache struct {
	mutex sync.Mutex
	lru   lru_k.Cache
}

const (
	maxitems = 10
)

func (c *cache) Add(key string, value []byte) {
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

func (c *cache) Get(key string) (value []byte, ok bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru == nil {
		value, ok = nil, false
	}
	gv, ok := c.lru.Get(key)
	value = gv.(*gvalue).GetBytes()
	return
}

func (c *cache) Remove(key string) (ok bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru == nil {
		ok = false
	}

	return
}

func (c *cache) Marshal() ([]byte, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// TODO 序列化所有数据
	return nil, nil
}

func (c *cache) UnMarshal(serialized io.ReadCloser) error {
	//var newData map[string]string
	// TODO 反序列化
	//newData = nil
	c.mutex.Lock()
	defer c.mutex.Unlock()

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
