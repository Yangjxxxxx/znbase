// Code generated by "stringer -type=Method"; DO NOT EDIT.

package roachpb

import "strconv"

const _Method_name = "GetPutConditionalPutIncrementDeleteDeleteRangeClearRangeRevertRangeScanReverseScanEndTransactionAdminSplitAdminMergeAdminTransferLeaseAdminChangeReplicasAdminRelocateRangeHeartbeatTxnGCPushTxnRecoverTxnQueryTxnQueryIntentQueryLockResolveIntentResolveIntentRangeMergeTruncateLogRequestLeaseTransferLeaseLeaseInfoComputeChecksumCheckConsistencyInitPutWriteBatchExportImportDumpDumpOnlineLoadAdminScatterAddSSTableRecomputeStatsRefreshRefreshRangeSubsumeRangeStatsRevert"

var _Method_index = [...]uint16{0, 3, 6, 20, 29, 35, 46, 56, 67, 71, 82, 96, 106, 116, 134, 153, 171, 183, 185, 192, 202, 210, 221, 230, 243, 261, 266, 277, 289, 302, 311, 326, 342, 349, 359, 365, 371, 375, 385, 389, 401, 411, 425, 432, 444, 451, 461, 467}

func (i Method) String() string {
	if i < 0 || i >= Method(len(_Method_index)-1) {
		return "Method(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _Method_name[_Method_index[i]:_Method_index[i+1]]
}
