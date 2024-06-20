package cache

import (
	"encoding/json"
	"fmt"
	"github.com/Emiliaab/gedis/consistenthash"
	"github.com/Emiliaab/gedis/singleflight"
	"github.com/hashicorp/raft"
	"github.com/spaolacci/murmur3"
	"io"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

const (
	ENABLE_WRITE_TRUE  = int32(1)
	ENABLE_WRITE_FALSE = int32(0)
)

type Cache_proxy struct {
	Opts        *Options
	Log         *log.Logger
	Cache       Cache
	Raft        *RaftNodeInfo
	Peers       *consistenthash.Map
	sfGroup     singleflight.Group
	enableWrite int32
}

func NewCacheProxy(httpPort int32, raftPort int32, node string, bootstrap bool, joinAddress string) *Cache_proxy {
	proxy := &Cache_proxy{}
	opts := NewOptions(httpPort, raftPort, node, bootstrap, joinAddress)
	log := log.New(os.Stderr, "Cache_proxy: ", log.Ldate|log.Ltime)
	raftNode, err := NewRaftNode(opts, proxy)
	if err != nil {
		log.Fatal("gedisraft create error!")
	}
	proxy.Opts = opts
	proxy.Log = log
	proxy.Raft = raftNode
	proxy.Cache = NewCache()
	proxy.enableWrite = ENABLE_WRITE_FALSE
	proxy.Peers = consistenthash.New(3, func(key []byte) uint32 {
		return uint32(murmur3.Sum64(key))
	})
	proxy.Peers.Add(proxy.Opts.HttpAddress)

	return proxy
}

func (c *Cache_proxy) checkWritePermission() bool {
	return atomic.LoadInt32(&c.enableWrite) == ENABLE_WRITE_TRUE
}

func (c *Cache_proxy) DoGet(key string, masterAddress string) ([]byte, bool) {
	if key == "" {
		log.Println("doGet() error, get nil key")
		return nil, false
	}

	// 尝试从本地缓存获取数据
	value, ok := c.Cache.Get(key)
	if ok {
		return value, true
	}

	// 使用 singleflight 来保证对于相同的 key 只有一个网络请求被发起
	result, err := c.sfGroup.Do(key, func() (interface{}, error) {
		// 确定应该从哪个节点获取数据
		if masterAddress == "" {
			peerAddress := c.Peers.Get(key)
			if peerAddress == c.Opts.HttpAddress {
				// 如果哈希算法确定本地是负责节点，说明前面缓存未命中已是正确结果
				return nil, fmt.Errorf("data not found locally")
			}
			// 从对应的远端节点获取数据
			return GetFromPeer(peerAddress, key)
		}
		// 如果提供了主节点地址，则直接从该地址获取数据
		return GetFromPeer(masterAddress, key)
	})

	if err != nil {
		log.Printf("DoGet singleflight failed, err: %v, shared: %t", err)
		return nil, false
	}

	// 类型断言以匹配返回类型
	finalValue, ok := result.([]byte)
	if !ok {
		log.Println("DoGet type assertion failed")
		return nil, false
	}
	return finalValue, true
}

func GetFromPeer(address string, key string) (res []byte, err error) {
	// 输出日志，表示一次网络请求
	log.Printf("发起网络请求:%s\n", key)
	url := "http://" + address + "/get?key=" + key
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, err
	}
	res, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed, err:%v", err)
	}
	return res, nil
}

func (c *Cache_proxy) SetWriteFlag(flag bool) {
	if flag {
		atomic.StoreInt32(&c.enableWrite, ENABLE_WRITE_TRUE)
	} else {
		atomic.StoreInt32(&c.enableWrite, ENABLE_WRITE_FALSE)
	}
}

func (c *Cache_proxy) DoSet(oper int8, key string, value string) bool {
	if !c.checkWritePermission() {
		log.Println("write method not allowed\n")
		return false
	}

	if key == "" || value == "" {
		log.Println("doSet() error, get nil key or nil value")
		return false
	}
	event := LogEntryData{Oper: oper, Key: key, Value: value}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		c.Log.Printf("json.Marshal failed, err:%v", err)
		return false
	}

	applyFuture := c.Raft.Raft.Apply(eventBytes, 5*time.Second)
	if err := applyFuture.Error(); err != nil {
		c.Log.Printf("gedisraft.Apply failed:%v", err)
		return false
	}
	return true
}

func (c *Cache_proxy) DoJoin(peerAddress string) bool {
	if peerAddress == "" {
		c.Log.Println("invalid peerAddress")
		return false
	}
	addPeerFuture := c.Raft.Raft.AddVoter(raft.ServerID(peerAddress), raft.ServerAddress(peerAddress), 0, 0)
	if err := addPeerFuture.Error(); err != nil {
		c.Log.Printf("Error joining peer to raft, peeraddress:%s, err:%v", peerAddress, err)
		return false
	}
	return true
}

func (c *Cache_proxy) GetRangeData(start, end int) ([]byte, error) {
	// 首先，我们需要从缓存中获取范围内的所有键的数据
	data, err := c.Cache.GetRangeData(start, end)
	if err != nil {
		c.Log.Printf("Error retrieving data from cache: %v", err)
		return nil, err
	}

	// 将数据序列化为JSON格式以便传输
	jsonData, err := json.Marshal(data)
	if err != nil {
		c.Log.Printf("Error marshaling data: %v", err)
		return nil, err
	}

	return jsonData, nil
}
