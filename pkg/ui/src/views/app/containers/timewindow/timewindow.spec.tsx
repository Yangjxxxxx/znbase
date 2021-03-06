  
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
import * as sinon from "sinon";
import moment from "moment";
import _ from "lodash";

import "src/enzymeInit";
import { TimeWindowManagerUnconnected as TimeWindowManager } from "./";
import * as timewindow from "src/redux/timewindow";

describe("<TimeWindowManager>", function() {
  let spy: sinon.SinonSpy;
  let state: timewindow.TimeWindowState;
  const now = () => moment("11-12-1955 10:04PM -0800", "MM-DD-YYYY hh:mma Z");

  beforeEach(function() {
    spy = sinon.spy();
    state = new timewindow.TimeWindowState();
  });

  const getManager = () => shallow(<TimeWindowManager timeWindow={_.clone(state)}
                                                    setTimeWindow={spy}
                                                    now={now} />);

  it("resets time window immediately it is empty", function() {
    getManager();
    assert.isTrue(spy.calledOnce);
    assert.deepEqual(spy.firstCall.args[0], {
      start: now().subtract(state.scale.windowSize),
      end: now(),
    });
  });

  it("resets time window immediately if expired", function() {
    state.currentWindow = {
      start: now().subtract(state.scale.windowSize),
      end: now().subtract(state.scale.windowValid).subtract(1),
    };

    getManager();
    assert.isTrue(spy.calledOnce);
    assert.deepEqual(spy.firstCall.args[0], {
      start: now().subtract(state.scale.windowSize),
      end: now(),
    });
  });

  it("resets time window immediately if scale has changed", function() {
    // valid window.
    state.currentWindow = {
      start: now().subtract(state.scale.windowSize),
      end: now(),
    };
    state.scaleChanged = true;

    getManager();
    assert.isTrue(spy.calledOnce);
    assert.deepEqual(spy.firstCall.args[0], {
      start: now().subtract(state.scale.windowSize),
      end: now(),
    });
  });

  it("resets time window later if current window is valid", function() {
    state.currentWindow = {
      start: now().subtract(state.scale.windowSize),
      // 5 milliseconds until expiration.
      end: now().subtract(state.scale.windowValid.asMilliseconds() - 5),
    };

    getManager();
    assert.isTrue(spy.notCalled);

    // Wait 11 milliseconds, then verify that window was updated.
    return new Promise<void>((resolve, _reject) => {
      setTimeout(
        () => {
          assert.isTrue(spy.calledOnce);
          assert.deepEqual(spy.firstCall.args[0], {
            start: now().subtract(state.scale.windowSize),
            end: now(),
          });
          resolve();
        },
        6);
    });
  });

  // TODO (maxlang): Fix this test to actually change the state to catch the
  // issue that caused #7590. Tracked in #8595.
  it("has only a single timeout at a time.", function() {
    state.currentWindow = {
      start: now().subtract(state.scale.windowSize),
      // 5 milliseconds until expiration.
      end: now().subtract(state.scale.windowValid.asMilliseconds() - 5),
    };

    const manager = getManager();
    assert.isTrue(spy.notCalled);

    // Set new props on currentWindow. The previous timeout should be abandoned.
    state.currentWindow = {
      start: now().subtract(state.scale.windowSize),
      // 10 milliseconds until expiration.
      end: now().subtract(state.scale.windowValid.asMilliseconds() - 10),
    };
    manager.setProps({
      timeWindow: state,
    });
    assert.isTrue(spy.notCalled);

    // Wait 11 milliseconds, then verify that window was updated a single time.
    return new Promise<void>((resolve, _reject) => {
      setTimeout(
        () => {
          assert.isTrue(spy.calledOnce);
          assert.deepEqual(spy.firstCall.args[0], {
            start: now().subtract(state.scale.windowSize),
            end: now(),
          });
          resolve();
        },
        11);
    });

  });
});
