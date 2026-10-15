//go:build linux

package panel

import (
 "os/exec"
 "os"
 "syscall"
 "time"
)
func configureProcessGroup(cmd *exec.Cmd){cmd.SysProcAttr=&syscall.SysProcAttr{Setpgid:true};cmd.Cancel=func()error{if cmd.Process==nil{return nil};return syscall.Kill(-cmd.Process.Pid,syscall.SIGKILL)};cmd.WaitDelay=2*time.Second}
func terminateProcess(pid int)error{return syscall.Kill(pid,syscall.SIGTERM)}
func lockFirewall()(func(),error){if err:=os.MkdirAll("/var/lib/yuanyang-firewall",0700);err!=nil{return nil,err};f,err:=os.OpenFile("/var/lib/yuanyang-firewall/lock",os.O_CREATE|os.O_RDWR,0600);if err!=nil{return nil,err};if err=syscall.Flock(int(f.Fd()),syscall.LOCK_EX);err!=nil{f.Close();return nil,err};return func(){syscall.Flock(int(f.Fd()),syscall.LOCK_UN);f.Close()},nil}
