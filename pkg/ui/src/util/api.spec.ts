  
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

import { assert } from "chai";
import _ from "lodash";
import moment from "moment";
import Long from "long";

import fetchMock from "./fetch-mock";

import * as protos from "src/js/protos";
import * as api from "./api";

describe("rest api", function() {
  describe("propsToQueryString", function () {
    interface PropBag {
      [k: string]: string;
    }

    // helper decoding function used to doublecheck querystring generation
    function decodeQueryString(qs: string): PropBag {
      return _.reduce<string, PropBag>(
        qs.split("&"),
        (memo: PropBag, v: string) => {
          const [key, value] = v.split("=");
          memo[decodeURIComponent(key)] = decodeURIComponent(value);
          return memo;
        },
        {},
      );
    }

    it("creates an appropriate querystring", function () {
      const testValues: { [k: string]: any } = {
        a: "testa",
        b: "testb",
      };

      const querystring = api.propsToQueryString(testValues);

      assert((/a=testa/).test(querystring));
      assert((/b=testb/).test(querystring));
      assert.lengthOf(querystring.match(/=/g), 2);
      assert.lengthOf(querystring.match(/&/g), 1);
      assert.deepEqual(testValues, decodeQueryString(querystring));
    });

    it("handles falsy values correctly", function () {
      const testValues: { [k: string]: any } = {
        // null and undefined should be ignored
        undefined: undefined,
        null: null,
        // other values should be added
        false: false,
        "": "",
        0: 0,
      };

      const querystring = api.propsToQueryString(testValues);

      assert((/false=false/).test(querystring));
      assert((/0=0/).test(querystring));
      assert((/([^A-Za-z]|^)=([^A-Za-z]|$)/).test(querystring));
      assert.lengthOf(querystring.match(/=/g), 3);
      assert.lengthOf(querystring.match(/&/g), 2);
      assert.notOk((/undefined/).test(querystring));
      assert.notOk((/null/).test(querystring));
      assert.deepEqual({ false: "false", "": "", 0: "0" }, decodeQueryString(querystring));
    });

    it("handles special characters", function () {
      const key = "!@#$%^&*()=+-_\\|\"`'?/<>";
      const value = key.split("").reverse().join(""); // key reversed
      const testValues: { [k: string]: any } = {
        [key] : value,
      };

      const querystring = api.propsToQueryString(testValues);

      assert(querystring.match(/%/g).length > (key + value).match(/%/g).length);
      assert.deepEqual(testValues, decodeQueryString(querystring));
    });

    it("handles non-string values", function () {
      const testValues: { [k: string]: any } = {
        boolean: true,
        number: 1,
        emptyObject: {},
        emptyArray: [],
        objectWithProps: { a: 1, b: 2 },
        arrayWithElts: [1, 2, 3],
        long: Long.fromNumber(1),
      };

      const querystring = api.propsToQueryString(testValues);
      assert.deepEqual(_.mapValues(testValues, _.toString), decodeQueryString(querystring));
    });
  });

  describe("databases request", function () {
    afterEach(fetchMock.restore);

    it("correctly requests info about all databases", function () {
      this.timeout(1000);
      // Mock out the fetch query to /databases
      fetchMock.mock({
        matcher: api.API_PREFIX + "/databases",
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          const encodedResponse = protos.znbase.server.serverpb.DatabasesResponse.encode({
            databases: ["system", "test"],
          }).finish();
          return {
            body: api.toArrayBuffer(encodedResponse),
          };
        },
      });

      return api.getDatabaseList(new protos.znbase.server.serverpb.DatabasesRequest()).then((result) => {
        assert.lengthOf(fetchMock.calls(api.API_PREFIX + "/databases"), 1);
        assert.lengthOf(result.databases, 2);
      });
    });

    it("correctly handles an error", function (done) {
      this.timeout(1000);
      // Mock out the fetch query to /databases, but return a promise that's never resolved to test the timeout
      fetchMock.mock({
        matcher: api.API_PREFIX + "/databases",
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return { throws: new Error() };
        },
      });

      api.getDatabaseList(new protos.znbase.server.serverpb.DatabasesRequest()).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.isError(e));
        done();
      });
    });

    it("correctly times out", function (done) {
      this.timeout(1000);
      // Mock out the fetch query to /databases, but return a promise that's never resolved to test the timeout
      fetchMock.mock({
        matcher: api.API_PREFIX + "/databases",
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return new Promise<any>(() => { });
        },
      });

      api.getDatabaseList(new protos.znbase.server.serverpb.DatabasesRequest(), moment.duration(0)).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.startsWith(e.message, "Promise timed out"), "Error is a timeout error.");
        done();
      });
    });
  });

  describe("database details request", function () {
    const dbName = "test";

    afterEach(fetchMock.restore);

    it("correctly requests info about a specific database", function () {
      this.timeout(1000);
      // Mock out the fetch query
      fetchMock.mock({
        matcher: `${api.API_PREFIX}/databases/${dbName}`,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          const encodedResponse = protos.znbase.server.serverpb.DatabaseDetailsResponse.encode({
            table_names: ["table1", "table2"],
            grants: [
              { user: "root", privileges: ["ALL"] },
              { user: "other", privileges: [] },
            ],
          }).finish();
          return {
            body: api.toArrayBuffer(encodedResponse),
          };
        },
      });

      return api.getDatabaseDetails(new protos.znbase.server.serverpb.DatabaseDetailsRequest({ database: dbName })).then((result) => {
        assert.lengthOf(fetchMock.calls(`${api.API_PREFIX}/databases/${dbName}`), 1);
        assert.lengthOf(result.table_names, 2);
        assert.lengthOf(result.grants, 2);
      });
    });

    it("correctly handles an error", function (done) {
      this.timeout(1000);
      // Mock out the fetch query, but return a 500 status code
      fetchMock.mock({
        matcher: `${api.API_PREFIX}/databases/${dbName}`,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return { throws: new Error() };
        },
      });

      api.getDatabaseDetails(new protos.znbase.server.serverpb.DatabaseDetailsRequest({ database: dbName })).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.isError(e));
        done();
      });
    });

    it("correctly times out", function (done) {
      this.timeout(1000);
      // Mock out the fetch query, but return a promise that's never resolved to test the timeout
      fetchMock.mock({
        matcher: `${api.API_PREFIX}/databases/${dbName}`,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return new Promise<any>(() => { });
        },
      });

      api.getDatabaseDetails(new protos.znbase.server.serverpb.DatabaseDetailsRequest({ database: dbName }), moment.duration(0)).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.startsWith(e.message, "Promise timed out"), "Error is a timeout error.");
        done();
      });
    });
  });

  describe("table details request", function () {
    const dbName = "testDB";
    const tableName = "testTable";

    afterEach(fetchMock.restore);

    it("correctly requests info about a specific table", function () {
      this.timeout(1000);
      // Mock out the fetch query
      fetchMock.mock({
        matcher: `${api.API_PREFIX}/databases/${dbName}/tables/${tableName}`,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          const encodedResponse = protos.znbase.server.serverpb.TableDetailsResponse.encode({}).finish();
          return {
            body: api.toArrayBuffer(encodedResponse),
          };
        },
      });

      return api.getTableDetails(new protos.znbase.server.serverpb.TableDetailsRequest({ database: dbName, table: tableName })).then((result) => {
        assert.lengthOf(fetchMock.calls(`${api.API_PREFIX}/databases/${dbName}/tables/${tableName}`), 1);
        assert.lengthOf(result.columns, 0);
        assert.lengthOf(result.indexes, 0);
        assert.lengthOf(result.grants, 0);
      });
    });

    it("correctly handles an error", function (done) {
      this.timeout(1000);
      // Mock out the fetch query, but return a 500 status code
      fetchMock.mock({
        matcher: `${api.API_PREFIX}/databases/${dbName}/tables/${tableName}`,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return { throws: new Error() };
        },
      });

      api.getTableDetails(new protos.znbase.server.serverpb.TableDetailsRequest({ database: dbName, table: tableName })).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.isError(e));
        done();
      });
    });

    it("correctly times out", function (done) {
      this.timeout(1000);
      // Mock out the fetch query, but return a promise that's never resolved to test the timeout
      fetchMock.mock({
        matcher: `${api.API_PREFIX}/databases/${dbName}/tables/${tableName}`,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return new Promise<any>(() => { });
        },
      });

      api.getTableDetails(new protos.znbase.server.serverpb.TableDetailsRequest({ database: dbName, table: tableName }), moment.duration(0)).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.startsWith(e.message, "Promise timed out"), "Error is a timeout error.");
        done();
      });
    });
  });

  describe("events request", function() {
    const eventsPrefixMatcher = `begin:${api.API_PREFIX}/events?`;

    afterEach(fetchMock.restore);

    it("correctly requests events", function () {
      this.timeout(1000);
      // Mock out the fetch query
      fetchMock.mock({
        matcher: eventsPrefixMatcher,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          const encodedResponse = protos.znbase.server.serverpb.EventsResponse.encode({
            events: [
              { event_type: "test" },
            ],
          }).finish();
          return {
            body: api.toArrayBuffer(encodedResponse),
          };
        },
      });

      return api.getEvents(new protos.znbase.server.serverpb.EventsRequest()).then((result) => {
        assert.lengthOf(fetchMock.calls(eventsPrefixMatcher), 1);
        assert.lengthOf(result.events, 1);
      });
    });

    it("correctly requests filtered events", function () {
      this.timeout(1000);

      const req = new protos.znbase.server.serverpb.EventsRequest({
        target_id: Long.fromNumber(1),
        type: "test type",
      });

      // Mock out the fetch query
      fetchMock.mock({
        matcher: eventsPrefixMatcher,
        method: "GET",
        response: (url: string, requestObj: RequestInit) => {
          const params = url.split("?")[1].split("&");
          assert.lengthOf(params, 2);
          _.each(params, (param) => {
            let [k, v] = param.split("=");
            k = decodeURIComponent(k);
            v = decodeURIComponent(v);
            switch (k) {
              case "target_id":
                assert.equal(req.target_id.toString(), v);
                break;

              case "type":
                assert.equal(req.type, v);
                break;

              default:
                 throw new Error(`Unknown property ${k}`);
            }
          });
          assert.isUndefined(requestObj.body);
          const encodedResponse = protos.znbase.server.serverpb.EventsResponse.encode({
            events: [
              { event_type: "test" },
            ],
          }).finish();
          return {
            body: api.toArrayBuffer(encodedResponse),
          };
        },
      });

      return api.getEvents(new protos.znbase.server.serverpb.EventsRequest(req)).then((result) => {
        assert.lengthOf(fetchMock.calls(eventsPrefixMatcher), 1);
        assert.lengthOf(result.events, 1);
      });
    });

    it("correctly handles an error", function (done) {
      this.timeout(1000);

      // Mock out the fetch query, but return a 500 status code
      fetchMock.mock({
        matcher: eventsPrefixMatcher,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return { throws: new Error() };
        },
      });

      api.getEvents(new protos.znbase.server.serverpb.EventsRequest()).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.isError(e));
        done();
      });
    });

    it("correctly times out", function (done) {
      this.timeout(1000);
      // Mock out the fetch query, but return a promise that's never resolved to test the timeout
      fetchMock.mock({
        matcher: eventsPrefixMatcher,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return new Promise<any>(() => { });
        },
      });

      api.getEvents(new protos.znbase.server.serverpb.EventsRequest(), moment.duration(0)).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.startsWith(e.message, "Promise timed out"), "Error is a timeout error.");
        done();
      });
    });
  });

  describe("health request", function() {
    const healthUrl = `${api.API_PREFIX}/health`;

    afterEach(fetchMock.restore);

    it("correctly requests health", function () {
      this.timeout(1000);
      // Mock out the fetch query
      fetchMock.mock({
        matcher: healthUrl,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          const encodedResponse = protos.znbase.server.serverpb.HealthResponse.encode({}).finish();
          return {
            body: api.toArrayBuffer(encodedResponse),
          };
        },
      });

      return api.getHealth(new protos.znbase.server.serverpb.HealthRequest()).then((result) => {
        assert.lengthOf(fetchMock.calls(healthUrl), 1);
        assert.deepEqual(result, new protos.znbase.server.serverpb.HealthResponse());
      });
    });

    it("correctly handles an error", function (done) {
      this.timeout(1000);

      // Mock out the fetch query, but return a 500 status code
      fetchMock.mock({
        matcher: healthUrl,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return { throws: new Error() };
        },
      });

      api.getHealth(new protos.znbase.server.serverpb.HealthRequest()).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.isError(e));
        done();
      });
    });

    it("correctly times out", function (done) {
      this.timeout(1000);
      // Mock out the fetch query, but return a promise that's never resolved to test the timeout
      fetchMock.mock({
        matcher: healthUrl,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return new Promise<any>(() => { });
        },
      });

      api.getHealth(new protos.znbase.server.serverpb.HealthRequest(), moment.duration(0)).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.startsWith(e.message, "Promise timed out"), "Error is a timeout error.");
        done();
      });
    });
  });

  describe("cluster request", function() {
    const clusterUrl = `${api.API_PREFIX}/cluster`;
    const clusterID = "12345abcde";

    afterEach(fetchMock.restore);

    it("correctly requests cluster info", function () {
      this.timeout(1000);
      fetchMock.mock({
        matcher: clusterUrl,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          const encodedResponse = protos.znbase.server.serverpb.ClusterResponse.encode({ cluster_id: clusterID }).finish();
          return {
            body: api.toArrayBuffer(encodedResponse),
          };
        },
      });

      return api.getCluster(new protos.znbase.server.serverpb.ClusterRequest()).then((result) => {
        assert.lengthOf(fetchMock.calls(clusterUrl), 1);
        assert.deepEqual(result.cluster_id, clusterID);
      });
    });

    it("correctly handles an error", function (done) {
      this.timeout(1000);

      // Mock out the fetch query, but return an error
      fetchMock.mock({
        matcher: clusterUrl,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return { throws: new Error() };
        },
      });

      api.getCluster(new protos.znbase.server.serverpb.ClusterRequest()).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.isError(e));
        done();
      });
    });

    it("correctly times out", function (done) {
      this.timeout(1000);
      // Mock out the fetch query, but return a promise that's never resolved to test the timeout
      fetchMock.mock({
        matcher: clusterUrl,
        method: "GET",
        response: (_url: string, requestObj: RequestInit) => {
          assert.isUndefined(requestObj.body);
          return new Promise<any>(() => { });
        },
      });

      api.getCluster(new protos.znbase.server.serverpb.ClusterRequest(), moment.duration(0)).then((_result) => {
        done(new Error("Request unexpectedly succeeded."));
      }).catch(function (e) {
        assert(_.startsWith(e.message, "Promise timed out"), "Error is a timeout error.");
        done();
      });
    });
  });
});
