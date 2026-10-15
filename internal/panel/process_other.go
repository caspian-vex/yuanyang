//go:build !linux

package panel

import (
 "os/exec"
 "errors"
)

func configureProcessGroup(cmd *exec.Cmd){}
func terminateProcess(pid int)error{return errors.New("Linux required")}
func lockFirewall()(func(),error){return nil,errors.New("Linux required")}
