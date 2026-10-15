package panel

import (
 "crypto/tls"
 "fmt"
 "net"
 "net/mail"
 "net/smtp"
 "mime"
 "strconv"
 "strings"
 "time"
 "runtime"
)
type Notice struct{Subject,Body string}
func(a *App) notice(subject,body string){if a.quiet(time.Now()){return};select{case a.notifications<-Notice{subject,body}:default:a.audit("system","notification.drop",subject,"bounded queue full")}}
func(a *App) quiet(now time.Time)bool{start,end:=a.setting("alerts.quiet.start",""),a.setting("alerts.quiet.end","");if start==""||end==""||start==end{return false};current:=now.Format("15:04");if start<end{return current>=start&&current<end};return current>=start||current<end}
func(a *App) alertState(key,message string,active bool){var previous int;var updated int64;a.db.QueryRow("SELECT active,updated FROM alerts WHERE key=?",key).Scan(&previous,&updated);now:=time.Now().Unix();if active{if previous==0||now-updated>=1800{a.exec("INSERT INTO alerts(key,message,active,updated) VALUES(?,?,1,?) ON CONFLICT(key) DO UPDATE SET message=excluded.message,active=1,updated=excluded.updated,ack_by=CASE WHEN alerts.active=0 THEN '' ELSE alerts.ack_by END,ack_at=CASE WHEN alerts.active=0 THEN 0 ELSE alerts.ack_at END",key,message,now);a.notice("Server alert: "+key,message)}}else if previous==1{a.exec("UPDATE alerts SET active=0,updated=? WHERE key=?",now,key);a.notice("Server recovered: "+key,message)}}
func(a *App) evaluate(m Metric){
 if a.setting("alerts.enabled","true")!="true"{return};var load float64;if f:=strings.Fields(m.Load);len(f)>0{load,_=strconv.ParseFloat(f[0],64)};for _,rule:=range []struct{key string;value,threshold float64}{{"cpu",m.CPU,90},{"memory",m.Memory,90},{"disk",m.Disk,85},{"load",load,float64(runtime.NumCPU()*2)}}{
 threshold:=rule.threshold;if t,e:=strconv.ParseFloat(a.setting("alerts."+rule.key,""),64);e==nil&&t>0&&t<=100{threshold=t}
 now:=time.Now();if rule.value>=threshold{if a.alertSince[rule.key].IsZero(){a.alertSince[rule.key]=now};if now.Sub(a.alertSince[rule.key])<5*time.Minute{continue};unit:="%";if rule.key=="load"{unit=""};a.alertState(rule.key,fmt.Sprintf("%s %.1f%s exceeds %.1f%s",rule.key,rule.value,unit,threshold,unit),true)}else{delete(a.alertSince,rule.key);a.alertState(rule.key,"Resource usage is below the configured threshold.",false)}
 }
}
func(a *App) mailer(){for{select{case <-a.ctx.Done():return;case n:=<-a.notifications:
 for attempt:=0;attempt<3;attempt++{if a.quiet(time.Now()){break};err:=a.sendMail(n);if err==nil{a.audit("system","mail",n.Subject,"sent");break};a.audit("system","mail",n.Subject,"delivery failed");if attempt<2{timer:=time.NewTimer(time.Duration(attempt+1)*10*time.Second);select{case <-a.ctx.Done():timer.Stop();return;case <-timer.C:}}}
 }}}
func(a *App) sendMail(n Notice)error{
 host:=a.setting("smtp.host","");port:=a.setting("smtp.port","587");from,err:=mail.ParseAddress(a.setting("smtp.from",""));if err!=nil{return err};to,err:=mail.ParseAddress(a.setting("smtp.to",""));if err!=nil{return err};if strings.ContainsAny(n.Subject,"\r\n"){return fmt.Errorf("invalid subject")}
 conn,err:=net.DialTimeout("tcp",net.JoinHostPort(host,port),10*time.Second);if err!=nil{return err};defer conn.Close();conn.SetDeadline(time.Now().Add(20*time.Second));c,err:=smtp.NewClient(conn,host);if err!=nil{return err};defer c.Close();if ok,_:=c.Extension("STARTTLS");!ok{return fmt.Errorf("SMTP server must support STARTTLS")};if err=c.StartTLS(&tls.Config{ServerName:host,MinVersion:tls.VersionTLS12});err!=nil{return err}
 user:=a.setting("smtp.user","");if user!=""{password,err:=a.unseal(a.setting("smtp.password",""));if err!=nil{return err};if err=c.Auth(smtp.PlainAuth("",user,string(password),host));err!=nil{return err}}
 if err=c.Mail(from.Address);err!=nil{return err};if err=c.Rcpt(to.Address);err!=nil{return err};writer,err:=c.Data();if err!=nil{return err};_,err=fmt.Fprintf(writer,"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",from.Address,to.Address,mime.QEncoding.Encode("utf-8",n.Subject),n.Body);if err!=nil{writer.Close();return err};if err=writer.Close();err!=nil{return err};return c.Quit()
}
