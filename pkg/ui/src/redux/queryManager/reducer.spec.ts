  
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
import moment from "moment";

import {
  managedQueryReducer,
  ManagedQueryState,
  queryBegin,
  queryComplete,
  queryError,
  queryManagerReducer,
  QueryManagerState,
} from "./reducer";

describe("Query Manager State", function () {
  describe("managed query reducer", function () {
    const testMoment = moment();
    const testError = new Error("err");
    let state: ManagedQueryState;

    beforeEach(function() {
      state = managedQueryReducer(undefined, {} as any);
    });

    it("has the correct initial state", function () {
      assert.deepEqual(state, new ManagedQueryState());
    });

    it("dispatches queryBegin correctly", function () {
      // We expect "isRunning" to be true and all other fields to be null.
      const expected = new ManagedQueryState();
      expected.isRunning = true;
      expected.lastError = null;
      expected.completedAt = null;

      state = managedQueryReducer(state, queryBegin("ID"));
      assert.deepEqual(state, expected);
    });

    it("dispatches queryError correctly", function () {
      // We expect "isRunning" to be false; both the error field and completedAt
      // should be populated with the supplied information from the action.
      const expected = new ManagedQueryState();
      expected.isRunning = false;
      expected.lastError = testError;
      expected.completedAt = testMoment;

      state = managedQueryReducer(state, queryBegin("ID"));
      state = managedQueryReducer(state, queryError("ID", testError, testMoment));
      assert.deepEqual(state, expected);
    });

    it("dispatches queryComplete correctly", function () {
      // We expect "isRunning" to be false, completedAt to be populated, and
      // the error field to be null.
      const expected = new ManagedQueryState();
      expected.isRunning = false;
      expected.lastError = null;
      expected.completedAt = testMoment;

      state = managedQueryReducer(state, queryBegin("ID"));
      state = managedQueryReducer(state, queryComplete("ID", testMoment));
      assert.deepEqual(state, expected);
    });

    it("clears error on queryBegin", function () {
      const expected = new ManagedQueryState();
      expected.isRunning = true;
      expected.lastError = null;
      expected.completedAt = null;

      state = managedQueryReducer(state, queryError("ID", testError, testMoment));
      state = managedQueryReducer(state, queryBegin("ID"));
      assert.deepEqual(state, expected);
    });

    it("ignores unrecognized actions", function () {
      const origState = state;
      state = managedQueryReducer(state, { type: "unsupported" } as any);
      assert.equal(state, origState);
    });
  });

  describe("query manager reducer", function () {
    const testMoment = moment();
    const testError = new Error("err");
    let state: QueryManagerState;

    beforeEach(function() {
      state = queryManagerReducer(undefined, {} as any);
    });

    it("has the correct initial value", function () {
      assert.deepEqual(state, {});
    });

    it("correctly dispatches based on ID", function () {
      const expected = {
        "1": managedQueryReducer(undefined, queryBegin("1")),
        "2": managedQueryReducer(undefined, queryError("2", testError, testMoment)),
        "3": managedQueryReducer(undefined, queryComplete("3", testMoment)),
      };

      state = queryManagerReducer(state, queryBegin("1"));
      state = queryManagerReducer(state, queryBegin("2"));
      state = queryManagerReducer(state, queryBegin("3"));
      state = queryManagerReducer(state, queryError("2", testError, testMoment));
      state = queryManagerReducer(state, queryError("3", testError, testMoment));
      state = queryManagerReducer(state, queryBegin("3"));
      state = queryManagerReducer(state, queryComplete("3", testMoment));

      assert.deepEqual(state, expected);
    });
  });
});
