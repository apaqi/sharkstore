_Pragma("once");

#include "base/status.h"

#include "options.h"
#include "status.h"

namespace fbase {
namespace raft {

class Raft;

class RaftServer {
public:
    RaftServer() {}
    virtual ~RaftServer() {}

    RaftServer(const RaftServer&) = delete;
    RaftServer& operator=(const RaftServer&) = delete;

    virtual Status Start() = 0;
    virtual Status Stop() = 0;

    virtual const RaftServerOptions& Options() const = 0;

    // 创建raft
    virtual Status CreateRaft(const RaftOptions&,
                              std::shared_ptr<Raft>* raft) = 0;

    // 删除raft，同时会删除所属日志文件
    virtual Status RemoveRaft(uint64_t id, bool backup = false) = 0;

    virtual std::shared_ptr<Raft> FindRaft(uint64_t id) const = 0;

    virtual void GetStatus(ServerStatus* status) = 0;
};

std::unique_ptr<RaftServer> CreateRaftServer(const RaftServerOptions& ops);

} /* namespace raft */
} /* namespace fbase */
