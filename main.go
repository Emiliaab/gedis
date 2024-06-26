package main

import (
	"fmt"
	"github.com/Emiliaab/gedis/cache"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
)

func main() {
	config := cache.NewConfig()

	proxy := cache.NewCacheProxy(config.HttpPort, config.RaftPort, config.NodeName, config.Bootstrap, config.JoinAddress)

	var l net.Listener
	var err error
	httpAddr := "127.0.0.1:" + strconv.Itoa(int(config.HttpPort))
	logger := log.New(os.Stderr, "httpserver: ", log.Ldate|log.Ltime)
	l, err = net.Listen("tcp", httpAddr)
	if err != nil {
		logger.Fatal(fmt.Sprintf("listen %s failed: %s", httpAddr, err))
	}
	logger.Printf("http server listen:%s", l.Addr())

	httpServer := NewHttpServer(proxy)
	go func() {
		http.Serve(l, httpServer.mutex)
	}()

	if config.JoinAddress != "" {
		err = cache.JoinRaftCluster(proxy.Opts)
		if err != nil {
			logger.Fatal(fmt.Sprintf("join raft cluster failed:%v", err))
		}
	}

	// monitor leadership
	for {
		select {
		case leader := <-proxy.Raft.LeaderNotifyCh:
			if leader {
				proxy.Log.Println("become leader, enable write api")
				proxy.SetWriteFlag(true)
				// 只有raft group中的leader node开启与数据库的写回策略
				go func() {
					proxy.Cache.FlushDirtyKeys()
				}()
			} else {
				proxy.Log.Println("become follower, close write api")
				proxy.SetWriteFlag(false)
				proxy.Cache.Close()
			}
		}
	}

}
