package delegate

import (
	"bytes"
	"context"
	"fmt"

	"github.com/znbasedb/znbase/pkg/sql/lex"
	"github.com/znbasedb/znbase/pkg/sql/parser"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

func (d *delegator) delegateShowSyntax(n *tree.ShowSyntax) (tree.Statement, error) {
	// Construct an equivalent SELECT query that produces the results:
	//
	// SELECT @1 AS field, @2 AS text
	//   FROM (VALUES
	//           ('file',     'foo.go'),
	//           ('line',     '123'),
	//           ('function', 'blix()'),
	//           ('detail',   'some details'),
	//           ('hint',     'some hints'))
	//
	var query bytes.Buffer
	fmt.Fprintf(
		&query, "SELECT @1 AS %s, @2 AS %s FROM (VALUES ",
		sqlbase.ShowSyntaxColumns[0].Name, sqlbase.ShowSyntaxColumns[1].Name,
	)

	comma := ""
	// TODO(knz): in the call below, reportErr is nil although we might
	// want to be able to capture (and report) these errors as well.
	//
	// However, this code path is only used when SHOW SYNTAX is used as
	// a data source, i.e. a client actively uses a query of the form
	// SELECT ... FROM [SHOW SYNTAX ' ... '] WHERE ....  This is not
	// what `znbase sql` does: the SQL shell issues a straight `SHOW
	// SYNTAX` that goes through the "statement observer" code
	// path. Since we care mainly about what users do in the SQL shell,
	// it's OK if we only deal with that case well for now and, for the
	// time being, forget/ignore errors when SHOW SYNTAX is used as data
	// source. This can be added later if deemed useful or necessary.
	if err := parser.RunShowSyntax(d.ctx, n.Statement, func(ctx context.Context, field, msg string) error {
		fmt.Fprintf(&query, "%s('%s', ", comma, field)
		lex.EncodeSQLString(&query, msg)
		query.WriteByte(')')
		comma = ", "
		return nil
	}, nil /* reportErr */); err != nil {
		return nil, err
	}
	query.WriteByte(')')
	return parse(query.String())
}
