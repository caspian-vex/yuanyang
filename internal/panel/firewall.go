package panel

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net"
 "net/http"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "time"
)
const firewallSnapshot="/var/lib/yuanyang-firewall/snapshot.json"
type firewallBackup struct{Active bool;Files map[string][]byte;Expires int64}
var firewallFiles=[]string{"/etc/ufw/user.rules","/etc/ufw/user6.rules","/etc/ufw/ufw.conf"}
func(a *App) firewall(w http.ResponseWriter,r *http.Request,u User){
 if r.Method=="GET"{out,err:=a.privileged(r.Context(),HelperRequest{Operation:"firewall",Action:"status"});if err!=nil{problem(w,503,fmt.Errorf("%s: %w",out,err));return};reply(w,200,map[string]any{"output":out,"pending":readFile("/run/yuanyang-firewall-pending")});return}
 if r.Method!="POST"{w.WriteHeader(405);return};var in struct{Action string;Port int;Protocol,Source,Policy string;Confirm bool};if !body(w,r,&in){return};if !in.Confirm{problem(w,400,errors.New("firewall change requires explicit confirmation"));return};out,err:=a.privileged(r.Context(),HelperRequest{Operation:"firewall",Action:in.Action,Port:in.Port,Protocol:in.Protocol,Source:in.Source,Policy:in.Policy});a.audit(u.Name,"firewall."+in.Action,strconv.Itoa(in.Port),result(err));if err!=nil{problem(w,400,fmt.Errorf("%s: %w",out,err));return};reply(w,200,map[string]string{"output":out,"notice":"Confirm connectivity within 60 seconds or rules will be restored."})
}
func firewallHelper(ctx context.Context,in HelperRequest)error{
 unlock,err:=lockFirewall();if err!=nil{return err};defer unlock()
 run:=func(args ...string)error{out,err:=execCombined(ctx,"/usr/sbin/ufw",args...);fmt.Print(out);return err}
 if in.Action=="status"{return run("status","verbose")}
 if in.Action=="confirm"{if _,err:=os.Stat(firewallSnapshot);err!=nil{return errors.New("no pending change")};var pending firewallBackup;if err:=decodeJSON(readFile(firewallSnapshot),&pending);err!=nil{return err};if time.Now().Unix()>=pending.Expires{return errors.New("confirmation window expired")};if err:=os.Remove(firewallSnapshot);err!=nil{return err};os.Remove("/run/yuanyang-firewall-pending");execCombined(ctx,"/usr/bin/systemctl","stop","yuanyang-firewall-rollback.timer");return nil}
 if in.Action!="allow"&&in.Action!="deny"&&in.Action!="delete"&&in.Action!="enable"&&in.Action!="disable"{return errors.New("invalid firewall action")}
 if in.Action=="allow"||in.Action=="deny"||in.Action=="delete"{if in.Port<1||in.Port>65535||(in.Protocol!="tcp"&&in.Protocol!="udp"){return errors.New("invalid port or protocol")};if in.Source!=""&&in.Source!="any"{if net.ParseIP(in.Source)==nil{if _,_,err:=net.ParseCIDR(in.Source);err!=nil{return errors.New("invalid source IP or CIDR")}}};if in.Action=="delete"&&in.Policy!="allow"&&in.Policy!="deny"{return errors.New("delete requires original policy")}}
 if _,err:=os.Stat(firewallSnapshot);err==nil{return errors.New("confirm or wait for pending change before changing more rules")}
 // Never take ownership of a host with another firewall manager.
 for _,name:=range []string{"firewalld","nftables"}{out,_:=execCombined(ctx,"/usr/bin/systemctl","is-active",name);if strings.TrimSpace(out)=="active"{return errors.New("another firewall manager is active; inspect manually")}}
 if _,err:=os.Stat("/usr/sbin/nft");err==nil{out,err:=execCombined(ctx,"/usr/sbin/nft","list","tables");if err!=nil{return err};for _,table:=range strings.Split(strings.TrimSpace(out),"\n"){table=strings.TrimSpace(table);if table!=""&&table!="table ip filter"&&table!="table ip6 filter"{return errors.New("custom nftables tables detected; refusing to take ownership")}}}
 for _,binary:=range []string{"/usr/sbin/iptables-save","/usr/sbin/ip6tables-save"}{if _,err:=os.Stat(binary);err!=nil{continue};out,err:=execCombined(ctx,binary);if err!=nil{return err};for _,line:=range strings.Split(out,"\n"){fields:=strings.Fields(line);if len(fields)>=3&&fields[0]=="-A"&&!strings.HasPrefix(fields[1],"ufw-"){if fields[2]!="-j"||len(fields)<4||!strings.HasPrefix(fields[3],"ufw-"){return errors.New("unmanaged firewall rules detected; refusing to take ownership")}}}}
 output,err:=execCombined(ctx,"/usr/sbin/ufw","status");if err!=nil{return err};backup:=firewallBackup{Active:strings.Contains(output,"Status: active"),Files:map[string][]byte{},Expires:time.Now().Add(60*time.Second).Unix()};for _,p:=range firewallFiles{b,err:=os.ReadFile(p);if err!=nil{return err};backup.Files[p]=b}
 if err=os.MkdirAll(filepath.Dir(firewallSnapshot),0700);err!=nil{return err};if err=writeAtomic(firewallSnapshot,[]byte(encode(backup)),0600);err!=nil{return err};exe,_:=os.Executable()
 if _,err=execCombined(ctx,"/usr/bin/systemd-run","--unit=yuanyang-firewall-rollback","--property=Slice=yuanyang.slice","--on-active=60s","--timer-property=AccuracySec=1s",exe,"firewall-rollback");err!=nil{os.Remove(firewallSnapshot);return err};os.WriteFile("/run/yuanyang-firewall-pending",[]byte(strconv.FormatInt(backup.Expires,10)),0644)
 switch in.Action{case "enable":
 ports:=map[int]bool{22:true,80:true,443:true,9090:true};if out,e:=execCombined(ctx,"/usr/sbin/sshd","-T");e==nil{for _,line:=range strings.Split(out,"\n"){f:=strings.Fields(line);if len(f)==2&&f[0]=="port"{if p,e:=strconv.Atoi(f[1]);e==nil{ports[p]=true}}}}
 for p:=range ports{if err=run("allow",strconv.Itoa(p)+"/tcp");err!=nil{return err}};return run("--force","enable")
 case "disable":return run("--force","disable")
 default:source:=in.Source;if source==""{source="any"};args:=[]string{in.Action,"from",source,"to","any","port",strconv.Itoa(in.Port),"proto",in.Protocol};if in.Action=="delete"{args=append([]string{"--force","delete",in.Policy},args[1:]...)};return run(args...)
 }
}
func FirewallRollback()error{if os.Geteuid()!=0{return errors.New("root required")};unlock,err:=lockFirewall();if err!=nil{return err};defer unlock();b,err:=os.ReadFile(firewallSnapshot);if os.IsNotExist(err){return nil};if err!=nil{return err};var backup firewallBackup;if err=json.Unmarshal(b,&backup);err!=nil{return err};for _,p:=range firewallFiles{v,ok:=backup.Files[p];if !ok{return errors.New("incomplete firewall snapshot")};if err=writeAtomic(p,v,0640);err!=nil{return err}};ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel();action:="disable";if backup.Active{action="enable"};if _,err=execCombined(ctx,"/usr/sbin/ufw","--force",action);err!=nil{return err};if backup.Active{if _,err=execCombined(ctx,"/usr/sbin/ufw","reload");err!=nil{return err}};os.Remove("/run/yuanyang-firewall-pending");return os.Remove(firewallSnapshot)}
