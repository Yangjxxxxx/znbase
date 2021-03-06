// Copyright 2018 The Cockroach Authors.
//
// Licensed as a CockroachDB Enterprise file under the Cockroach Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/cockroachdb/cockroach/blob/master/licenses/CCL.txt

// Package cdctest is a utility package for constructing schema objects
// in the context of cdc.
package cdctest

import (
	"strconv"

	"github.com/gogo/protobuf/proto"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/hlc"
)

// MakeTableDesc makes a generic table descriptor with the provided properties.
func MakeTableDesc(
	tableID sqlbase.ID, version sqlbase.DescriptorVersion, modTime hlc.Timestamp, cols int,
) *sqlbase.TableDescriptor {
	td := sqlbase.TableDescriptor{
		Name:             "foo",
		ID:               tableID,
		Version:          version,
		ModificationTime: modTime,
		NextColumnID:     1,
	}
	for i := 0; i < cols; i++ {
		td.Columns = append(td.Columns, *MakeColumnDesc(td.NextColumnID))
		td.NextColumnID++
	}
	return &td
}

// MakeColumnDesc makes a generic column descriptor with the provided id.
func MakeColumnDesc(id sqlbase.ColumnID) *sqlbase.ColumnDescriptor {
	return &sqlbase.ColumnDescriptor{
		Name:        "c" + strconv.Itoa(int(id)),
		ID:          id,
		Type:        sqlbase.ColumnType{SemanticType: sqlbase.ColumnType_BOOL},
		DefaultExpr: proto.String("true"),
	}
}

// AddColumnDropBackfillMutation adds a mutation to desc to drop a column.
// Yes, this does modify an Immutable.
func AddColumnDropBackfillMutation(desc *sqlbase.TableDescriptor) *sqlbase.TableDescriptor {
	desc.Mutations = append(desc.Mutations, sqlbase.DescriptorMutation{
		State:     sqlbase.DescriptorMutation_DELETE_AND_WRITE_ONLY,
		Direction: sqlbase.DescriptorMutation_DROP,
	})
	return desc
}

// AddNewColumnBackfillMutation adds a mutation to desc to add a column.
// Yes, this does modify an Immutable.
func AddNewColumnBackfillMutation(desc *sqlbase.TableDescriptor) *sqlbase.TableDescriptor {
	desc.Mutations = append(desc.Mutations, sqlbase.DescriptorMutation{
		Descriptor_: &sqlbase.DescriptorMutation_Column{Column: MakeColumnDesc(desc.NextColumnID)},
		State:       sqlbase.DescriptorMutation_DELETE_AND_WRITE_ONLY,
		Direction:   sqlbase.DescriptorMutation_ADD,
		MutationID:  0,
		Rollback:    false,
	})
	return desc
}
