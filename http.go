package main

import (
	"encoding/json"
	"fmt"
	"github.com/Emiliaab/gedis/cache"
	"github.com/Emiliaab/gedis/gedisraft"
	"github.com/hashicorp/raft"
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

	ret, ok := h.cache.DoGet(key)
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

	event := gedisraft.LogEntryData{Oper: oper, Key: key, Value: value}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		h.log.Printf("json.Marshal failed, err:%v", err)
		fmt.Fprint(w, "internal error\n")
		return
	}

	applyFuture := h.cache.Raft.Raft.Apply(eventBytes, 5*time.Second)
	if err := applyFuture.Error(); err != nil {
		h.log.Printf("raft.Apply failed:%v", err)
		fmt.Fprint(w, "internal error\n")
		return
	}

	fmt.Fprintf(w, "ok\n")
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
