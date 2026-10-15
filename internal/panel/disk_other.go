//go:build !linux

package panel

func diskUsage(path string)(uint64,uint64){return 0,0}
