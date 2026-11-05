// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build linux

package shark

import (
	"os"
	"os/exec"
	"syscall"
)

// applyCredentials puts tshark in its own process group and, when the
// coordinator is root and both ids are set, drops to that uid/gid.
// HOMER 7 did the same credential drop.
func applyCredentials(cmd *exec.Cmd, uid, gid uint32) {
	attr := &syscall.SysProcAttr{Setpgid: true}
	if os.Geteuid() == 0 && uid != 0 && gid != 0 {
		attr.Credential = &syscall.Credential{
			Uid:         uid,
			Gid:         gid,
			NoSetGroups: true,
		}
	}
	cmd.SysProcAttr = attr
}

func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
