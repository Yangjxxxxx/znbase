// Copyright 2019 The Cockroach Authors.
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

package errors

import "github.com/znbasedb/errors/markers"

// Is determines whether one of the causes of the given error or any
// of its causes is equivalent to some reference error.
//
// Note: if any of the error types has been migrated from a previous
// package location or a different type, ensure that
// RegisterTypeMigration() was called prior to Is().
func Is(err, reference error) bool { return markers.Is(err, reference) }

// HasType returns true iff err contains an error whose concrete type
// matches that of referenceType.
func HasType(err, referenceType error) bool { return markers.HasType(err, referenceType) }

// If iterates on the error's causal chain and returns a predicate's
// return value the first time the predicate returns true.
//
// Note: if any of the error types has been migrated from a previous
// package location or a different type, ensure that
// RegisterTypeMigration() was called prior to If().
func If(err error, pred func(err error) (interface{}, bool)) (interface{}, bool) {
	return markers.If(err, pred)
}

// IsAny is like Is except that multiple references are compared.
//
// Note: if any of the error types has been migrated from a previous
// package location or a different type, ensure that
// RegisterTypeMigration() was called prior to IsAny().
func IsAny(err error, references ...error) bool { return markers.IsAny(err, references...) }

// Mark creates an explicit mark for the given error, using
// the same mark as some reference error.
//
// Note: if any of the error types has been migrated from a previous
// package location or a different type, ensure that
// RegisterTypeMigration() was called prior to Mark().
func Mark(err error, reference error) error { return markers.Mark(err, reference) }
