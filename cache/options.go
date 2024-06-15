package cache

import (
	"strconv"
)

type Options struct {
	dataDir        string
	HttpAddress    string
	raftTCPAddress string
	bootstrap      bool
	joinAddress    string
}

func NewOptions(httpPort int32, raftPort int32, node string, bootstrap bool, joinAddress string) *Options {
	opts := &Options{}

	opts.dataDir = "./" + node
	opts.HttpAddress = "127.0.0.1:" + strconv.Itoa(int(httpPort))
	opts.bootstrap = bootstrap
	opts.raftTCPAddress = "127.0.0.1:" + strconv.Itoa(int(raftPort))
	opts.joinAddress = joinAddress
	return opts
}
