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

	"cloud.google.com/go/spanner"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// RunTransaction executes a transaction on Spanner using the given
// gorm database, and retries the transaction if it is aborted by Spanner.
func RunTransaction(
	ctx context.Context,
	db *gorm.DB,
	fc func(tx *gorm.DB) error,
	opts ...*sql.TxOptions) error {
	for {
		err := db.Transaction(fc, opts...)
		if err == nil {
			return nil
		}
		s, ok := status.FromError(err)
		if !ok || s.Code() != codes.Aborted {
			return err
		}
		delay, ok := spanner.ExtractRetryDelay(err)
		if ok {
			if err := gax.Sleep(ctx, delay); err != nil {
				return err
			}
		}
	}
}
