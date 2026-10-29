// Copyright 2026 Tellomi Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0

// Package langpackdata embeds the language packs served by langpack.* RPCs.
package langpackdata

import "embed"

// FS is data/langpack/<platform>/<lang>.json, loaded when the server starts.
//
//go:embed ios android
var FS embed.FS
