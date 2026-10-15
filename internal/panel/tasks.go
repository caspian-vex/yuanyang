package panel

import (
 "context"
 "errors"
 "net/http"
 "os/exec"
 "regexp"
 "path/filepath"
 "strconv"
 "strings"
 "time"

 "github.com/robfig/cron/v3"
)
type Task struct{ID int64 `json:"id"`;Name string `json:"name"`;Schedule string `json:"schedule"`;Command string `json:"command"`;Account string `json:"account"`;Enabled bool `json:"enabled"`;Timeout int `json:"timeout"`;Workdir string `json:"workdir"`}
var accountName=regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
func(a *App) tasks(w http.ResponseWriter,r *http.Request,u User){
 if r.Method=="GET"{limit,offset:=page(r);rows,err:=a.db.Query("SELECT id,name,schedule,command,account,enabled,timeout,workdir FROM tasks ORDER BY id DESC LIMIT ? OFFSET ?",limit,offset);if err!=nil{problem(w,500,err);return};defer rows.Close();items:=[]map[string]any{};for rows.Next(){var t Task;rows.Scan(&t.ID,&t.Name,&t.Schedule,&t.Command,&t.Account,&t.Enabled,&t.Timeout,&t.Workdir);next:=[]string{};if s,e:=cron.ParseStandard(t.Schedule);e==nil{now:=time.Now();for i:=0;i<5;i++{now=s.Next(now);next=append(next,now.Format(time.RFC3339))}};items=append(items,map[string]any{"task":t,"next":next})};reply(w,200,items);return}
 if r.Method!="POST"&&r.Method!="DELETE"{w.WriteHeader(405);return};var in struct{Task;Action string `json:"action"`};if !body(w,r,&in){return};t:=in.Task
 if in.Action=="run"{if err:=a.db.QueryRow("SELECT id,name,schedule,command,account,enabled,timeout,workdir FROM tasks WHERE id=?",t.ID).Scan(&t.ID,&t.Name,&t.Schedule,&t.Command,&t.Account,&t.Enabled,&t.Timeout,&t.Workdir);err!=nil{problem(w,404,err);return};if err:=a.enqueue(t);err!=nil{problem(w,429,err);return};a.audit(u.Name,"task.run",t.Name,"queued");reply(w,202,map[string]bool{"queued":true});return}
 if r.Method=="DELETE"{err:=a.exec("DELETE FROM tasks WHERE id=?",t.ID);if err!=nil{problem(w,500,err);return};a.audit(u.Name,"task.delete",t.Name,"ok");reply(w,200,map[string]bool{"ok":true});return}
 if len(strings.Fields(t.Schedule))!=5{problem(w,400,errors.New("use a standard five-field cron expression"));return};if t.Workdir!=""&&(!filepath.IsAbs(t.Workdir)||len(t.Workdir)>4096||strings.ContainsRune(t.Workdir,0)){problem(w,400,errors.New("working directory must be an absolute path"));return}
 if _,err:=cron.ParseStandard(t.Schedule);err!=nil{problem(w,400,err);return};if !accountName.MatchString(t.Account)||t.Account=="root"||t.Account=="yuanyang"||t.Name==""||len(t.Name)>100||len(t.Command)>8192||t.Command==""||t.Timeout<1||t.Timeout>900{problem(w,400,errors.New("invalid task; use a non-root account and timeout 1–900 seconds"));return}
 a.mutations.Lock();defer a.mutations.Unlock();if t.ID==0{var count int;a.db.QueryRow("SELECT count(*) FROM tasks").Scan(&count);if count>=1000{problem(w,409,errors.New("single-machine task limit is 1000"));return}}
 var err error;if t.ID==0{err=a.exec("INSERT INTO tasks(name,schedule,command,account,enabled,timeout,workdir) VALUES(?,?,?,?,?,?,?)",t.Name,t.Schedule,t.Command,t.Account,t.Enabled,t.Timeout,t.Workdir)}else{err=a.exec("UPDATE tasks SET name=?,schedule=?,command=?,account=?,enabled=?,timeout=?,workdir=? WHERE id=?",t.Name,t.Schedule,t.Command,t.Account,t.Enabled,t.Timeout,t.Workdir,t.ID)};if err!=nil{problem(w,400,err);return};a.audit(u.Name,"task.save",t.Name,"ok");reply(w,200,map[string]bool{"ok":true})
}
func(a *App) enqueue(t Task)error{a.jobMu.Lock();defer a.jobMu.Unlock();if a.running[t.ID]{return errors.New("previous execution still running or queued")};select{case a.jobs<-t:a.running[t.ID]=true;return nil;default:return errors.New("task queue full")}}
func(a *App) schedule(){
 a.exec("UPDATE runs SET status='interrupted',finished=? WHERE status='running'",time.Now().Unix())
 for{now:=time.Now();timer:=time.NewTimer(time.Until(now.Truncate(time.Minute).Add(time.Minute)));select{case <-a.ctx.Done():timer.Stop();return;case tick:=<-timer.C:
 rows,err:=a.db.Query("SELECT id,name,schedule,command,account,enabled,timeout,workdir FROM tasks WHERE enabled=1 ORDER BY id LIMIT 1000");if err!=nil{continue};items:=[]Task{};for rows.Next(){var t Task;rows.Scan(&t.ID,&t.Name,&t.Schedule,&t.Command,&t.Account,&t.Enabled,&t.Timeout,&t.Workdir);items=append(items,t)};rows.Close()
 for _,t:=range items{s,e:=cron.ParseStandard(t.Schedule);if e!=nil{continue};if s.Next(tick.Truncate(time.Minute).Add(-time.Second)).Equal(tick.Truncate(time.Minute)){if err=a.enqueue(t);err!=nil{a.exec("INSERT INTO runs(task_id,started,finished,status,output) VALUES(?,?,?,?,?)",t.ID,tick.Unix(),tick.Unix(),"skipped",err.Error())}}}
 }}
}
func(a *App) worker(){for{select{case <-a.ctx.Done():return;case t:=<-a.jobs:
 started:=time.Now().Unix();id,err:=a.insert("INSERT INTO runs(task_id,started,status) VALUES(?,?,'running')",t.ID,started);if err!=nil{a.jobMu.Lock();delete(a.running,t.ID);a.jobMu.Unlock();continue}
 ctx,cancel:=context.WithTimeout(a.ctx,time.Duration(t.Timeout)*time.Second);out,err:=a.taskCommand(ctx,t);timedOut:=err!=nil&&errors.Is(ctx.Err(),context.DeadlineExceeded);cancel();status:="success";var exitCode any=0;if err!=nil{status="failed";exitCode=nil;var exit *exec.ExitError;if errors.As(err,&exit){exitCode=exit.ExitCode()};out+="\n"+err.Error()};if timedOut{status="timeout"};a.exec("UPDATE runs SET finished=?,status=?,output=?,exit_code=? WHERE id=?",time.Now().Unix(),status,out,exitCode,id);a.alertState("task:"+strconv.FormatInt(t.ID,10),"Task "+t.Name+": "+status,err!=nil);a.jobMu.Lock();delete(a.running,t.ID);a.jobMu.Unlock()
 }}}
