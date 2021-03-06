// Copyright 2019  The Cockroach Authors.
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

package pgerror

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgcode"
	"github.com/znbasedb/znbase/pkg/util/log"
)

// WrapWithDepthf wraps an error into a pgerror. If the code is not
// CodeInternalError, the code is used only if the original error was
// not a pgerror already. If the code is CodeInternalError, the code
// overrides the original error code and the result becomes an
// internal error.
//
// Note: the Wrap constructors return `error` instead of *Error
// because a nil err in the input must be propagated as a nill err in
// the output, and in Go a nil interface reference is different from a
// non-nil interface reference to a nil pointer.
func WrapWithDepthf(
	depth int, err error, code pgcode.Code, format string, args ...interface{},
) error {
	if err == nil {
		return nil
	}

	// Grab as much details as we can from the error that's already there.
	pgErr := collectErrForWrap(err, code)

	// Omit the prefix if the format string is empty.
	prefix := ""
	if format != "" {
		prefix = fmt.Sprintf(format+": ", args...)
	}

	// Compute a message. We have to base it off the full input error,
	// in case this contains intermediate stages wrapped with
	// errors.Wrap.
	baseMsg := err.Error()
	// Strip the internal error prefix if present; we'll add it back below.
	baseMsg = strings.TrimPrefix(baseMsg, internalErrorPrefix)

	// Store it.
	pgErr.Message = prefix + baseMsg

	// If there's no source yet, make one.
	if pgErr.Source == nil {
		srcCtx := makeSrcCtx(depth + 1)
		pgErr.Source = &srcCtx
	}

	// If wrapping a non-internal error using the internal error code,
	// the internal error prevails.
	if code == pgcode.Internal {
		// Override the result to become an internal error.
		pgErr.Code = code
		pgErr.Hint = assertionErrorHint
	}

	// If either the original error was an internal error or we just
	// turned into one, we want to store stack trace details.
	if pgErr.Code == pgcode.Internal {
		// Ensure the prefix is present.
		pgErr.Message = internalErrorPrefix + pgErr.Message

		// We'll want to report what we can from the provided context.
		st := log.NewStackTrace(depth + 1)
		pgErr.SafeDetail = append(pgErr.SafeDetail, &Error_SafeDetail{
			SafeMessage:       log.ReportablesToSafeError(depth+1, format, args).Error(),
			EncodedStackTrace: log.EncodeStackTrace(st),
		})

		// Add the stack trace to the user-visible details as well.
		var buf bytes.Buffer
		if pgErr.Detail != "" {
			fmt.Fprintf(&buf, "%s\n", pgErr.Detail)
		}
		fmt.Fprintf(&buf, "stack trace:\n%s", log.PrintStackTrace(st))
		pgErr.Detail = buf.String()
	}
	return pgErr
}

// Wrap wraps an error into a pgerror. See
// the doc on WrapWithDepthf for details.
func Wrap(err error, code pgcode.Code, msg string) error {
	return WrapWithDepthf(1, err, code, "%s", msg)
}

// Wrapf wraps an error into a pgerror. See
// the doc on WrapWithDepthf for details.
func Wrapf(err error, code pgcode.Code, format string, args ...interface{}) error {
	return WrapWithDepthf(1, err, code, format, args...)
}

// collectErrForWrap disassembles the provided error and
// collect details.
func collectErrForWrap(err error, fallbackCode pgcode.Code) *Error {
	pgErr, ok := err.(*Error)
	if ok {
		// Make a copy so we're not modifying the original error below.
		newErr := *pgErr
		return &newErr
	}

	// We're handling a non-Error object.
	// Is there a pgerr underneath we can work with?
	type causer interface {
		Cause() error
	}
	if cause, ok := err.(causer); ok {
		// Yes: collect it.
		pgErr = collectErrForWrap(cause.Cause(), fallbackCode)
	} else {
		// There's no discernible cause. Build a new pgerr from scratch.
		pgErr = &Error{Code: fallbackCode}
		//
		// For certain types of errors, we'll override the fall back code,
		// to ensure the error flows back up properly.
		// We also need to re-define the interfaces here explicitly
		// because we can't use `roachpb` directly (import loop)
		type ClientVisibleAmbiguousError interface{ ClientVisibleAmbiguousError() }
		type ClientVisibleRetryError interface{ ClientVisibleRetryError() }
		switch err.(type) {
		case ClientVisibleRetryError:
			pgErr.Code = pgcode.SerializationFailure
		case ClientVisibleAmbiguousError:
			pgErr.Code = pgcode.StatementCompletionUnknown
		}
	}

	// If we're assembling an internal error, keep the details too.
	var detail *Error_SafeDetail
	if pgErr.Code == pgcode.Internal {
		detail = &Error_SafeDetail{
			SafeMessage: log.Redact(err),
		}
	}

	// If a stack trace was available in the original
	// non-Error error (e.g. when constructed via errors.Wrap),
	// try to get useful information from it.
	type stackTracer interface {
		StackTrace() errors.StackTrace
	}
	if e, ok := err.(stackTracer); ok {
		tr := e.StackTrace()

		if pgErr.Source == nil && len(tr) > 0 {
			// Assemble a source information from scratch using
			// the provided stack trace.
			line, _ := strconv.Atoi(fmt.Sprintf("%d", tr[0]))
			pgErr.Source = &Error_Source{
				File:     fmt.Sprintf("%s", tr[0]),
				Line:     int32(line),
				Function: fmt.Sprintf("%n", tr[0]),
			}
		}

		if detail != nil {
			// TODO(knz): convert e.StackTrace() to a log.StackTrace then
			// populate EncodedStackTrace properly.
			if len(tr) > 0 {
				detail.SafeMessage = fmt.Sprintf("%v: %s", tr[0], detail.SafeMessage)
			}
			detail.EncodedStackTrace = fmt.Sprintf("%+v", tr)
		}
	}

	if detail != nil {
		pgErr.SafeDetail = append(pgErr.SafeDetail, detail)
	}

	return pgErr
}
