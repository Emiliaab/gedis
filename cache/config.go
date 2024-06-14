package cache

import (
	"flag"
)

type Config struct {
	//raftNodes int32
	HttpPort    int32
	RaftPort    int32
	NodeName    string
	Bootstrap   bool
	JoinAddress string
}

func NewConfig() *Config {
	config := &Config{}

	//var raftNodes = flag.String("nodes", "3", "count of gedisraft nodes")
	var httpPort = flag.Int("httpport", 8000, "http tcp address port")
	var raftPort = flag.Int("raftport", 9000, "gedisraft tcp address port")
	var nodeName = flag.String("node", "default", "node name")
	var bootstrap = flag.Bool("bootstrap", false, "boostrap")
	var joinAddress = flag.String("joinaddr", "", "join addr")

	flag.Parse()
	//nodes, err := strconv.Atoi(*raftNodes)
	//if err != nil {
	//	log.Fatal("gedisraft nodes input error!")
	//}
	//config.raftNodes = int32(nodes)
	config.HttpPort = int32(*httpPort)
	config.RaftPort = int32(*raftPort)
	config.Bootstrap = *bootstrap
	config.NodeName = *nodeName
	config.JoinAddress = *joinAddress
	return config
}
