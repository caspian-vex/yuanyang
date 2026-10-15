package panel

import (
 "bufio"
 "encoding/json"
 "errors"
 "fmt"
 "net"
 "net/http"
 "os"
 "runtime"
 "strconv"
 "strings"
 "time"
)

type Metric struct {
 Time int64 `json:"time"`
 LastSuccess int64 `json:"lastSuccess"`
 Available bool `json:"available"`
 CPU float64 `json:"cpu"`
 Memory float64 `json:"memory"`
 MemoryTotal uint64 `json:"memoryTotal"`
 MemoryUsed uint64 `json:"memoryUsed"`
 Swap float64 `json:"swap"`
 Load string `json:"load"`
 RX uint64 `json:"rx"`
 TX uint64 `json:"tx"`
 DiskTotal uint64 `json:"diskTotal"`
 DiskUsed uint64 `json:"diskUsed"`
 Disk float64 `json:"disk"`
 DiskRead uint64 `json:"diskRead"`
 DiskWrite uint64 `json:"diskWrite"`
 Error string `json:"error,omitempty"`
}
func readFile(path string)string{b,_:=os.ReadFile(path);return strings.TrimSpace(string(b))}
func number(s string)uint64{n,_:=strconv.ParseUint(s,10,64);return n}
func cpuCounters()(uint64,uint64){f:=strings.Fields(readFile("/proc/stat"));if len(f)<9{return 0,0};var total uint64;for i:=1;i<=8;i++{total+=number(f[i])};return total,number(f[4])+number(f[5])}
func networkCounters()(uint64,uint64,bool){f,err:=os.Open("/proc/net/dev");if err!=nil{return 0,0,false};defer f.Close();s:=bufio.NewScanner(f);var rx,tx uint64;for s.Scan(){p:=strings.SplitN(s.Text(),":",2);if len(p)!=2||strings.TrimSpace(p[0])=="lo"{continue};v:=strings.Fields(p[1]);if len(v)>8{rx+=number(v[0]);tx+=number(v[8])}};return rx,tx,s.Err()==nil}
func sample(prevTotal,prevIdle,prevRX,prevTX uint64,elapsed float64)(Metric,uint64,uint64,uint64,uint64){
 m:=Metric{Time:time.Now().Unix(),Available:runtime.GOOS=="linux"};total,idle:=cpuCounters();rx,tx,networkOK:=networkCounters()
 if !m.Available{m.Error="Linux /proc metrics unavailable on this platform";return m,total,idle,rx,tx}
 if total==0{m.Available=false;m.Error="cannot read /proc/stat";return m,total,idle,rx,tx}
 if total>prevTotal&&idle>=prevIdle{m.CPU=100*(1-float64(idle-prevIdle)/float64(total-prevTotal))}
 values:=map[string]uint64{};for _,line:=range strings.Split(readFile("/proc/meminfo"),"\n"){f:=strings.Fields(line);if len(f)>=2{values[strings.TrimSuffix(f[0],":")]=number(f[1])*1024}}
 m.MemoryTotal=values["MemTotal"];if m.MemoryTotal>=values["MemAvailable"]{m.MemoryUsed=m.MemoryTotal-values["MemAvailable"]};if m.MemoryTotal>0{m.Memory=100*float64(m.MemoryUsed)/float64(m.MemoryTotal)}
 if values["SwapTotal"]>0{m.Swap=100*(1-float64(values["SwapFree"])/float64(values["SwapTotal"]))}
 m.Load=readFile("/proc/loadavg");if elapsed>0&&rx>=prevRX&&tx>=prevTX{m.RX=uint64(float64(rx-prevRX)/elapsed);m.TX=uint64(float64(tx-prevTX)/elapsed)}
 m.DiskTotal,m.DiskUsed=diskUsage("/");if m.DiskTotal>0{m.Disk=100*float64(m.DiskUsed)/float64(m.DiskTotal)}
 if !networkOK||m.MemoryTotal==0||m.DiskTotal==0||prevTotal==0||total<=prevTotal{m.Available=false;m.Error="waiting for valid CPU, memory, disk and network counters"}else{m.LastSuccess=m.Time}
 return m,total,idle,rx,tx
}
func(a *App) collect(){
 total,idle:=cpuCounters();rx,tx,_:=networkCounters();previousIO:=diskCounters();last:=time.Now();ticker:=time.NewTicker(10*time.Second);defer ticker.Stop()
 m,_,_,_,_:=sample(total,idle,rx,tx,0);processes:=readProcesses(nil,0);a.mu.Lock();a.metric=m;a.processes=processes;a.mu.Unlock()
 var minute,hour []Metric;minute=make([]Metric,0,6);hour=make([]Metric,0,360);lastMinute,lastHour:=time.Now().Unix()/60,time.Now().Unix()/3600
 for{select{case <-a.ctx.Done():return;case now:=<-ticker.C:
 elapsed:=now.Sub(last).Seconds();m,t,i,r,x:=sample(total,idle,rx,tx,elapsed);total,idle,rx,tx,last=t,i,r,x,now;currentIO:=diskCounters();for name,counts:=range currentIO{if before,ok:=previousIO[name];ok&&elapsed>0&&counts[0]>=before[0]&&counts[1]>=before[1]{m.DiskRead+=uint64(float64(counts[0]-before[0])/elapsed);m.DiskWrite+=uint64(float64(counts[1]-before[1])/elapsed)}};previousIO=currentIO
 processes=readProcesses(processes,elapsed);a.mu.Lock();if !m.Available{m.LastSuccess=a.metric.LastSuccess};a.metric=m;a.processes=processes;expired:=0;for expired<len(a.history)&&a.history[expired].Time<m.Time-3600{expired++};if expired>0{copy(a.history,a.history[expired:]);a.history=a.history[:len(a.history)-expired]};if m.Available{if len(a.history)==360{copy(a.history,a.history[1:]);a.history=a.history[:359]};a.history=append(a.history,m)};a.mu.Unlock()
 if !m.Available{minute=minute[:0];hour=hour[:0];lastMinute=now.Unix()/60;lastHour=now.Unix()/3600;continue};minute=append(minute,m);hour=append(hour,m)
 if now.Unix()/60!=lastMinute{v:=average(minute);a.exec("INSERT OR REPLACE INTO metrics VALUES(?,60,?)",lastMinute*60,encode(v));minute=minute[:0];lastMinute=now.Unix()/60}
 if now.Unix()/3600!=lastHour{v:=average(hour);a.exec("INSERT OR REPLACE INTO metrics VALUES(?,3600,?)",lastHour*3600,encode(v));hour=hour[:0];lastHour=now.Unix()/3600;a.exec("DELETE FROM metrics WHERE (resolution=60 AND bucket<?) OR bucket<?",now.Add(-7*24*time.Hour).Unix(),now.Add(-30*24*time.Hour).Unix());a.exec("DELETE FROM sessions WHERE seen<? OR created<?",now.Add(-time.Duration(a.intSetting("session.idleMinutes",30))*time.Minute).Unix(),now.Add(-time.Duration(a.intSetting("session.maxHours",12))*time.Hour).Unix());a.exec("DELETE FROM audit WHERE time<?",now.Add(-time.Duration(a.intSetting("retention.auditDays",180))*24*time.Hour).Unix());a.exec("DELETE FROM runs WHERE started<?",now.Add(-time.Duration(a.intSetting("retention.taskDays",30))*24*time.Hour).Unix())}
 a.evaluate(m)
 }}
}
func average(values []Metric)Metric{if len(values)==0{return Metric{}};m:=values[len(values)-1];m.CPU=0;m.Memory=0;m.Disk=0;for _,v:=range values{m.CPU+=v.CPU;m.Memory+=v.Memory;m.Disk+=v.Disk};n:=float64(len(values));m.CPU/=n;m.Memory/=n;m.Disk/=n;return m}
func(a *App) historyAPI(w http.ResponseWriter,r *http.Request){
 hours:=1;switch r.URL.Query().Get("range"){case "24h":hours=24;case "7d":hours=168;case "30d":hours=720}
 if hours==1{a.mu.RLock();items:=append([]Metric{},a.history...);a.mu.RUnlock();reply(w,200,items);return}
 resolution:=60;if hours>24{resolution=3600};rows,err:=a.db.Query("SELECT payload FROM metrics WHERE resolution=? AND bucket>=? ORDER BY bucket LIMIT 1440",resolution,time.Now().Add(-time.Duration(hours)*time.Hour).Unix());if err!=nil{problem(w,500,err);return};defer rows.Close();items:=[]Metric{};for rows.Next(){var s string;var m Metric;if rows.Scan(&s)==nil&&json.Unmarshal([]byte(s),&m)==nil{items=append(items,m)}};reply(w,200,items)
}
func hostInfo()map[string]any{host,_:=os.Hostname();interfaces:=[]map[string]any{};if list,e:=net.Interfaces();e==nil{for _,iface:=range list{addrs,_:=iface.Addrs();ips:=[]string{};for _,addr:=range addrs{ips=append(ips,addr.String())};interfaces=append(interfaces,map[string]any{"name":iface.Name,"mtu":iface.MTU,"mac":iface.HardwareAddr.String(),"addresses":ips})}};return map[string]any{"hostname":host,"os":runtime.GOOS,"architecture":runtime.GOARCH,"distribution":readFile("/etc/os-release"),"kernel":readFile("/proc/sys/kernel/osrelease"),"uptime":readFile("/proc/uptime"),"time":time.Now().Format(time.RFC3339),"timezone":time.Now().Location().String(),"cpus":runtime.NumCPU(),"interfaces":interfaces}}
func(a *App) processAPI(w http.ResponseWriter,r *http.Request,u User){
 if r.Method=="POST"{if !admin(w,u){return};var in struct{PID int};if !body(w,r,&in){return};if in.PID<=1||in.PID==os.Getpid(){problem(w,400,errors.New("protected process"));return};name:=readFile(fmt.Sprintf("/proc/%d/comm",in.PID));for _,v:=range []string{"sshd","systemd","nginx","yuanyang"}{if strings.Contains(name,v){problem(w,403,errors.New("protected process"));return}};out,err:=a.privileged(r.Context(),HelperRequest{Operation:"terminate",Name:strconv.Itoa(in.PID)});a.audit(u.Name,"process.terminate",strconv.Itoa(in.PID),result(err));if err!=nil{problem(w,400,fmt.Errorf("%s: %w",out,err));return};reply(w,200,map[string]bool{"ok":true});return}
 if r.Method!="GET"{w.WriteHeader(405);return};a.processList(w,r)
}
