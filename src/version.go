// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package main

import (
	"fmt"
	"runtime"
)

// Version information for homer-core
var (
	// VERSION_APPLICATION is the application version
	VERSION_APPLICATION = "11.0.212"

	// BuildDate is the build date
	BuildDate = ""

	// BuildTime is the build time
	BuildTime = ""

	// GitCommit is the git commit hash
	GitCommit = ""

	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()

	// BuildOS is the operating system
	BuildOS = runtime.GOOS

	// BuildArch is the architecture
	BuildArch = runtime.GOARCH
)

// GetVersionString returns a formatted version string
func GetVersionString() string {
	s := fmt.Sprintf("homer-core %s", VERSION_APPLICATION)
	if BuildDate != "" {
		s += fmt.Sprintf("\nbuilt %s %s", BuildDate, BuildTime)
	}
	if GitCommit != "" {
		s += fmt.Sprintf(", commit %s", GitCommit)
	}
	s += fmt.Sprintf(", go %s, %s/%s", GoVersion, BuildOS, BuildArch)
	return s
}

// PrintVersion prints version information
func PrintVersion() {
	fmt.Println(GetVersionString())
}
