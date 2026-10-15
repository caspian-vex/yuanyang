package panel

import (
 "context"
 "strings"
 "time"
)
func(a *App) watchServices(){ticker:=time.NewTicker(30*time.Second);defer ticker.Stop();for{select{case <-a.ctx.Done():return;case <-ticker.C:
 if a.setting("alerts.enabled","true")!="true"{continue};names:=strings.Split(a.setting("alerts.services",""),",");args:=[]string{"show","--no-pager","--property=Id","--property=ActiveState"};for _,n:=range names{n=strings.TrimSpace(n);if serviceName.MatchString(n){args=append(args,n)}};if len(args)==4{continue};ctx,cancel:=context.WithTimeout(a.ctx,3*time.Second);out,err:=a.run(ctx,"systemctl",args...);cancel();if err!=nil{continue};for _,part:=range strings.Split(out,"\n\n"){name,state:="","";for _,line:=range strings.Split(part,"\n"){if strings.HasPrefix(line,"Id="){name=strings.TrimPrefix(line,"Id=")};if strings.HasPrefix(line,"ActiveState="){state=strings.TrimPrefix(line,"ActiveState=")}};if name!=""&&state!=""{a.alertState("service:"+name,"Service "+name+" is "+state,state!="active")}}
 }}}
