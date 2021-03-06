// Copyright 2019  The Cockroach Authors.
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

import React from "react";

import { LineGraph } from "src/views/cluster/components/linegraph";
import { Metric, Axis, AxisUnits } from "src/views/shared/components/metricQuery";

import { GraphDashboardProps } from "./dashboardUtils";

export default function (props: GraphDashboardProps) {
  const { storeSources } = props;

  return [
    <LineGraph title="CDC最大延迟" sources={storeSources}>
      <Axis units={AxisUnits.Duration} label="时间">
        <Metric name="cr.node.changefeed.max_behind_nanos" title="CDC最大延迟" downsampleMax aggregateMax />
      </Axis>
    </LineGraph>,

    <LineGraph title="Sink字节流量" sources={storeSources}>
      <Axis units={AxisUnits.Bytes} label="字节">
        <Metric name="cr.node.changefeed.emitted_bytes" title="Emitted Bytes" nonNegativeRate />
      </Axis>
    </LineGraph>,

    <LineGraph title="Sink 数目" sources={storeSources}>
      <Axis units={AxisUnits.Count} label="数目">
        <Metric name="cr.node.changefeed.emitted_messages" title="Messages" nonNegativeRate />
        <Metric name="cr.node.changefeed.flushes" title="Flushes" nonNegativeRate />
      </Axis>
    </LineGraph>,

    <LineGraph title="Sink Timings" sources={storeSources}>
      <Axis units={AxisUnits.Duration} label="time">
        <Metric name="cr.node.changefeed.emit_nanos" title="Message Emit Time" nonNegativeRate />
        <Metric name="cr.node.changefeed.flush_nanos" title="Flush Time" nonNegativeRate />
      </Axis>
    </LineGraph>,
  ];
}
