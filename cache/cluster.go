package cache

import (
	"math/rand"
	"sync"
)

type Cluster struct {
	master *Cache_proxy
	nodes  []*Cache_proxy
	mutex  sync.Mutex
}

func createClsuter() *Cluster {
	cl := &Cluster{}
	cl.nodes = make([]*Cache_proxy, 0)

	return cl

}

func (cl *Cluster) RegisterCluster(proxy *Cache_proxy) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	cl.nodes = append(cl.nodes, proxy)
}

func (cl *Cluster) SetMaster(proxy *Cache_proxy) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	cl.master = proxy
}

func (cl *Cluster) Robin() *Cache_proxy {
	r := rand.Int() % (len(cl.nodes) - 1)
	return cl.nodes[r]
}

func (cl *Cluster) GetMaster() *Cache_proxy {
	return cl.master
}
