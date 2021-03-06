  
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
import { connect } from "react-redux";
import moment from "moment";

import { AdminUIState } from "src/redux/state";
import * as timewindow from "src/redux/timewindow";

interface TimeWindowManagerProps {
  // The current timewindow redux state.
  timeWindow: timewindow.TimeWindowState;
  // Callback function used to set a new time window.
  setTimeWindow: typeof timewindow.setTimeWindow;
  // Optional override method to obtain the current time. Used for tests.
  now?: () => moment.Moment;
}

interface TimeWindowManagerState {
  // Identifier from an outstanding call to setTimeout.
  timeout: number;
}

/**
 * TimeWindowManager takes responsibility for advancing the current global
 * time window used by metric graphs. It renders nothing, but will dispatch an
 * updated time window into the redux store whenever the previous time window is
 * expired.
 */
class TimeWindowManager extends React.Component<TimeWindowManagerProps, TimeWindowManagerState> {
  constructor(props?: TimeWindowManagerProps, context?: any) {
    super(props, context);
    this.state = { timeout: null };
  }

  /**
   * checkWindow determines when the current time window will expire. If it is
   * already expired, a new time window is dispatched immediately. Otherwise,
   * setTimeout is used to asynchronously set a new time window when the current
   * one expires.
   */
  checkWindow(props: TimeWindowManagerProps) {
    // Clear any existing timeout.
    if (this.state.timeout) {
      clearTimeout(this.state.timeout);
      this.setState({ timeout: null });
    }

    // If there is no current window, or if scale have changed since this
    // window was generated, set one immediately.
    if (!props.timeWindow.currentWindow || props.timeWindow.scaleChanged) {
      this.setWindow(props);
      return;
    }

    // Exact time ranges can't expire.
    if (props.timeWindow.scale.windowEnd) {
      // this.setWindow(props);
      return;
    }

    const now = props.now ? props.now() : moment();
    const currentEnd = props.timeWindow.currentWindow.end;
    const expires = currentEnd.clone().add(props.timeWindow.scale.windowValid);
    if (now.isAfter(expires))  {
      // Current time window is expired, reset it.
      this.setWindow(props);
    } else {
      // Set a timeout to reset the window when the current window expires.
      const newTimeout = setTimeout(() => this.setWindow(props), expires.diff(now).valueOf());
      this.setState({
        timeout: newTimeout,
      });
    }
  }

  /**
   * setWindow dispatches a new time window, extending backwards from the
   * current time.
   */
  setWindow(props: TimeWindowManagerProps) {
    if (!props.timeWindow.scale.windowEnd) {
      const now = props.now ? props.now() : moment();
      props.setTimeWindow({
        start: now.clone().subtract(props.timeWindow.scale.windowSize),
        end: now,
      });
    } else {
      const windowEnd = props.timeWindow.scale.windowEnd;
      props.setTimeWindow({
        start: windowEnd.clone().subtract(props.timeWindow.scale.windowSize),
        end: windowEnd,
      });
    }
  }

  componentWillMount() {
    this.checkWindow(this.props);
  }

  componentWillReceiveProps(props: TimeWindowManagerProps) {
    this.checkWindow(props);
  }

  render(): any {
    // Render nothing.
    return null;
  }
}

const timeWindowManagerConnected = connect(
  (state: AdminUIState) => {
    return {
      timeWindow: state.timewindow,
    };
  },
  {
    setTimeWindow: timewindow.setTimeWindow,
  },
)(TimeWindowManager);

export default timeWindowManagerConnected;
export { TimeWindowManager as TimeWindowManagerUnconnected };
