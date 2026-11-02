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

// Command iguazu-engine is the Iguazu consensus engine.
// It loads gwat as a shared-object plugin and runs the DAG consensus loop.
//
// Usage:
//
//	iguazu-engine [--plugin=./gwat.so] -- <gwat flags...>
//
// Everything before "--" is parsed by iguazu-engine itself.
// Everything after "--" is forwarded verbatim to the gwat plugin.
package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/LFDT-Iguazu/iguazu-engine/dag"
	"github.com/LFDT-Iguazu/iguazu-engine/loader"
	"github.com/LFDT-Iguazu/iguazu-engine/token"
	"github.com/LFDT-Iguazu/iguazu-engine/validator"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
)

func main() {
	// Split os.Args at "--": everything before goes to iguazu-engine flags,
	// everything after is forwarded to the gwat plugin.
	hostArgs, gwatArgs := splitArgs(os.Args[1:])

	fs := flag.NewFlagSet("iguazu-engine", flag.ExitOnError)
	pluginPath := fs.String("plugin", "./gwat.so", "path to gwat shared-object plugin")
	if err := fs.Parse(hostArgs); err != nil {
		slog.Error("Failed to parse flags", "err", err)
		os.Exit(1)
	}

	plug, err := loader.Load(*pluginPath)
	if err != nil {
		slog.Error("Failed to load gwat plugin", "err", err)
		os.Exit(1)
	}

	cfg := &iface.NodeConfig{
		Args:    gwatArgs,
		DevMode: containsDevFlag(gwatArgs),
	}
	if err := plug.Init(cfg); err != nil {
		slog.Error("Failed to init gwat plugin", "err", err)
		os.Exit(1)
	}

	if err := plug.Start(); err != nil {
		slog.Error("Failed to start gwat plugin", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := plug.Stop(); err != nil {
			slog.Error("Error stopping gwat plugin", "err", err)
		}
	}()

	plug.SetProcessorFactories(
		token.NewFactory(plug.BlockChain()),
		validator.NewFactory(plug.ValidatorChain()),
	)

	backend := &dagBackend{plugin: plug}
	d := dag.New(backend)
	plug.SetDag(d)
	d.StartWork()
	defer d.StopWork()

	slog.Info("iguazu-engine started", "plugin", *pluginPath, "dev", cfg.DevMode)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("iguazu-engine shutting down")
}

// splitArgs splits args at the first "--" separator.
// Everything before "--" is returned as hostArgs; everything after as pluginArgs.
// If "--" is absent, all args are treated as hostArgs.
func splitArgs(args []string) (hostArgs, pluginArgs []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

// containsDevFlag reports whether "--dev" or "--dev=true" is present in args.
func containsDevFlag(args []string) bool {
	for _, a := range args {
		if a == "--dev" || a == "--dev=true" || strings.HasPrefix(a, "--dev=") && a != "--dev=false" {
			return true
		}
	}
	return false
}

// dagBackend adapts iface.GwatPlugin to the dag.Backend interface.
type dagBackend struct {
	plugin iface.GwatPlugin
}

func (b *dagBackend) BlockChain() iface.BlockChain { return b.plugin.BlockChain() }
func (b *dagBackend) Downloader() iface.Downloader { return b.plugin.Downloader() }
func (b *dagBackend) Creator() iface.BlockCreator  { return b.plugin.Creator() }
func (b *dagBackend) IsDevMode() bool              { return b.plugin.IsDevMode() }
