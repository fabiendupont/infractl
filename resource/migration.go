// Copyright 2025 The infractl Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resource

import (
	"fmt"

	"gorm.io/gorm"
)

// AutoMigrate runs GORM's AutoMigrate for one or more resource model types.
// Each model must be a pointer to a struct that maps to a database table
// (typically embedding resource.Resource).
//
// Example:
//
//	resource.AutoMigrate(db, &VirtualMachine{}, &Network{})
func AutoMigrate(db *gorm.DB, models ...interface{}) error {
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("auto-migrating resource models: %w", err)
	}
	return nil
}
