package panel

import (
 "bufio"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "os/exec"
 "strings"
 "time"
)
func(a *App) streamLogs(w http.ResponseWriter,r *http.Request){
 if r.Method!="GET"{w.WriteHeader(405);return};select{case a.logs<-struct{}{}:defer func(){<-a.logs}();default:problem(w,429,errors.New("two log streams already active"));return}
 ctx,cancel:=context.WithTimeout(r.Context(),30*time.Minute);defer cancel();var cmd *exec.Cmd
 cookie,_:=r.Cookie("session");go func(){ticker:=time.NewTicker(30*time.Second);defer ticker.Stop();for{select{case <-ctx.Done():return;case <-a.ctx.Done():cancel();return;case <-ticker.C:var role string;var created,seen int64;err:=a.db.QueryRow("SELECT u.role,s.created,s.seen FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=? AND u.disabled=0",cookie.Value).Scan(&role,&created,&seen);now:=time.Now().Unix();if err!=nil||role=="viewer"||now-created>int64(a.intSetting("session.maxHours",12)*3600)||now-seen>int64(a.intSetting("session.idleMinutes",30)*60){cancel();return}}}}()
 if site:=r.URL.Query().Get("site");site!=""{if !siteName.MatchString(site){problem(w,400,errors.New("invalid site"));return};kind:="access";if r.URL.Query().Get("kind")=="error"{kind="error"};cmd=exec.CommandContext(ctx,"tail","-n","200","-F","/var/log/nginx/yuanyang-"+site+"."+kind+".log")}else{unit:=r.URL.Query().Get("unit");if !serviceName.MatchString(unit){problem(w,400,errors.New("invalid unit"));return};cmd=exec.CommandContext(ctx,"journalctl","--unit",unit,"--lines=200","--follow","--no-pager","--output=short-iso")}
 pipe,err:=cmd.StdoutPipe();if err!=nil{problem(w,500,err);return};cmd.WaitDelay=2*time.Second;stderr:=&cappedOutput{limit:4096};cmd.Stderr=stderr;if err=cmd.Start();err!=nil{problem(w,503,err);return};defer func(){cancel();pipe.Close();cmd.Wait()}()
 f,ok:=w.(http.Flusher);if !ok{return};w.Header().Set("Content-Type","text/event-stream");w.Header().Set("X-Accel-Buffering","no");f.Flush();scanner:=bufio.NewScanner(pipe);scanner.Buffer(make([]byte,4096),64<<10);filter:=r.URL.Query().Get("q")
 for scanner.Scan(){line:=scanner.Text();if filter!=""&&!strings.Contains(line,filter){continue};b,_:=json.Marshal(line);controller:=http.NewResponseController(w);controller.SetWriteDeadline(time.Now().Add(10*time.Second));if _,err=fmt.Fprintf(w,"data: %s\n\n",b);err!=nil{return};f.Flush()}
 if err=scanner.Err();err!=nil{b,_:=json.Marshal(err.Error());fmt.Fprintf(w,"event: error\ndata: %s\n\n",b);f.Flush()}
}
