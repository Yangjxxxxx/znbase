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

package coltypes

import (
	"bytes"

	"github.com/znbasedb/znbase/pkg/sql/lex"
)

// TArray represents an ARRAY column type.
type TArray struct {
	// ParamTyp is the type of the elements in this array.
	ParamType T
	Bounds    []int32
}

// TypeName implements the ColTypeFormatter interface.
func (node *TArray) TypeName() string {
	return node.ParamType.TypeName() + "[]"
}

// Format implements the ColTypeFormatter interface.
func (node *TArray) Format(buf *bytes.Buffer, f lex.EncodeFlags) {
	if collation, ok := node.ParamType.(*TCollatedString); ok {
		// We cannot use node.ParamType.Format() directly here (and DRY
		// across the two branches of the if) because if we have an array
		// of collated strings, the COLLATE string must appear after the
		// square brackets.
		collation.TString.Format(buf, f)
		buf.WriteString("[] COLLATE ")
		lex.EncodeUnrestrictedSQLIdent(buf, collation.Locale, f)
	} else {
		node.ParamType.Format(buf, f)
		buf.WriteString("[]")
	}
}

// TEnum represents an enum column type.
type TEnum struct {
	// ParamTyp is the type of the elements in this array.
	ParamType T
	Bounds    []string
}

// TypeName implements the ColTypeFormatter interface.
func (node *TEnum) TypeName() string {
	return "enum" + "(" + ")"
}

// Format implements the ColTypeFormatter interface.
func (node *TEnum) Format(buf *bytes.Buffer, f lex.EncodeFlags) {
	if collation, ok := node.ParamType.(*TCollatedString); ok {
		// We cannot use node.ParamType.Format() directly here (and DRY
		// across the two branches of the if) because if we have an array
		// of collated strings, the COLLATE string must appear after the
		// square brackets.
		collation.TString.Format(buf, f)
		buf.WriteString("[] COLLATE ")
		lex.EncodeUnrestrictedSQLIdent(buf, collation.Locale, f)
	} else {
		node.ParamType.Format(buf, f)
		//buf.WriteString("(" + fmt.Sprint(node.Bounds) + ")")
	}
}

// TSet represents an SET column type.
type TSet struct {
	Variant TStringVariant
	VisibleType
	Bounds []string
}

// TypeName implements the ColTypeFormatter interface.
func (node *TSet) TypeName() string {
	return node.Variant.String()
}

// Format implements the ColTypeFormatter interface.
func (node *TSet) Format(buf *bytes.Buffer, f lex.EncodeFlags) {
	buf.WriteString(node.TypeName())
}

// canBeInArrayColType returns true if the given T is a valid
// element type for an array column type.
// If the valid return is false, the issue number should
// be included in the error report to inform the user.
func canBeInArrayColType(t T) (valid bool, issueNum int) {
	switch t.(type) {
	case *TJSON:
		return false, 23468
	default:
		return true, 0
	}
}

// TVector is the base for VECTOR column types, which are Postgres's
// older, limited version of ARRAYs. These are not meant to be persisted,
// because ARRAYs are a strict superset.
type TVector struct {
	Name      string
	ParamType T
}

// TypeName implements the ColTypeFormatter interface.
func (node *TVector) TypeName() string { return node.Name }

// Format implements the ColTypeFormatter interface.
func (node *TVector) Format(buf *bytes.Buffer, _ lex.EncodeFlags) {
	buf.WriteString(node.TypeName())
}
