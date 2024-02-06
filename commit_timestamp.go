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
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CommitTimestamp can be used for columns that should write the PENDING_COMMIT_TIMESTAMP().
// Use it as the type for a field in a model. The corresponding database column must be of
// type TIMESTAMP, and the option `allow_commit_timestamp=true` must have been set.
// The Spanner gorm migrator will automatically create a TIMESTAMP column with the
// `allow_commit_timestamp=true` option enabled for any field that has type CommitTimestamp.
//
// Note that the commit timestamp is not returned directly after inserting/updating a row.
// Instead, the value can only be read after the transaction has been committed.
//
// Example:
//
//	type Singer struct {
//	  ID          int64
//	  Name        string
//	  LastUpdated CommitTimestamp
//	}
type CommitTimestamp struct {
	Timestamp sql.NullTime
}

// GormDataType implements gorm.GormDataTypeInterface.
func (ct CommitTimestamp) GormDataType() string {
	return "TIMESTAMP OPTIONS (allow_commit_timestamp=true)"
}

// GormValue implements the gorm.Valuer interface.
func (ct CommitTimestamp) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	return clause.Expr{
		SQL: "PENDING_COMMIT_TIMESTAMP()",
	}
}

// Scan implements the sql.Scanner interface
func (ct *CommitTimestamp) Scan(v interface{}) error {
	switch v.(type) {
	default:
		return fmt.Errorf("invalid type for a commit timestamp column: %v", v)
	case nil:
		ct.Timestamp = sql.NullTime{}
	case time.Time:
		ct.Timestamp = sql.NullTime{Valid: true, Time: v.(time.Time)}
	}
	return nil
}
