  
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

/**
 * This module maintains the state of ZNBaseDB time series queries needed by
 * the web application. Cached query data is maintained separately for
 * individual components (e.g. different graphs); components are distinguished
 * in the reducer by a unique ID.
 */

import _ from "lodash";
import { Action } from "redux";
import { delay } from "redux-saga";
import { take, fork, call, all, put } from "redux-saga/effects";

import * as protos from  "src/js/protos";
import { PayloadAction } from "src/interfaces/action";
import { queryTimeSeries } from "src/util/api";

type TSRequest = protos.znbase.ts.tspb.TimeSeriesQueryRequest;
type TSResponse = protos.znbase.ts.tspb.TimeSeriesQueryResponse;

export const REQUEST = "znbaseui/metrics/REQUEST";
export const BEGIN = "znbaseui/metrics/BEGIN";
export const RECEIVE = "znbaseui/metrics/RECEIVE";
export const ERROR = "znbaseui/metrics/ERROR";
export const FETCH = "znbaseui/metrics/FETCH";
export const FETCH_COMPLETE = "znbaseui/metrics/FETCH_COMPLETE";

/**
 * WithID is a convenient interface for associating arbitrary data structures
 * with a component ID.
 */
interface WithID<T> {
  id: string;
  data: T;
}

/**
 * A request/response pair.
 */
interface RequestWithResponse {
  request: TSRequest;
  response: TSResponse;
}

/**
 * MetricsQuery maintains the cached data for a single component.
 */
export class MetricsQuery {
  // ID of the component which owns this data.
  id: string;
  // The currently cached response data for this component.
  data: TSResponse;
  // If the immediately previous request attempt returned an error, rather than
  // a response, it is maintained here. Null if the previous request was
  // successful.
  error: Error;
  // The previous request, which will have resulted in either "data" or "error"
  // being populated.
  request: TSRequest;
  // A possibly outstanding request used to retrieve data from the server for this
  // component. This may represent a currently in-flight query, and thus is not
  // necessarily the request used to retrieve the current value of "data".
  nextRequest: TSRequest;

  constructor(id: string) {
    this.id = id;
  }
}

/**
 * metricsQueryReducer is a reducer which modifies the state of a single
 * MetricsQuery object.
 */
function metricsQueryReducer(state: MetricsQuery, action: Action) {
  switch (action.type) {
    // This component has requested a new set of metrics from the server.
    case REQUEST:
      const { payload: request } = action as PayloadAction<WithID<TSRequest>>;
      state = _.clone(state);
      state.nextRequest = request.data;
      return state;

    // Results for a previous request have been received from the server.
    case RECEIVE:
      const { payload: response } = action as PayloadAction<WithID<RequestWithResponse>>;
      if (response.data.request === state.nextRequest) {
        state = _.clone(state);
        state.data = response.data.response;
        state.request = response.data.request;
        state.error = undefined;
      }
      return state;

    // The previous query for metrics for this component encountered an error.
    case ERROR:
      const { payload: error } = action as PayloadAction<WithID<Error>>;
      state = _.clone(state);
      state.error = error.data;
      return state;

    default:
      return state;
  }
}

/**
 * MetricsQueries is a collection of individual MetricsQuery objects, indexed by
 * component id.
 */
interface MetricQuerySet {
  [id: string]: MetricsQuery;
}

/**
 * metricsQueriesReducer dispatches actions to the correct MetricsQuery, based
 * on the ID of the actions.
 */
export function metricQuerySetReducer(state: MetricQuerySet = {}, action: Action) {
  switch (action.type) {
    case REQUEST:
    case RECEIVE:
    case ERROR:
      // All of these requests should be dispatched to a MetricQuery in the
      // collection. If a MetricQuery with that ID does not yet exist, create it.
      const { id } = (action as PayloadAction<WithID<any>>).payload;
      state = _.clone(state);
      state[id] = metricsQueryReducer(state[id] || new MetricsQuery(id), action);
      return state;

    default:
      return state;
  }
}

/**
 * MetricsState maintains a MetricQuerySet collection, along with some
 * metadata relevant to server queries.
 */
export class MetricsState {
  // A count of the number of in-flight fetch requests.
  inFlight = 0;
  // The collection of MetricQuery objects.
  queries: MetricQuerySet;
}

/**
 * The metrics reducer accepts events for individual MetricQuery objects,
 * dispatching them based on ID. It also accepts actions which indicate the
 * state of the connection to the server.
 */
export function metricsReducer(state: MetricsState = new MetricsState(), action: Action): MetricsState {
  switch (action.type) {
    // A new fetch request to the server is now in flight.
    case FETCH:
      state = _.clone(state);
      state.inFlight += 1;
      return state;

    // A fetch request to the server has completed.
    case FETCH_COMPLETE:
      state = _.clone(state);
      state.inFlight -= 1;
      return state;

    // Other actions may be handled by the metricsQueryReducer.
    default:
      state = _.clone(state);
      state.queries = metricQuerySetReducer(state.queries, action);
      return state;
  }
}

/**
 * requestMetrics indicates that a component is requesting new data from the
 * server.
 */
export function requestMetrics(id: string, request: TSRequest): PayloadAction<WithID<TSRequest>> {
  return {
    type: REQUEST,
    payload: {
      id: id,
      data: request,
    },
  };
}

/**
 * beginMetrics is dispatched by the processing saga to indicate that it has
 * begun the process of dispatching a request.
 */
export function beginMetrics(id: string, request: TSRequest): PayloadAction<WithID<TSRequest>> {
  return {
    type: BEGIN,
    payload: {
      id: id,
      data: request,
    },
  };
}

/**
 * receiveMetrics indicates that a previous request from this component has been
 * fulfilled by the server.
 */
export function receiveMetrics(
  id: string,
  request: TSRequest,
  response: TSResponse,
): PayloadAction<WithID<RequestWithResponse>> {
  return {
    type: RECEIVE,
    payload: {
      id: id,
      data: {
        request: request,
        response: response,
      },
    },
  };
}

/**
 * errorMetrics indicates that a previous request from this component could not
 * be fulfilled due to an error.
 */
export function errorMetrics(id: string, error: Error): PayloadAction<WithID<Error>> {
  return {
    type: ERROR,
    payload: {
      id: id,
      data: error,
    },
  };
}

/**
 * fetchMetrics indicates that a new asynchronous request to the server is in-flight.
 */
export function fetchMetrics(): Action {
  return {
    type: FETCH,
  };
}

/**
 * fetchMetricsComplete indicates that an in-flight request to the server has
 * completed.
 */
export function fetchMetricsComplete(): Action {
  return {
    type: FETCH_COMPLETE,
  };
}

/**
 * queryMetricsSaga is a redux saga which listens for REQUEST actions and sends
 * those requests to the server asynchronously.
 *
 * Metric queries can be batched when sending to the to the server -
 * specifically, queries which have the same time span can be handled by the
 * server in a single call. This saga will attempt to batch any requests which
 * are dispatched as part of the same event (e.g. if a rendering page displays
 * several graphs which need data).
 */
export function* queryMetricsSaga() {
  let requests: WithID<TSRequest>[] = [];

  while (true) {
    const requestAction: PayloadAction<WithID<TSRequest>> = yield take((REQUEST));

    // Dispatch action to underlying store.
    yield put(beginMetrics(requestAction.payload.id, requestAction.payload.data));
    requests.push(requestAction.payload);

    // If no other requests are queued, fork a process which will send the
    // request (and any other subsequent requests that are queued).
    if (requests.length === 1) {
      yield fork(sendRequestsAfterDelay);
    }
  }

  function* sendRequestsAfterDelay() {
    // Delay of zero will defer execution to the message queue, allowing the
    // currently executing event (e.g. rendering a new page or a timespan change)
    // to dispatch additional requests which can be batched.
    yield call(delay, 0);

    const requestsToSend = requests;
    requests = [];
    yield call(batchAndSendRequests, requestsToSend);
  }
}

/**
 * batchAndSendRequests attempts to send the supplied requests in the
 * smallest number of batches possible.
 */
export function* batchAndSendRequests(requests: WithID<TSRequest>[]) {
  // Construct queryable batches from the set of queued queries. Queries can
  // be dispatched in a batch if they are querying over the same timespan.
  const batches = _.groupBy(requests, (qr) => timespanKey(qr.data));
  requests = [];

  yield put(fetchMetrics());
  yield all(_.map(batches, batch => call(sendRequestBatch, batch)));
  yield put(fetchMetricsComplete());
}

/**
 * sendRequestBatch sends the supplied requests in a single batch.
 */
export function* sendRequestBatch(requests: WithID<TSRequest>[]) {
  // Flatten the queries from the batch into a single request.
  const unifiedRequest = _.clone(requests[0].data);
  unifiedRequest.queries = _.flatMap(requests, req => req.data.queries);

  let response: protos.znbase.ts.tspb.TimeSeriesQueryResponse;
  try {
    response = yield call(queryTimeSeries, unifiedRequest);
    // The number of results should match the queries exactly, and should
    // be in the exact order passed.
    if (response.results.length !== unifiedRequest.queries.length) {
      throw `mismatched count of results (${response.results.length}) and queries (${unifiedRequest.queries.length})`;
    }
  } catch (e) {
    // Dispatch the error to each individual MetricsQuery which was
    // requesting data.
    for (const request of requests) {
      yield put(errorMetrics(request.id, e));
    }
    return;
  }

  // Match each result in the unified response to its corresponding original
  // query. Each request may have sent multiple queries in the batch.
  const results = response.results;
  for (const request of requests) {
    yield put(receiveMetrics(
      request.id,
      request.data,
      new protos.znbase.ts.tspb.TimeSeriesQueryResponse({
        results: results.splice(0, request.data.queries.length),
      }),
    ));
  }
}

interface SimpleTimespan {
  start_nanos?: Long;
  end_nanos?: Long;
}

function timespanKey(timewindow: SimpleTimespan): string {
  return (timewindow.start_nanos && timewindow.start_nanos.toString()) + ":" + (timewindow.end_nanos && timewindow.end_nanos.toString());
}
