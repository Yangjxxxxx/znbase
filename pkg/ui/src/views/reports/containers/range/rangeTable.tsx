  
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

import classNames from "classnames";
import _ from "lodash";
import Long from "long";
import moment from "moment";
import React from "react";

import * as protos from "src/js/protos";
import { FixLong } from "src/util/fixLong";
import { LongToMoment, NanoToMilli } from "src/util/convert";
import { Bytes } from "src/util/format";
import Lease from "src/views/reports/containers/range/lease";
import Print from "src/views/reports/containers/range/print";
import RangeInfo from "src/views/reports/containers/range/rangeInfo";

interface RangeTableProps {
  infos: protos.znbase.server.serverpb.IRangeInfo[];
  replicas: protos.znbase.roachpb.IReplicaDescriptor[];
}

interface RangeTableRow {
  readonly variable: string;
  readonly display: string;
  readonly compareToLeader: boolean; // When true, displays a warning when a
  // value doesn't match the leader's.
}

interface RangeTableCellContent {
  value: string[];
  title?: string[];
  className?: string[];
}

interface RangeTableDetail {
  [name: string]: RangeTableCellContent;
}

const rangeTableDisplayList: RangeTableRow[] = [
  { variable: "id", display: "ID", compareToLeader: false },
  { variable: "keyRange", display: "主 Range", compareToLeader: true },
  { variable: "problems", display: "问题", compareToLeader: true },
  { variable: "raftState", display: "Raft 状态", compareToLeader: false },
  { variable: "quiescent", display: "静止的", compareToLeader: true },
  { variable: "ticking", display: "时钟", compareToLeader: true },
  { variable: "leaseType", display: "租约类型", compareToLeader: true },  
  { variable: "leaseState", display: "租约状态", compareToLeader: true },
  { variable: "leaseHolder", display: "Lease Holder", compareToLeader: true },
  { variable: "leaseEpoch", display: "Lease Epoch", compareToLeader: true },
  { variable: "leaseStart", display: "租约开始时间", compareToLeader: true },
  { variable: "leaseExpiration", display: "租约到期时间", compareToLeader: true },
  { variable: "leaseAppliedIndex", display: "租约申请索引", compareToLeader: true },
  { variable: "raftLeader", display: "主Raft", compareToLeader: true },
  { variable: "vote", display: "投票", compareToLeader: false },
  { variable: "term", display: "Term", compareToLeader: true },
  { variable: "leadTransferee", display: "Lead Transferee", compareToLeader: false },
  { variable: "applied", display: "申请", compareToLeader: true },
  { variable: "commit", display: "提交", compareToLeader: true },
  { variable: "lastIndex", display: "最后的索引", compareToLeader: true },
  { variable: "logSize", display: "日志大小", compareToLeader: false },
  { variable: "logSizeTrusted", display: "日志大小是可信的?", compareToLeader: false },
  { variable: "leaseHolderQPS", display: "Lease Holder QPS", compareToLeader: false },
  { variable: "keysWrittenPS", display: "每秒写入的平均密钥", compareToLeader: false },
  { variable: "approxProposalQuota", display: "Approx Proposal Quota", compareToLeader: false },
  { variable: "pendingCommands", display: "挂起的命令", compareToLeader: false },
  { variable: "droppedCommands", display: "放弃的命令", compareToLeader: false },
  { variable: "truncatedIndex", display: "截断索引", compareToLeader: true },
  { variable: "truncatedTerm", display: "Truncated Term", compareToLeader: true },
  { variable: "mvccLastUpdate", display: "最新更新的MVCC", compareToLeader: true },
  { variable: "mvccIntentAge", display: "MVCC Intent Age", compareToLeader: true },
  { variable: "mvccGGBytesAge", display: "MVCC GG Bytes Age", compareToLeader: true },
  { variable: "mvccLiveBytesCount", display: "MVCC Live Bytes/Count", compareToLeader: true },
  { variable: "mvccKeyBytesCount", display: "MVCC Key Bytes/Count", compareToLeader: true },
  { variable: "mvccValueBytesCount", display: "MVCC Value Bytes/Count", compareToLeader: true },
  { variable: "mvccIntentBytesCount", display: "MVCC Intent Bytes/Count", compareToLeader: true },
  { variable: "mvccSystemBytesCount", display: "MVCC System Bytes/Count", compareToLeader: true },
  { variable: "rangeMaxBytes", display: "分割前的最大Range大小", compareToLeader: true },
  { variable: "writeLatches", display: "写琐 本地/全局", compareToLeader: false },
  { variable: "readLatches", display: "读锁 本地/全局", compareToLeader: false },
];

const rangeTableEmptyContent: RangeTableCellContent = {
  value: ["-"],
};

const rangeTableEmptyContentWithWarning: RangeTableCellContent = {
  value: ["-"],
  className: ["range-table__cell--warning"],
};

const rangeTableQuiescent: RangeTableCellContent = {
  value: ["quiescent"],
  className: ["range-table__cell--quiescent"],
};

function convertLeaseState(leaseState: protos.znbase.storage.LeaseState) {
  return protos.znbase.storage.LeaseState[leaseState].toLowerCase();
}

export default class RangeTable extends React.Component<RangeTableProps, {}> {
  cleanRaftState(state: string) {
    switch (_.toLower(state)) {
      case "statedormant": return "dormant";
      case "stateleader": return "leader";
      case "statefollower": return "follower";
      case "statecandidate": return "candidate";
      case "stateprecandidate": return "precandidate";
      default: return "unknown";
    }
  }

  contentRaftState(state: string): RangeTableCellContent {
    const cleanedState = this.cleanRaftState(state);
    return {
      value: [cleanedState],
      className: [`range-table__cell--raftstate-${cleanedState}`],
    };
  }

  contentNanos(nanos: Long): RangeTableCellContent {
    const humanized = Print.Time(LongToMoment(nanos));
    return {
      value: [humanized],
      title: [humanized, nanos.toString()],
    };
  }

  contentDuration(nanos: Long): RangeTableCellContent {
    const humanized = Print.Duration(moment.duration(NanoToMilli(nanos.toNumber())));
    return {
      value: [humanized],
      title: [humanized, nanos.toString()],
    };
  }

  contentMVCC(bytes: Long, count: Long): RangeTableCellContent {
    const humanizedBytes = Bytes(bytes.toNumber());
    return {
      value: [`${humanizedBytes} / ${count.toString()} count`],
      title: [`${humanizedBytes} / ${count.toString()} count`,
      `${bytes.toString()} bytes / ${count.toString()} count`],
    };
  }

  contentBytes(bytes: Long): RangeTableCellContent {
    const humanized = Bytes(bytes.toNumber());
    return {
      value: [humanized],
      title: [humanized, bytes.toString()],
    };
  }

  createContent(value: string | Long | number, className: string = null): RangeTableCellContent {
    if (_.isNull(className)) {
      return {
        value: [value.toString()],
      };
    }
    return {
      value: [value.toString()],
      className: [className],
    };
  }

  contentLatchInfo(
    local: Long | number, global: Long | number, isRaftLeader: boolean,
  ): RangeTableCellContent {
    if (isRaftLeader) {
      return this.createContent(`${local.toString()} local / ${global.toString()} global`);
    }
    if (local.toString() === "0" && global.toString() === "0") {
      return rangeTableEmptyContent;
    }
    return this.createContent(
      `${local.toString()} local / ${global.toString()} global`,
      "range-table__cell--warning",
    );
  }

  contentTimestamp(timestamp: protos.znbase.util.hlc.ITimestamp): RangeTableCellContent {
    if (_.isNil(timestamp) || _.isNil(timestamp.wall_time)) {
      return {
        value: ["no timestamp"],
        className: ["range-table__cell--warning"],
      };
    }
    const humanized = Print.Timestamp(timestamp);
    return {
      value: [humanized],
      title: [humanized, FixLong(timestamp.wall_time).toString()],
    };
  }

  contentProblems(
    problems: protos.znbase.server.serverpb.IRangeProblems,
    awaitingGC: boolean,
  ): RangeTableCellContent {
    let results: string[] = [];
    if (problems.no_lease) {
      results = _.concat(results, "Invalid Lease");
    }
    if (problems.leader_not_lease_holder) {
      results = _.concat(results, "Leader is Not Lease holder");
    }
    if (problems.underreplicated) {
      results = _.concat(results, "Underreplicated (or slow)");
    }
    if (problems.overreplicated) {
      results = _.concat(results, "Overreplicated");
    }
    if (problems.no_raft_leader) {
      results = _.concat(results, "No Raft Leader");
    }
    if (problems.unavailable) {
      results = _.concat(results, "Unavailable");
    }
    if (problems.quiescent_equals_ticking) {
      results = _.concat(results, "Quiescent equals ticking");
    }
    if (problems.raft_log_too_large) {
      results = _.concat(results, "Raft log too large");
    }
    if (awaitingGC) {
      results = _.concat(results, "Awaiting GC");
    }
    return {
      value: results,
      title: results,
      className: results.length > 0 ? ["range-table__cell--warning"] : [],
    };
  }

  // contentIf returns an empty value if the condition is false, and if true,
  // executes and returns the content function.
  contentIf(
    showContent: boolean,
    content: () => RangeTableCellContent,
  ): RangeTableCellContent {
    if (!showContent) {
      return rangeTableEmptyContent;
    }
    return content();
  }

  renderRangeCell(
    row: RangeTableRow,
    cell: RangeTableCellContent,
    key: number,
    dormant: boolean,
    leaderCell?: RangeTableCellContent,
  ) {
    const title = _.join(_.isNil(cell.title) ? cell.value : cell.title, "\n");
    const differentFromLeader = !dormant && !_.isNil(leaderCell) && row.compareToLeader && (!_.isEqual(cell.value, leaderCell.value) || !_.isEqual(cell.title, leaderCell.title));
    const className = classNames(
      "range-table__cell",
      {
        "range-table__cell--dormant": dormant,
        "range-table__cell--different-from-leader-warning": differentFromLeader,
      },
      (!dormant && !_.isNil(cell.className) ? cell.className : []),
    );
    return (
      <td key={key} className={className} title={title}>
        <ul className="range-entries-list">
          {
            _.map(cell.value, (value, k) => (
              <li key={k}>
                {value}
              </li>
            ))
          }
        </ul>
      </td>
    );
  }

  renderRangeRow(
    row: RangeTableRow,
    detailsByStoreID: Map<number, RangeTableDetail>,
    dormantStoreIDs: Set<number>,
    leaderStoreID: number,
    sortedStoreIDs: number[],
    key: number,
  ) {
    const leaderDetail = detailsByStoreID.get(leaderStoreID);
    const values: Set<string> = new Set();
    if (row.compareToLeader) {
      detailsByStoreID.forEach((detail, storeID) => {
        if (!dormantStoreIDs.has(storeID)) {
          values.add(_.join(detail[row.variable].value, " "));
        }
      });
    }
    const headerClassName = classNames(
      "range-table__cell",
      "range-table__cell--header",
      { "range-table__cell--header-warning": values.size > 1 },
    );
    return (
      <tr key={key} className="range-table__row">
        <th className={headerClassName}>
          {row.display}
        </th>
        {
          _.map(sortedStoreIDs, (storeID) => {
            const cell = detailsByStoreID.get(storeID)[row.variable];
            const leaderCell = (storeID === leaderStoreID) ? null : leaderDetail[row.variable];
            return (
              this.renderRangeCell(
                row,
                cell,
                storeID,
                dormantStoreIDs.has(storeID),
                leaderCell,
              )
            );
          })
        }
      </tr>
    );
  }

  renderRangeReplicaCell(
    leaderReplicaIDs: Set<number>,
    replicaID: number,
    replica: protos.znbase.roachpb.IReplicaDescriptor,
    rangeID: Long,
    localStoreID: number,
    dormant: boolean,
  ) {
    const differentFromLeader = !dormant && (_.isNil(replica) ? leaderReplicaIDs.has(replicaID) : !leaderReplicaIDs.has(replica.replica_id));
    const localReplica = !dormant && !differentFromLeader && replica && replica.store_id === localStoreID;
    const className = classNames({
      "range-table__cell": true,
      "range-table__cell--dormant": dormant,
      "range-table__cell--different-from-leader-warning": differentFromLeader,
      "range-table__cell--local-replica": localReplica,
    });
    if (_.isNil(replica)) {
      return (
        <td key={localStoreID} className={className}>
          -
        </td>
      );
    }
    const value = Print.ReplicaID(rangeID, replica);
    return (
      <td key={localStoreID} className={className} title={value}>
        {value}
      </td>
    );
  }

  renderRangeReplicaRow(
    replicasByReplicaIDByStoreID: Map<number, Map<number, protos.znbase.roachpb.IReplicaDescriptor>>,
    referenceReplica: protos.znbase.roachpb.IReplicaDescriptor,
    leaderReplicaIDs: Set<number>,
    dormantStoreIDs: Set<number>,
    sortedStoreIDs: number[],
    rangeID: Long,
    key: string,
  ) {
    const headerClassName = "range-table__cell range-table__cell--header";
    return (
      <tr key={key} className="range-table__row">
        <th className={headerClassName}>
          副本 {referenceReplica.replica_id} - ({Print.ReplicaID(rangeID, referenceReplica)})
        </th>
        {
          _.map(sortedStoreIDs, storeID => {
            let replica: protos.znbase.roachpb.IReplicaDescriptor = null;
            if (replicasByReplicaIDByStoreID.has(storeID) &&
              replicasByReplicaIDByStoreID.get(storeID).has(referenceReplica.replica_id)) {
              replica = replicasByReplicaIDByStoreID.get(storeID).get(referenceReplica.replica_id);
            }
            return this.renderRangeReplicaCell(
              leaderReplicaIDs,
              referenceReplica.replica_id,
              replica,
              rangeID,
              storeID,
              dormantStoreIDs.has(storeID),
            );
          })
        }
      </tr>
    );
  }

  render() {
    const { infos, replicas } = this.props;
    const leader = _.head(infos);
    const rangeID = leader.state.state.desc.range_id;

    // We want to display ordered by store ID.
    const sortedStoreIDs = _.chain(infos)
      .map(info => info.source_store_id)
      .sortBy(id => id)
      .value();

    const dormantStoreIDs: Set<number> = new Set();

    // Convert the infos to a simpler object for display purposes. This helps when trying to
    // determine if any warnings should be displayed.
    const detailsByStoreID: Map<number, RangeTableDetail> = new Map();
    _.forEach(infos, info => {
      const localReplica = RangeInfo.GetLocalReplica(info);
      const awaitingGC = _.isNil(localReplica);
      const lease = info.state.state.lease;
      const epoch = Lease.IsEpoch(lease);
      const raftLeader = !awaitingGC && FixLong(info.raft_state.lead).eq(localReplica.replica_id);
      const leaseHolder = !awaitingGC && localReplica.replica_id === lease.replica.replica_id;
      const mvcc = info.state.state.stats;
      const raftState = this.contentRaftState(info.raft_state.state);
      const vote = FixLong(info.raft_state.hard_state.vote);
      let leaseState: RangeTableCellContent;
      if (_.isNil(info.lease_status)) {
        leaseState = rangeTableEmptyContentWithWarning;
      } else {
        leaseState = this.createContent(
          convertLeaseState(info.lease_status.state),
          info.lease_status.state === protos.znbase.storage.LeaseState.VALID ? "" :
            "range-table__cell--warning",
        );
      }
      const dormant = raftState.value[0] === "dormant";
      if (dormant) {
        dormantStoreIDs.add(info.source_store_id);
      }
      detailsByStoreID.set(info.source_store_id, {
        id: this.createContent(Print.ReplicaID(
          rangeID,
          localReplica,
          info.source_node_id,
          info.source_store_id,
        )),
        keyRange: this.createContent(`${info.span.start_key} to ${info.span.end_key}`),
        problems: this.contentProblems(info.problems, awaitingGC),
        raftState: raftState,
        quiescent: info.quiescent ? rangeTableQuiescent : rangeTableEmptyContent,
        ticking: this.createContent(info.ticking.toString()),
        leaseState: leaseState,
        leaseHolder: this.createContent(
          Print.ReplicaID(rangeID, lease.replica),
          leaseHolder ? "range-table__cell--lease-holder" : "range-table__cell--lease-follower",
        ),
        leaseType: this.createContent(epoch ? "epoch" : "expiration"),
        leaseEpoch: epoch ? this.createContent(lease.epoch) : rangeTableEmptyContent,
        leaseStart: this.contentTimestamp(lease.start),
        leaseExpiration: epoch ? rangeTableEmptyContent : this.contentTimestamp(lease.expiration),
        leaseAppliedIndex: this.createContent(FixLong(info.state.state.lease_applied_index)),
        raftLeader: this.contentIf(!dormant, () => this.createContent(
          FixLong(info.raft_state.lead),
          raftLeader ? "range-table__cell--raftstate-leader" : "range-table__cell--raftstate-follower",
        )),
        vote: this.contentIf(!dormant, () => this.createContent(vote.greaterThan(0) ? vote : "-")),
        term: this.contentIf(!dormant, () => this.createContent(FixLong(info.raft_state.hard_state.term))),
        leadTransferee: this.contentIf(!dormant, () => {
          const leadTransferee = FixLong(info.raft_state.lead_transferee);
          return this.createContent(leadTransferee.greaterThan(0) ? leadTransferee : "-");
        }),
        applied: this.contentIf(!dormant, () => this.createContent(FixLong(info.raft_state.applied))),
        commit: this.contentIf(!dormant, () => this.createContent(FixLong(info.raft_state.hard_state.commit))),
        lastIndex: this.createContent(FixLong(info.state.last_index)),
        logSize: this.contentBytes(FixLong(info.state.raft_log_size)),
        logSizeTrusted: this.createContent(info.state.raft_log_size_trusted.toString()),
        leaseHolderQPS: leaseHolder ? this.createContent(info.stats.queries_per_second.toFixed(4)) : rangeTableEmptyContent,
        keysWrittenPS: this.createContent(info.stats.writes_per_second.toFixed(4)),
        approxProposalQuota: raftLeader ? this.createContent(FixLong(info.state.approximate_proposal_quota)) : rangeTableEmptyContent,
        pendingCommands: this.createContent(FixLong(info.state.num_pending)),
        droppedCommands: this.createContent(
          FixLong(info.state.num_dropped),
          FixLong(info.state.num_dropped).greaterThan(0) ? "range-table__cell--warning" : "",
        ),
        truncatedIndex: this.createContent(FixLong(info.state.state.truncated_state.index)),
        truncatedTerm: this.createContent(FixLong(info.state.state.truncated_state.term)),
        mvccLastUpdate: this.contentNanos(FixLong(mvcc.last_update_nanos)),
        mvccIntentAge: this.contentDuration(FixLong(mvcc.intent_age)),
        mvccGGBytesAge: this.contentDuration(FixLong(mvcc.gc_bytes_age)),
        mvccLiveBytesCount: this.contentMVCC(FixLong(mvcc.live_bytes), FixLong(mvcc.live_count)),
        mvccKeyBytesCount: this.contentMVCC(FixLong(mvcc.key_bytes), FixLong(mvcc.key_count)),
        mvccValueBytesCount: this.contentMVCC(FixLong(mvcc.val_bytes), FixLong(mvcc.val_count)),
        mvccIntentBytesCount: this.contentMVCC(FixLong(mvcc.intent_bytes), FixLong(mvcc.intent_count)),
        mvccSystemBytesCount: this.contentMVCC(FixLong(mvcc.sys_bytes), FixLong(mvcc.sys_count)),
        rangeMaxBytes: this.contentBytes(FixLong(info.state.range_max_bytes)),
        writeLatches: this.contentLatchInfo(
          FixLong(info.latches_local.write_count),
          FixLong(info.latches_global.write_count),
          raftLeader,
        ),
        readLatches: this.contentLatchInfo(
          FixLong(info.latches_local.read_count),
          FixLong(info.latches_global.read_count),
          raftLeader,
        ),
      });
    });

    const leaderReplicaIDs = new Set(_.map(leader.state.state.desc.replicas, rep => rep.replica_id));

    // Go through all the replicas and add them to map for easy printing.
    const replicasByReplicaIDByStoreID: Map<number, Map<number, protos.znbase.roachpb.IReplicaDescriptor>> = new Map();
    _.forEach(infos, info => {
      const replicasByReplicaID: Map<number, protos.znbase.roachpb.IReplicaDescriptor> = new Map();
      _.forEach(info.state.state.desc.replicas, rep => {
        replicasByReplicaID.set(rep.replica_id, rep);
      });
      replicasByReplicaIDByStoreID.set(info.source_store_id, replicasByReplicaID);
    });

    return (
      <div>
        <h2>Range r{rangeID.toString()} at {Print.Time(moment().utc())} UTC</h2>
        <table className="range-table">
          <tbody>
            {
              _.map(rangeTableDisplayList, (title, key) => (
                this.renderRangeRow(
                  title,
                  detailsByStoreID,
                  dormantStoreIDs,
                  leader.source_store_id,
                  sortedStoreIDs,
                  key,
                )
              ))
            }
            {
              _.map(replicas, (replica, key) => (
                this.renderRangeReplicaRow(
                  replicasByReplicaIDByStoreID,
                  replica,
                  leaderReplicaIDs,
                  dormantStoreIDs,
                  sortedStoreIDs,
                  rangeID,
                  "replica" + key,
                )
              ))
            }
          </tbody>
        </table>
      </div>
    );
  }
}
