  
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
import React from "react";

import * as protos from "src/js/protos";
import { CachedDataReducerState } from "src/redux/cachedDataReducer";
import Loading from "src/views/shared/components/loading";

interface ConnectionsTableProps {
  range: CachedDataReducerState<protos.znbase.server.serverpb.RangeResponse>;
}

export default function ConnectionsTable(props: ConnectionsTableProps) {
  const { range } = props;
  let ids: number[];
  let viaNodeID = "";
  if (range && !range.inFlight && !_.isNil(range.data)) {
    ids = _.chain(_.keys(range.data.responses_by_node_id))
      .map(id => parseInt(id, 10))
      .sortBy(id => id)
      .value();
    viaNodeID = ` (via n${range.data.node_id.toString()})`;
  }

  return (
    <div>
      <h2>连接 {viaNodeID}</h2>
      <Loading
        loading={!range || range.inFlight}
        error={range && range.lastError}
        render={() => (
          <table className="connections-table">
            <tbody>
              <tr className="connections-table__row connections-table__row--header">
                <th className="connections-table__cell connections-table__cell--header">节点</th>
                <th className="connections-table__cell connections-table__cell--header">是否有效</th>
                <th className="connections-table__cell connections-table__cell--header">副本</th>
                <th className="connections-table__cell connections-table__cell--header">错误</th>
              </tr>
              {
                _.map(ids, id => {
                  const resp = range.data.responses_by_node_id[id];
                  const rowClassName = classNames(
                    "connections-table__row",
                    { "connections-table__row--warning": !resp.response || !_.isEmpty(resp.error_message) },
                  );
                  return (
                    <tr key={id} className={rowClassName}>
                      <td className="connections-table__cell">n{id}</td>
                      <td className="connections-table__cell">
                        {resp.response ? "ok" : "error"}
                      </td>
                      <td className="connections-table__cell">{resp.infos.length}</td>
                      <td className="connections-table__cell">{resp.error_message}</td>
                    </tr>
                  );
                })
              }
            </tbody>
          </table>
        )}
      />
    </div>
  );
}
