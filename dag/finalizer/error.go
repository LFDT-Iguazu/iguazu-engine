// Copyright 2026 Digital Clever Solution LLC
// SPDX-License-Identifier: Apache-2.0
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

package finalizer

import "errors"

var (
	ErrBusy                 = errors.New("finalizer is busy")
	ErrSpineNotFound        = errors.New("spine not found")
	ErrBadParams            = errors.New("bad params")
	ErrEmptyFinSeq          = errors.New("empty finalization sequence")
	ErrFinNrrUsed           = errors.New("fin nr already used")
	ErrInsertUncompletedDag = errors.New("insert uncompleted dag chain")
)
