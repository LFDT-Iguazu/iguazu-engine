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

//go:build !windows

// Package loader opens a gwat shared-object plugin and returns its GwatPlugin
// entry point. Only supported on Linux and macOS (plugin package restriction).
package loader

import (
	"fmt"
	"plugin"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
)

// PluginSymbol is the exported variable name that gwat.so must expose.
const PluginSymbol = "GwatPlugin"

// Load opens the shared object at path, looks up the GwatPlugin symbol, and
// returns it. The returned interface is ready to be initialised with Init.
func Load(path string) (iface.GwatPlugin, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("loader: open %s: %w", path, err)
	}
	sym, err := p.Lookup(PluginSymbol)
	if err != nil {
		return nil, fmt.Errorf("loader: lookup %s in %s: %w", PluginSymbol, path, err)
	}
	plug, ok := sym.(*iface.GwatPlugin)
	if !ok {
		return nil, fmt.Errorf("loader: symbol %s has type %T, want *iface.GwatPlugin", PluginSymbol, sym)
	}
	return *plug, nil
}
