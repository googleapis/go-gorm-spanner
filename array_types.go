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

//goland:noinspection GoMixedReceiverTypes
func (a StringArray) Value() (driver.Value, error) {
	return []string(a), nil
}

//goland:noinspection GoMixedReceiverTypes
func (a *StringArray) Scan(v any) error {
	if val, ok := v.([]string); ok {
		*a = val
		return nil
	}
	if val, ok := v.([]spanner.NullString); ok {
		*a = make([]string, len(val))
		for i, b := range val {
			if !b.Valid {
				return fmt.Errorf("index %d of string array contains a null value", i)
			}
			(*a)[i] = b.StringVal
		}
		return nil
	}
	return fmt.Errorf("invalid value for StringArray: %v", v)
}

//goland:noinspection GoMixedReceiverTypes
func (a StringArray) GormDataType() string {
	return "ARRAY<STRING(MAX)>"
}

//goland:noinspection GoMixedReceiverTypes
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

//goland:noinspection GoMixedReceiverTypes
func (a BoolArray) Value() (driver.Value, error) {
	return []bool(a), nil
}

//goland:noinspection GoMixedReceiverTypes
func (a *BoolArray) Scan(v any) error {
	if val, ok := v.([]bool); ok {
		*a = val
		return nil
	}
	if val, ok := v.([]spanner.NullBool); ok {
		*a = make([]bool, len(val))
		for i, b := range val {
			if !b.Valid {
				return fmt.Errorf("index %d of bool array contains a null value", i)
			}
			(*a)[i] = b.Bool
		}
		return nil
	}
	return fmt.Errorf("invalid value for BoolArray: %v", v)
}

//goland:noinspection GoMixedReceiverTypes
func (a BoolArray) GormDataType() string {
	return "ARRAY<BOOL>"
}

//goland:noinspection GoMixedReceiverTypes
func (a BoolArray) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<BOOL>"
}

// BytesArray is a named type for storing bytes arrays in Spanner.
// We must use a named type for this to implement the driver.Valuer interface.
// This is required, because gorm otherwise translates arrays/slices to
// literals in the form `(item1, item2, ..., itemN)`.
type BytesArray [][]byte

//goland:noinspection GoMixedReceiverTypes
func (a BytesArray) Value() (driver.Value, error) {
	return [][]byte(a), nil
}

//goland:noinspection GoMixedReceiverTypes
func (a *BytesArray) Scan(v any) error {
	if val, ok := v.([][]byte); ok {
		*a = val
		return nil
	}
	return fmt.Errorf("invalid value for BytesArray: %v", v)
}

//goland:noinspection GoMixedReceiverTypes
func (a BytesArray) GormDataType() string {
	return "ARRAY<BYTES>"
}

//goland:noinspection GoMixedReceiverTypes
func (a BytesArray) GormDBDataType(_ *gorm.DB, _ *schema.Field) string {
	return "ARRAY<BYTES>"
}

// NullBytesArray is a synonym for BytesArray. It is only defined for consistency
// with the other array data types.
type NullBytesArray BytesArray

//goland:noinspection GoMixedReceiverTypes
func (a NullBytesArray) Value() (driver.Value, error) {
	return BytesArray(a).Value()
}

//goland:noinspection GoMixedReceiverTypes
func (a *NullBytesArray) Scan(v any) error {
	return (*BytesArray)(a).Scan(v)
}

//goland:noinspection GoMixedReceiverTypes
func (a NullBytesArray) GormDataType() string {
	return BytesArray(a).GormDataType()
}

//goland:noinspection GoMixedReceiverTypes
func (a NullBytesArray) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	return BytesArray(a).GormDBDataType(db, field)
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

//goland:noinspection GoMixedReceiverTypes
func (a Int64Array) Value() (driver.Value, error) {
	return []int64(a), nil
}

//goland:noinspection GoMixedReceiverTypes
func (a *Int64Array) Scan(v any) error {
	if val, ok := v.([]int64); ok {
		*a = val
		return nil
	}
	if val, ok := v.([]spanner.NullInt64); ok {
		*a = make([]int64, len(val))
		for i, b := range val {
			if !b.Valid {
				return fmt.Errorf("index %d of int64 array contains a null value", i)
			}
			(*a)[i] = b.Int64
		}
		return nil
	}
	return fmt.Errorf("invalid value for Int64Array: %v", v)
}

//goland:noinspection GoMixedReceiverTypes
func (a Int64Array) GormDataType() string {
	return "ARRAY<INT64>"
}

//goland:noinspection GoMixedReceiverTypes
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

//goland:noinspection GoMixedReceiverTypes
func (a Float32Array) Value() (driver.Value, error) {
	return []float32(a), nil
}

//goland:noinspection GoMixedReceiverTypes
func (a *Float32Array) Scan(v any) error {
	if val, ok := v.([]float32); ok {
		*a = val
		return nil
	}
	if val, ok := v.([]spanner.NullFloat32); ok {
		*a = make([]float32, len(val))
		for i, b := range val {
			if !b.Valid {
				return fmt.Errorf("index %d of float32 array contains a null value", i)
			}
			(*a)[i] = b.Float32
		}
		return nil
	}
	return fmt.Errorf("invalid value for Float32Array: %v", v)
}

//goland:noinspection GoMixedReceiverTypes
func (a Float32Array) GormDataType() string {
	return "ARRAY<FLOAT32>"
}

//goland:noinspection GoMixedReceiverTypes
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

//goland:noinspection GoMixedReceiverTypes
func (a Float64Array) Value() (driver.Value, error) {
	return []float64(a), nil
}

//goland:noinspection GoMixedReceiverTypes
func (a *Float64Array) Scan(v any) error {
	if val, ok := v.([]float64); ok {
		*a = val
		return nil
	}
	if val, ok := v.([]spanner.NullFloat64); ok {
		*a = make([]float64, len(val))
		for i, b := range val {
			if !b.Valid {
				return fmt.Errorf("index %d of float64 array contains a null value", i)
			}
			(*a)[i] = b.Float64
		}
		return nil
	}
	return fmt.Errorf("invalid value for FLoat64Array: %v", v)
}

//goland:noinspection GoMixedReceiverTypes
func (a Float64Array) GormDataType() string {
	return "ARRAY<FLOAT64>"
}

//goland:noinspection GoMixedReceiverTypes
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

//goland:noinspection GoMixedReceiverTypes
func (a DateArray) Value() (driver.Value, error) {
	return []civil.Date(a), nil
}

//goland:noinspection GoMixedReceiverTypes
func (a *DateArray) Scan(v any) error {
	if val, ok := v.([]civil.Date); ok {
		*a = val
		return nil
	}
	if val, ok := v.([]spanner.NullDate); ok {
		*a = make([]civil.Date, len(val))
		for i, b := range val {
			if !b.Valid {
				return fmt.Errorf("index %d of date array contains a null value", i)
			}
			(*a)[i] = b.Date
		}
		return nil
	}
	return fmt.Errorf("invalid value for DateArray: %v", v)
}

//goland:noinspection GoMixedReceiverTypes
func (a DateArray) GormDataType() string {
	return "ARRAY<DATE>"
}

//goland:noinspection GoMixedReceiverTypes
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

//goland:noinspection GoMixedReceiverTypes
func (a TimeArray) Value() (driver.Value, error) {
	return []time.Time(a), nil
}

//goland:noinspection GoMixedReceiverTypes
func (a *TimeArray) Scan(v any) error {
	if val, ok := v.([]time.Time); ok {
		*a = val
		return nil
	}
	if val, ok := v.([]spanner.NullTime); ok {
		*a = make([]time.Time, len(val))
		for i, b := range val {
			if !b.Valid {
				return fmt.Errorf("index %d of time array contains a null value", i)
			}
			(*a)[i] = b.Time
		}
		return nil
	}
	return fmt.Errorf("invalid value for TimeArray: %v", v)
}

//goland:noinspection GoMixedReceiverTypes
func (a TimeArray) GormDataType() string {
	return "ARRAY<TIMESTAMP>"
}

//goland:noinspection GoMixedReceiverTypes
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
