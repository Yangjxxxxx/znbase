// Code generated by "stringer -type refreshRaftReason"; DO NOT EDIT.

package storage

import "strconv"

const _refreshRaftReason_name = "noReasonreasonNewLeaderreasonNewLeaderOrConfigChangereasonSnapshotAppliedreasonReplicaIDChangedreasonTicks"

var _refreshRaftReason_index = [...]uint8{0, 8, 23, 52, 73, 95, 106}

func (i refreshRaftReason) String() string {
	if i < 0 || i >= refreshRaftReason(len(_refreshRaftReason_index)-1) {
		return "refreshRaftReason(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _refreshRaftReason_name[_refreshRaftReason_index[i]:_refreshRaftReason_index[i+1]]
}
