#include "server.h"

#include <ostream>
#include <sstream>
#include "address_manager.h"
#include "command_line.h"
#include "statemachine.h"
#include "telnet_service.h"

#include "raft/src/impl/raft_snapshot.h"
#include "raft/src/impl/raft_fsm.h"

namespace fbase {
namespace raft {
namespace playground {

static const uint64_t kRaftID = 1;
static const int kSnapshotConcurrency = 10;

Server::Server() {
    addrs_mgr_ = std::make_shared<AddressManager>();
    sm_ = std::shared_ptr<PGStateMachine>(new PGStateMachine);
    setupHandleMap();
}

Server::~Server() { delete telnet_service_; }

void Server::run() {
    // start raft server
    RaftServerOptions sops;
    sops.node_id = gNodeID;
    sops.listen_port = addrs_mgr_->GetRaftPort(gNodeID);
    sops.resolver = std::static_pointer_cast<NodeResolver>(addrs_mgr_);
    sops.max_snapshot_concurrency = kSnapshotConcurrency;
    rs_ = CreateRaftServer(sops);
    auto status = rs_->Start();
    if (!status.ok()) {
        std::cerr << "ERR: start raft server failed: " << status.ToString() << std::endl;
        exit(EXIT_FAILURE);
    }

    // create raft
    RaftOptions ops;
    ops.statemachine = std::static_pointer_cast<StateMachine>(sm_);
    ops.use_memory_storage = false;
    ops.storage_path = std::string("./raft") + std::to_string(gNodeID);
    ops.id = kRaftID;
    for (auto p : gPeers) {
        Peer peer;
        peer.node_id = p;
        peer.type = PeerType::kNormal;
        ops.peers.push_back(peer);
    }
    status = rs_->CreateRaft(ops, &raft_);
    if (!status.ok()) {
        std::cerr << "ERR: create raft failed: " << status.ToString() << std::endl;
        exit(EXIT_FAILURE);
    }

    uint16_t port = addrs_mgr_->GetTelnetPort(gNodeID);
    telnet_service_ = new TelnetService(this, port);
}

void Server::stop() {
    // TODO:
}

static void parseCommand(std::string& cmd, std::vector<std::string>* args) {
    std::istringstream ss(cmd);
    std::string arg;
    while (std::getline(ss, arg, ' ')) {
        args->push_back(arg);
    }
}

std::string Server::handleCommand(std::string& cmd) {
    if (cmd.size() <= 2) {
        return "";
    } else {
        // remove
        cmd.pop_back();
        cmd.pop_back();
    }

    std::vector<std::string> args;
    parseCommand(cmd, &args);
    if (args.empty()) return "";

    auto resp = doCommand(args);
    resp += "\r\n";
    return resp;
}

std::string Server::doCommand(std::vector<std::string>& args) {
    assert(!args.empty());

    std::string cmd = args[0];
    args.erase(args.begin());

    auto it = telnet_commands_.find(cmd);
    if (it != telnet_commands_.end()) {
        if (args.size() < it->second.argc) {
            return std::string("ERR: wrong command paramters");
        } else {
            return it->second.handler(args);
        }
    } else {
        return std::string("ERR: unsupported command(") + cmd + ")";
    }
}

void Server::addHandler(const std::string& command, int argc, CommandHandler handler,
                        const std::string& usage) {
    TelnetCommand c;
    c.argc = argc;
    c.handler = handler;
    c.usage = usage;
    telnet_commands_[command] = c;
}

void Server::setupHandleMap() {
    telnet_commands_.clear();
    addHandler("submit", 1, std::bind(&Server::handleSubmit, this, std::placeholders::_1),
               "submit new request. usage: submit {number}");

    addHandler("member", 2,
               std::bind(&Server::handleMemeber, this, std::placeholders::_1),
               "change raft member. usage: member {add | remove} {nodeID}");

    addHandler("info", 0, std::bind(&Server::handleInfo, this, std::placeholders::_1),
               "print info. usage: \r\n\tinfo\r\n\tinfo leader\r\n\tinfo "
               "term\r\n\tinfo member\r\n\tinfo sum\r\n\tinfo replica");

    addHandler("truncate", 1,
               std::bind(&Server::handleTruncate, this, std::placeholders::_1),
               "truncate raft log. usage: truncate {index}");

    addHandler("elect", 0, std::bind(&Server::handleElect, this, std::placeholders::_1),
               "try to leader");

    addHandler("help", 0, std::bind(&Server::handleHelp, this, std::placeholders::_1),
               "print help message");

    addHandler("test", 0, std::bind(&Server::handleTest, this, std::placeholders::_1),
               "test snapshot");
}

std::string Server::handleHelp(const std::vector<std::string>& args) {
    std::ostringstream ss;
    ss << "supported commands: \r\n";
    for (auto it = telnet_commands_.begin(); it != telnet_commands_.end(); ++it) {
        if (it->first == "help") continue;
        ss << it->first << "\r\n";
        ss << "\t" << it->second.usage << "\r\n";
    }
    ss << "\r\n";
    return ss.str();
}

std::string Server::handleInfo(const std::vector<std::string>& args) {
    if (args.size() >= 1) {
        if (args[0] == "leader") {
            uint64_t leader = 0;
            uint64_t term = 0;
            raft_->GetLeaderTerm(&leader, &term);
            return std::string("Leader: ") + std::to_string(leader);
        } else if (args[0] == "term") {
            uint64_t leader = 0;
            uint64_t term = 0;
            raft_->GetLeaderTerm(&leader, &term);
            return std::string("Term: ") + std::to_string(term);
        } else if (args[0] == "member") {
        } else if (args[0] == "sum") {
            return std::string("Sum: ") + std::to_string(sm_->sum());
        } else if (args[0] == "replica") {
            std::string ret = "peers: ";
            std::vector<raft::Peer> peers;
            raft_->GetPeers(&peers);
            for (const auto& p : peers) {
                ret += std::to_string(p.node_id) + " ";
            }

            std::vector<raft::DownPeer> downs;
            raft_->GetDownPeers(&downs);
            if (!downs.empty()) {
                ret += "\r\ndowns: ";
                for (const auto& d : downs) {
                    ret += std::to_string(d.node_id) + ":" +
                           std::to_string(d.down_seconds) + " ";
                }
            }

            std::vector<uint64_t> pendings;
            raft_->GetPeedingPeers(&pendings);
            if (!pendings.empty()) {
                ret += "\r\n: pendings: ";
                for (const auto& p : pendings) {
                    ret += std::to_string(p) + " ";
                }
            }
            return ret;
        }
    }
    return "info response";
}

std::string Server::handleSubmit(const std::vector<std::string>& args) {
    std::string cmd = args[0];
    auto s = raft_->Submit(cmd);
    if (!s.ok()) {
        return std::string("Submit Error: ") + s.ToString();
    } else {
        return "OK.";
    }
}

std::string Server::handleMemeber(const std::vector<std::string>& args) {
    assert(args.size() >= 2);

    ConfChange cc;
    if (args[0] == "add") {
        cc.type = ConfChangeType::kAdd;
    } else if (args[0] == "remove") {
        cc.type = ConfChangeType::kRemove;
    } else {
        return "ERR: unsupported command";
    }

    int node = atoi(args[1].c_str());
    if (node == 0 || static_cast<uint64_t>(node) > kMaxNodeID) {
        return "ERR: invalid node id";
    } else {
        cc.peer.type = PeerType::kNormal;
        cc.peer.node_id = node;
    }

    auto s = raft_->ChangeMemeber(cc);
    if (!s.ok()) {
        return std::string("ERR: change memeber failed: ") + s.ToString();
    } else {
        return "OK";
    }
}

std::string Server::handleTruncate(const std::vector<std::string>& args) {
    assert(args.size() >= 1);
    int index = atoi(args[0].c_str());
    if (index == 0) {
        return "ERR: invalid truncate argument";
    } else {
        raft_->Truncate(index);
        return "OK";
    }
}

std::string Server::handleElect(const std::vector<std::string>& args) {
    auto s = raft_->TryToLeader();
    if (!s.ok()) {
        return std::string("ERR: ") + s.ToString();
    } else {
        return "OK";
    }
}


// 开关，使快照并发达到最大
std::string Server::handleTest(const std::vector<std::string>& args) {
    // static std::vector<raft::impl::SnapshotSendContext*> ctxs;
    // if (!ctxs.empty()) {
    //     for (auto ctx: ctxs) {
    //         delete ctx;
    //     }
    //     ctxs.clear();
    //     return "Cleared";
    // } else {
    //     while (true) {
    //         auto ctx = raft::impl::RaftFsm::snap_send_mgr_.Get(kSnapshotConcurrency);
    //         if (ctx == nullptr) {
    //             break;
    //         } else {
    //             ctxs.push_back(ctx);
    //         }
    //     }
    //     return "OK";
    // }
    return "not implemented";
}

} /* namespace playground  */
} /* namespace raft */
} /* namespace fbase */