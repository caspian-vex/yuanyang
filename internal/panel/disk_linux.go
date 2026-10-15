//go:build linux

package panel

import "syscall"

func diskUsage(path string)(uint64,uint64){var s syscall.Statfs_t;if syscall.Statfs(path,&s)!=nil{return 0,0};return s.Blocks*uint64(s.Bsize),(s.Blocks-s.Bfree)*uint64(s.Bsize)}
