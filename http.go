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
	"time"
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
	mutex.HandleFunc("/getrange", s.doGetRange)

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
	// 获取 h.cache.Peers.Keys 的副本
	keys := make([]int, len(h.cache.Peers.Keys))
	copy(keys, h.cache.Peers.Keys)

	// peers中加入自己，并向peers中其他节点都通知加入自己
	h.cache.Peers.Add(h.cache.Opts.HttpAddress)
	// 得到数据迁移的区间以及数据来源节点名称
	getRange := h.cache.Peers.GetRange(h.cache.Opts.HttpAddress, keys)
	// 数据迁移
	h.dataMigration(getRange)
	peerset := h.cache.Peers.GetPeers()
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
	log.Printf("当前node: %s 的一致性hash map所包含的peers有: %s", h.cache.Opts.HttpAddress, h.cache.Peers.GetPeers())
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
	log.Printf("当前node: %s 的一致性hash map所包含的peers有: %s", h.cache.Opts.HttpAddress, h.cache.Peers.GetPeers())
}

// doGetRange 处理范围请求，返回start和end之间的数据
func (h *httpServer) doGetRange(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()

	startStr := vars.Get("start")
	endStr := vars.Get("end")
	if startStr == "" || endStr == "" {
		h.log.Println("doGetRange() error, start or end parameter is missing")
		http.Error(w, "Missing start or end parameter", http.StatusBadRequest)
		return
	}

	start, err := strconv.Atoi(startStr)
	if err != nil {
		h.log.Printf("doGetRange() error, invalid start parameter: %v", err)
		http.Error(w, "Invalid start parameter", http.StatusBadRequest)
		return
	}

	end, err := strconv.Atoi(endStr)
	if err != nil {
		h.log.Printf("doGetRange() error, invalid end parameter: %v", err)
		http.Error(w, "Invalid end parameter", http.StatusBadRequest)
		return
	}

	// 获取数据
	jsonData, err := h.cache.GetRangeData(start, end)
	if err != nil {
		h.log.Printf("Error retrieving range data: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

// 根据数据迁移的区间以及数据来源节点名称进行数据迁移
func (h *httpServer) dataMigration(getRange []consistenthash.RangeNode) {
	for _, rangeNode := range getRange {
		// 从数据来源节点获取数据
		jsonData, err := getDataFromPeer(rangeNode.Start, rangeNode.End, rangeNode.RealNode)
		if err != nil {
			h.log.Printf("getDataFromPeer failed, err: %v", err)
			continue
		}
		log.Printf("Raw data received: %s", jsonData)
		// 反序列化数据
		var dataMap map[string]string
		err = json.Unmarshal([]byte(jsonData), &dataMap)
		if err != nil {
			h.log.Printf("Error unmarshaling data from peer: %v", err)
			continue
		}

		// 逐项应用数据
		for key, value := range dataMap {
			event := cache.LogEntryData{Oper: 1, Key: key, Value: value} // 假设 Oper: 1 表示设置操作
			eventBytes, err := json.Marshal(event)
			if err != nil {
				h.log.Printf("json.Marshal failed, err:%v", err)
				continue
			}

			applyFuture := h.cache.Raft.Raft.Apply(eventBytes, 5*time.Second)
			if err := applyFuture.Error(); err != nil {
				h.log.Printf("raft.Apply failed:%v", err)
				continue
			}
		}
	}
}

// 调用readnode的doGetRange请求获得数据
func getDataFromPeer(start, end int, peerAddress string) ([]byte, error) {
	url := fmt.Sprintf("http://%s/getrange?start=%d&end=%d", peerAddress, start, end)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	//fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", data)
	// 序列化data
	return data, nil
}
