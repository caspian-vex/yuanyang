package panel

import (
 "net/http"
 "os"
 "sort"
 "strconv"
 "strings"
)
type Process struct{PID int `json:"pid"`;Name string `json:"name"`;Memory uint64 `json:"memory"`;CPU float64 `json:"cpu"`;Ticks uint64 `json:"-"`;Start uint64 `json:"-"`}
func readProcesses(previous []Process,seconds float64)[]Process{
 old:=make(map[int]Process,len(previous));for _,p:=range previous{old[p.PID]=p};dir,e:=os.Open("/proc");if e!=nil{return nil};defer dir.Close();out:=make([]Process,0,256)
 for{entries,err:=dir.ReadDir(128);for _,entry:=range entries{pid,e:=strconv.Atoi(entry.Name());if e!=nil{continue};raw:=readFile("/proc/"+entry.Name()+"/stat");start,end:=strings.IndexByte(raw,'('),strings.LastIndexByte(raw,')');if start<0||end<=start{continue};fields:=strings.Fields(raw[end+1:]);if len(fields)<22{continue};rss,_:=strconv.ParseInt(fields[21],10,64);p:=Process{PID:pid,Name:raw[start+1:end],Ticks:number(fields[11])+number(fields[12]),Start:number(fields[19])};if rss>0{p.Memory=uint64(rss)*uint64(os.Getpagesize())};if before,ok:=old[pid];ok&&before.Start==p.Start&&seconds>0&&p.Ticks>=before.Ticks{p.CPU=float64(p.Ticks-before.Ticks)/seconds};out=append(out,p);if len(out)>=10000{return out}};if err!=nil{return out}}
}
func(a *App) processList(w http.ResponseWriter,r *http.Request){a.mu.RLock();items:=append([]Process{},a.processes...);a.mu.RUnlock();query:=r.URL.Query().Get("q");filtered:=items[:0];for _,p:=range items{if query==""||strings.Contains(p.Name,query)||strconv.Itoa(p.PID)==query{filtered=append(filtered,p)}};sort.Slice(filtered,func(i,j int)bool{switch r.URL.Query().Get("sort"){case "cpu":return filtered[i].CPU>filtered[j].CPU;case "memory":return filtered[i].Memory>filtered[j].Memory;default:return filtered[i].PID<filtered[j].PID}});limit,offset:=page(r);if offset>len(filtered){offset=len(filtered)};end:=offset+limit;if end>len(filtered){end=len(filtered)};reply(w,200,filtered[offset:end])}
