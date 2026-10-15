package panel

import (
 "bufio"
 "os"
 "strings"
)
func diskCounters()map[string][2]uint64{out:=map[string][2]uint64{};f,err:=os.Open("/proc/diskstats");if err!=nil{return out};defer f.Close();s:=bufio.NewScanner(f);for s.Scan(){fields:=strings.Fields(s.Text());if len(fields)<10{continue};name:=fields[2];if _,err=os.Stat("/sys/block/"+name+"/device");err!=nil{continue};out[name]=[2]uint64{number(fields[5])*512,number(fields[9])*512};if len(out)>=32{break}};return out}
