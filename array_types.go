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
	"database/sql/driver"
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// NullStringArray is a named type for storing string arrays in Spanner.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
// ARRAY<STRING> is by default mapped to []spanner.NullString in the Spanner
// database/sql driver. This is because Spanner always allows arrays to contain
// null elements, even if the column itself is defined as NOT NULL.
type NullStringArray []spanner.NullString

func (a NullStringArray) Value() (driver.Value, error) {
	return []spanner.NullString(a), nil
}

func (a NullStringArray) GormDataType() string {
	return "ARRAY<STRING(MAX)>"
}

func (a NullStringArray) GormDBDataType(_ *gorm.DB, field *schema.Field) string {
	if field.Size > 0 {
		return fmt.Sprintf("ARRAY<STRING(%v)>", field.Size)
	}
	return "ARRAY<STRING(MAX)>"
}

// StringArray is a named type for storing string arrays in Spanner.
// This type cannot contain any NULL elements.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	return []string(a), nil
}

func (a StringArray) Scan(_ any) error {
	// Just return nil. The conversion is done automatically by
	// the Spanner database/sql driver.
	return nil
}

func (a StringArray) GormDataType() string {
	return "ARRAY<STRING(MAX)>"
}

func (a StringArray) GormDBDataType(_ *gorm.DB, field *schema.Field) string {
	if field.Size > 0 {
		return fmt.Sprintf("ARRAY<STRING(%v)>", field.Size)
	}
	return "ARRAY<STRING(MAX)>"
}

// NullBoolArray is a named type for storing bool arrays in Spanner.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
// ARRAY<BOOL> is by default mapped to []spanner.NullBool in the Spanner
// database/sql driver. This is because Spanner always allows arrays to contain
// null elements, even if the column itself is defined as NOT NULL.
type NullBoolArray []spanner.NullBool

func (a NullBoolArray) Value() (driver.Value, error) {
	return []spanner.NullBool(a), nil
}

func (a NullBoolArray) GormDataType() string {
	return "ARRAY<BOOL>"
}

func (a NullBoolArray) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<BOOL>"
}

// BoolArray is a named type for storing bool arrays in Spanner.
// This type cannot contain any NULL elements.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
type BoolArray []bool

func (a BoolArray) Value() (driver.Value, error) {
	return []bool(a), nil
}

func (a BoolArray) Scan(_ any) error {
	// Just return nil. The conversion is done automatically by
	// the Spanner database/sql driver.
	return nil
}

func (a BoolArray) GormDataType() string {
	return "ARRAY<BOOL>"
}

func (a BoolArray) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<BOOL>"
}

// NullInt64Array is a named type for storing int64 arrays in Spanner.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
// ARRAY<INT64> is by default mapped to []spanner.NullInt64 in the Spanner
// database/sql driver. This is because Spanner always allows arrays to contain
// null elements, even if the column itself is defined as NOT NULL.
type NullInt64Array []spanner.NullInt64

func (a NullInt64Array) Value() (driver.Value, error) {
	return []spanner.NullInt64(a), nil
}

func (a NullInt64Array) GormDataType() string {
	return "ARRAY<INT64>"
}

func (a NullInt64Array) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<INT64>"
}

// Int64Array is a named type for storing int64 arrays in Spanner.
// This type cannot contain any NULL elements.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
type Int64Array []int64

func (a Int64Array) Value() (driver.Value, error) {
	return []int64(a), nil
}

func (a Int64Array) Scan(_ any) error {
	// Just return nil. The conversion is done automatically by
	// the Spanner database/sql driver.
	return nil
}

func (a Int64Array) GormDataType() string {
	return "ARRAY<INT64>"
}

func (a Int64Array) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<INT64>"
}

// NullFloat32Array is a named type for storing float32 arrays in Spanner.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
// ARRAY<FLOAT32> is by default mapped to []spanner.NullFloat32 in the Spanner
// database/sql driver. This is because Spanner always allows arrays to contain
// null elements, even if the column itself is defined as NOT NULL.
type NullFloat32Array []spanner.NullFloat32

func (a NullFloat32Array) Value() (driver.Value, error) {
	return []spanner.NullFloat32(a), nil
}

func (a NullFloat32Array) GormDataType() string {
	return "ARRAY<FLOAT32>"
}

func (a NullFloat32Array) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<FLOAT32>"
}

// Float32Array is a named type for storing float32 arrays in Spanner.
// This type cannot contain any NULL elements.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
type Float32Array []float32

func (a Float32Array) Value() (driver.Value, error) {
	return []float32(a), nil
}

func (a Float32Array) Scan(_ any) error {
	// Just return nil. The conversion is done automatically by
	// the Spanner database/sql driver.
	return nil
}

func (a Float32Array) GormDataType() string {
	return "ARRAY<FLOAT32>"
}

func (a Float32Array) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<FLOAT32>"
}

// NullFloat64Array is a named type for storing float64 arrays in Spanner.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
// ARRAY<FLOAT64> is by default mapped to []spanner.NullFloat64 in the Spanner
// database/sql driver. This is because Spanner always allows arrays to contain
// null elements, even if the column itself is defined as NOT NULL.
type NullFloat64Array []spanner.NullFloat64

func (a NullFloat64Array) Value() (driver.Value, error) {
	return []spanner.NullFloat64(a), nil
}

func (a NullFloat64Array) GormDataType() string {
	return "ARRAY<FLOAT64>"
}

func (a NullFloat64Array) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<FLOAT64>"
}

// Float64Array is a named type for storing float64 arrays in Spanner.
// This type cannot contain any NULL elements.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
type Float64Array []float64

func (a Float64Array) Value() (driver.Value, error) {
	return []float64(a), nil
}

func (a Float64Array) Scan(_ any) error {
	// Just return nil. The conversion is done automatically by
	// the Spanner database/sql driver.
	return nil
}

func (a Float64Array) GormDataType() string {
	return "ARRAY<FLOAT64>"
}

func (a Float64Array) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<FLOAT64>"
}

// NullDateArray is a named type for storing date arrays in Spanner.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
// ARRAY<DATE> is by default mapped to []spanner.NullDate in the Spanner
// database/sql driver. This is because Spanner always allows arrays to contain
// null elements, even if the column itself is defined as NOT NULL.
type NullDateArray []spanner.NullDate

func (a NullDateArray) Value() (driver.Value, error) {
	return []spanner.NullDate(a), nil
}

func (a NullDateArray) GormDataType() string {
	return "ARRAY<DATE>"
}

func (a NullDateArray) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<DATE>"
}

// DateArray is a named type for storing date arrays in Spanner.
// This type cannot contain any NULL elements.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
type DateArray []civil.Date

func (a DateArray) Value() (driver.Value, error) {
	return []civil.Date(a), nil
}

func (a DateArray) Scan(_ any) error {
	// Just return nil. The conversion is done automatically by
	// the Spanner database/sql driver.
	return nil
}

func (a DateArray) GormDataType() string {
	return "ARRAY<DATE>"
}

func (a DateArray) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<DATE>"
}

// NullTimeArray is a named type for storing timestamp arrays in Spanner.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
// ARRAY<TIMESTAMP> is by default mapped to []spanner.NullTime in the Spanner
// database/sql driver. This is because Spanner always allows arrays to contain
// null elements, even if the column itself is defined as NOT NULL.
type NullTimeArray []spanner.NullTime

func (a NullTimeArray) Value() (driver.Value, error) {
	return []spanner.NullTime(a), nil
}

func (a NullTimeArray) GormDataType() string {
	return "ARRAY<TIMESTAMP>"
}

func (a NullTimeArray) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<TIMESTAMP>"
}

// TimeArray is a named type for storing date arrays in Spanner.
// This type cannot contain any NULL elements.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
type TimeArray []time.Time

func (a TimeArray) Value() (driver.Value, error) {
	return []time.Time(a), nil
}

func (a TimeArray) Scan(_ any) error {
	// Just return nil. The conversion is done automatically by
	// the Spanner database/sql driver.
	return nil
}

func (a TimeArray) GormDataType() string {
	return "ARRAY<TIMESTAMP>"
}

func (a TimeArray) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<TIMESTAMP>"
}

// NullJSONArray is a named type for storing JSON arrays in Spanner.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
// ARRAY<JSON> is by default mapped to []spanner.NullJSON in the Spanner
// database/sql driver. This is because Spanner always allows arrays to contain
// null elements, even if the column itself is defined as NOT NULL.
type NullJSONArray []spanner.NullJSON

func (a NullJSONArray) Value() (driver.Value, error) {
	return []spanner.NullJSON(a), nil
}

func (a NullJSONArray) GormDataType() string {
	return "ARRAY<JSON>"
}

func (a NullJSONArray) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<JSON>"
}
