package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Emiliaab/gedis/cache"
	"github.com/Emiliaab/gedis/consistenthash"
	"github.com/hashicorp/raft"
	"github.com/spaolacci/murmur3"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

type httpServer struct {
	cache *cache.Cache_proxy
	log   *log.Logger
	mutex *http.ServeMux
}

type Stu struct {
	Age int
	Sex int
}

func NewHttpServer(cache *cache.Cache_proxy) *httpServer {
	mutex := http.NewServeMux()
	s := &httpServer{
		cache: cache,
		log:   log.New(os.Stderr, "http_server: ", log.Ldate|log.Ltime),
		mutex: mutex,
	}

	mutex.HandleFunc("/get", s.doGet)
	mutex.HandleFunc("/set", s.doSet)
	mutex.HandleFunc("/join", s.doJoin)
	mutex.HandleFunc("/sharepeers", s.sharePeers)
	mutex.HandleFunc("/sendpeers", s.sendPeers)
	mutex.HandleFunc("/addpeer", s.addPeer)

	return s
}

func (h *httpServer) doGet(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()

	key := vars.Get("key")
	if key == "" {
		h.log.Println("doGet() error, get nil key")
		fmt.Fprint(w, "")
		return
	}
	// 判断是否是主节点，不是的话获取主节点的地址
	masterAddress := h.cache.Opts.JoinAddress

	ret, ok := h.cache.DoGet(key, masterAddress)
	if !ok {
		h.log.Println("doGet() error, get false ok")
		fmt.Fprint(w, "")
	}
	fmt.Fprintf(w, "%s\n", ret)
}

func (h *httpServer) doSet(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()

	operInt, error := strconv.Atoi(vars.Get("oper"))
	if error != nil {
		h.log.Println("doSet() error, get error oper")
	}
	key := vars.Get("key")
	value := vars.Get("value")
	oper := int8(operInt)
	if key == "" || value == "" {
		h.log.Println("doSet() error, get nil key or nil value")
		fmt.Fprint(w, "param error\n")
		return
	}

	event := cache.LogEntryData{Oper: oper, Key: key, Value: value}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		h.log.Printf("json.Marshal failed, err:%v", err)
		fmt.Fprint(w, "internal error\n")
		return
	}
	// 通过一致性hash找到应该写入的节点
	// 如果是本机，则利用raft协议直接写入, 如果不是本机，则通过http协议写入
	peerAddress := h.cache.Peers.Get(key)
	if peerAddress == h.cache.Opts.HttpAddress {
		applyFuture := h.cache.Raft.Raft.Apply(eventBytes, 5)
		if err := applyFuture.Error(); err != nil {
			h.log.Printf("raft.Apply failed:%v", err)
			fmt.Fprint(w, "internal error\n")
			return
		}
	} else {
		if !doSetFromPeer(peerAddress, key, value, oper) {
			h.log.Println("doSetFromPeer failed")
			fmt.Fprint(w, "internal error\n")
			return
		}
	}

	fmt.Fprintf(w, "ok\n")
}

// 非本机节点，通过http协议写入
func doSetFromPeer(peerAddress string, key string, value string, oper int8) bool {
	url := "http://" + peerAddress + "/set?oper=" + strconv.Itoa(int(oper)) + "&key=" + key + "&value=" + value
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	return true
}

func (h *httpServer) doJoin(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()

	peerAddress := vars.Get("peerAddress")
	if peerAddress == "" {
		h.log.Println("invalid PeerAddress")
		fmt.Fprint(w, "invalid peerAddress\n")
		return
	}

	fmt.Println("=======")
	fmt.Println(peerAddress)
	addPeerFuture := h.cache.Raft.Raft.AddVoter(raft.ServerID(peerAddress), raft.ServerAddress(peerAddress), 0, 0)
	if err := addPeerFuture.Error(); err != nil {
		h.log.Printf("Error joining peer to raft, peeraddress:%s, err:%v, code:%d", peerAddress, err, http.StatusInternalServerError)
		fmt.Fprint(w, "internal error\n")
		return
	}
	fmt.Fprint(w, "ok")
}

func (h *httpServer) sharePeers(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()

	dest := vars.Get("dest")
	if dest == "" {
		h.log.Println("invalid dest")
		fmt.Fprint(w, "invalid dest\n")
		return
	}

	url := fmt.Sprintf("http://%s/sendpeers", dest)
	data, err := json.Marshal(*(h.cache.Peers))
	if err != nil {
		h.log.Println("peers json error!")
		fmt.Fprint(w, "peers json error!\n")
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		h.log.Println("send peers error!")
		fmt.Fprint(w, "send peers error!\n")
		return
	}
	defer resp.Body.Close()

	code, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		h.log.Println("send peers get resp error!")
		fmt.Fprint(w, "send peers get resp error!\n")
		return
	}
	fmt.Println(code)
	fmt.Fprintf(w, "%s share to peer %s peers\n", h.cache.Opts.HttpAddress, dest)
}

func (h *httpServer) sendPeers(w http.ResponseWriter, r *http.Request) {
	var data consistenthash.Map

	// 从请求体中读取数据
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.log.Println("Error reading request body!")
		fmt.Fprint(w, "Error reading request body!\n")
		return
	}

	json.Unmarshal(body, &data)
	//err := json.NewDecoder(r.Body).Decode(&data)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	//h.pool.mu.Lock()
	//defer h.pool.mu.Unlock()
	// 更新 peers 变量
	data.Hash = func(key []byte) uint32 {
		return uint32(murmur3.Sum64(key))
	}

	h.cache.Peers = &data
	//fmt.Println(h.cache.Peers)
	//for k, v := range h.cache.Peers.HashMap {
	//	fmt.Printf("%d -> %s\n", k, v)
	//}
	fmt.Println("11111")
	// peers中加入自己，并向peers中其他节点都通知加入自己
	h.cache.Peers.Add(h.cache.Opts.HttpAddress)
	//fmt.Println(h.cache.Peers)
	//for k, v := range h.cache.Peers.HashMap {
	//	fmt.Printf("%d -> %s\n", k, v)
	//}

	peerset := h.cache.Peers.GetPeers()
	fmt.Println(peerset)
	for _, peer := range peerset {
		if peer == h.cache.Opts.HttpAddress {
			continue
		}
		url := fmt.Sprintf("http://%s/addpeer?peerAddress=%s", peer, h.cache.Opts.HttpAddress)

		resp, err := http.Get(url)
		h.log.Printf("send to peer %s peerAddress %s\n", peer, h.cache.Opts.HttpAddress)
		fmt.Fprintf(w, "send to peer %s peerAddress %s\n", peer, h.cache.Opts.HttpAddress)
		if err != nil {
			h.log.Println("send peers get resp error!")
			fmt.Fprint(w, "send peers get resp error!\n")
			return
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		//if err != nil {
		//	h.log.Println("send peers get resp error!")
		//	fmt.Fprint(w, "send peers get resp error!\n")
		//	return
		//}
		fmt.Println(body)
		//if string(body) != "ok" {
		//	http.Error(w, err.Error(), http.StatusBadRequest)
		//	return
		//}
	}

	// 返回响应
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Peers updated successfully")
	log.Println(h.cache.Peers.GetPeers())
}

func (h *httpServer) addPeer(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()

	peerAddress := vars.Get("peerAddress")
	if peerAddress == "" {
		h.log.Println("add peer invalid peerAddress")
		fmt.Fprint(w, "add peer invalid peerAddress\n")
		return
	}

	h.cache.Peers.Add(peerAddress)
	log.Printf("%s addPeer %s success!", h.cache.Opts.HttpAddress, peerAddress)
	fmt.Fprintf(w, "%s addPeer %s success!", h.cache.Opts.HttpAddress, peerAddress)
	log.Println(h.cache.Peers.GetPeers())
}
