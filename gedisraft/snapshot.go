package gedisraft

import (
	"github.com/Emiliaab/gedis/cache"
	"github.com/hashicorp/raft"
)

type snapshot struct {
	proxy *cache.Cache_proxy
}

func (s *snapshot) Persist(sink raft.SnapshotSink) error {
	snapshotBytes, err := s.proxy.Cache.Marshal()
	if err != nil {
		sink.Cancel()
		return err
	}

	if _, err := sink.Write(snapshotBytes); err != nil {
		sink.Cancel()
		return err
	}

	if err := sink.Close(); err != nil {
		sink.Cancel()
		return err
	}
	return nil
}

func (f *snapshot) Release() {}
