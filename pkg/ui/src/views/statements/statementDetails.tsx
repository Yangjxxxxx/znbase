  
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

import d3 from "d3";
import _ from "lodash";
import React, { ReactNode } from "react";
import { Helmet } from "react-helmet";
import { connect } from "react-redux";
import { Link, RouterState } from "react-router";
import { createSelector } from "reselect";

import Loading from "src/views/shared/components/loading";
import { refreshStatements } from "src/redux/apiReducers";
import { nodeDisplayNameByIDSelector } from "src/redux/nodes";
import { AdminUIState } from "src/redux/state";
import { NumericStat, stdDev, combineStatementStats, flattenStatementStats, StatementStatistics, ExecutionStatistics } from "src/util/appStats";
import { statementAttr, appAttr } from "src/util/constants";
import { FixLong } from "src/util/fixLong";
import { Duration } from "src/util/format";
import { intersperse } from "src/util/intersperse";
import { Pick } from "src/util/pick";
import { PlanView } from "src/views/statements/planView";
import { SortSetting } from "src/views/shared/components/sortabletable";
import { SqlBox } from "src/views/shared/components/sql/box";
import { SummaryBar, SummaryHeadlineStat } from "src/views/shared/components/summaryBar";
import { ToolTipWrapper } from "src/views/shared/components/toolTip";

import { countBreakdown, rowsBreakdown, latencyBreakdown, approximify } from "./barCharts";
import { AggregateStatistics, StatementsSortedTable, makeNodesColumns } from "./statementsTable";

interface Fraction {
  numerator: number;
  denominator: number;
}

interface SingleStatementStatistics {
  statement: string;
  app: string[];
  distSQL: Fraction;
  opt: Fraction;
  failed: Fraction;
  node_id: number[];
  stats: StatementStatistics;
  byNode: AggregateStatistics[];
}

function AppLink(props: { app: string }) {
  if (!props.app) {
    return <span className="app-name app-name__unset">??????????????????</span>;
  }

  return (
    <Link className="app-name" to={ `/statements/${encodeURIComponent(props.app)}` }>
      ??????????????????
    </Link>
  );
}

interface StatementDetailsOwnProps {
  statement: SingleStatementStatistics;
  statementsError: Error | null;
  nodeNames: { [nodeId: string]: string };
  refreshStatements: typeof refreshStatements;
}

type StatementDetailsProps = StatementDetailsOwnProps & RouterState;

interface StatementDetailsState {
  sortSetting: SortSetting;
}

interface NumericStatRow {
  name: string;
  value: NumericStat;
  bar?: () => ReactNode;
  summary?: boolean;
}

interface NumericStatTableProps {
  title?: string;
  description?: string;
  measure: string;
  rows: NumericStatRow[];
  count: number;
  format?: (v: number) => string;
}

class NumericStatTable extends React.Component<NumericStatTableProps> {
  static defaultProps = {
    format: (v: number) => `${v}`,
  };

  render() {
    const tooltip = !this.props.description ? null : (
        <div className="numeric-stats-table__tooltip">
          <ToolTipWrapper text={this.props.description}>
            <div className="numeric-stats-table__tooltip-hover-area">
              <div className="numeric-stats-table__info-icon">i</div>
            </div>
          </ToolTipWrapper>
        </div>
      );

    return (
      <table className="numeric-stats-table">
        <thead>
          <tr className="numeric-stats-table__row--header">
            <th className="numeric-stats-table__cell">
              { this.props.title }
              { tooltip }
            </th>
            <th className="numeric-stats-table__cell">??????{this.props.measure}</th>
            <th className="numeric-stats-table__cell">?????????</th>
          </tr>
        </thead>
        <tbody style={{ textAlign: "right" }}>
          {
            this.props.rows.map((row: NumericStatRow) => {
              const classNames = "numeric-stats-table__row--body" +
                (row.summary ? " numeric-stats-table__row--summary" : "");
              return (
                <tr className={classNames}>
                  <th className="numeric-stats-table__cell" style={{ textAlign: "left" }}>{ row.name }</th>
                  <td className="numeric-stats-table__cell">{ row.bar ? row.bar() : null }</td>
                  <td className="numeric-stats-table__cell">{ this.props.format(stdDev(row.value, this.props.count)) }</td>
                </tr>
              );
            })
          }
        </tbody>
      </table>
    );
  }
}

class StatementDetails extends React.Component<StatementDetailsProps, StatementDetailsState> {

  constructor(props: StatementDetailsProps) {
    super(props);
    this.state = {
      sortSetting: {
        sortKey: 1,
        ascending: false,
      },
    };
  }

  changeSortSetting = (ss: SortSetting) => {
    this.setState({
      sortSetting: ss,
    });
  }

  componentWillMount() {
    this.props.refreshStatements();
  }

  componentWillReceiveProps() {
    this.props.refreshStatements();
  }

  render() {
    return (
      <div>
        <Helmet>
          <title>
            { "?????? | " + (this.props.params[appAttr] ? this.props.params[appAttr] + " ?????? | " : "") + "??????" }
          </title>
        </Helmet>
        <section className="section"><h1>??????????????????</h1></section>
        <section className="section section--container">
          <Loading
            loading={_.isNil(this.props.statement)}
            error={this.props.statementsError}
            render={this.renderContent}
          />
        </section>
      </div>
    );
  }

  renderContent = () => {
    if (!this.props.statement) {
      return null;
    }

    const { stats, statement, app, distSQL, opt, failed } = this.props.statement;

    if (!stats) {
      const sourceApp = this.props.params[appAttr];
      const listUrl = "/statements" + (sourceApp ? "/" + sourceApp : "");

      return (
        <React.Fragment>
          <section className="section">
            <SqlBox value={ statement } />
          </section>
          <section className="section">
            <h3>??????????????????</h3>
            {/* There are no execution statistics for this statement.{" "} */}
            ????????????????????????????????????{" "}
            <Link className="back-link" to={ listUrl }>
              ????????????
            </Link>
          </section>
        </React.Fragment>
      );
    }

    const count = FixLong(stats.count).toInt();
    const firstAttemptCount = FixLong(stats.first_attempt_count).toInt();

    const { firstAttemptsBarChart, retriesBarChart, maxRetriesBarChart, totalCountBarChart } = countBreakdown(this.props.statement);
    const { rowsBarChart } = rowsBreakdown(this.props.statement);
    const { parseBarChart, planBarChart, runBarChart, overheadBarChart, overallBarChart } = latencyBreakdown(this.props.statement);

    const statsByNode = this.props.statement.byNode;
    const logicalPlan = stats.sensitive_info && stats.sensitive_info.most_recent_plan_description;

    return (
      <div className="content l-columns">
        <div className="l-columns__left">
          <section className="section section--heading">
            <SqlBox value={ statement } />
          </section>
          <section className="section">
            <PlanView
              title="????????????"
              plan={logicalPlan} />
          </section>
          <section className="section">
            <NumericStatTable
              title="??????"
              description="??????????????????????????????????????????"
              measure="??????"
              count={ count }
              format={ (v: number) => Duration(v * 1e9) }
              rows={[
                { name: "??????", value: stats.parse_lat, bar: parseBarChart },
                { name: "??????", value: stats.plan_lat, bar: planBarChart },
                { name: "??????", value: stats.run_lat, bar: runBarChart },
                { name: "??????", value: stats.overhead_lat, bar: overheadBarChart },
                { name: "??????", summary: true, value: stats.service_lat, bar: overallBarChart },
              ]}
            />
          </section>
          <section className="section">
            <StatementsSortedTable
              className="statements-table"
              data={statsByNode}
              columns={makeNodesColumns(statsByNode, this.props.nodeNames)}
              sortSetting={this.state.sortSetting}
              onChangeSortSetting={this.changeSortSetting}
            />
          </section>
          <section className="section">
            <table className="numeric-stats-table">
              <thead>
                <tr className="numeric-stats-table__row--header">
                  <th className="numeric-stats-table__cell" colSpan={ 3 }>
                   ????????????
                    <div className="numeric-stats-table__tooltip">
                      {/* <ToolTipWrapper text="The number of times this statement has been executed."> */}
                      <ToolTipWrapper text="???????????????????????????">
                        <div className="numeric-stats-table__tooltip-hover-area">
                          <div className="numeric-stats-table__info-icon">i</div>
                        </div>
                      </ToolTipWrapper>
                    </div>
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr className="numeric-stats-table__row--body">
                  <th className="numeric-stats-table__cell" style={{ textAlign: "left" }}>??????????????????</th>
                  <td className="numeric-stats-table__cell">{ firstAttemptsBarChart() }</td>
                </tr>
                <tr className="numeric-stats-table__row--body">
                  <th className="numeric-stats-table__cell" style={{ textAlign: "left" }}>????????????</th>
                  <td className="numeric-stats-table__cell">{ retriesBarChart() }</td>
                </tr>
                <tr className="numeric-stats-table__row--body">
                  <th className="numeric-stats-table__cell" style={{ textAlign: "left" }}>??????????????????</th>
                  <td className="numeric-stats-table__cell">{ maxRetriesBarChart() }</td>
                </tr>
                <tr className="numeric-stats-table__row--body numeric-stats-table__row--summary">
                  <th className="numeric-stats-table__cell" style={{ textAlign: "left" }}>??????</th>
                  <td className="numeric-stats-table__cell">{ totalCountBarChart() }</td>
                </tr>
              </tbody>
            </table>
          </section>
          <section className="section">
            <NumericStatTable
              measure="???"
              count={ count }
              format={ (v: number) => "" + (Math.round(v * 100) / 100) }
              rows={[
                { name: "????????????", value: stats.num_rows, bar: rowsBarChart },
              ]}
            />
          </section>
        </div>
        <div className="l-columns__right">
          <SummaryBar>
            <SummaryHeadlineStat
              title="????????????"
              // tooltip="Cumulative time spent servicing this statement."
              tooltip="??????????????????????????????????????????"
              value={ count * stats.service_lat.mean }
              format={ v => Duration(v * 1e9) } />
            <SummaryHeadlineStat
              title="????????????"
              // tooltip="Number of times this statement has executed."
              tooltip="???????????????????????????"
              value={ count }
              format={ approximify } />
            <SummaryHeadlineStat
              title="??????????????????"
              // tooltip="Portion of executions free of retries."
              tooltip="???????????????????????????"
              value={ firstAttemptCount / count }
              format={ d3.format("%") } />
            <SummaryHeadlineStat
              title="??????????????????"
              // tooltip="Latency to parse, plan, and execute the statement."
              tooltip="??????????????????????????????????????????"
              value={ stats.service_lat.mean }
              format={ v => Duration(v * 1e9) } />
            <SummaryHeadlineStat
              title="????????????"
              // tooltip="The average number of rows returned or affected."
              tooltip="????????????????????????????????????"
              value={ stats.num_rows.mean }
              format={ approximify } />
          </SummaryBar>
          <table className="numeric-stats-table">
            <tbody>
              <tr className="numeric-stats-table__row--body">
                <th className="numeric-stats-table__cell" style={{ textAlign: "left" }}>??????</th>
                <td className="numeric-stats-table__cell" style={{ textAlign: "right" }}>
                  {/* { intersperse<ReactNode>(app.map(a => <AppLink app={ a } key={ a } />), ", ") } */}
                  { intersperse<ReactNode>(app.map(a => <AppLink app={ a } key={ a } />), ", ") }
                </td>
              </tr>
              <tr className="numeric-stats-table__row--body">
                <th className="numeric-stats-table__cell" style={{ textAlign: "left" }}>?????????????????????</th>
                <td className="numeric-stats-table__cell" style={{ textAlign: "right" }}>{ renderBools(distSQL) }</td>
              </tr>
              <tr className="numeric-stats-table__row--body">
                <th className="numeric-stats-table__cell" style={{ textAlign: "left" }}>????????????????????????????????????</th>
                <td className="numeric-stats-table__cell" style={{ textAlign: "right" }}>{ renderBools(opt) }</td>
              </tr>
              <tr className="numeric-stats-table__row--body">
                <th className="numeric-stats-table__cell" style={{ textAlign: "left" }}>????????????</th>
                <td className="numeric-stats-table__cell" style={{ textAlign: "right" }}>{ renderBools(failed) }</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    );
  }
}

function renderBools(fraction: Fraction) {
  if (Number.isNaN(fraction.numerator)) {
    return "(unknown)";
  }
  if (fraction.numerator === 0) {
    return "No";
  }
  if (fraction.numerator === fraction.denominator) {
    return "Yes";
  }
  return approximify(fraction.numerator) + " of " + approximify(fraction.denominator);
}

type StatementsState = Pick<AdminUIState, "cachedData", "statements">;

function coalesceNodeStats(stats: ExecutionStatistics[]): AggregateStatistics[] {
  const byNode: { [nodeId: string]: StatementStatistics[] } = {};

  stats.forEach(stmt => {
    const nodeStats = (byNode[stmt.node_id] = byNode[stmt.node_id] || []);
    nodeStats.push(stmt.stats);
  });

  return Object.keys(byNode).map(nodeId => ({
      label: nodeId,
      stats: combineStatementStats(byNode[nodeId]),
  }));
}

function fractionMatching(stats: ExecutionStatistics[], predicate: (stmt: ExecutionStatistics) => boolean): Fraction {
  let numerator = 0;
  let denominator = 0;

  stats.forEach(stmt => {
    const count = FixLong(stmt.stats.first_attempt_count).toInt();
    denominator += count;
    if (predicate(stmt)) {
      numerator += count;
    }
  });

  return { numerator, denominator };
}

export const selectStatement = createSelector(
  (state: StatementsState) => state.cachedData.statements.data && state.cachedData.statements.data.statements,
  (_state: StatementsState, props: { params: { [key: string]: string } }) => props,
  (statements, props) => {
    if (!statements) {
      return null;
    }

    const statement = props.params[statementAttr];
    let app = props.params[appAttr];
    let predicate = (stmt: ExecutionStatistics) => stmt.statement === statement;

    if (app) {
        if (app === "(unset)") {
            app = "";
        }
        predicate = (stmt: ExecutionStatistics) => stmt.statement === statement && stmt.app === app;
    }

    const flattened = flattenStatementStats(statements);
    const results = _.filter(flattened, predicate);

    return {
      statement,
      stats: combineStatementStats(results.map(s => s.stats)),
      byNode: coalesceNodeStats(results),
      app: _.uniq(results.map(s => s.app)),
      distSQL: fractionMatching(results, s => s.distSQL),
      opt: fractionMatching(results, s => s.opt),
      failed: fractionMatching(results, s => s.failed),
      node_id: _.uniq(results.map(s => s.node_id)),
    };
  },
);

// tslint:disable-next-line:variable-name
const StatementDetailsConnected = connect(
  (state: AdminUIState, props: RouterState) => ({
    statement: selectStatement(state, props),
    statementsError: state.cachedData.statements.lastError,
    nodeNames: nodeDisplayNameByIDSelector(state),
  }),
  {
    refreshStatements,
  },
)(StatementDetails);

export default StatementDetailsConnected;
