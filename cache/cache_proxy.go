package cache

import (
	"github.com/Emiliaab/gedis/datasource/mysql"
	"gorm.io/gorm"
	"log"
	"sync"
)

/**
cache代理，封装lru kv并提供并发控制
 */
type cache_proxy struct {
	mutex sync.Mutex
	lru   lru.Cache
}

func (c *cache_proxy) add(key string, value []byte) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru == nil {
		c.lru =
	}
	c.lru.Add(key, value)
}

func (c *cache_proxy) get(key string) (value []byte, ok bool){
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru == nil {
		value, ok = nil, false
	}
	value, ok = c.lru.Get(key)
	return
}

func (c *cache_proxy) remove(key string) (ok bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.lru == nil {
		ok = false
	}
	ok = lru.Remove(key)
	return
}

/**
group维护一个命名空间
 */
type Group struct {
	name string   // 命名空间
	localCache cache_proxy  // 本地缓存
	db *gorm.DB
}

var (
	mu   sync.RWMutex
	groups  = make(map[string]*Group)
)

func NewGroup(name string) (g *Group) {
	mu.Lock()
	defer mu.Unlock()
	g = &Group {
		name: name,
		db: mysql.New(),
	}
	groups[name] = g
	return
}

func GetGroup(name string) (g *Group) {
	mu.RLock()
	defer mu.RUnlock()
	g = groups[name]
	return
}

func (g *Group) Get(key string) ([]byte, bool) {
	// 1. 从local cache中取
	if v, ok := g.localCache.get(key); ok {
		log.Println("local cache hit!")
		return v, ok
	}
	// 2. 从remote peers中获取

	// 3. 从mysql数据源中获取
	if v, ok := g.getFromDataSource(key); ok {
		log.Println("datasource cache hit!")
		g.localCache.add(key, v)
		return v, ok
	}
	return nil, false
}

func (g *Group) Write(key string, value []byte) bool{
	// TODO 应该通过一致性Hash判断位于哪个节点？
	// 这里假定指定了就是写入到本节点的缓存中
	g.localCache.add(key, value)

	if ok := g.writeToDataSource(key, value);ok {
		// 保证数据库缓存的一致性，则只有写入数据库成功才响应给客户端成功，否则回滚缓存
		return true
	} else {
		g.localCache.remove(key)
		return false
	}
}

/**
写入数据源（mysql）中数据
 */
func (g *Group) writeToDataSource(key string, value []byte) bool {
	// 要插入的记录
	data := mysql.Data{
		Key:   key,
		Value: string(value),
	}

	// 保存记录到数据库
	result := g.db.Create(&data)
	if result.Error != nil {
		log.Println("写入数据库失败!", result.Error)
		return false
	}
	return true
}

/**
从数据源（mysql）中获取value
 */
func (g *Group) getFromDataSource(key string) ([]byte, bool) {
	var data mysql.Data
	result := g.db.Where("key = ?", key).First(&data)
	if result.Error != nil {
		log.Println("查询数据库失败!", result.Error)
		return nil, false
	}
	return []byte(data.Value), true
}




