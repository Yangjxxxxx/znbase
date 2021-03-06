// Copyright 2019  The Cockroach Authors.

// {{/*
// +build execgen_template
//
// This file is the execgen template for proj_const_{left,right}_ops.eg.go.
// It's formatted in a special way, so it's both valid Go and a valid
// text/template input. This permits editing this file with editor support.
//
// */}}

package vecexec

import (
	"bytes"
	"context"
	"math"
	"time"

	"github.com/pkg/errors"
	"github.com/znbasedb/apd"
	"github.com/znbasedb/znbase/pkg/col/coldata"
	"github.com/znbasedb/znbase/pkg/col/coltypes"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/vecexec/execerror"
	// {{/*
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/vecexec/execgen"
	// */}}
	"github.com/znbasedb/znbase/pkg/sql/sem"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
)

// {{/*
// Declarations to make the template compile properly.

// Dummy import to pull in "bytes" package.
var _ bytes.Buffer

// Dummy import to pull in "apd" package.
var _ apd.Decimal

// Dummy import to pull in "tree" package.
var _ tree.Datum

// Dummy import to pull in "math" package.
var _ = math.MaxInt64

// Dummy import to pull in "time" package.
var _ time.Time

// Dummy import to pull in "coltypes" package.
var _ coltypes.T

// _ASSIGN is the template function for assigning the first input to the result
// of computation an operation on the second and the third inputs.
func _ASSIGN(_, _, _ interface{}) {
	execerror.VectorizedInternalPanic("")
}

// _RET_UNSAFEGET is the template function that will be replaced by
// "execgen.UNSAFEGET" which uses _RET_TYP.
func _RET_UNSAFEGET(_, _ interface{}) interface{} {
	execerror.VectorizedInternalPanic("")
}

// */}}

// {{define "projConstOp" }}

type _OP_CONST_NAME struct {
	projConstOpBase
	// {{ if _IS_CONST_LEFT }}
	constArg _L_GO_TYPE
	// {{ else }}
	constArg _R_GO_TYPE
	// {{ end }}
}

func (p _OP_CONST_NAME) Next(ctx context.Context) coldata.Batch {
	batch := p.input.Next(ctx)
	n := batch.Length()
	if p.outputIdx == batch.Width() {
		p.allocator.AppendColumn(batch, coltypes._RET_TYP)
	}
	if n == 0 {
		return batch
	}
	vec := batch.ColVec(p.colIdx)
	// {{if _IS_CONST_LEFT}}
	col := vec._R_TYP()
	// {{else}}
	col := vec._L_TYP()
	// {{end}}
	projVec := batch.ColVec(p.outputIdx)
	projCol := projVec._RET_TYP()
	if vec.Nulls().MaybeHasNulls() {
		_SET_PROJECTION(true)
	} else {
		_SET_PROJECTION(false)
	}
	return batch
}

func (p _OP_CONST_NAME) Init() {
	p.input.Init()
}

// {{end}}

// {{/*
func _SET_PROJECTION(_HAS_NULLS bool) {
	// */}}
	// {{define "setProjection" -}}
	// {{$hasNulls := $.HasNulls}}
	// {{with $.Overload}}
	// {{if _HAS_NULLS}}
	colNulls := vec.Nulls()
	// {{end}}
	if sel := batch.Selection(); sel != nil {
		sel = sel[:n]
		for _, i := range sel {
			_SET_SINGLE_TUPLE_PROJECTION(_HAS_NULLS)
		}
	} else {
		col = execgen.SLICE(col, 0, int(n))
		_ = _RET_UNSAFEGET(projCol, int(n)-1)
		for execgen.RANGE(i, col, 0, int(n)) {
			_SET_SINGLE_TUPLE_PROJECTION(_HAS_NULLS)
		}
	}
	// {{if _HAS_NULLS}}
	colNullsCopy := colNulls.Copy()
	projVec.SetNulls(&colNullsCopy)
	// {{end}}
	// {{end}}
	// {{end}}
	// {{/*
}

// */}}

// {{/*
func _SET_SINGLE_TUPLE_PROJECTION(_HAS_NULLS bool) { // */}}
	// {{define "setSingleTupleProjection" -}}
	// {{$hasNulls := $.HasNulls}}
	// {{with $.Overload}}
	// {{if _HAS_NULLS}}
	if !colNulls.NullAt(uint16(i)) {
		// We only want to perform the projection operation if the value is not null.
		// {{end}}
		arg := execgen.UNSAFEGET(col, int(i))
		// {{if _IS_CONST_LEFT}}
		_ASSIGN("projCol[i]", "p.constArg", "arg")
		// {{else}}
		_ASSIGN("projCol[i]", "arg", "p.constArg")
		// {{end}}
		// {{if _HAS_NULLS }}
	}
	// {{end}}
	// {{end}}
	// {{end}}
	// {{/*
}

// */}}

// {{/*
// The outer range is a coltypes.T (the left type). The middle range is also a
// coltypes.T (the right type). The inner is the overloads associated with
// those two types.
// */}}
// {{range .}}
// {{range .}}
// {{range .}}

// {{template "projConstOp" .}}

// {{end}}
// {{end}}
// {{end}}

// GetProjection_CONST_SIDEConstOperator returns the appropriate constant
// projection operator for the given left and right column types and operation.
func GetProjection_CONST_SIDEConstOperator(
	allocator *Allocator,
	leftColType *types.T,
	rightColType *types.T,
	op tree.Operator,
	input Operator,
	colIdx int,
	constArg tree.Datum,
	outputIdx int,
) (Operator, error) {
	projConstOpBase := projConstOpBase{
		OneInputNode: NewOneInputNode(input),
		allocator:    allocator,
		colIdx:       colIdx,
		outputIdx:    outputIdx,
	}
	// {{if _IS_CONST_LEFT}}
	c, err := sem.GetDatumToPhysicalFn(sem.ToNewType(*leftColType))(constArg)
	// {{else}}
	c, err := sem.GetDatumToPhysicalFn(sem.ToNewType(*rightColType))(constArg)
	// {{end}}
	if err != nil {
		return nil, err
	}
	switch leftType := sem.FromColumnType(sem.ToNewType(*leftColType)); leftType {
	// {{range $lTyp, $rTypToOverloads := .}}
	case coltypes._L_TYP_VAR:
		switch rightType := sem.FromColumnType(sem.ToNewType(*rightColType)); rightType {
		// {{range $rTyp, $overloads := $rTypToOverloads}}
		case coltypes._R_TYP_VAR:
			switch op.(type) {
			case tree.BinaryOperator:
				switch op {
				// {{range $overloads}}
				// {{if .IsBinOp}}
				case tree._NAME:
					return &_OP_CONST_NAME{
						projConstOpBase: projConstOpBase,
						// {{if _IS_CONST_LEFT}}
						constArg: c.(_L_GO_TYPE),
						// {{else}}
						constArg: c.(_R_GO_TYPE),
						// {{end}}
					}, nil
				// {{end}}
				// {{end}}
				default:
					return nil, errors.Errorf("unhandled binary operator: %s", op)
				}
			case tree.ComparisonOperator:
				switch op {
				// {{range $overloads}}
				// {{if .IsCmpOp}}
				case tree._NAME:
					return &_OP_CONST_NAME{
						projConstOpBase: projConstOpBase,
						// {{if _IS_CONST_LEFT}}
						constArg: c.(_L_GO_TYPE),
						// {{else}}
						constArg: c.(_R_GO_TYPE),
						// {{end}}
					}, nil
				// {{end}}
				// {{end}}
				default:
					return nil, errors.Errorf("unhandled comparison operator: %s", op)
				}
			default:
				return nil, errors.New("unhandled operator type")
			}
		// {{end}}
		default:
			return nil, errors.Errorf("unhandled right type: %s", rightType)
		}
	// {{end}}
	default:
		return nil, errors.Errorf("unhandled left type: %s", leftType)
	}
}
