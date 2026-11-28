// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package callbacks

import (
	"slices"

	"gorm.io/gorm"
)

// BeforeUpdate preserves any existing Statement.Omits entries and appends
// all primary key column names so that primary key fields are never included
// in the SET clause of an UPDATE statement.
func BeforeUpdate(db *gorm.DB) {
	if db == nil || db.Statement == nil || db.Statement.Schema == nil || len(db.Statement.Schema.PrimaryFieldDBNames) == 0 {
		return
	}
	omits := make([]string, len(db.Statement.Omits), len(db.Statement.Omits)+len(db.Statement.Schema.PrimaryFieldDBNames))
	copy(omits, db.Statement.Omits)
	for _, pk := range db.Statement.Schema.PrimaryFieldDBNames {
		if !slices.Contains(omits, pk) {
			omits = append(omits, pk)
		}
	}
	db.Statement.Omits = omits
}
