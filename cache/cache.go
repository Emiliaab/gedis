package cache

import (
	"encoding/json"
	"fmt"
	"github.com/Emiliaab/gedis/datasource/mysql"
	lru_k "github.com/Emiliaab/gedis/lru-k"
	"gorm.io/gorm"
	"io"
	"sync"
	"time"
)

/*
*
cache代理，封装lru kv并提供并发控制
*/
type Cache struct {
	mutex     sync.Mutex
	lru       lru_k.Cache
	dirtyKeys chan string   // 脏key队列
	ticker    *time.Ticker  // 定时器，定期将缓存中的脏key持久化到磁盘
	stop      chan struct{} // 停止信号
	db        *gorm.DB
}

const (
	maxitems = 10
	chansize = 1024
)

func NewCache() Cache {
	c := Cache{
		dirtyKeys: make(chan string, chansize),
		ticker:    time.NewTicker(10 * time.Second),
		stop:      make(chan struct{}),
		db:        mysql.New(),
	}
	return c
}

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

	// 加入到脏key队列，如果队列满了就丢弃
	select {
	case c.dirtyKeys <- key:
	default:
		return
	}
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

func (c *Cache) GetAll() map[string]string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru == nil {
		return nil
	}
	ans := make(map[string]string)
	for k, v := range c.lru.GetAll() {
		ans[k] = string(v.(*gvalue).GetBytes())

	}
	return ans
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
	//var newData map[string]*list.Element
	//if err := json.NewDecoder(serialized).Decode(&newData); err != nil {
	//	return err
	//}
	//c.mutex.Lock()
	//defer c.mutex.Unlock()
	//
	//c.lru.SetData(newData)
	return nil
}

func (c *Cache) GetRangeData(start, end int) ([]byte, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru != nil {
		return c.lru.GetRangeData(start, end)
	}
	return nil, fmt.Errorf("LRU cache is not initialized")
}

func (c *Cache) FlushDirtyKeys() {
	for {
		select {
		case <-c.ticker.C:
			c.flushOnce()
		case <-c.stop:
			c.ticker.Stop()
			return
		}
	}
}

/*
*
一次刷盘操作，将脏key的chan里面的数据刷盘
*/
func (c *Cache) flushOnce() {
	for {
		select {
		case key := <-c.dirtyKeys:
			value, ok := c.lru.Get(key)
			if !ok {
				continue
			}
			// key-value持久化到mysql
			err := c.flushToDataSource(key, string(value.(*gvalue).GetBytes()))
			if !err {
				// 刷盘失败, 将 key 重新加入队列
				//select {
				//case c.dirtyKeys <- key:
				//default:
				//}
			}
		default:
			return
		}

	}
}

/*
*
将key-value持久化到相应的数据源
*/
func (c *Cache) flushToDataSource(key string, value string) bool {
	data := mysql.Data{Key: key}

	result := c.db.Where("gedis_key = ?", key).First(&data)
	if result.Error == nil {
		// 记录已存在,直接更新 Value 字段
		data.Value = value
		result := c.db.Model(&data).Where("gedis_key = ?", key).Update("gedis_value", value)
		if result.Error != nil {
			fmt.Errorf("datasource save data error, the reason is %e", result.Error)
			return false
		}
		return true
	}

	// 记录不存在,创建新记录
	result = c.db.Create(&mysql.Data{
		Key:   key,
		Value: value,
	})
	if result.Error != nil {
		fmt.Errorf("datasource create data error, the reason is %e", result.Error)
		return false
	}

	return true
}

func (c *Cache) Close() {
	close(c.stop)
	// 最后刷一次盘
	c.flushOnce()
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
