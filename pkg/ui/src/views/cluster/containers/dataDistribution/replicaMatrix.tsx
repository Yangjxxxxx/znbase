  
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

import _ from "lodash";
import React, { Component } from "react";
import classNames from "classnames";

import {
  TreeNode,
  TreePath,
  layoutTreeHorizontal,
  flatten,
  sumValuesUnderPaths,
  LayoutCell,
  FlattenedNode,
} from "./tree";
import { ToolTipWrapper } from "src/views/shared/components/toolTip";
import { TimestampToMoment } from "src/util/convert";

import { znbase } from "src/js/protos";
import NodeDescriptor$Properties = znbase.roachpb.INodeDescriptor;
import { google } from "src/js/protos";
import ITimestamp = google.protobuf.ITimestamp;

import "./replicaMatrix.styl";

const DOWN_ARROW = "▼";
const SIDE_ARROW = "▶";

interface ReplicaMatrixState {
  collapsedRows: TreePath[];
  collapsedCols: TreePath[];
}

interface ReplicaMatrixProps {
  cols: TreeNode<NodeDescriptor$Properties>;
  rows: TreeNode<SchemaObject>;
  getValue: (rowPath: TreePath, colPath: TreePath) => number;
}

// Amount to indent for a row each level of depth in the tree.
const ROW_TREE_INDENT_PX = 18;
// Margin for all rows in the matrix. Strangely, <th>s can't have margins
// applied in CSS.
const ROW_LEFT_MARGIN_PX = 5;

class ReplicaMatrix extends Component<ReplicaMatrixProps, ReplicaMatrixState> {

  constructor(props: ReplicaMatrixProps) {
    super(props);
    this.state = {
      collapsedRows: [["defaultdb"], ["system"], ["postgres"]],
      collapsedCols: [],
    };
  }

  expandRow = (path: TreePath) => {
    this.setState({
      collapsedRows: this.state.collapsedRows.filter((tp) => !_.isEqual(tp, path)),
    });
  }

  collapseRow = (path: TreePath) => {
    this.setState({
      collapsedRows: [...this.state.collapsedRows, path],
    });
  }

  expandCol = (path: TreePath) => {
    this.setState({
      collapsedCols: this.state.collapsedCols.filter((tp) => !_.isEqual(tp, path)),
    });
  }

  collapseCol = (path: TreePath) => {
    this.setState({
      collapsedCols: [...this.state.collapsedCols, path],
    });
  }

  colLabel(col: LayoutCell<NodeDescriptor$Properties>): string {
    if (col.isPlaceholder) {
      return null;
    }

    if (col.isLeaf) {
      return `n${col.data.node_id}`;
    }

    const arrow = col.isCollapsed ? SIDE_ARROW : DOWN_ARROW;
    const localityLabel = col.path.length === 0 ? "Cluster" : col.path[col.path.length - 1];
    return `${arrow} ${localityLabel}`;
  }

  rowLabelText(row: FlattenedNode<SchemaObject>) {
    if (row.isLeaf) {
      return row.data.tableName;
    }

    const arrow = row.isCollapsed ? SIDE_ARROW : DOWN_ARROW;
    const label = row.data.dbName ? `DB: ${row.data.dbName}` : "Cluster";

    return `${arrow} ${label}`;
  }

  rowLabel(row: FlattenedNode<SchemaObject>) {
    const text = this.rowLabelText(row);

    const label = (
      <span className={classNames("table-label", { "table-label--dropped": !!row.data.droppedAt })}>
        {text}
      </span>
    );

    if (row.data.droppedAt) {
      return (
        <ToolTipWrapper
          text={
            <span>
              下降的 {TimestampToMoment(row.data.droppedAt).format()}.
              最终是否会根据此模式进行垃圾收集
            </span>
          }
        >
          {label}
        </ToolTipWrapper>
      );
    } else {
      return label;
    }
  }

  render() {
    const {
      cols,
      rows,
      getValue,
    } = this.props;
    const {
      collapsedRows,
      collapsedCols,
    } = this.state;

    const flattenedRows = flatten(rows, collapsedRows, true /* includeNodes */);
    const headerRows = layoutTreeHorizontal(cols, collapsedCols);
    const flattenedCols = flatten(cols, collapsedCols, false /* includeNodes */);

    return (
      <table className="matrix">
        <thead>
          {headerRows.map((row, idx) => (
            <tr key={idx}>
              {idx === 0
                ? <th className="matrix__metric-label"># 副本</th>
                : <th />}
              {row.map((col) => (
                <th
                  key={col.path.join("/")}
                  colSpan={col.width}
                  className={classNames(
                    "matrix__column-header",
                    { "matrix__column-header--internal-node": !(col.isLeaf || col.isPlaceholder) },
                  )}
                  onClick={() => (
                    col.isCollapsed
                      ? this.expandCol(col.path)
                      : this.collapseCol(col.path)
                  )}
                >
                  {this.colLabel(col)}
                </th>
              ))}
            </tr>
          ))}
        </thead>
        <tbody>
          {flattenedRows.map((row) => {
            return (
              <tr
                key={row.path.join("/")}
                className={classNames(
                  "matrix__row",
                  { "matrix__row--internal-node": !row.isLeaf },
                )}
                onClick={() => (
                  row.isCollapsed
                    ? this.expandRow(row.path)
                    : this.collapseRow(row.path)
                )}
              >
                <th
                  className={classNames(
                    "matrix__row-header",
                    { "matrix__row-header--internal-node": !row.isLeaf },
                  )}
                  style={{ paddingLeft: row.depth * ROW_TREE_INDENT_PX + ROW_LEFT_MARGIN_PX }}
                >
                  {this.rowLabel(row)}
                </th>
                {flattenedCols.map((col) => {
                  return (
                    <td
                      key={col.path.join("/")}
                      className="matrix__cell-value"
                    >
                      {row.isLeaf || row.isCollapsed
                        ? emptyIfZero(sumValuesUnderPaths(rows, cols, row.path, col.path, getValue))
                        : null}
                    </td>
                  );
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
    );
  }

}

function emptyIfZero(n: number): string {
  if (n === 0) {
    return "";
  }
  return `${n}`;
}

export default ReplicaMatrix;

export interface SchemaObject {
  dbName?: string;
  tableName?: string;
  droppedAt?: ITimestamp;
}
