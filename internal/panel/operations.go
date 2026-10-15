package panel

import (
 "bytes"
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "os"
 "os/exec"
 "path/filepath"
 "net"
 "regexp"
 "strconv"
 "strings"
 "sync"
 "time"

 "golang.org/x/crypto/bcrypt"
)
func decodeJSON(raw string,v any)error{return json.Unmarshal([]byte(raw),v)}
func escapeName(s string)string{return url.PathEscape(s)}
func result(err error)string{if err!=nil{return err.Error()};return "ok"}
type cappedOutput struct{ mu sync.Mutex; b bytes.Buffer; limit int; truncated bool }
func(c *cappedOutput)Write(p []byte)(int,error){c.mu.Lock();defer c.mu.Unlock();n:=len(p);left:=c.limit-c.b.Len();if left>0{if len(p)>left{p=p[:left]};c.b.Write(p)};if n>left{c.truncated=true};return n,nil}
func(c *cappedOutput)String()string{c.mu.Lock();defer c.mu.Unlock();s:=c.b.String();if c.truncated{s+="\n[output truncated]"};return s}
func(a *App) run(ctx context.Context,name string,args ...string)(string,error){
 select{case a.commands<-struct{}{}:defer func(){<-a.commands}();default:return "",errors.New("system operation busy; retry later")}
 ctx,cancel:=context.WithTimeout(ctx,60*time.Second);defer cancel();cmd:=exec.CommandContext(ctx,name,args...);cmd.Env=append(os.Environ(),"LC_ALL=C","LANG=C");cmd.WaitDelay=2*time.Second;out:=&cappedOutput{limit:64<<10};cmd.Stdout=out;cmd.Stderr=out;err:=cmd.Run();return out.String(),err
}
func(a *App) privileged(ctx context.Context,in HelperRequest)(string,error){
 executable,err:=os.Executable();if err!=nil{return "",err}
 select{case a.commands<-struct{}{}:defer func(){<-a.commands}();default:return "",errors.New("system operation busy")}
 ctx,cancel:=context.WithTimeout(ctx,60*time.Second);defer cancel();cmd:=exec.CommandContext(ctx,"sudo","-n",executable,"helper");cmd.Stdin=strings.NewReader(encode(in));cmd.WaitDelay=2*time.Second;out:=&cappedOutput{limit:64<<10};cmd.Stdout=out;cmd.Stderr=out;err=cmd.Run();return out.String(),err
}
var serviceName=regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@-]{0,120}$`)
func protectedService(s string)bool{for _,v:=range []string{"ssh","sshd","systemd","network","NetworkManager","yuanyang","dbus","ufw"}{if strings.HasPrefix(s,v){return true}};return false}
func(a *App) services(w http.ResponseWriter,r *http.Request,u User){
 if r.Method=="GET"{a.serviceList(w,r);return}
 if r.Method!="POST"{w.WriteHeader(405);return};var in struct{Name,Action string};if !body(w,r,&in){return};if !serviceName.MatchString(in.Name){problem(w,400,errors.New("invalid service name"));return}
 if in.Action=="logs"{out,err:=a.run(r.Context(),"journalctl","--unit",in.Name,"--lines=200","--no-pager","--output=short-iso");if err!=nil{problem(w,503,err);return};reply(w,200,map[string]string{"output":out});return}
 if protectedService(in.Name){problem(w,403,errors.New("protected service"));return};if in.Action!="start"&&in.Action!="stop"&&in.Action!="restart"&&in.Action!="enable"&&in.Action!="disable"{problem(w,400,errors.New("invalid action"));return}
 out,err:=a.privileged(r.Context(),HelperRequest{Operation:"service",Name:in.Name,Action:in.Action});a.serviceMu.Lock();a.serviceTime=time.Time{};a.serviceMu.Unlock();a.audit(u.Name,"service."+in.Action,in.Name,result(err));if err!=nil{problem(w,400,fmt.Errorf("%s: %w",out,err));return};reply(w,200,map[string]string{"output":out})
}
func(a *App) users(w http.ResponseWriter,r *http.Request,u User){
 if r.Method=="GET"{rows,err:=a.db.Query("SELECT id,name,role,disabled,totp<>'',terminal FROM users ORDER BY id LIMIT 200");if err!=nil{problem(w,500,err);return};defer rows.Close();items:=[]map[string]any{};for rows.Next(){var id int;var name,role string;var disabled,totp,terminal bool;rows.Scan(&id,&name,&role,&disabled,&totp,&terminal);items=append(items,map[string]any{"id":id,"name":name,"role":role,"disabled":disabled,"totp":totp,"terminal":terminal})};reply(w,200,items);return}
 if r.Method!="POST"{w.WriteHeader(405);return}
 var in struct{ID int;Name,Password,Role,Action string;Disabled,Terminal bool};if !body(w,r,&in){return};if in.Password!=""&&(len(in.Password)<12||len(in.Password)>72){problem(w,400,errors.New("password must be 12–72 bytes"));return}
 if in.Role!="admin"&&in.Role!="operator"&&in.Role!="viewer"{problem(w,400,errors.New("invalid role"));return}
 if in.ID==int(u.ID)&&(in.Disabled||in.Role!="admin"||in.Action=="delete"){problem(w,400,errors.New("cannot disable, demote or delete your own administrator"));return}
 if in.Action!=""&&in.Action!="save"&&in.Action!="delete"{problem(w,400,errors.New("invalid user action"));return}
 var hash []byte;var err error;if in.Password!=""{hash,err=bcrypt.GenerateFromPassword([]byte(in.Password),bcrypt.DefaultCost);if err!=nil{problem(w,500,err);return}}
 err=a.transaction(func(tx *sql.Tx)error{
 if in.ID==0{if len(in.Name)<1||len(in.Name)>64||len(hash)==0||in.Action=="delete"{return errors.New("name and password are required")};var count int;if err:=tx.QueryRow("SELECT count(*) FROM users").Scan(&count);err!=nil{return err};if count>=200{return errors.New("user limit is 200")};_,err:=tx.Exec("INSERT INTO users(name,password,role,disabled,terminal) VALUES(?,?,?,?,?)",in.Name,string(hash),in.Role,in.Disabled,in.Terminal);return err}
 var actualName string;if err:=tx.QueryRow("SELECT name FROM users WHERE id=?",in.ID).Scan(&actualName);err!=nil{return err};in.Name=actualName
 if in.Action=="delete"{if _,err:=tx.Exec("DELETE FROM users WHERE id=?",in.ID);err!=nil{return err}}else{if _,err:=tx.Exec("UPDATE users SET role=?,disabled=?,terminal=? WHERE id=?",in.Role,in.Disabled,in.Terminal,in.ID);err!=nil{return err};if len(hash)>0{if _,err:=tx.Exec("UPDATE users SET password=? WHERE id=?",string(hash),in.ID);err!=nil{return err}}}
 _,err:=tx.Exec("DELETE FROM sessions WHERE user_id=?",in.ID);return err
 });a.audit(u.Name,"user.change",in.Name,result(err));if err!=nil{problem(w,400,err);return};reply(w,200,map[string]bool{"ok":true})
}
func(a *App) settings(w http.ResponseWriter,r *http.Request,u User){
 if r.Method=="GET"{items:=map[string]string{};rows,err:=a.db.Query("SELECT key,value FROM settings WHERE key NOT LIKE 'totp-%'");if err!=nil{problem(w,500,err);return};defer rows.Close();for rows.Next(){var k,v string;rows.Scan(&k,&v);if k=="smtp.password"{v="[configured]"};items[k]=v};reply(w,200,items);return}
 if r.Method!="POST"{w.WriteHeader(405);return};var in map[string]string;if !body(w,r,&in){return};allowed:=map[string]bool{"smtp.host":true,"smtp.port":true,"smtp.user":true,"smtp.password":true,"smtp.from":true,"smtp.to":true,"panel.name":true,"alerts.cpu":true,"alerts.memory":true,"alerts.disk":true,"alerts.load":true,"alerts.enabled":true,"alerts.services":true,"alerts.quiet.start":true,"alerts.quiet.end":true,"security.allowIPs":true,"session.idleMinutes":true,"session.maxHours":true,"files.root":true,"uploads.maxMB":true,"retention.auditDays":true,"retention.taskDays":true}
 for k,v:=range in{if bounds,ok:=map[string][2]int{"session.idleMinutes":{5,240},"session.maxHours":{1,24},"uploads.maxMB":{1,100},"retention.auditDays":{1,365},"retention.taskDays":{1,90},"alerts.cpu":{1,100},"alerts.memory":{1,100},"alerts.disk":{1,100},"alerts.load":{1,100}}[k];ok{n,e:=strconv.Atoi(v);if e!=nil||n<bounds[0]||n>bounds[1]{problem(w,400,fmt.Errorf("%s out of range",k));return}};if k=="security.allowIPs"{if !allowedIP(v,a.clientIP(r)){problem(w,400,errors.New("whitelist would exclude your current source IP"));return};for _,item:=range strings.Split(v,","){item=strings.TrimSpace(item);if item==""{continue};if net.ParseIP(item)==nil{if _,_,e:=net.ParseCIDR(item);e!=nil{problem(w,400,e);return}}}};if k=="alerts.services"{units:=strings.Split(v,",");if len(units)>20{problem(w,400,errors.New("at most 20 services"));return};for _,unit:=range units{if unit!=""&&!serviceName.MatchString(strings.TrimSpace(unit)){problem(w,400,errors.New("invalid service name"));return}}};if (k=="alerts.quiet.start"||k=="alerts.quiet.end")&&v!=""{if _,e:=time.Parse("15:04",v);e!=nil{problem(w,400,e);return}}}
 var newRoot *os.Root;if p,ok:=in["files.root"];ok{if !filepath.IsAbs(p){problem(w,400,errors.New("file root must be absolute"));return};var err error;newRoot,err=os.OpenRoot(p);if err!=nil{problem(w,400,err);return};defer func(){if newRoot!=nil{newRoot.Close()}}()}
 for k,v:=range in{if !allowed[k]||len(v)>1024{problem(w,400,errors.New("invalid setting"));return};if k=="smtp.password"&&v!=""{sealed,err:=a.seal([]byte(v));if err!=nil{problem(w,500,err);return};in[k]=sealed}}
 err:=a.transaction(func(tx *sql.Tx)error{for k,v:=range in{if _,err:=tx.Exec("INSERT OR REPLACE INTO settings VALUES(?,?)",k,v);err!=nil{return err}};return nil});if err!=nil{problem(w,500,err);return};a.configMu.Lock();for k,v:=range in{a.config[k]=v};a.configMu.Unlock();if newRoot!=nil{a.fileMu.Lock();a.root.Close();a.root=newRoot;newRoot=nil;a.rootPath=in["files.root"];a.fileMu.Unlock()};a.audit(u.Name,"settings.update","settings","ok");reply(w,200,map[string]bool{"ok":true})
}
func(a *App) setting(key,defaultValue string)string{a.configMu.RLock();s,ok:=a.config[key];a.configMu.RUnlock();if !ok{return defaultValue};return s}
func writeAtomic(path string,data []byte,mode os.FileMode)error{f,err:=os.CreateTemp(filepath.Dir(path),".panel-*");if err!=nil{return err};temp:=f.Name();defer os.Remove(temp);if err=f.Chmod(mode);err==nil{_,err=f.Write(data)};if err==nil{err=f.Sync()};closeErr:=f.Close();if err==nil{err=closeErr};if err!=nil{return err};return os.Rename(temp,path)}
var _ io.Writer=(*cappedOutput)(nil)
