package parser

import (
	"context"
	"fmt"

	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
)

// RunShowSyntax analyzes the syntax and reports its structure as data
// for the client. Even an error is reported as data.
//
// Since errors won't propagate to the client as an error, but as
// a result, the usual code path to capture and record errors will not
// be triggered. Instead, the caller can pass a reportErr closure to
// capture errors instead. May be nil.
func RunShowSyntax(
	ctx context.Context,
	stmt string,
	report func(ctx context.Context, field, msg string) error,
	reportErr func(ctx context.Context, err error),
) error {
	stmts, err := Parse(stmt, false)
	if err != nil {
		if reportErr != nil {
			reportErr(ctx, err)
		}

		pqErr, ok := pgerror.GetPGCause(err)
		if !ok {
			return pgerror.NewAssertionErrorf("unknown parser error: %v", err)
		}
		if err := report(ctx, "error", pqErr.Message); err != nil {
			return err
		}
		if err := report(ctx, "code", pqErr.Code.String()); err != nil {
			return err
		}
		if pqErr.Source != nil {
			if pqErr.Source.File != "" {
				if err := report(ctx, "file", pqErr.Source.File); err != nil {
					return err
				}
			}
			if pqErr.Source.Line > 0 {
				if err := report(ctx, "line", fmt.Sprintf("%d", pqErr.Source.Line)); err != nil {
					return err
				}
			}
			if pqErr.Source.Function != "" {
				if err := report(ctx, "function", pqErr.Source.Function); err != nil {
					return err
				}
			}
		}
		if pqErr.Detail != "" {
			if err := report(ctx, "detail", pqErr.Detail); err != nil {
				return err
			}
		}
		if pqErr.Hint != "" {
			if err := report(ctx, "hint", pqErr.Hint); err != nil {
				return err
			}
		}
	} else {
		for i := range stmts {
			str := tree.AsStringWithFlags(stmts[i].AST, tree.FmtParsable)
			if err := report(ctx, "sql", str); err != nil {
				return err
			}
		}
	}
	return nil
}
