_Pragma("once");

#include <memory>
#include <vector>
#include "base/status.h"
#include "raft/snapshot.h"
#include "raft/types.h"

namespace fbase {
namespace raft {

class StateMachine {
public:
    StateMachine() {}
    virtual ~StateMachine() {}

    virtual Status Apply(const std::string& cmd, uint64_t index) = 0;
    virtual Status ApplyMemberChange(const ConfChange& cc, uint64_t index) = 0;

    // raft复制命令时发生错误，如当前节点不是leader等
    virtual void OnReplicateError(const std::string& cmd,
                                  const Status& status) = 0;

    virtual void OnLeaderChange(uint64_t leader, uint64_t term) = 0;

    virtual std::shared_ptr<Snapshot> GetSnapshot() = 0;

    virtual Status ApplySnapshotStart(const std::string& context) = 0;
    virtual Status ApplySnapshotData(const std::vector<std::string>& datas) = 0;
    virtual Status ApplySnapshotFinish(uint64_t index) = 0;
};

} /* namespace raft */
} /* namespace fbase */
