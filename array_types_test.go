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
	"testing"

	"github.com/googleapis/go-sql-spanner/testutil"
)

type arrayEntity struct {
	ID int64
	//BoolArray BoolArray
	// BytesArray     [][]byte
	//DateArray      DateArray
	//Int64Array     Int64Array
	//Float32Array   Float32Array
	//Float64Array   Float64Array
	StringArray StringArray
	//TimestampArray TimeArray
}

func TestInsertNativeArrays(t *testing.T) {
	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	entity := arrayEntity{
		ID: 1,
		//BoolArray: BoolArray{true, false},
		// BytesArray:     [][]byte{[]byte("bytes1"), []byte("bytes2")},
		//DateArray:      []civil.Date{{Year: 2025, Month: 2, Day: 10}, {Year: 2000, Month: 1, Day: 1}},
		//Int64Array:     []int64{1, 2},
		//Float32Array:   []float32{3.14, 6.626},
		//Float64Array:   []float64{3.14, 6.626},
		StringArray: []string{"string1", "string2"},
		//TimestampArray: []time.Time{time.UnixMilli(1739192107974), time.UnixMilli(0)},
	}
	_ = server.TestSpanner.PutStatementResult("INSERT INTO `array_entities` (`id`, foo) VALUES (@p1) THEN RETURN `id`", &testutil.StatementResult{
		Type:        testutil.StatementResultUpdateCount,
		UpdateCount: 1,
	})
	if err := db.Create(&entity).Error; err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}
	req := getLastSqlRequest(server)
	if g, w := len(req.Params.Fields), 3; g != w {
		t.Errorf("param value mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := len(req.Params.Fields), 3; g != w {
		t.Errorf("param value mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := req.Params.Fields["p3"].GetNumberValue(), float64(float32(3.14)); g != w {
		t.Errorf("rating value mismatch\n Got: %v\nWant: %v", g, w)
	}
}
