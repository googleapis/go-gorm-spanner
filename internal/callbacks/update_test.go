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
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestBeforeUpdate_NilAndEmptyGuards(t *testing.T) {
	t.Parallel()

	BeforeUpdate(nil)
	BeforeUpdate(&gorm.DB{})
	BeforeUpdate(&gorm.DB{Statement: &gorm.Statement{}})
	BeforeUpdate(&gorm.DB{Statement: &gorm.Statement{Schema: &schema.Schema{}}})

	stmt := &gorm.Statement{
		Omits:  []string{"role"},
		Schema: &schema.Schema{PrimaryFieldDBNames: []string{}},
	}
	BeforeUpdate(&gorm.DB{Statement: stmt})
	if g, w := stmt.Omits, []string{"role"}; !slices.Equal(g, w) {
		t.Errorf("Omits mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_PreservesOmitsAndDeduplicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		omits []string
		pks   []string
		want  []string
	}{
		{
			name:  "empty omits with single pk",
			omits: nil,
			pks:   []string{"id"},
			want:  []string{"id"},
		},
		{
			name:  "existing omits with composite pk",
			omits: []string{"role", "is_admin"},
			pks:   []string{"tenant_id", "id"},
			want:  []string{"role", "is_admin", "tenant_id", "id"},
		},
		{
			name:  "existing omits already contain pk",
			omits: []string{"id", "role"},
			pks:   []string{"id"},
			want:  []string{"id", "role"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stmt := &gorm.Statement{
				Omits: tc.omits,
				Schema: &schema.Schema{
					PrimaryFieldDBNames: tc.pks,
				},
			}
			tx := &gorm.DB{Statement: stmt}
			BeforeUpdate(tx)
			BeforeUpdate(tx)

			if g, w := stmt.Omits, tc.want; !slices.Equal(g, w) {
				t.Errorf("Omits mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestBeforeUpdate_DoesNotMutateParentBackingArray(t *testing.T) {
	t.Parallel()

	parentOmits := make([]string, 1, 4)
	parentOmits[0] = "role"
	stmt := &gorm.Statement{
		Omits: parentOmits,
		Schema: &schema.Schema{
			PrimaryFieldDBNames: []string{"id"},
		},
	}
	BeforeUpdate(&gorm.DB{Statement: stmt})

	if g, w := stmt.Omits, []string{"role", "id"}; !slices.Equal(g, w) {
		t.Errorf("Omits mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := parentOmits[:2], []string{"role", ""}; !slices.Equal(g, w) {
		t.Errorf("parent backing array mutated\nGot:  %v\nWant: %v", g, w)
	}
}
