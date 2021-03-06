  
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

import { Action } from "redux";
import {
  LocalSettingData, LocalSetting, setLocalSetting,
  LocalSettingsState, localSettingsReducer,
} from "./localsettings";
import { assert } from "chai";

describe("Local Settings", function() {
  describe("actions", function() {
    it("should create the correct action to set a ui setting", function() {
      const settingName = "test-setting";
      const settingValue = { val: "arbitrary-value" };
      const expectedSetting: LocalSettingData = {
        key: settingName,
        value: settingValue,
      };
      assert.deepEqual(
        setLocalSetting(settingName, settingValue).payload,
        expectedSetting);
    });
  });

  describe("reducer", function() {
    it("should have the correct default value.", function() {
      assert.deepEqual(
        localSettingsReducer(undefined, { type: "unknown" }),
        {},
      );
    });

    describe("SET_UI_VALUE", function() {
      it("should correctly set UI values by key.", function() {
        const key = "test-setting";
        const value = "test-value";
        const expected: LocalSettingsState = {
          [key]: value,
        };
        let actual = localSettingsReducer(undefined, setLocalSetting(key, value));
        assert.deepEqual(actual, expected);

        const key2 = "another-setting";
        expected[key2] = value;
        actual = localSettingsReducer(actual, setLocalSetting(key2, value));
        assert.deepEqual(actual, expected);
      });

      it("should correctly overwrite previous values.", function() {
        const key = "test-setting";
        const value = "test-value";
        const expected: LocalSettingsState = {
          [key]: value,
        };
        const initial: LocalSettingsState = {
          [key]: "oldvalue",
        };
        assert.deepEqual(
          localSettingsReducer(initial, setLocalSetting(key, value)),
          expected,
        );
      });
    });
  });

  describe("LocalSetting helper class", function() {
    let topLevelState: { localSettings: LocalSettingsState};
    const dispatch = function(action: Action) {
      topLevelState = {
        localSettings: localSettingsReducer(topLevelState.localSettings, action),
      };
    };

    beforeEach(function() {
      topLevelState = {
        localSettings: {},
      };
    });

    const settingName = "test-setting";
    const settingName2 = "test-setting-2";

    it("returns default values correctly.", function() {
      const numberSetting = new LocalSetting(
        settingName, (s: typeof topLevelState) => s.localSettings, 99,
      );
      assert.equal(numberSetting.selector(topLevelState), 99);
    });

    it("sets values correctly.", function() {
      const numberSetting = new LocalSetting(
        settingName, (s: typeof topLevelState) => s.localSettings, 99,
      );
      dispatch(numberSetting.set(20));
      assert.deepEqual(topLevelState, {
        localSettings: {
          [settingName]: 20,
        },
      });
    });

    it("works with multiple values correctly.", function() {
      const numberSetting = new LocalSetting(
        settingName, (s: typeof topLevelState) => s.localSettings, 99,
      );
      const stringSetting = new LocalSetting<typeof topLevelState, string>(
        settingName2, (s: typeof topLevelState) => s.localSettings,
      );
      dispatch(numberSetting.set(20));
      dispatch(stringSetting.set("hello"));
      assert.deepEqual(topLevelState, {
        localSettings: {
          [settingName]: 20,
          [settingName2]: "hello",
        },
      });
    });

    it("should select values correctly.", function() {
      const numberSetting = new LocalSetting(
        settingName, (s: typeof topLevelState) => s.localSettings, 99,
      );
      assert.equal(numberSetting.selector(topLevelState), 99);
      dispatch(numberSetting.set(5));
      assert.equal(numberSetting.selector(topLevelState), 5);
    });
  });
});
