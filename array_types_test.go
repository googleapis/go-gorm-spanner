// Copyright 2025 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gorm

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/googleapis/go-sql-spanner/testutil"
)

type arrayEntity struct {
	ID             int64
	BoolArray      BoolArray
	BytesArray     BytesArray
	DateArray      DateArray
	Int64Array     Int64Array
	Float32Array   Float32Array
	Float64Array   Float64Array
	StringArray    StringArray
	TimestampArray TimeArray
}

type nullArrayEntity struct {
	ID             int64
	BoolArray      NullBoolArray
	BytesArray     NullBytesArray
	DateArray      NullDateArray
	Int64Array     NullInt64Array
	Float32Array   NullFloat32Array
	Float64Array   NullFloat64Array
	StringArray    NullStringArray
	TimestampArray NullTimeArray
}

func TestInsertNativeArrays(t *testing.T) {
	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	entity := arrayEntity{
		ID:             1,
		BoolArray:      BoolArray{true, false},
		BytesArray:     [][]byte{[]byte("bytes1"), []byte("bytes2")},
		DateArray:      []civil.Date{{Year: 2025, Month: 2, Day: 10}, {Year: 2000, Month: 1, Day: 1}},
		Int64Array:     []int64{1, 2},
		Float32Array:   []float32{3.14, 6.626},
		Float64Array:   []float64{3.14, 6.626},
		StringArray:    []string{"string1", "string2"},
		TimestampArray: []time.Time{time.UnixMilli(1739192107974), time.UnixMilli(0)},
	}
	putInsertStatementResult(server.TestSpanner, "INSERT INTO `array_entities` (`bool_array`,`bytes_array`,`date_array`,`int64_array`,`float32_array`,`float64_array`,`string_array`,`timestamp_array`,`id`) VALUES (@p1,@p2,@p3,@p4,@p5,@p6,@p7,@p8,@p9) THEN RETURN `id`")
	if err := db.Create(&entity).Error; err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}
	req := getLastSqlRequest(server)
	if g, w := len(req.Params.Fields), 9; g != w {
		t.Errorf("num params mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := len(req.ParamTypes), 9; g != w {
		t.Errorf("num param types mismatch\n Got: %v\nWant: %v", g, w)
	}
	wantParamTypes := []spannerpb.TypeCode{
		spannerpb.TypeCode_BOOL,
		spannerpb.TypeCode_BYTES,
		spannerpb.TypeCode_DATE,
		spannerpb.TypeCode_INT64,
		spannerpb.TypeCode_FLOAT32,
		spannerpb.TypeCode_FLOAT64,
		spannerpb.TypeCode_STRING,
		spannerpb.TypeCode_TIMESTAMP,
	}
	for i, code := range wantParamTypes {
		param := fmt.Sprintf("p%d", i+1)
		if g, w := req.ParamTypes[param].ArrayElementType.Code, code; g != w {
			t.Errorf("%s: param type mismatch\n Got: %v\nWant: %v", param, g, w)
		}
	}
	wantValues := []string{
		"list_value:{values:{bool_value:true} values:{bool_value:false}}",
		"list_value:{values:{string_value:\"Ynl0ZXMx\"} values:{string_value:\"Ynl0ZXMy\"}}",
		"list_value:{values:{string_value:\"2025-02-10\"} values:{string_value:\"2000-01-01\"}}",
		"list_value:{values:{string_value:\"1\"} values:{string_value:\"2\"}}",
		"list_value:{values:{number_value:3.140000104904175} values:{number_value:6.625999927520752}}",
		"list_value:{values:{number_value:3.14} values:{number_value:6.626}}",
		"list_value:{values:{string_value:\"string1\"} values:{string_value:\"string2\"}}",
		"list_value:{values:{string_value:\"2025-02-10T12:55:07.974Z\"} values:{string_value:\"1970-01-01T00:00:00Z\"}}",
		"string_value:\"1\"",
	}
	for i, val := range wantValues {
		param := fmt.Sprintf("p%d", i+1)
		if g, w := strings.ReplaceAll(req.Params.Fields[param].String(), "  ", " "), val; g != w {
			t.Errorf("%s: param value mismatch\n Got: %v\nWant: %v", param, g, w)
		}
	}
}

func TestSelectNativeArrays(t *testing.T) {
	for _, decodeToNativeArrays := range []bool{true, false} {
		db, server, teardown := setupTestGormConnectionWithParams(t, fmt.Sprintf("decodetonativearrays=%v", decodeToNativeArrays))
		//goland:noinspection GoDeferInLoop
		defer teardown()

		_ = server.TestSpanner.PutStatementResult("SELECT * FROM `array_entities` ORDER BY `array_entities`.`id` LIMIT @p1",
			&testutil.StatementResult{
				Type: testutil.StatementResultResultSet,
				ResultSet: &spannerpb.ResultSet{
					Metadata: &spannerpb.ResultSetMetadata{
						RowType: &spannerpb.StructType{
							Fields: []*spannerpb.StructType_Field{
								{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "id"},
								{Type: arrayType(spannerpb.TypeCode_BOOL), Name: "bool_array"},
								{Type: arrayType(spannerpb.TypeCode_BYTES), Name: "bytes_array"},
								{Type: arrayType(spannerpb.TypeCode_DATE), Name: "date_array"},
								{Type: arrayType(spannerpb.TypeCode_INT64), Name: "int64_array"},
								{Type: arrayType(spannerpb.TypeCode_FLOAT32), Name: "float32_array"},
								{Type: arrayType(spannerpb.TypeCode_FLOAT64), Name: "float64_array"},
								{Type: arrayType(spannerpb.TypeCode_STRING), Name: "string_array"},
								{Type: arrayType(spannerpb.TypeCode_TIMESTAMP), Name: "timestamp_array"},
							},
						},
					},
					Rows: []*structpb.ListValue{
						{Values: []*structpb.Value{
							{Kind: &structpb.Value_StringValue{StringValue: "1"}},
							{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_BoolValue{BoolValue: true}}, {Kind: &structpb.Value_BoolValue{BoolValue: false}}}}}},
							{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: base64.StdEncoding.EncodeToString([]byte("bytes1"))}}, {Kind: &structpb.Value_StringValue{StringValue: base64.StdEncoding.EncodeToString([]byte("bytes2"))}}}}}},
							{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: "2025-02-10"}}, {Kind: &structpb.Value_StringValue{StringValue: "2000-01-01"}}}}}},
							{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: "1"}}, {Kind: &structpb.Value_StringValue{StringValue: "2"}}}}}},
							{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: float64(float32(3.14))}}, {Kind: &structpb.Value_NumberValue{NumberValue: float64(float32(6.626))}}}}}},
							{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 3.14}}, {Kind: &structpb.Value_NumberValue{NumberValue: 6.626}}}}}},
							{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: "string1"}}, {Kind: &structpb.Value_StringValue{StringValue: "string2"}}}}}},
							{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: "2025-02-10T14:37:11Z"}}, {Kind: &structpb.Value_StringValue{StringValue: "2025-02-10T14:37:11Z"}}}}}},
						}},
					},
				},
			})
		var entity arrayEntity
		if err := db.First(&entity).Error; err != nil {
			t.Fatalf("failed to fetch entity: %v", err)
		}
		if g, w := entity.ID, int64(1); g != w {
			t.Errorf("ID mismatch\n Got: %v\nWant: %v", g, w)
		}
		if g, w := []bool(entity.BoolArray), []bool{true, false}; !reflect.DeepEqual(g, w) {
			t.Errorf("bool array mismatch\n Got: %v\nWant: %v", g, w)
		}
		if g, w := [][]byte(entity.BytesArray), [][]byte{[]byte("bytes1"), []byte("bytes2")}; !reflect.DeepEqual(g, w) {
			t.Errorf("bytes array mismatch\n Got: %v\nWant: %v", g, w)
		}
		if g, w := []civil.Date(entity.DateArray), []civil.Date{{Year: 2025, Month: 2, Day: 10}, {Year: 2000, Month: 1, Day: 1}}; !reflect.DeepEqual(g, w) {
			t.Errorf("date array mismatch\n Got: %v\nWant: %v", g, w)
		}
		if g, w := []int64(entity.Int64Array), []int64{1, 2}; !reflect.DeepEqual(g, w) {
			t.Errorf("int64 array mismatch\n Got: %v\nWant: %v", g, w)
		}
		if g, w := []float32(entity.Float32Array), []float32{3.14, 6.626}; !reflect.DeepEqual(g, w) {
			t.Errorf("float32 array mismatch\n Got: %v\nWant: %v", g, w)
		}
		if g, w := []float64(entity.Float64Array), []float64{3.14, 6.626}; !reflect.DeepEqual(g, w) {
			t.Errorf("float64 array mismatch\n Got: %v\nWant: %v", g, w)
		}
		if g, w := []string(entity.StringArray), []string{"string1", "string2"}; !reflect.DeepEqual(g, w) {
			t.Errorf("string array mismatch\n Got: %v\nWant: %v", g, w)
		}
		if g, w := []time.Time(entity.TimestampArray), []time.Time{time.UnixMilli(1739198231000).UTC(), time.UnixMilli(1739198231000).UTC()}; !reflect.DeepEqual(g, w) {
			t.Errorf("timestamp array mismatch\n Got: %v\nWant: %v", g, w)
		}
	}
}

func TestInsertNullArrays(t *testing.T) {
	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	entity := nullArrayEntity{
		ID:             1,
		BoolArray:      NullBoolArray{{Bool: true, Valid: true}, {}, {Bool: false, Valid: true}},
		BytesArray:     NullBytesArray{[]byte("bytes1"), nil, []byte("bytes2")},
		DateArray:      NullDateArray{{Date: civil.Date{Year: 2025, Month: 2, Day: 10}, Valid: true}, {}, {Date: civil.Date{Year: 2000, Month: 1, Day: 1}, Valid: true}},
		Int64Array:     NullInt64Array{{Int64: 1, Valid: true}, {}, {Int64: 2, Valid: true}},
		Float32Array:   NullFloat32Array{{Float32: 3.14, Valid: true}, {}, {Float32: 6.626, Valid: true}},
		Float64Array:   NullFloat64Array{{Float64: 3.14, Valid: true}, {}, {Float64: 6.626, Valid: true}},
		StringArray:    NullStringArray{{StringVal: "string1", Valid: true}, {}, {StringVal: "string2", Valid: true}},
		TimestampArray: NullTimeArray{{Time: time.UnixMilli(1739192107974), Valid: true}, {}, {Time: time.UnixMilli(0), Valid: true}},
	}
	putInsertStatementResult(server.TestSpanner, "INSERT INTO `null_array_entities` (`bool_array`,`bytes_array`,`date_array`,`int64_array`,`float32_array`,`float64_array`,`string_array`,`timestamp_array`,`id`) VALUES (@p1,@p2,@p3,@p4,@p5,@p6,@p7,@p8,@p9) THEN RETURN `id`")
	if err := db.Create(&entity).Error; err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}
	req := getLastSqlRequest(server)
	if g, w := len(req.Params.Fields), 9; g != w {
		t.Errorf("num params mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := len(req.ParamTypes), 9; g != w {
		t.Errorf("num param types mismatch\n Got: %v\nWant: %v", g, w)
	}
	wantParamTypes := []spannerpb.TypeCode{
		spannerpb.TypeCode_BOOL,
		spannerpb.TypeCode_BYTES,
		spannerpb.TypeCode_DATE,
		spannerpb.TypeCode_INT64,
		spannerpb.TypeCode_FLOAT32,
		spannerpb.TypeCode_FLOAT64,
		spannerpb.TypeCode_STRING,
		spannerpb.TypeCode_TIMESTAMP,
	}
	for i, code := range wantParamTypes {
		param := fmt.Sprintf("p%d", i+1)
		if g, w := req.ParamTypes[param].ArrayElementType.Code, code; g != w {
			t.Errorf("%s: param type mismatch\n Got: %v\nWant: %v", param, g, w)
		}
	}
	wantValues := []string{
		"list_value:{values:{bool_value:true} values:{null_value:NULL_VALUE} values:{bool_value:false}}",
		"list_value:{values:{string_value:\"Ynl0ZXMx\"} values:{null_value:NULL_VALUE} values:{string_value:\"Ynl0ZXMy\"}}",
		"list_value:{values:{string_value:\"2025-02-10\"} values:{null_value:NULL_VALUE} values:{string_value:\"2000-01-01\"}}",
		"list_value:{values:{string_value:\"1\"} values:{null_value:NULL_VALUE} values:{string_value:\"2\"}}",
		"list_value:{values:{number_value:3.140000104904175} values:{null_value:NULL_VALUE} values:{number_value:6.625999927520752}}",
		"list_value:{values:{number_value:3.14} values:{null_value:NULL_VALUE} values:{number_value:6.626}}",
		"list_value:{values:{string_value:\"string1\"} values:{null_value:NULL_VALUE} values:{string_value:\"string2\"}}",
		"list_value:{values:{string_value:\"2025-02-10T12:55:07.974Z\"} values:{null_value:NULL_VALUE} values:{string_value:\"1970-01-01T00:00:00Z\"}}",
		"string_value:\"1\"",
	}
	for i, val := range wantValues {
		param := fmt.Sprintf("p%d", i+1)
		if g, w := strings.ReplaceAll(req.Params.Fields[param].String(), "  ", " "), val; g != w {
			t.Errorf("%s: param value mismatch\n Got: %v\nWant: %v", param, g, w)
		}
	}
}

func TestSelectNullArrays(t *testing.T) {
	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	_ = server.TestSpanner.PutStatementResult("SELECT * FROM `null_array_entities` ORDER BY `null_array_entities`.`id` LIMIT @p1",
		&testutil.StatementResult{
			Type: testutil.StatementResultResultSet,
			ResultSet: &spannerpb.ResultSet{
				Metadata: &spannerpb.ResultSetMetadata{
					RowType: &spannerpb.StructType{
						Fields: []*spannerpb.StructType_Field{
							{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "id"},
							{Type: arrayType(spannerpb.TypeCode_BOOL), Name: "bool_array"},
							{Type: arrayType(spannerpb.TypeCode_BYTES), Name: "bytes_array"},
							{Type: arrayType(spannerpb.TypeCode_DATE), Name: "date_array"},
							{Type: arrayType(spannerpb.TypeCode_INT64), Name: "int64_array"},
							{Type: arrayType(spannerpb.TypeCode_FLOAT32), Name: "float32_array"},
							{Type: arrayType(spannerpb.TypeCode_FLOAT64), Name: "float64_array"},
							{Type: arrayType(spannerpb.TypeCode_STRING), Name: "string_array"},
							{Type: arrayType(spannerpb.TypeCode_TIMESTAMP), Name: "timestamp_array"},
						},
					},
				},
				Rows: []*structpb.ListValue{
					{Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "1"}},
						{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_BoolValue{BoolValue: true}}, {Kind: &structpb.Value_NullValue{}}, {Kind: &structpb.Value_BoolValue{BoolValue: false}}}}}},
						{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: base64.StdEncoding.EncodeToString([]byte("bytes1"))}}, {Kind: &structpb.Value_NullValue{}}, {Kind: &structpb.Value_StringValue{StringValue: base64.StdEncoding.EncodeToString([]byte("bytes2"))}}}}}},
						{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: "2025-02-10"}}, {Kind: &structpb.Value_NullValue{}}, {Kind: &structpb.Value_StringValue{StringValue: "2000-01-01"}}}}}},
						{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: "1"}}, {Kind: &structpb.Value_NullValue{}}, {Kind: &structpb.Value_StringValue{StringValue: "2"}}}}}},
						{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: float64(float32(3.14))}}, {Kind: &structpb.Value_NullValue{}}, {Kind: &structpb.Value_NumberValue{NumberValue: float64(float32(6.626))}}}}}},
						{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 3.14}}, {Kind: &structpb.Value_NullValue{}}, {Kind: &structpb.Value_NumberValue{NumberValue: 6.626}}}}}},
						{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: "string1"}}, {Kind: &structpb.Value_NullValue{}}, {Kind: &structpb.Value_StringValue{StringValue: "string2"}}}}}},
						{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: "2025-02-10T12:55:07.974Z"}}, {Kind: &structpb.Value_NullValue{}}, {Kind: &structpb.Value_StringValue{StringValue: "1970-01-01T00:00:00Z"}}}}}},
					}},
				},
			},
		})
	var entity nullArrayEntity
	if err := db.First(&entity).Error; err != nil {
		t.Fatalf("failed to fetch entity: %v", err)
	}
	if g, w := entity.ID, int64(1); g != w {
		t.Errorf("ID mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := []spanner.NullBool(entity.BoolArray), []spanner.NullBool(NullBoolArray{{Bool: true, Valid: true}, {}, {Bool: false, Valid: true}}); !reflect.DeepEqual(g, w) {
		t.Errorf("bool array mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := [][]byte(entity.BytesArray), [][]byte(NullBytesArray{[]byte("bytes1"), nil, []byte("bytes2")}); !reflect.DeepEqual(g, w) {
		t.Errorf("bytes array mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := []spanner.NullDate(entity.DateArray), []spanner.NullDate(NullDateArray{{Date: civil.Date{Year: 2025, Month: 2, Day: 10}, Valid: true}, {}, {Date: civil.Date{Year: 2000, Month: 1, Day: 1}, Valid: true}}); !reflect.DeepEqual(g, w) {
		t.Errorf("date array mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := []spanner.NullInt64(entity.Int64Array), []spanner.NullInt64(NullInt64Array{{Int64: 1, Valid: true}, {}, {Int64: 2, Valid: true}}); !reflect.DeepEqual(g, w) {
		t.Errorf("int64 array mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := []spanner.NullFloat32(entity.Float32Array), []spanner.NullFloat32(NullFloat32Array{{Float32: 3.14, Valid: true}, {}, {Float32: 6.626, Valid: true}}); !reflect.DeepEqual(g, w) {
		t.Errorf("float32 array mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := []spanner.NullFloat64(entity.Float64Array), []spanner.NullFloat64(NullFloat64Array{{Float64: 3.14, Valid: true}, {}, {Float64: 6.626, Valid: true}}); !reflect.DeepEqual(g, w) {
		t.Errorf("float64 array mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := []spanner.NullString(entity.StringArray), []spanner.NullString(NullStringArray{{StringVal: "string1", Valid: true}, {}, {StringVal: "string2", Valid: true}}); !reflect.DeepEqual(g, w) {
		t.Errorf("string array mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := []spanner.NullTime(entity.TimestampArray), []spanner.NullTime(NullTimeArray{{Time: time.UnixMilli(1739192107974).UTC(), Valid: true}, {}, {Time: time.UnixMilli(0).UTC(), Valid: true}}); !reflect.DeepEqual(g, w) {
		t.Errorf("timestamp array mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func arrayType(c spannerpb.TypeCode) *spannerpb.Type {
	return &spannerpb.Type{Code: spannerpb.TypeCode_ARRAY, ArrayElementType: &spannerpb.Type{Code: c}}
}

func putInsertStatementResult(spanner testutil.InMemSpannerServer, query string) {
	_ = spanner.PutStatementResult(query,
		&testutil.StatementResult{
			Type: testutil.StatementResultResultSet,
			ResultSet: &spannerpb.ResultSet{
				Metadata: &spannerpb.ResultSetMetadata{
					RowType: &spannerpb.StructType{
						Fields: []*spannerpb.StructType_Field{
							{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "ID"},
						},
					},
				},
				Rows: []*structpb.ListValue{
					{Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "1"}},
					}},
				},
				Stats: &spannerpb.ResultSetStats{RowCount: &spannerpb.ResultSetStats_RowCountExact{RowCountExact: 1}},
			},
		})
}
