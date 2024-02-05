// Copyright 2024 Google LLC.
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
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/go-sql-spanner/testutil"
	"google.golang.org/protobuf/types/known/structpb"
	"strconv"
	"testing"
	"time"
)

type singerWithCommitTimestamp struct {
	ID          int64
	FirstName   string
	LastName    string
	LastUpdated CommitTimestamp
}

func (singerWithCommitTimestamp) TableName() string {
	return "singers"
}

func TestCommitTimestamp(t *testing.T) {
	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	s := singerWithCommitTimestamp{
		FirstName: "First",
		LastName:  "Last",
	}
	_ = putSingerResult(server, "INSERT INTO `singers` (`first_name`,`last_name`,`last_updated`) VALUES (@p1,@p2,PENDING_COMMIT_TIMESTAMP()) THEN RETURN *", s)
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("failed to create singer: %v", err)
	}
	cs, _ := time.Parse(time.RFC3339, "2024-02-05T17:17:00Z")
	if g, w := s.LastUpdated.Timestamp.Time, cs; g != w {
		t.Fatalf("commit timestamp mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func putSingerResult(server *testutil.MockedSpannerInMemTestServer, sql string, s singerWithCommitTimestamp) error {
	return server.TestSpanner.PutStatementResult(sql, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "ID"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "first_name"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "last_name"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_TIMESTAMP}, Name: "last_updated"},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: strconv.Itoa(int(s.ID))}},
					{Kind: &structpb.Value_StringValue{StringValue: s.FirstName}},
					{Kind: &structpb.Value_StringValue{StringValue: s.LastName}},
					{Kind: &structpb.Value_StringValue{StringValue: "2024-02-05T17:17:00Z"}},
				}},
			},
		},
	})
}
