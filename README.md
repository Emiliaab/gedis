# Gedis

>  Gedis是哈尔滨工业大学2024春季学期课程设计小组实现的项目，简易的可支持动态一致性Hash进行集群分片，通过Raft保证多副本高可用性和一致性的分布式KV，通过go语言实现

## 项目管理与开发

### 贡献成员

@[**Emiliaab** Shuxiang Wang](https://github.com/Emiliaab)（**组长**）

@[**Aurorein** Aurorain.](https://github.com/Aurorein)（**组员**）

### 使用的开源库

- [[hashicorp/raft: Golang implementation of the Raft consensus protocol (github.com)](https://github.com/hashicorp/raft)]([hashicorp/raft: Golang implementation of the Raft consensus protocol (github.com)](https://github.com/hashicorp/raft)) Go语言实现的raft库

- [[spaolacci/murmur3: Native MurmurHash3 Go implementation (github.com)](https://github.com/spaolacci/murmur3)]([spaolacci/murmur3: Native MurmurHash3 Go implementation (github.com)](https://github.com/spaolacci/murmur3)) 适用于对节点名称进行Hash的函数

- [[go-gorm/gorm: The fantastic ORM library for Golang, aims to be developer friendly (github.com)](https://github.com/go-gorm/gorm)]([go-gorm/gorm: The fantastic ORM library for Golang, aims to be developer friendly (github.com)](https://github.com/go-gorm/gorm)) 基于go语言的gorm框架

### 架构

**开发语言**：采用Go语言(版本1.20)，之所以采用go语言，首先在于它出色的并发性能，这对于分布式系统很重要。其次go语言内置强大的网络库，这在我们对不同节点间进行复杂的网络通信提供了很多的便捷。再者，go语言有丰富的开源生态，比如著名的hashicorp的raft库实现。另外go语言本身很好的容错能力，错误报错也很友好，方便我们进行定位错误。

**项目架构**：我们的项目意在实现Multi-Raft架构，同时也参考了很多互联网和工业界的优秀资源，遗憾的是，介于课设的期限（2 weeks）原因，在实现不同Raft Group的集群分片管理时没有使用工业界常用的The Shard Controller分片控制器，而是基于大量的网络通信来完成的，这势必会带来操作的一些操作的不可靠和大量的网络开销。如果有对Multi-Raft更高一致性要求的实现的学习需求，可以参考MIT 6.5840 Lab4或者TiDB中的PD组件的实现。

![gedis架构](.\assets\gedis架构.png)

**功能模块**：该项目实现的主要功能模块主要可包括以下几个：

1. 基于LRU-K的缓存淘汰策略的KV存储结构（@[**Emiliaab**](https://github.com/Emiliaab)）
2. Raft模块保证单一分片集群的高可用性和一致性（@[**Aurorein**](https://github.com/Aurorein)）
3. 基于网络通信协议和解决数据倾斜的一致性Hash算法实现的集群分片（@[**Emiliaab**](https://github.com/Emiliaab)）（@[**Aurorein**](https://github.com/Aurorein)）
4. 基于singleflight实现的高性能并发读（@[**Emiliaab**](https://github.com/Emiliaab)）
5. 网络通信实现的动态扩容数据迁移机制（@[**Emiliaab**](https://github.com/Emiliaab)）
6. 缓存控制的回写策略的实现（@[**Aurorein**](https://github.com/Aurorein)）

**Coding周期**：累计大约 10 days

## 功能模块详解

### 基于LRU-K的缓存淘汰策略的KV存储

传统的LRU缓存淘汰算法充分的利用了“如果数据最近被访问过，那么将来被访问的几率也更高”的规律，但是对于不满足该规律的数据，会破坏缓存，导致性能下降等问题。还有可能导致一些热点搜索词在缓存较小的情况下被替换出去，于是我们使用LRU-K改进了传统的LRU算法。

LRU-K需要多维护一个List，记录所有缓存数据被访问的历史，只有达到K次才会放入第二级List。这样就保证了不被淘汰的缓存数据不仅是最近访问的，而且将来也确实访问的几率会比较高。而对于没有到达K次的数据，会在第一级List中随着其他数据的加入而移动到链表的底部直至被淘汰。这样有效避免了不满足上面规律的数据破坏缓存，防止缓存击穿问题。

### Raft模块保证单一分片集群的高可用性和一致性

Raft 算法是一种分布式一致性算法,用于在分布式系统中维护一致的状态。它是由 Diego Ongaro 和 John Ousterhout 在 2013 年提出的。Raft 算法被广泛应用于构建分布式数据库、消息队列、配置管理等系统中,是当前分布式系统领域非常流行和重要的一种一致性算法。它提供了一种简单、可靠的方式来管理分布式系统中的状态一致性问题。

本系统使用了hashicorp/raft开源库，在底层保证cache并发正确的情况下，Master节点对写请求通过Raft.Apply()写入日志，而对于Master和Slave节点的Read请求则是直接调用底层cache的Get()函数，而不经过Raft模块。hashicorp/raft使用boltdb存储log、snapshot等，并提供了fsm数据结构接口供应用层调用，当raft.Apply()的日志被传入，会由Master发放给所有Slave节点，Slave节点也在本地写入Log日志，待到超过半数都写入以后则可以commit()，即执行fsm.Apply()调用底层cache的Get()和Set()操作。项目中通过指定leaderCh作为leader和follower身份改变的监听通知。

### 基于网络通信协议和解决数据倾斜的一致性Hash算法实现的集群分片

我们知道一致性Hash算法将节点和数据都放在一个环上，这样有利于对一个新加入的节点进行动态地扩容。但是，对于新加入的节点，需要让它的加入被现有集群其他节点感知，即对于集群所有节点的一致性Hash环需要被同步，在当前没有shard controller的情况下需要通过网络通信做到。具体的时序图如下：

![gedis一致性hash同步peers](https://github.com/Emiliaab/gedis/raw/main/assets/gedis一致性hash同步peers.png)

上图中Master Node1和Master Node2是一个现有的peers同步的Master节点集群，Master Node3是新加入的节点。首先用户发送sharePeers请求给现有集群的任意一个节点，这里假设为Node1节点，让它向Master Node3同步当前的Peers数据结构。Node3接收到的Peers是未加入自己的一致性Hash环，现在Node3将自己加入到Peers一致性Hash环中，此时原来的集群尚未感知到Node3，于是Node3仍然需要向原有Peers，即Node1和Node2发送请求，并让它们都将Node3更新到一致性Hash环中。

关于一致性Hash算法的数据倾斜问题，可以理解为一致性Hash加入的新节点可能都在一个方向，导致某个节点负载过大。所以，我们的系统对该算法加入了负载均衡，即让一个节点生成三个副本（即虚拟节点）再加入到一致性Hash环，并用一个Map存虚拟节点与真实节点的映射，以做到一致性Hash算法的负载均衡。

每次进行Get()和Set()操作的时候，先路由到对应的Raft Group再执行对应的操作。当然对于Get操作，可以先从本地缓存尝试寻找，主要是如果在本地缓存命中则可以减少很多网络IO的开销。

### 基于singleflight实现的高性能并发读

在一些并发查询的场景，经常会出现一些问题比如缓存雪崩，缓存击穿，缓存穿透，它们都是由于大量请求瞬时到达DB造成的。对于缓存层面，解决办法可以是化多次读请求为一次，即后面的请求在头请求未执行完之前会等待直接取到头请求的返回值结果，具体实现是使用Go语言的Sync.Mutex和Sync.WaitGroup，将后来对同一key访问的请求都加入Sync.waitGroup等待。

### 网络通信实现的动态扩容数据迁移机制

当一个现有的集群中加入新的节点，不光要有peers一致性Hash环的同步，还需要完成原来集群的数据迁移被一致性Hash分出来的那块迁移到新的节点。而这一步操作关键在于确定哪部分数据需要迁移，而由于为了解决一致性Hash的数据倾斜，所以要迁移的数据在环上可能分为多个片段。这又衍生出了新的困难，比如区间之间的重叠，以及新加入的两个虚拟节点相邻的情况。

![gedis扩容数据迁移](https://github.com/Emiliaab/gedis/raw/main/assets/gedis一致性hash同步peers.png)

最容易的想法是这样，以新插入的节点的Hash值作为end，在哈希环它前面的节点的Hash值+1作为start，这个虚拟节点这一段要迁移过来的数据就是从start-end范围的数据。而要从哪个节点要数据，在上面这幅图中很直观的看出来，应该是当前虚拟节点（Node31）的下一个Node，也就是从Node1要数据。

但是这幅图忽略了几种情况，首先是多个虚拟节点构成的区间会产生合并，然而这个可以将计算出的三段[start ,end]区间合并。

另外一个问题是，假如新加入的两个虚拟节点在Hash环上是相邻的，按照上面的逻辑会把一个新加入的虚拟节点当作要数据的节点，就会出问题了。然而解决办法也很简单，Node应该从原来未插入新节点的哈希环上找下一个。

### 缓存控制的回写策略的实现

缓存与数据库的三种写策略中，写回和写穿策略都是需要缓存做控制的。该项目简单通过gorm框架做了可插拔数据源的写回策略，即对数据的更新都是基于缓存的，客户端不能直接对数据库进行操作。而对缓存数据的修改，会将缓存标记为脏数据，定时器后台异步的批量将缓存的脏数据更新到数据库。注意本项目中通过leaderCh协调实现只有Raft Group中的Leader角色才能进行定时写回操作。

## 项目启动

./main 

\ -httpport {httpport}	httpPort即Http通信（一般为非Raft Group内的节点之间的通信）端口

\ -raftport {raftport}	raftGroup即raftGroup集群内节点之间的通信端口

\ -node {node}	node是节点名称，将会根据该名称在项目目录下生成对应的raft数据文件夹

\ -bootstrap {isLeader}	true则表示作为Leader方式启动并建立以它为基础的集群

\ -joinaddr {cluster address}	一般用于Follower节点加入Leader集群中，地址为Leader的http地址

默认项目是需要连接mysql数据库的，可以根据datasource文件夹下的配置信息自行修改

## 测试结果

测试环境：Go 1.20 / Windows11 64位

### Raft Group的一致性和高可用性的测试

```
// 启动一个master node和一个slave node1作为一个集群
master node: -httpport 8001 -raftport 9001 -node node1 -bootstrap true
slave node1: -httpport 8002 -raftport 9002 -node node2 -joinaddr 127.0.0.1:8001
// 向master node写入两条KV数据
http://127.0.0.1:8001/set?key=test&value=20&oper=1
http://127.0.0.1:8001/set?key=test2&value=30&oper=1
// master node和slave node1都可以查询到数据，说明master node的写入会同步到slave node
http://127.0.0.1:8001/get?key=test    // 20
http://127.0.0.1:8002/get?key=test2    // 30
// 此时加入一个新的slave node2
slave node2：-httpport 8003 -raftport 9003 -node node3 -joinaddr 127.0.0.1:8001
// slave node2可以查询到test和test2的数据，说明了数据经过了同步
http://127.0.0.1:8003/get?key=test    // 20
http://127.0.0.1:8003/get?key=test2    // 30
-- 此时关闭master node
// 此时slave node1或者slave node2会选举为新的leader
http://127.0.0.1:8001/set?key=mike&value=40&oper=1  ×
http://127.0.0.1:8002/set?key=mike&value=40&oper=1  √
-- 重启master
// 这里可以自行验证master node此刻是follower状态，而slave node1或者slave node2现在是leader
```

### 验证一致性Hash集群分片和数据迁移

其中9个node的启动可以通过项目目录下的9nodeshard.cmd脚本启动

```
// 首先开启9个节点，包括三个Raft Group，每个Raft Group包括一个Master和两个Slave，方便后续分片迁移
master node1：-httpport 8001 -raftport 9001 -node node1 -bootstrap true
master node2： -httpport 8002 -raftport 9002 -node node2 -bootstrap true
master node3： -httpport 8003 -raftport 9003 -node node3 -bootstrap true
slave node11：-httpport 8004 -raftport 9004 -node node11 -joinaddr 127.0.0.1:8001
slave node12：-httpport 8005 -raftport 9005 -node node12 -joinaddr 127.0.0.1:8001
slave node21：-httpport 8006 -raftport 9006 -node node21 -joinaddr 127.0.0.1:8002
slave node22：-httpport 8007 -raftport 9007 -node node22 -joinaddr 127.0.0.1:8002
slave node31：-httpport 8008 -raftport 9008 -node node31 -joinaddr 127.0.0.1:8003
slave node32：-httpport 8009 -raftport 9009 -node node32 -joinaddr 127.0.0.1:8003
// 三个master节点之间进行一致性Hash环Peers的同步
http://127.0.0.1:8001/sharepeers?dest=127.0.0.1:8002
http://127.0.0.1:8002/sharepeers?dest=127.0.0.1:8003
// 随机选一个raft group的master node进行插入
http://127.0.0.1:8001/set?key=test&value=test&oper=1
// 可以从任意raft group读出来数据，说明一致性Hash路由起了作用
http://127.0.0.1:8001/get?key=test
http://127.0.0.1:8002/get?key=test
http://127.0.0.1:8004/get?key=test
http://127.0.0.1:8005/get?key=test
// 此时加入一个新的master node4
master node4： -httpport 8010 -raftport 9010 -node node10 -bootstrap true
// 此时查询master node4的缓存，查不到数据
http://127.0.0.1:8010/getall    // {}
// share peers操作将master node4加入分片集群，即同步一致性Hash环
http://127.0.0.1:8001/sharepeers?dest=127.0.0.1:8010
// 经过上一步，原来属于master node2节点的key为test数据会被重新分片到master node4节点，所以会发生数据迁移，test数据会迁移到master node4
http://127.0.0.1:8010/getall    // {test: test}
```

### 验证singleflight实现的高并发场景的高性能读

采用JMeter压测工具高并发环境下读请求同一个key，下图是测试结果

![gedis高并发读](https://github.com/Emiliaab/gedis/raw/main/assets/gedis高并发读.png)

可以看出来，后面的请求由于都使用了前面的返回值，所以请求的相应数在1s之后趋于个位数

### 写回策略的验证

默认10s异步刷盘一次，可以开启一个Raft Group，通过写入缓存和更新缓存，观察数据库表内的数据变化自行验证