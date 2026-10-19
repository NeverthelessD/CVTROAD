//go:build windows

package main

// Windows 판은 나중에 만들어요. 지금은 빌드만 되도록 둬요.

import (
	"os/exec"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}

func detach(cmd *exec.Cmd) {}

func openPath(p string)           { exec.Command("explorer", p).Start() }
func revealPath(p string)         { exec.Command("explorer", "/select,", p).Start() }
func quickLook(p string)          { openPath(p) }
func notifyOS(title, msg string)  {}
func freeSpace(path string) int64 { return 0 }
