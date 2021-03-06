package server

import (
	"golang.org/x/net/context"

	"util/log"
	"model/pkg/metapb"

	"time"
)

const (
	Min_leader_balance_num = 5
)

type balanceNodeLeaderWorker struct {
	name     string
	ctx      context.Context
	cancel   context.CancelFunc
	interval time.Duration
	option   *scheduleOption
}

func NewBalanceNodeLeaderWorker(wm *WorkerManager, interval time.Duration) Worker {
	ctx, cancel := context.WithCancel(wm.ctx)
	return &balanceNodeLeaderWorker{
		name:     balanceLeaderWorkerName,
		ctx:      ctx,
		cancel:   cancel,
		interval: interval,
		option:   wm.opt,
	}
}

func (w *balanceNodeLeaderWorker) GetName() string {
	return w.name
}

func (w *balanceNodeLeaderWorker) Work(cluster *Cluster) {
	log.Debug("start %s", w.GetName())
	rng, newLeader := selectChangeLeader(cluster, w.GetName())
	if rng == nil {
		log.Debug("%v: no node need to change leader", w.GetName())
		return
	}

	id, err := cluster.GenId()
	if err != nil {
		log.Debug("generate task id error")
		return
	}

	cluster.hbManager.dealIngNodes.set(newLeader.NodeId)

	cluster.metric.CollectScheduleCounter(w.GetName(), "new_operator")
	log.Debug("start to transfer leader, range:[%v], new leader:[%v]", rng.GetId(), newLeader.GetId())
	cluster.eventDispatcher.pushEvent(NewTryChangeLeaderEvent(id, rng.GetId(), rng.GetLeader(), newLeader, w.GetName()))
	return
}

func (w *balanceNodeLeaderWorker) AllowWork(cluster *Cluster) bool {
	if cluster.autoFailoverUnable {
		return false
	}
	return true
}

func (w *balanceNodeLeaderWorker) GetInterval() time.Duration {
	return w.interval
}

func (w *balanceNodeLeaderWorker) Stop() {
	w.cancel()
}

//count node leader average number,
func countLeaderAvg(nodes []*Node) float64 {
	var averageLeader float64
	for _, s := range nodes {
		averageLeader += float64(s.GetLeaderCount()) / float64(len(nodes))
	}
	return averageLeader
}

/**
选择需要切换leader的range
 */
func selectChangeLeader(cluster *Cluster, workerName string) (*Range, *metapb.Peer) {
	nodes := cluster.GetAllActiveNode()
	if len(nodes) == 0 {
		log.Debug("%v: node is nil", workerName)
		cluster.metric.CollectScheduleCounter(workerName, "no_node")
		return nil, nil
	}

	newSelectors := []NodeSelector{
		NewWriterOpsThresholdSelector(cluster.opt),
		NewStorageThresholdSelector(cluster.opt),
		NewDifferCacheNodeSelector(cluster.hbManager.dealIngNodes),
	}

	//todo avg 应该 是过滤后的node的平均值
	avgLeaderNum := countLeaderAvg(nodes)
	mostLeaderNode, leastLeaderNode := SelectMostAndLeastLeaderNode(nodes, newSelectors)
	var mostLeaderNum, leastLeaderNum = float64(0), float64(0)
	if mostLeaderNode != nil {
		mostLeaderNum = mostLeaderNode.leaderScore()
	}
	if leastLeaderNode != nil {
		leastLeaderNum = leastLeaderNode.leaderScore()
	}

	if log.IsEnableDebug() {
		log.Debug("%v: mostLeaderNum  %v, leastLeaderNum %v, avg leader num :%v", workerName, mostLeaderNum, leastLeaderNum, avgLeaderNum)
	}

	if (mostLeaderNum - avgLeaderNum) > float64(Min_leader_balance_num) {
		// 在Node上选择一个leader
		for _, r := range mostLeaderNode.GetAllRanges() {
			if r.GetLeader().GetNodeId() == mostLeaderNode.GetId() && r.require(cluster) {
				tarGetAllNode := cluster.getFollowerNodes(r)
				node := SelectLeaderNode(tarGetAllNode, newSelectors, mostLeaderNum)
				if node != nil {
					return r, r.GetNodePeer(node.GetId())
				}
			}
		}
	}

	if (avgLeaderNum - leastLeaderNum) > float64(Min_leader_balance_num) {
		// 在Node上选择一个不是leader的
		for _, r := range leastLeaderNode.GetAllRanges() {
			if r.GetLeader().GetNodeId() != leastLeaderNode.GetId() && r.require(cluster) {
				leaderNode := cluster.getLeaderNode(r)
				if float64(leaderNode.GetLeaderCount()- leastLeaderNode.GetLeaderCount()) > float64(Min_leader_balance_num) {
					return r, r.GetNodePeer(leastLeaderNode.GetId())
				}
			}
		}
	}

	return nil, nil

}
