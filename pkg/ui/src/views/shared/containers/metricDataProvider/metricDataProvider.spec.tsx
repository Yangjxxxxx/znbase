  
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
import { assert } from "chai";
import { shallow } from "enzyme";
import _ from "lodash";
import Long from "long";
import * as sinon from "sinon";

import "src/enzymeInit";
import * as protos from  "src/js/protos";
import {
  Metric, Axis, MetricsDataComponentProps, QueryTimeInfo,
} from "src/views/shared/components/metricQuery";
import {
  MetricsDataProviderUnconnected as MetricsDataProvider,
} from "src/views/shared/containers/metricDataProvider";
import { requestMetrics, MetricsQuery } from "src/redux/metrics";

// TextGraph is a proof-of-concept component used to demonstrate that
// MetricsDataProvider is working correctly. Used in tests.
class TextGraph extends React.Component<MetricsDataComponentProps, {}> {
  render() {
    return (
      <div>
        {
          (this.props.data && this.props.data.results)
            ? this.props.data.results.join(":")
            : ""
        }
      </div>
    );
  }
}

function makeDataProvider(
  id: string,
  metrics: MetricsQuery,
  timeInfo: QueryTimeInfo,
  rm: typeof requestMetrics,
) {
  return shallow(<MetricsDataProvider id={id} metrics={metrics} timeInfo={timeInfo} requestMetrics={rm}>
    <TextGraph>
      <Axis>
        <Metric name="test.metric.1" />
        <Metric name="test.metric.2" />
      </Axis>
      <Axis>
        <Metric name="test.metric.3" />
      </Axis>
    </TextGraph>
  </MetricsDataProvider>);
}

function makeMetricsRequest(timeInfo: QueryTimeInfo, sources?: string[]) {
  return new protos.znbase.ts.tspb.TimeSeriesQueryRequest({
    start_nanos: timeInfo.start,
    end_nanos: timeInfo.end,
    sample_nanos: timeInfo.sampleDuration,
    queries: [
      {
        name: "test.metric.1",
        sources: sources,
        downsampler: protos.znbase.ts.tspb.TimeSeriesQueryAggregator.AVG,
        source_aggregator: protos.znbase.ts.tspb.TimeSeriesQueryAggregator.SUM,
        derivative: protos.znbase.ts.tspb.TimeSeriesQueryDerivative.NONE,
      },
      {
        name: "test.metric.2",
        sources: sources,
        downsampler: protos.znbase.ts.tspb.TimeSeriesQueryAggregator.AVG,
        source_aggregator: protos.znbase.ts.tspb.TimeSeriesQueryAggregator.SUM,
        derivative: protos.znbase.ts.tspb.TimeSeriesQueryDerivative.NONE,
      },
      {
        name: "test.metric.3",
        sources: sources,
        downsampler: protos.znbase.ts.tspb.TimeSeriesQueryAggregator.AVG,
        source_aggregator: protos.znbase.ts.tspb.TimeSeriesQueryAggregator.SUM,
        derivative: protos.znbase.ts.tspb.TimeSeriesQueryDerivative.NONE,
      },
    ],
  });
}

function makeMetricsQuery(id: string, timeSpan: QueryTimeInfo, sources?: string[]): MetricsQuery {
  const request = makeMetricsRequest(timeSpan, sources);
  const data = new protos.znbase.ts.tspb.TimeSeriesQueryResponse({
    results: _.map(request.queries, (q) => {
      return {
        query: q,
        datapoints: [],
      };
    }),
  });
  return {
    id,
    request,
    data,
    nextRequest: request,
    error: undefined,
  };
}

describe("<MetricsDataProvider>", function() {
  let spy: sinon.SinonSpy;
  const timespan1: QueryTimeInfo = {
    start: Long.fromNumber(0),
    end: Long.fromNumber(100),
    sampleDuration: Long.fromNumber(300),
  };
  const timespan2: QueryTimeInfo = {
    start: Long.fromNumber(100),
    end: Long.fromNumber(200),
    sampleDuration: Long.fromNumber(300),
  };
  const graphid = "testgraph";

  beforeEach(function() {
    spy = sinon.spy();
  });

  describe("refresh", function() {
    it("refreshes query data when mounted", function () {
      makeDataProvider(graphid, null, timespan1, spy);
      assert.isTrue(spy.called);
      assert.isTrue(spy.calledWith(graphid, makeMetricsRequest(timespan1)));
    });

    it("does nothing when mounted if current request fulfilled", function () {
      makeDataProvider(graphid, makeMetricsQuery(graphid, timespan1), timespan1, spy);
      assert.isTrue(spy.notCalled);
    });

    it("does nothing when mounted if current request is in flight", function () {
      const query = makeMetricsQuery(graphid, timespan1);
      query.request = null;
      query.data = null;
      makeDataProvider(graphid, query, timespan1, spy);
      assert.isTrue(spy.notCalled);
    });

    it("refreshes query data when receiving props", function () {
      const provider = makeDataProvider(graphid, makeMetricsQuery(graphid, timespan1), timespan1, spy);
      assert.isTrue(spy.notCalled);
      provider.setProps({
        metrics: undefined,
      });
      assert.isTrue(spy.called);
      assert.isTrue(spy.calledWith(graphid, makeMetricsRequest(timespan1)));
    });

    it("refreshes if timespan changes", function () {
      const provider = makeDataProvider(graphid, makeMetricsQuery(graphid, timespan1), timespan1, spy);
      assert.isTrue(spy.notCalled);
      provider.setProps({
        timeInfo: timespan2,
      });
      assert.isTrue(spy.called);
      assert.isTrue(spy.calledWith(graphid, makeMetricsRequest(timespan2)));
    });

    it("refreshes if query changes", function () {
      const provider = makeDataProvider(graphid, makeMetricsQuery(graphid, timespan1), timespan1, spy);
      assert.isTrue(spy.notCalled);
      // Modify "sources" parameter.
      provider.setProps({
        metrics: makeMetricsQuery(graphid, timespan1, ["1"]),
      });
      assert.isTrue(spy.called);
      assert.isTrue(spy.calledWith(graphid, makeMetricsRequest(timespan1)));
    });

  });

  describe("attach", function() {
    it("attaches metrics data to contained component", function() {
      const provider = makeDataProvider(graphid, makeMetricsQuery(graphid, timespan1), timespan1, spy);
      const props: any = provider.first().props();
      assert.isDefined(props.data);
      assert.deepEqual(props.data, makeMetricsQuery(graphid, timespan1).data);
    });

    it("attaches metrics data if timespan doesn't match", function() {
      const provider = makeDataProvider(graphid, makeMetricsQuery(graphid, timespan1), timespan2, spy);
      const props: any = provider.first().props();
      assert.isDefined(props.data);
      assert.deepEqual(props.data, makeMetricsQuery(graphid, timespan1).data);
    });

    it("does not attach metrics data if query doesn't match", function() {
      const provider = makeDataProvider(graphid, makeMetricsQuery(graphid, timespan1, ["1"]), timespan1, spy);
      const props: any = provider.first().props();
      assert.isUndefined(props.data);
    });

    it("throws error if it contains multiple graph components", function() {
      try {
        shallow(
          <MetricsDataProvider id="id"
                               metrics={null}
                               timeInfo={timespan1}
                               requestMetrics={spy}>
            <TextGraph>
              <Axis>
                <Metric name="test.metrics.1" />
              </Axis>
            </TextGraph>
            <TextGraph>
              <Axis>
                <Metric name="test.metrics.2" />
              </Axis>
            </TextGraph>
          </MetricsDataProvider>,
        );
        assert.fail("expected error from MetricsDataProvider");
      } catch (e) {
        assert.match(e, /Invariant Violation/);
      }
    });
  });
});
