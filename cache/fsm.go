package cache

import (
	"encoding/json"
	"github.com/hashicorp/raft"
	"io"
	"log"
)

type FSM struct {
	proxy *Cache_proxy
	log   *log.Logger
}

func (f *FSM) Apply(logEntry *raft.Log) interface{} {
	e := LogEntryData{}
	if err := json.Unmarshal(logEntry.Data, &e); err != nil {
		panic("Failed unmarshaling Raft log entry. This is a bug.")
	}
	switch e.Oper {
	case 0:
		{
			f.proxy.Cache.Add(e.Key, []byte(e.Value))
		}
	case 1:
		{
			f.proxy.Cache.Add(e.Key, []byte(e.Value))
		}
	case 2:
		{
			f.proxy.Cache.Remove(e.Key)
		}
	default:
		panic("oper val error!")
	}
	f.log.Printf("fms.Apply(), logEntry:%s\n", logEntry.Data)
	return nil
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return &snapshot{proxy: f.proxy}, nil
}

func (f *FSM) Restore(snapshot io.ReadCloser) error {
	return f.proxy.Cache.UnMarshal(snapshot)
}

type LogEntryData struct {
	Oper  int8 // 0->ADD   1->SET   2->REMOVE
	Key   string
	Value string
}
