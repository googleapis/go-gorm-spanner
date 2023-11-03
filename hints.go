// Copyright 2023 Google LLC.
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
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Exprs []clause.Expression

func (exprs Exprs) Build(builder clause.Builder) {
	for idx, expr := range exprs {
		if idx > 0 {
			builder.WriteByte(' ')
		}
		expr.Build(builder)
	}
}

type IndexHint struct {
	Type string
	Key  string
}

func (indexHint IndexHint) ModifyStatement(stmt *gorm.Statement) {
	clause := stmt.Clauses["FROM"]

	if clause.AfterExpression == nil {
		clause.AfterExpression = indexHint
	} else {
		clause.AfterExpression = Exprs{clause.AfterExpression, indexHint}
	}

	stmt.Clauses["FROM"] = clause
}

func (indexHint IndexHint) Build(builder clause.Builder) {
	if indexHint.Key != "" {
		builder.WriteString("@{")
		builder.WriteString(indexHint.Type)
		builder.WriteQuoted(indexHint.Key)
		builder.WriteByte('}')
	}
}

func ForceIndex(name string) IndexHint {
	return IndexHint{Type: "FORCE_INDEX=", Key: name}
}
