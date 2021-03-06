// Copyright 2017 The Cockroach Authors.
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

package sql

import (
	"context"
	"fmt"

	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/security/privilege"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/log"
)

type renameTableNode struct {
	n            *tree.RenameTable
	oldTn, newTn *tree.TableName
	tableDesc    *sqlbase.MutableTableDescriptor
}

// RenameTable renames the table, view or sequence.
// Privileges: DROP on source table/view/sequence, CREATE on destination database.
//   Notes: postgres requires the table owner.
//          mysql requires ALTER, DROP on the original table, and CREATE, INSERT
//          on the new table (and does not copy privileges over).
func (p *planner) RenameTable(ctx context.Context, n *tree.RenameTable) (planNode, error) {
	oldTn := &n.Name
	newTn := &n.NewName
	toRequire := requireTableOrViewDesc
	if n.IsView {
		toRequire = requireViewDesc
	} else if n.IsSequence {
		toRequire = requireSequenceDesc
	}

	tableDesc, err := p.ResolveMutableTableDescriptor(ctx, oldTn, !n.IfExists, toRequire)
	if err != nil {
		return nil, err
	}
	if tableDesc == nil {
		// Noop.
		return newZeroNode(nil /* columns */), nil
	}

	if err := checkViewMatchesMaterialized(*tableDesc, n.IsView, n.IsMaterialized); err != nil {
		return nil, err
	}

	if tableDesc.State != sqlbase.TableDescriptor_PUBLIC {
		return nil, sqlbase.NewUndefinedRelationError(oldTn)
	}

	// Check if any views depend on this table/view. Because our views
	// are currently just stored as strings, they explicitly specify the name
	// of everything they depend on. Rather than trying to rewrite the view's
	// query with the new name, we simply disallow such renames for now.
	if len(tableDesc.DependedOnBy) > 0 {
		return nil, p.dependentViewRenameError(
			ctx, tableDesc.TypeName(), oldTn.String(), tableDesc.ParentID, tableDesc.DependedOnBy[0].ID)
	}

	return &renameTableNode{n: n, oldTn: oldTn, newTn: newTn, tableDesc: tableDesc}, nil
}

func (n *renameTableNode) startExec(params runParams) error {
	p := params.p
	ctx := params.ctx
	oldTn := n.oldTn
	newTn := n.newTn
	tableDesc := n.tableDesc
	//temporary table
	tempscname := tree.Name(p.TemporarySchemaName())
	if oldTn.TableNamePrefix.SchemaName == tempscname {
		//if user has explicit database and schema
		if newTn.TableNamePrefix.ExplicitCatalog == true {
			if newTn.TableNamePrefix.CatalogName != oldTn.TableNamePrefix.CatalogName {
				return fmt.Errorf("cannot move objects into or out of temporary schemas")
			}
		}
		if newTn.TableNamePrefix.ExplicitSchema == true {
			if newTn.TableNamePrefix.SchemaName != tempscname {
				return fmt.Errorf("cannot move objects into or out of temporary schemas")
			}
		}
		//not explicit database and schema
		if oldTn.TableNamePrefix.SchemaName == tempscname {
			newTn.TableNamePrefix.SchemaName = oldTn.TableNamePrefix.SchemaName
			newTn.TableNamePrefix.ExplicitSchema = true
			tableDesc.Temporary = true
			//if this is set to false, oldTn.TableNamePrefix.SchemaName will
			//be set to public after ResolveUncachedDatabase() function
			oldTn.TableNamePrefix.ExplicitSchema = true
		}
	}
	//permanent table cannot convert to temp table
	if oldTn.TableNamePrefix.SchemaName != tempscname && newTn.TableNamePrefix.SchemaName == tempscname {
		return fmt.Errorf("cannot move objects into or out of temporary schemas")
	}

	prevDbDesc, err := p.ResolveUncachedDatabase(ctx, oldTn)
	if err != nil {
		return err
	}

	// Check if target database exists.
	// We also look at uncached descriptors here.
	targetDbDesc, err := p.ResolveUncachedDatabase(ctx, newTn)
	if err != nil {
		return err
	}

	if isInternal := CheckVirtualSchema(newTn.Schema()); isInternal {
		return fmt.Errorf("cannot create table in virtual schema: %q", newTn.Schema())
	}
	// we get the target schema, and check the targetScDesc CREATE privilege
	// and the table DROP privilege
	ParentID := targetDbDesc.GetSchemaID(newTn.Schema())
	targetScDesc, err := sqlbase.GetSchemaDescFromID(ctx, p.txn, ParentID)
	if err != nil {
		return err
	}

	if err := p.CheckPrivilege(ctx, tableDesc, privilege.DROP); err != nil {
		return err
	}

	if err := p.CheckPrivilege(ctx, targetScDesc, privilege.CREATE); err != nil {
		return err
	}

	tableDesc.ParentID = ParentID

	// oldTn and newTn are already normalized, so we can compare directly here.
	if oldTn.Catalog() == newTn.Catalog() &&
		oldTn.Schema() == newTn.Schema() &&
		oldTn.Table() == newTn.Table() {
		// Noop.
		return nil
	}
	tableDesc.SetName(newTn.Table())
	descKey := sqlbase.MakeDescMetadataKey(tableDesc.GetID())
	newTbKey := tableKey{parentID: tableDesc.ParentID, name: newTn.Table()}.Key()

	if err := tableDesc.RefreshValidate(ctx, p.txn, p.EvalContext().Settings); err != nil {
		return err
	}

	descID := tableDesc.GetID()
	descDesc := sqlbase.WrapDescriptor(tableDesc)

	renameDetails := sqlbase.TableDescriptor_NameInfo{
		ParentID: prevDbDesc.GetSchemaID(oldTn.Schema()),
		Name:     oldTn.Table()}
	tableDesc.DrainingNames = append(tableDesc.DrainingNames, renameDetails)
	if err := p.writeSchemaChange(ctx, tableDesc, sqlbase.InvalidMutationID); err != nil {
		return err
	}

	// We update the descriptor to the new name, but also leave the mapping of the
	// old name to the id, so that the name is not reused until the schema changer
	// has made sure it's not in use any more.
	b := &client.Batch{}
	if p.extendedEvalCtx.Tracing.KVTracingEnabled() {
		log.VEventf(ctx, 2, "Put %s -> %s", descKey, descDesc)
		log.VEventf(ctx, 2, "CPut %s -> %d", newTbKey, descID)
	}
	b.Put(descKey, descDesc)
	b.CPut(newTbKey, descID, nil)

	if err := p.txn.Run(ctx, b); err != nil {
		if _, ok := err.(*roachpb.ConditionFailedError); ok {
			return sqlbase.NewRelationAlreadyExistsError(newTn.Table())
		}
		return err
	}

	return nil
}

func (n *renameTableNode) Next(runParams) (bool, error) { return false, nil }
func (n *renameTableNode) Values() tree.Datums          { return tree.Datums{} }
func (n *renameTableNode) Close(context.Context)        {}

// TODO(a-robinson): Support renaming objects depended on by views once we have
// a better encoding for view queries (#10083).
func (p *planner) dependentViewRenameError(
	ctx context.Context, typeName, objName string, parentID, viewID sqlbase.ID,
) error {
	viewDesc, err := sqlbase.GetTableDescFromID(ctx, p.txn, viewID)
	if err != nil {
		return err
	}
	viewName := viewDesc.Name
	if viewDesc.ParentID != parentID {
		var err error
		viewName, err = p.getQualifiedTableName(ctx, viewDesc)
		if err != nil {
			log.Warningf(ctx, "unable to retrieve name of view %d: %v", viewID, err)
			msg := fmt.Sprintf("cannot rename %s %q because a view depends on it",
				typeName, objName)
			return sqlbase.NewDependentObjectError(msg)
		}
	}
	msg := fmt.Sprintf("cannot rename %s %q because view %q depends on it",
		typeName, objName, viewName)
	if viewDesc.IsTable() {
		msg = fmt.Sprintf("cannot rename %s %q because relation %q depends on it",
			typeName, objName, viewName)
	}
	hint := fmt.Sprintf("you can drop %s instead.", viewName)
	return sqlbase.NewDependentObjectErrorWithHint(msg, hint)
}
