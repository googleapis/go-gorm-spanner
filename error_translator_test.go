// Copyright 2024 Google LLC
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

package gorm

import (
	"errors"
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

var testErrors = map[codes.Code]error{
	codes.AlreadyExists:   spanner.ToSpannerError(status.Errorf(codes.AlreadyExists, "already exists")),
	codes.InvalidArgument: spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "invalid argument")),
	codes.Unknown:         errors.New("random error"),
}

func TestErrorTranslator(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "AlreadyExists returns gorm.ErrDuplicatedKey",
			err:  testErrors[codes.AlreadyExists],
			want: gorm.ErrDuplicatedKey,
		},
		{
			name: "InvalidArgument returns original error",
			err:  testErrors[codes.InvalidArgument],
			want: testErrors[codes.InvalidArgument],
		},
		{
			name: "Random error returns original error",
			err:  testErrors[codes.Unknown],
			want: testErrors[codes.Unknown],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialector := Dialector{}
			got := dialector.Translate(tt.err)
			if got != tt.want {
				t.Errorf("translate error mismatch:\n Got: %v\nWant: %v", got, tt.want)
			}
		})
	}
}
