package server

import (
	"fmt"
	"sync"
	"time"

	"model/pkg/metapb"
	"model/pkg/mspb"
	"util/deepcopy"
	"sync/atomic"
)

type Range struct {
	lock sync.RWMutex
	*metapb.Range
	Leader        *metapb.Peer
	DownPeers     []*mspb.PeerStats
	PendingPeers  []*metapb.Peer

	BytesWritten uint64
	BytesRead    uint64

	KeysWritten uint64
	KeysRead    uint64
	opsStat RangeOpsStat
	// Approximate range size.
	ApproximateSize uint64

	State         metapb.RangeState
	Trace bool

	LastHbTimeTS    time.Time
}

type RangeOpsStat struct {
	writeOps [CacheSize]uint64
	hit uint64
}

func (opsStat *RangeOpsStat) Hit(v uint64){
	hit := atomic.AddUint64(&(opsStat.hit),1)
	opsStat.writeOps[hit%CacheSize] = v
}

func (opsStat *RangeOpsStat) GetMax() uint64{
	var max uint64 = 0
	for i:=0 ; i<CacheSize ;i++ {
		v := opsStat.writeOps[i]
		if  v > max {
			max = v
		}
	}
	return max
}

func (opsStat *RangeOpsStat) Clear() uint64{
	var max uint64 = 0
	for i:=0 ; i<CacheSize ;i++ {
		opsStat.writeOps[i] = 0
	}
	return max
}

func NewRange(r *metapb.Range, leader *metapb.Peer) *Range {
	if leader == nil && r.GetPeers() != nil {
		leader = deepcopy.Iface(r.GetPeers()[0]).(*metapb.Peer)
	}
	region := &Range{
		Range:  r,
		Leader: leader,
		LastHbTimeTS: time.Now(),
	}
	return region
}

func (r *Range) SString() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", r.GetTableId(), r.GetId())
}

func (r *Range) ID() uint64 {
	if r == nil {
		return 0
	}
	return r.GetId()
}

func (r *Range) GetLeader() *metapb.Peer {
	if r == nil {
		return nil
	}
	if r.Leader == nil {
		return nil
	}
	return r.Leader
}

// GetPeer return the peer with specified peer id
func (r *Range) GetPeer(peerID uint64) *metapb.Peer {
	if r == nil {
		return nil
	}
	for _, peer := range r.GetPeers() {
		if peer.GetId() == peerID {
			return peer
		}
	}
	return nil
}

// GetDownPeer return the down peers with specified peer id
func (r *Range) GetDownPeer(peerID uint64) *metapb.Peer {
	if r == nil {
		return nil
	}
	for _, down := range r.DownPeers {
		if down.GetPeer().GetId() == peerID {
			return down.GetPeer()
		}
	}
	return nil
}

func (r *Range) GetDownPeers() []*metapb.Peer {
	if r == nil {
		return nil
	}
	var peers []*metapb.Peer
	for _, down := range r.DownPeers {
		peers = append(peers, down.GetPeer())
	}
	return peers
}

// GetPendingPeer return the pending peer with specified peer id
func (r *Range) GetPendingPeer(peerID uint64) *metapb.Peer {
	if r == nil {
		return nil
	}
	for _, peer := range r.PendingPeers {
		if peer.GetId() == peerID {
			return peer
		}
	}
	return nil
}

// GetNodePeer return the peer in specified Node
func (r *Range) GetNodePeer(nodeID uint64) *metapb.Peer {
	if r == nil {
		return nil
	}
	for _, peer := range r.GetPeers() {
		if peer.GetNodeId() == nodeID {
			return peer
		}
	}
	return nil
}

// RemoveNodePeer remove the peer in specified Node
func (r *Range) RemoveNodePeer(NodeID uint64) {
	if r == nil {
		return
	}
	var peers []*metapb.Peer
	for _, peer := range r.GetPeers() {
		if peer.GetNodeId() != NodeID {
			peers = append(peers, peer)
		}
	}
	r.Peers = peers
}

func (r *Range) GetNodes(cluster *Cluster) (nodes []*Node) {
	if r == nil {
		return nil
	}

	peers := r.GetPeers()
	for _, peer := range peers {
		node := cluster.FindNodeById(peer.GetNodeId())
		if node == nil {
			continue
		}
		nodes = append(nodes, node)
	}
	return
}

// GetNodeIds return a map indicate the region distributed
func (r *Range) GetNodeIds() map[uint64]struct{} {
	if r == nil {
		return make(map[uint64]struct{})
	}
	peers := r.GetPeers()
	nodes := make(map[uint64]struct{}, len(peers))
	for _, peer := range peers {
		nodes[peer.GetNodeId()] = struct{}{}
	}
	return nodes
}

// GetFollowers return a map indicate the follow peers distributed
func (r *Range) GetFollowers() map[uint64]*metapb.Peer {
	if r == nil {
		return nil
	}
	peers := r.GetPeers()
	followers := make(map[uint64]*metapb.Peer, len(peers))
	for _, peer := range peers {
		if r.Leader == nil || r.Leader.GetId() != peer.GetId() {
			followers[peer.GetNodeId()] = peer
		}
	}
	return followers
}

func (r *Range) GetRandomFollower() *metapb.Peer {
	if r == nil {
		return nil
	}
	for _, peer := range r.GetPeers() {
		if r.Leader == nil || r.Leader.GetId() != peer.GetId() {
			return peer
		}
	}
	return nil
}


func (r *Range) GetPendingPeers() []*metapb.Peer {
	if r == nil {
		return nil
	}
	return r.PendingPeers
}

func (r *Range) IsHealthy() bool {
	if r == nil {
		return false
	}
	if len(r.GetDownPeers()) > 0 {
		return false
	}
	if len(r.GetPendingPeers()) > 0 {
		return false
	}
	// 分片需要删除，无需补充副本
	if r.State == metapb.RangeState_R_Remove {
		return false
	}
	return true
}

func (r *Range) clone() *Range {
	if r == nil {
		return nil
	}
	return &Range{
		Range:  deepcopy.Iface(r.Range).(*metapb.Range),
		Leader:        r.Leader,
		DownPeers:     r.DownPeers,
		PendingPeers:  r.PendingPeers,

		BytesWritten: r.BytesWritten,
		BytesRead:    r.BytesRead,

		KeysWritten: r.KeysWritten,
		KeysRead:    r.KeysRead,
		// Approximate range size.
		ApproximateSize: r.ApproximateSize,

		State:         r.State,
		Trace:         r.Trace,

		LastHbTimeTS:    r.LastHbTimeTS,
	}
}
