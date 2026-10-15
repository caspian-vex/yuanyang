package panel

import (
 "bufio"
 "context"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "net"
 "net/http"
 "os"
 "strconv"
 "strings"
 "time"
)
type Port struct{Address string `json:"address"`;Port int `json:"port"`;Protocol string `json:"protocol"`;PID int `json:"pid,omitempty"`;Process string `json:"process,omitempty"`;Inode string `json:"-"`}
func socketAddress(encoded string)(string,int){parts:=strings.Split(encoded,":");if len(parts)!=2{return "",0};b,err:=hex.DecodeString(parts[0]);if err!=nil{return "",0};for i:=0;i+4<=len(b);i+=4{b[i],b[i+3]=b[i+3],b[i];b[i+1],b[i+2]=b[i+2],b[i+1]};port,_:=strconv.ParseInt(parts[1],16,32);return net.IP(b).String(),int(port)}
func readPorts(ctx context.Context)[]Port{
 ports:=[]Port{};for _,protocol:=range []string{"tcp","tcp6","udp","udp6"}{f,err:=os.Open("/proc/net/"+protocol);if err!=nil{continue};s:=bufio.NewScanner(f);for s.Scan(){fields:=strings.Fields(s.Text());if len(fields)<10{continue};if strings.HasPrefix(protocol,"tcp")&&fields[3]!="0A"{continue};if strings.HasPrefix(protocol,"udp")&&fields[3]!="07"{continue};address,port:=socketAddress(fields[1]);if port==0{continue};ports=append(ports,Port{Address:address,Port:port,Protocol:protocol,Inode:fields[9]});if len(ports)>=200{break}};f.Close();if len(ports)>=200{break}}
 wanted:=map[string][]int{};for i,p:=range ports{wanted[p.Inode]=append(wanted[p.Inode],i)}
 for _,p:=range readProcesses(nil,0){if ctx.Err()!=nil||len(wanted)==0{break};dir,err:=os.Open(fmt.Sprintf("/proc/%d/fd",p.PID));if err!=nil{continue};entries,_:=dir.ReadDir(512);dir.Close();for _,entry:=range entries{link,err:=os.Readlink(fmt.Sprintf("/proc/%d/fd/%s",p.PID,entry.Name()));if err!=nil||!strings.HasPrefix(link,"socket:["){continue};inode:=strings.TrimSuffix(strings.TrimPrefix(link,"socket:["),"]");if indices,ok:=wanted[inode];ok{for _,i:=range indices{ports[i].PID=p.PID;ports[i].Process=p.Name};delete(wanted,inode)}}};return ports
}
func(a *App) ports(w http.ResponseWriter,r *http.Request){if r.Method!="GET"{w.WriteHeader(405);return};a.portMu.Lock();defer a.portMu.Unlock();if time.Since(a.portTime)>time.Minute{ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);out,err:=a.privileged(ctx,HelperRequest{Operation:"ports"});cancel();if err!=nil{problem(w,503,err);return};if err=json.Unmarshal([]byte(out),&a.portCache);err!=nil{problem(w,500,err);return};a.portTime=time.Now()};reply(w,200,a.portCache)}
