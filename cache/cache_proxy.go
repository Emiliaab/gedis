package cache

import (
	"encoding/json"
	"fmt"
	"github.com/Emiliaab/gedis/consistenthash"
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
	proxy.Cache = Cache{}
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

	value, ok := c.Cache.Get(key)
	// 如果本地缓存没有，先给主节点发请求，然后如果自己就是主节点的话，就用一致性哈希算法找到应该查找的节点
	if !ok {
		// 如果masterAddress为空，说明自己就是主节点
		if masterAddress == "" {
			// 直接通过一致性hash算法找到应该查找的节点
			peerAddress := c.Peers.Get(key)
			if peerAddress == c.Opts.HttpAddress {
				value, ok = c.Cache.Get(key)
			} else {
				res, err := GetFromPeer(peerAddress, key)
				if err != nil {
					log.Printf("GetFromPeer failed, err:%v", err)
					return nil, false
				}
				value = res
				ok = true
			}
		} else {
			res, err := GetFromPeer(masterAddress, key)
			if err != nil {
				log.Printf("GetFromPeer failed, err:%v", err)
				return nil, false
			}
			value = res
			ok = true

		}

	}
	return value, ok
}

func GetFromPeer(address string, key string) (res []byte, err error) {
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
