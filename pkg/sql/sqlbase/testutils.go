// Copyright 2016  The Cockroach Authors.
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

package sqlbase

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sort"
	"unicode"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql/coltypes"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/util/bitarray"
	"github.com/znbasedb/znbase/pkg/util/duration"
	"github.com/znbasedb/znbase/pkg/util/ipaddr"
	"github.com/znbasedb/znbase/pkg/util/json"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/randutil"
	"github.com/znbasedb/znbase/pkg/util/timeofday"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
	"github.com/znbasedb/znbase/pkg/util/uuid"
)

// This file contains utility functions for tests (in other packages).

// GetTableDescriptor retrieves a table descriptor directly from the KV layer.
func GetTableDescriptor(
	kvDB *client.DB, database string, schema string, table string,
) *TableDescriptor {
	// log.VEventf(context.TODO(), 2, "GetTableDescriptor %q %q", database, table)
	dbNameKey := MakeNameMetadataKey(keys.RootNamespaceID, database)
	gr, err := kvDB.Get(context.TODO(), dbNameKey)
	if err != nil {
		panic(err)
	}
	if !gr.Exists() {
		panic("database missing")
	}
	//dbDescID := ID(gr.ValueInt())
	ctx := context.TODO()
	r, err := kvDB.Get(ctx, dbNameKey)
	if err != nil {
		panic(err)
	}
	dbDescKey := MakeDescMetadataKey(ID(r.ValueInt()))
	desc := &Descriptor{}
	if err := kvDB.GetProto(ctx, dbDescKey, desc); err != nil {
		panic(err)
	}
	dbDesc := desc.GetDatabase()
	var scDesc SchemaDescriptor
	for _, v := range dbDesc.Schemas {
		if v.Name == schema {
			scDesc = v
		}
	}
	tableNameKey := MakeNameMetadataKey(scDesc.ID, table)
	gr, err = kvDB.Get(context.TODO(), tableNameKey)
	if err != nil {
		panic(err)
	}
	if !gr.Exists() {
		panic("table missing")
	}

	descKey := MakeDescMetadataKey(ID(gr.ValueInt()))
	tbDesc := &Descriptor{}
	ts, err := kvDB.GetProtoTs(context.TODO(), descKey, tbDesc)
	if err != nil || (*tbDesc == Descriptor{}) {
		log.Fatalf(context.TODO(), "proto with id %d missing. err: %v", gr.ValueInt(), err)
	}

	return tbDesc.Table(ts)
}

// GetImmutableTableDescriptor retrieves an immutable table descriptor directly from the KV layer.
func GetImmutableTableDescriptor(
	kvDB *client.DB, database string, schema string, table string,
) *ImmutableTableDescriptor {
	return NewImmutableTableDescriptor(*GetTableDescriptor(kvDB, database, schema, table))
}

// RandDatum generates a random Datum of the given type.
// If nullOk is true, the datum can be DNull.
// Note that if typ.SemanticType is ColumnType_NULL, the datum will always be DNull,
// regardless of the null flag.
func RandDatum(rng *rand.Rand, typ ColumnType, nullOk bool) tree.Datum {
	nullDenominator := 10
	if !nullOk {
		nullDenominator = 0
	}
	return RandDatumWithNullChance(rng, typ, nullDenominator)
}

// RandDatumWithNullChance generates a random Datum of the given type.
// nullChance is the chance of returning null, expressed as a fraction
// denominator. For example, a nullChance of 5 means that there's a 1/5 chance
// that DNull will be returned. A nullChance of 0 means that DNull will not
// be returned.
// Note that if typ.SemanticType is ColumnType_NULL, the datum will always be
// DNull, regardless of the null flag.
func RandDatumWithNullChance(rng *rand.Rand, typ ColumnType, nullChance int) tree.Datum {
	if nullChance != 0 && rng.Intn(nullChance) == 0 {
		return tree.DNull
	}
	switch typ.SemanticType {
	case ColumnType_BOOL:
		return tree.MakeDBool(rng.Intn(2) == 1)
	case ColumnType_INT:
		// int64(rng.Uint64()) to get negative numbers, too
		return tree.NewDInt(tree.DInt(int64(rng.Uint64())))
	case ColumnType_FLOAT:
		return tree.NewDFloat(tree.DFloat(rng.NormFloat64()))
	case ColumnType_DECIMAL:
		d := &tree.DDecimal{}
		// int64(rng.Uint64()) to get negative numbers, too
		d.Decimal.SetFinite(int64(rng.Uint64()), int32(rng.Intn(40)-20))
		return d
	case ColumnType_DATE:
		return tree.NewDDate(tree.DDate(rng.Intn(10000)))
	case ColumnType_TIME:
		return tree.MakeDTime(timeofday.Random(rng))
	case ColumnType_TIMESTAMP:
		return &tree.DTimestamp{Time: timeutil.Unix(rng.Int63n(1000000), rng.Int63n(1000000))}
	case ColumnType_INTERVAL:
		sign := 1 - rng.Int63n(2)*2
		return &tree.DInterval{Duration: duration.MakeDuration(
			sign*rng.Int63n(25*3600*int64(1000000000)),
			sign*rng.Int63n(1000),
			sign*rng.Int63n(1000),
		)}
	case ColumnType_UUID:
		return tree.NewDUuid(tree.DUuid{UUID: uuid.MakeV4()})
	case ColumnType_INET:
		ipAddr := ipaddr.RandIPAddr(rng)
		return tree.NewDIPAddr(tree.DIPAddr{IPAddr: ipAddr})
	case ColumnType_JSONB:
		j, err := json.Random(20, rng)
		if err != nil {
			return nil
		}
		return &tree.DJSON{JSON: j}
	case ColumnType_TUPLE:
		tuple := tree.DTuple{D: make(tree.Datums, len(typ.TupleContents))}
		for i, internalType := range typ.TupleContents {
			tuple.D[i] = RandDatum(rng, internalType, true)
		}
		return &tuple
	case ColumnType_BIT:
		width := typ.Width
		if width == 0 {
			width = rng.Int31n(100)
		}
		r := bitarray.Rand(rng, uint(width))
		return &tree.DBitArray{BitArray: r}
	case ColumnType_STRING:
		// Generate a random ASCII string.
		p := make([]byte, rng.Intn(10))
		for i := range p {
			p[i] = byte(1 + rng.Intn(127))
		}
		return tree.NewDString(string(p))
	case ColumnType_BYTES:
		p := make([]byte, rng.Intn(10))
		_, _ = rng.Read(p)
		return tree.NewDBytes(tree.DBytes(p))
	case ColumnType_TIMESTAMPTZ:
		return &tree.DTimestampTZ{Time: timeutil.Unix(rng.Int63n(1000000), rng.Int63n(1000000))}
	case ColumnType_COLLATEDSTRING:
		if typ.Locale == nil {
			panic("locale is required for COLLATEDSTRING")
		}
		// Generate a random Unicode string.
		var buf bytes.Buffer
		n := rng.Intn(10)
		for i := 0; i < n; i++ {
			var r rune
			for {
				r = rune(rng.Intn(unicode.MaxRune + 1))
				if !unicode.Is(unicode.C, r) {
					break
				}
			}
			buf.WriteRune(r)
		}
		return tree.NewDCollatedString(buf.String(), *typ.Locale, &tree.CollationEnvironment{})
	case ColumnType_NAME:
		// Generate a random ASCII string.
		p := make([]byte, rng.Intn(10))
		for i := range p {
			p[i] = byte(1 + rng.Intn(127))
		}
		return tree.NewDName(string(p))
	case ColumnType_OID:
		return tree.NewDOid(tree.DInt(rng.Uint32()))
	case ColumnType_NULL:
		return tree.DNull
	case ColumnType_ARRAY:
		if typ.ArrayContents == nil {
			var contentsTyp = RandArrayContentsColumnType(rng)
			typ.ArrayContents = &contentsTyp.SemanticType
			typ.Locale = contentsTyp.Locale
		}
		eltTyp := typ.elementColumnType()
		datumType := columnSemanticTypeToDatumType(eltTyp, eltTyp.SemanticType)
		arr := tree.NewDArray(datumType)
		for i := 0; i < rng.Intn(10); i++ {
			if err := arr.Append(RandDatumWithNullChance(rng, *eltTyp, 0)); err != nil {
				panic(err)
			}
		}
		return arr
	case ColumnType_INT2VECTOR:
		return tree.DNull
	case ColumnType_OIDVECTOR:
		return tree.DNull
	case ColumnType_ENUM, ColumnType_SET:
		return tree.DNull
	default:
		panic(fmt.Sprintf("invalid type %s", typ.String()))
	}
}

var (
	columnSemanticTypes []ColumnType_SemanticType
	// arrayElemSemanticTypes contains all of the semantic types that are valid
	// to store within an array.
	arrayElemSemanticTypes []ColumnType_SemanticType
	collationLocales       = [...]string{"da", "de", "en"}
)

func init() {
	for k := range ColumnType_SemanticType_name {
		columnSemanticTypes = append(columnSemanticTypes, ColumnType_SemanticType(k))
	}
	for _, t := range types.AnyNonArray {
		encTyp, err := datumTypeToArrayElementEncodingType(t)
		if err != nil || encTyp == 0 {
			continue
		}
		semTyp, err := datumTypeToColumnSemanticType(t)
		if err != nil {
			continue
		}
		arrayElemSemanticTypes = append(arrayElemSemanticTypes, semTyp)
	}
}

// RandCollationLocale returns a random element of collationLocales.
func RandCollationLocale(rng *rand.Rand) *string {
	return &collationLocales[rng.Intn(len(collationLocales))]
}

// RandColumnType returns a random ColumnType value.
func RandColumnType(rng *rand.Rand) ColumnType {
	return randColumnType(rng, columnSemanticTypes)
}

// RandArrayContentsColumnType returns a random ColumnType that's guaranteed
// to be valid to use as the contents of an array.
func RandArrayContentsColumnType(rng *rand.Rand) ColumnType {
	return randColumnType(rng, arrayElemSemanticTypes)
}

func randColumnType(rng *rand.Rand, types []ColumnType_SemanticType) ColumnType {
	typ := ColumnType{SemanticType: types[rng.Intn(len(types))]}
	if typ.SemanticType == ColumnType_TIME {
		return randColumnType(rng, types)
	}
	if typ.SemanticType == ColumnType_BIT {
		typ.Width = int32(rng.Intn(50))
	}
	if typ.SemanticType == ColumnType_COLLATEDSTRING {
		typ.Locale = RandCollationLocale(rng)
	}
	if typ.SemanticType == ColumnType_ARRAY {
		inner := RandArrayContentsColumnType(rng)
		if inner.SemanticType == ColumnType_COLLATEDSTRING {
			// TODO(justin): change this when collated arrays are supported.
			inner.SemanticType = ColumnType_STRING
		}
		typ.ArrayContents = &inner.SemanticType
	}
	if typ.SemanticType == ColumnType_TUPLE {
		// Generate tuples between 0 and 4 datums in length
		len := rng.Intn(5)
		typ.TupleContents = make([]ColumnType, len)
		for i := range typ.TupleContents {
			typ.TupleContents[i] = RandColumnType(rng)
		}
	}
	return typ
}

// RandSortingColumnType returns a column type which can be key-encoded.
func RandSortingColumnType(rng *rand.Rand) ColumnType {
	typ := RandColumnType(rng)
	for MustBeValueEncoded(typ.SemanticType) {
		typ = RandColumnType(rng)
	}
	return typ
}

// RandColumnTypes returns a slice of numCols random ColumnType values.
func RandColumnTypes(rng *rand.Rand, numCols int) []ColumnType {
	types := make([]ColumnType, numCols)
	for i := range types {
		types[i] = RandColumnType(rng)
	}
	return types
}

// RandSortingColumnTypes returns a slice of numCols random ColumnType values
// which are key-encodable.
func RandSortingColumnTypes(rng *rand.Rand, numCols int) []ColumnType {
	types := make([]ColumnType, numCols)
	for i := range types {
		types[i] = RandSortingColumnType(rng)
	}
	return types
}

// RandDatumEncoding returns a random DatumEncoding value.
func RandDatumEncoding(rng *rand.Rand) DatumEncoding {
	return DatumEncoding(rng.Intn(len(DatumEncoding_value)))
}

// RandEncDatum generates a random EncDatum (of a random type).
func RandEncDatum(rng *rand.Rand) (EncDatum, ColumnType) {
	typ := RandColumnType(rng)
	datum := RandDatum(rng, typ, true /* nullOk */)
	return DatumToEncDatum(typ, datum), typ
}

// RandSortingEncDatumSlice generates a slice of random EncDatum values of the
// same random type which is key-encodable.
func RandSortingEncDatumSlice(rng *rand.Rand, numVals int) ([]EncDatum, ColumnType) {
	typ := RandSortingColumnType(rng)
	vals := make([]EncDatum, numVals)
	for i := range vals {
		vals[i] = DatumToEncDatum(typ, RandDatum(rng, typ, true))
	}
	return vals, typ
}

// RandSortingEncDatumSlice generates a slice of random EncDatum values of the
// int type which is key-encodable. it contains 1 and range(0, 10000). for mixHashrouter test
func RandSortingEncDatumSliceWithIntAndManyOneTwo(rng *rand.Rand, numVals int) ([]EncDatum, ColumnType) {
	typ := IntType
	vals := make([]EncDatum, numVals)
	for i := range vals {
		if i % 5 == 0 {
			vals[i] = DatumToEncDatum(typ, tree.NewDInt(tree.DInt(int64(1))))
			continue
		}
		if i % 8 == 0 {
			vals[i] = DatumToEncDatum(typ, tree.NewDInt(tree.DInt(int64(2))))
			continue
		}
		vals[i] = DatumToEncDatum(typ, tree.NewDInt(tree.DInt(int64(rng.Uint64() % 10000))))
	}
	return vals, typ
}

// RandSortingEncDatumSlices generates EncDatum slices, each slice with values of the same
// random type which is key-encodable.
func RandSortingEncDatumSlices(
	rng *rand.Rand, numSets, numValsPerSet int,
) ([][]EncDatum, []ColumnType) {
	vals := make([][]EncDatum, numSets)
	types := make([]ColumnType, numSets)
	for i := range vals {
		vals[i], types[i] = RandSortingEncDatumSlice(rng, numValsPerSet)
	}
	return vals, types
}

// RandSortingEncDatumSlices generates EncDatum slices, each slice with values of the
// int type which is key-encodable.
func RandSortingEncDatumSlicesWithInt(
	rng *rand.Rand, numSets, numValsPerSet int,
) ([][]EncDatum, []ColumnType) {
	vals := make([][]EncDatum, numSets)
	types := make([]ColumnType, numSets)
	for i := range vals {
		vals[i], types[i] = RandSortingEncDatumSliceWithIntAndManyOneTwo(rng, numValsPerSet)
	}
	return vals, types
}

// RandEncDatumRowOfTypes generates a slice of random EncDatum values for the
// corresponding type in types.
func RandEncDatumRowOfTypes(rng *rand.Rand, types []ColumnType) EncDatumRow {
	vals := make([]EncDatum, len(types))
	for i, typ := range types {
		vals[i] = DatumToEncDatum(typ, RandDatum(rng, typ, true))
	}
	return vals
}

// RandEncDatumRows generates EncDatumRows where all rows follow the same random
// []ColumnType structure.
func RandEncDatumRows(rng *rand.Rand, numRows, numCols int) (EncDatumRows, []ColumnType) {
	types := RandColumnTypes(rng, numCols)
	return RandEncDatumRowsOfTypes(rng, numRows, types), types
}

// RandEncDatumRowsOfTypes generates EncDatumRows, each row with values of the
// corresponding type in types.
func RandEncDatumRowsOfTypes(rng *rand.Rand, numRows int, types []ColumnType) EncDatumRows {
	vals := make(EncDatumRows, numRows)
	for i := range vals {
		vals[i] = RandEncDatumRowOfTypes(rng, types)
	}
	return vals
}

// TestingMakePrimaryIndexKey creates a key prefix that corresponds to
// a table row (in the primary index); it is intended for tests.
//
// It is exported because it is used by tests outside of this package.
//
// The value types must match the primary key columns (or a prefix of them);
// supported types are: - Datum
//  - bool (converts to DBool)
//  - int (converts to DInt)
//  - string (converts to DString)
func TestingMakePrimaryIndexKey(desc *TableDescriptor, vals ...interface{}) (roachpb.Key, error) {
	index := &desc.PrimaryIndex
	if len(vals) > len(index.ColumnIDs) {
		return nil, errors.Errorf("got %d values, PK has %d columns", len(vals), len(index.ColumnIDs))
	}
	datums := make([]tree.Datum, len(vals))
	for i, v := range vals {
		switch v := v.(type) {
		case bool:
			datums[i] = tree.MakeDBool(tree.DBool(v))
		case int:
			datums[i] = tree.NewDInt(tree.DInt(v))
		case string:
			datums[i] = tree.NewDString(v)
		case tree.Datum:
			datums[i] = v
		default:
			return nil, errors.Errorf("unexpected value type %T", v)
		}
		// Check that the value type matches.
		colID := index.ColumnIDs[i]
		for _, c := range desc.Columns {
			if c.ID == colID {
				colTyp, err := DatumTypeToColumnType(datums[i].ResolvedType())
				if err != nil {
					return nil, err
				}
				if t := colTyp.SemanticType; t != c.Type.SemanticType {
					return nil, errors.Errorf("column %d of type %s, got value of type %s", i, c.Type.SemanticType, t)
				}
				break
			}
		}
	}
	// Create the ColumnID to index in datums slice map needed by
	// MakeIndexKeyPrefix.
	colIDToRowIndex := make(map[ColumnID]int)
	for i := range vals {
		colIDToRowIndex[index.ColumnIDs[i]] = i
	}

	keyPrefix := MakeIndexKeyPrefix(desc, index.ID)
	key, _, err := EncodeIndexKey(desc, index, colIDToRowIndex, datums, keyPrefix)
	if err != nil {
		return nil, err
	}
	return roachpb.Key(key), nil
}

// TestingDatumTypeToColumnSemanticType is used in pgwire tests.
func TestingDatumTypeToColumnSemanticType(ptyp types.T) (ColumnType_SemanticType, error) {
	return datumTypeToColumnSemanticType(ptyp)
}

// RandCreateTable creates a random CreateTable definition.
func RandCreateTable(rng *rand.Rand, tableIdx int) *tree.CreateTable {
	// columnDefs contains the list of Columns we'll add to our table.
	columnDefs := make([]*tree.ColumnTableDef, randutil.RandIntInRange(rng, 1, 20))
	// defs contains the list of Columns and other attributes (indexes, column
	// families, etc) we'll add to our table.
	defs := make(tree.TableDefs, len(columnDefs))

	for i := range columnDefs {
		columnDef := randColumnTableDef(rng, i)
		columnDefs[i] = columnDef
		defs[i] = columnDef
	}

	// Shuffle our column definitions in preparation for random partitioning into
	// column families.
	rng.Shuffle(len(columnDefs), func(i, j int) {
		columnDefs[i], columnDefs[j] = columnDefs[j], columnDefs[i]
	})

	// Partition into column families.
	numColFams := randNumColFams(rng, len(columnDefs))

	// Create a slice of indexes into the columnDefs slice. We'll use this to make
	// the random partitioning by picking some indexes at random to use as
	// Partitions boundaries.
	indexes := make([]int, len(columnDefs)-1)
	for i := range indexes {
		indexes[i] = i + 1
	}
	rng.Shuffle(len(indexes), func(i, j int) {
		indexes[i], indexes[j] = indexes[j], indexes[i]
	})

	// Grab our random partition boundaries, and re-sort back into sorted index
	// order.
	numSeparators := numColFams - 1
	indexes = indexes[:numSeparators]
	sort.Slice(indexes, func(i, j int) bool {
		return indexes[i] < indexes[j]
	})

	indexesWithZero := make([]int, len(indexes)+2)
	copy(indexesWithZero[1:], indexes)
	indexesWithZero[len(indexesWithZero)-1] = len(columnDefs)
	indexes = indexesWithZero

	// Now (finally), indexes is the list of Partitions we're going to slice the
	// column def list into. Create our column families by grabbing the slice of
	// columns from the column list bounded by each partition index at the end.
	// Also, save column family 0 for later as all primary keys have to be part of
	// that column family.
	var colFamZero []*tree.ColumnTableDef
	for i := 0; i+1 < len(indexes); i++ {
		start, end := indexes[i], indexes[i+1]

		names := make(tree.NameList, end-start)
		for j := start; j < end; j++ {
			names[j-start] = columnDefs[j].Name
		}
		if colFamZero == nil {
			for j := start; j < end; j++ {
				colFamZero = append(colFamZero, columnDefs[j])
			}
		}

		famDef := &tree.FamilyTableDef{
			Name:    tree.Name(fmt.Sprintf("fam%d", i)),
			Columns: names,
		}
		defs = append(defs, famDef)
	}

	// Make a random primary key with high likelihood.
	if rng.Intn(8) != 0 {
		indexDef := randIndexTableDefFromCols(rng, colFamZero)
		if len(indexDef.Columns) > 0 {
			defs = append(defs, &tree.UniqueConstraintTableDef{
				PrimaryKey:    true,
				IndexTableDef: indexDef,
			})
		}
	}

	colNames := make(tree.NameList, len(columnDefs))
	for i := range columnDefs {
		colNames[i] = columnDefs[i].Name
	}

	// Make indexes.
	nIdxs := rng.Intn(10)
	for i := 0; i < nIdxs; i++ {
		indexDef := randIndexTableDefFromCols(rng, columnDefs)
		if len(indexDef.Columns) == 0 {
			continue
		}
		unique := rng.Intn(2) == 0
		if unique {
			defs = append(defs, &tree.UniqueConstraintTableDef{
				IndexTableDef: indexDef,
			})
		} else {
			defs = append(defs, &indexDef)
		}
	}

	// We're done! Return a new table with all of the attributes we've made.
	ret := &tree.CreateTable{
		Table: tree.MakeUnqualifiedTableName(tree.Name(fmt.Sprintf("table%d", tableIdx))),
		Defs:  defs,
	}
	return ret
}

// randColumnTableDef produces a random ColumnTableDef, with a random type and
// nullability.
func randColumnTableDef(rand *rand.Rand, colIdx int) *tree.ColumnTableDef {
	err := errors.New("fail")
	var colType coltypes.T
	for err != nil {
		columnType := RandSortingColumnType(rand)
		datumType := columnType.ToDatumType()
		colType, err = coltypes.DatumTypeToColumnType(datumType)
	}
	columnDef := &tree.ColumnTableDef{
		Name: tree.Name(fmt.Sprintf("col%d", colIdx)),
		Type: colType,
	}
	columnDef.Nullable.Nullability = tree.Nullability(rand.Intn(int(tree.SilentNull) + 1))
	return columnDef
}

func randIndexTableDefFromCols(
	rng *rand.Rand, columnTableDefs []*tree.ColumnTableDef,
) tree.IndexTableDef {
	cpy := make([]*tree.ColumnTableDef, len(columnTableDefs))
	copy(cpy, columnTableDefs)
	rng.Shuffle(len(cpy), func(i, j int) { cpy[i], cpy[j] = cpy[j], cpy[i] })
	nCols := rng.Intn(len(cpy)) + 1

	cols := cpy[:nCols]

	indexElemList := make(tree.IndexElemList, 0, len(cols))
	for i := range cols {
		semType, err := TestingDatumTypeToColumnSemanticType(coltypes.CastTargetToDatumType(cols[i].Type))
		if err != nil || MustBeValueEncoded(semType) {
			continue
		}
		indexElemList = append(indexElemList, tree.IndexElem{
			Column:    cols[i].Name,
			Direction: tree.Direction(rng.Intn(int(tree.Descending) + 1)),
		})
	}
	return tree.IndexTableDef{Columns: indexElemList}
}

func randNumColFams(rng *rand.Rand, nCols int) int {
	if rng.Intn(3) == 0 {
		return 1
	}
	return rng.Intn(nCols) + 1
}

// The following variables are useful for testing.
var (
	// IntType is the int ColumnType.
	IntType = ColumnType{SemanticType: ColumnType_INT}
	// BoolType is the bool ColumnType.
	BoolType = ColumnType{SemanticType: ColumnType_BOOL}
	// DecType is the decimal ColumnType.
	DecType = ColumnType{SemanticType: ColumnType_DECIMAL}
	// StrType is the string ColumnType.
	StrType = ColumnType{SemanticType: ColumnType_STRING}
	// OneIntCol is a slice of one IntType.
	OneIntCol = []ColumnType{IntType}
	// TwoIntCols is a slice of two IntTypes.
	TwoIntCols = []ColumnType{IntType, IntType}
	// ThreeIntCols is a slice of three IntTypes.
	ThreeIntCols = []ColumnType{IntType, IntType, IntType}
	// FourIntCols is a slice of three IntTypes.
	FourIntCols = []ColumnType{IntType, IntType, IntType, IntType}
)

// MakeIntCols makes a slice of numCols IntTypes.
func MakeIntCols(numCols int) []ColumnType {
	ret := make([]ColumnType, numCols)
	for i := 0; i < numCols; i++ {
		ret[i] = IntType
	}
	return ret
}

// IntEncDatum returns an EncDatum representation of DInt(i).
func IntEncDatum(i int) EncDatum {
	return EncDatum{Datum: tree.NewDInt(tree.DInt(i))}
}

// StrEncDatum returns an EncDatum representation of DString(s).
func StrEncDatum(s string) EncDatum {
	return EncDatum{Datum: tree.NewDString(s)}
}

// NullEncDatum returns and EncDatum representation of tree.DNull.
func NullEncDatum() EncDatum {
	return EncDatum{Datum: tree.DNull}
}

// GenEncDatumRowsInt converts rows of ints to rows of EncDatum DInts.
// If an int is negative, the corresponding value is NULL.
func GenEncDatumRowsInt(inputRows [][]int) EncDatumRows {
	rows := make(EncDatumRows, len(inputRows))
	for i, inputRow := range inputRows {
		for _, x := range inputRow {
			if x < 0 {
				rows[i] = append(rows[i], NullEncDatum())
			} else {
				rows[i] = append(rows[i], IntEncDatum(x))
			}
		}
	}
	return rows
}

// MakeIntRows constructs a numRows x numCols table where rows[i][j] = i + j.
func MakeIntRows(numRows, numCols int) EncDatumRows {
	rows := make(EncDatumRows, numRows)
	for i := range rows {
		rows[i] = make(EncDatumRow, numCols)
		for j := 0; j < numCols; j++ {
			rows[i][j] = IntEncDatum(i + j)
		}
	}
	return rows
}

// MakeRandIntRows constructs a numRows x numCols table where the values are random.
func MakeRandIntRows(rng *rand.Rand, numRows int, numCols int) EncDatumRows {
	rows := make(EncDatumRows, numRows)
	for i := range rows {
		rows[i] = make(EncDatumRow, numCols)
		for j := 0; j < numCols; j++ {
			rows[i][j] = IntEncDatum(rng.Int())
		}
	}
	return rows
}

// MakeRepeatedIntRows constructs a numRows x numCols table where blocks of n
// consecutive rows have the same value.
func MakeRepeatedIntRows(n int, numRows int, numCols int) EncDatumRows {
	rows := make(EncDatumRows, numRows)
	for i := range rows {
		rows[i] = make(EncDatumRow, numCols)
		for j := 0; j < numCols; j++ {
			rows[i][j] = IntEncDatum(i/n + j)
		}
	}
	return rows
}
