// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package schemaexpr

import (
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

// testCol includes the information needed to create a column descriptor for
// testing purposes.
type testCol struct {
	name string
	typ  *types.T
}

// testTableDesc is a helper functions for creating table descriptors in a
// less verbose way.
func testTableDesc(
	name string, columns []testCol, mutationColumns []testCol,
) sqlbase.MutableTableDescriptor {
	cols := make([]sqlbase.ColumnDescriptor, len(columns))
	for i := range columns {
		cols[i] = sqlbase.ColumnDescriptor{
			Name: columns[i].name,
			Type: ConvertTestType(columns[i].typ),
			// Column IDs start at 1 to mimic "real" table descriptors.
			ID: sqlbase.ColumnID(i + 1),
		}
	}

	muts := make([]sqlbase.DescriptorMutation, len(mutationColumns))
	for i := range mutationColumns {
		muts[i] = sqlbase.DescriptorMutation{
			Descriptor_: &sqlbase.DescriptorMutation_Column{
				Column: &sqlbase.ColumnDescriptor{
					Name: mutationColumns[i].name,
					Type: ConvertTestType(mutationColumns[i].typ),
					ID:   sqlbase.ColumnID(len(columns) + i + 1),
				},
			},
			Direction: sqlbase.DescriptorMutation_ADD,
		}
	}
	return *sqlbase.NewMutableCreatedTableDescriptor(sqlbase.TableDescriptor{
		Name:      name,
		ID:        1,
		Columns:   cols,
		Mutations: muts,
	})
}

//ConvertTestType is to convert Type for Test
func ConvertTestType(testtype *types.T) sqlbase.ColumnType {
	if *testtype == types.Bool {
		return sqlbase.ColumnType{
			SemanticType:    sqlbase.ColumnType_BOOL,
			VisibleTypeName: "bool",
		}
	} else if *testtype == types.String {
		return sqlbase.ColumnType{
			SemanticType:    sqlbase.ColumnType_STRING,
			VisibleTypeName: "string",
		}
	} else if *testtype == types.Int {
		return sqlbase.ColumnType{
			SemanticType:    sqlbase.ColumnType_INT,
			VisibleTypeName: "int",
		}
	}
	return sqlbase.ColumnType{}
}
