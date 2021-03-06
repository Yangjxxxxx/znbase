  
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
import _ from "lodash";
import { assert } from "chai";
import { shallow } from "enzyme";
import * as sinon from "sinon";

import "src/enzymeInit";
import { SortableTable, SortableColumn, SortSetting } from "src/views/shared/components/sortabletable";

const columns: SortableColumn[] = [
  {
    title: "first",
    cell: (index) => index.toString() + ".first",
    sortKey: 1,
  },
  {
    title: "second",
    cell: (index) => index.toString() + ".second",
    sortKey: 2,
  },
  {
    title: "unsortable",
    cell: (index) => index.toString() + ".unsortable",
  },
];

function makeTable(count: number, sortSetting?: SortSetting,
                   onChangeSortSetting?: (ss: SortSetting) => void) {
  return shallow(<SortableTable count={count}
                                sortSetting={sortSetting}
                                onChangeSortSetting={onChangeSortSetting}
                                columns={columns}/>);
}

describe("<SortableTable>", () => {
  describe("renders correctly.", () => {
    it("renders the expected table structure.", () => {
      const wrapper = makeTable(1);
      assert.lengthOf(wrapper.find("table"), 1, "one table");
      assert.lengthOf(wrapper.find("thead").find("tr"), 1, "one header row");
      assert.lengthOf(wrapper.find("tr.sort-table__row--header"), 1, "column header row");
      assert.lengthOf(wrapper.find("tbody"), 1, "tbody element");
    });

    it("renders rows and columns correctly.", () => {
      const rowCount = 5;
      const wrapper = makeTable(rowCount);

      // Verify header structure.
      assert.equal(wrapper.find("tbody").find("tr").length, rowCount, "correct number of rows");
      const headers = wrapper.find("tr.sort-table__row--header");
      _.each(columns, (c, index) => {
        const header = headers.childAt(index);
        assert.isTrue(header.is(".sort-table__cell"), "header is correct class.");
        assert.equal(header.text(), c.title, "header has correct title.");
      });

      // Verify column contents.
      const rows = wrapper.find("tbody");
      _.times(rowCount, (rowIndex) => {
        const row = rows.childAt(rowIndex);
        assert.isTrue(row.is("tr"), "tbody contains rows");
        _.each(columns, (c, columnIndex) => {
          assert.equal(row.childAt(columnIndex).text(), c.cell(rowIndex), "table columns match");
        });
      });

      // Nothing is sorted.
      assert.lengthOf(wrapper.find("th.sort-table__cell--ascending"), 0, "expected zero sorted columns.");
      assert.lengthOf(wrapper.find("th.sort-table__cell--descending"), 0, "expected zero sorted columns.");
    });

    it("renders sorted column correctly.", () => {
      // ascending = false.
      let wrapper = makeTable(1, { sortKey: 1, ascending: false });

      let sortHeader = wrapper.find("th.sort-table__cell--descending");
      assert.lengthOf(sortHeader, 1, "only a single column is sorted descending.");
      assert.equal(sortHeader.text(), columns[0].title, "first column should be sorted.");
      sortHeader = wrapper.find("th.sort-table__cell--ascending");
      assert.lengthOf(sortHeader, 0, "no columns are sorted ascending.");

      // ascending = true
      wrapper = makeTable(1, { sortKey: 2, ascending: true });

      sortHeader = wrapper.find("th.sort-table__cell--ascending");
      assert.lengthOf(sortHeader, 1, "only a single column is sorted ascending.");
      assert.equal(sortHeader.text(), columns[1].title, "second column should be sorted.");
      sortHeader = wrapper.find("th.sort-table__cell--descending");
      assert.lengthOf(sortHeader, 0, "no columns are sorted descending.");
    });
  });

  describe("changes sort setting on clicks.", () => {
    it("sorts descending on initial click.", () => {
      const spy = sinon.spy();
      const wrapper = makeTable(1, undefined, spy);
      wrapper.find("th.sort-table__cell--sortable").first().simulate("click");
      assert.isTrue(spy.calledOnce);
      assert.isTrue(spy.calledWith({
        sortKey: 1,
        ascending: false,
      }));
    });

    // Click on sorted data, different column.
    it("sorts descending on new column.", () => {
      const spy = sinon.spy();
      const wrapper = makeTable(1, {sortKey: 2, ascending: true}, spy);

      wrapper.find("th.sort-table__cell--sortable").first().simulate("click");
      assert.isTrue(spy.calledOnce);
      assert.isTrue(spy.calledWith({
        sortKey: 1,
        ascending: false,
      }));
    });

    it("sorts ascending if same column is clicked twice.", () => {
      const spy = sinon.spy();
      const wrapper = makeTable(1, {sortKey: 1, ascending: false}, spy);

      wrapper.find("th.sort-table__cell--sortable").first().simulate("click");
      assert.isTrue(spy.calledOnce);
      assert.isTrue( spy.calledWith({
        sortKey: 1,
        ascending: true,
      }));
    });

    it("removes sorting if same column is clicked thrice.", () => {
      const spy = sinon.spy();
      const wrapper = makeTable(1, {sortKey: 1, ascending: true}, spy);

      wrapper.find("th.sort-table__cell--sortable").first().simulate("click");
      assert.isTrue(spy.calledOnce);
      assert.isTrue( spy.calledWith({
        sortKey: null,
        ascending: false,
      }));
    });

    // Click on unsortable column does nothing.
    it("does nothing if unsortable column is clicked.", () => {
      const spy = sinon.spy();
      const wrapper = makeTable(1, {sortKey: 1, ascending: true}, spy);

      wrapper.find("thead th.sort-table__cell").last().simulate("click");
      assert.isTrue(spy.notCalled);
    });
  });
});
