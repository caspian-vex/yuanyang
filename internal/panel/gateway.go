package panel

import (
 "context"
 "crypto/sha256"
 "crypto/tls"
 "crypto/x509"
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

const gatewayDir="/var/lib/yuanyang-gateway"
const gatewayConfig="/etc/nginx/conf.d/00-yuanyang-panel.conf"
const gatewayBackupPath=gatewayDir+"/backup.json"
type GatewayState struct{Domain string `json:"domain"`;Port int `json:"port"`;PendingUntil int64 `json:"pendingUntil"`;CertificateName string `json:"certificateName"`}
type GatewayInput struct{Action,Domain,Certificate,PrivateKey,Password,Code,CertificateName string;Port int;Confirm bool}
type gatewayBackup struct{Files map[string][]byte;Exists map[string]bool}
var gatewayFiles=[]string{gatewayConfig,"/etc/yuanyang/panel.crt","/etc/yuanyang/panel.key",gatewayDir+"/state.json"}

func(a *App) gateway(w http.ResponseWriter,r *http.Request,u User){
 if !admin(w,u){return};action:="status";input:=GatewayInput{}
 if r.Method=="POST"{if !body(w,r,&input){return};action=input.Action;if action!="apply"&&action!="confirm"{problem(w,400,errors.New("invalid gateway action"));return};if action=="apply"&&(!input.Confirm||!a.verify(u,input.Password,input.Code)){problem(w,403,errors.New("identity verification and confirmation required"));return};input.Password="";input.Code=""}else if r.Method!="GET"{w.WriteHeader(405);return}
 out,err:=a.privileged(r.Context(),HelperRequest{Operation:"gateway",Action:action,Config:encode(input)});if r.Method=="POST"{a.audit(u.Name,"gateway."+action,input.Domain,result(err))};if err!=nil{problem(w,400,fmt.Errorf("%s: %w",out,err));return};var state GatewayState;if err=json.Unmarshal([]byte(out),&state);err!=nil{problem(w,500,err);return};reply(w,200,state)
}
func gatewayHelper(ctx context.Context,request HelperRequest)error{
 unlock,err:=lockFirewall();if err!=nil{return err};defer unlock()
 state:=GatewayState{};decodeJSON(readFile(gatewayDir+"/state.json"),&state)
 if request.Action=="status"{return json.NewEncoder(os.Stdout).Encode(state)}
 if request.Action=="confirm"{if _,err=os.Stat(gatewayBackupPath);err!=nil{return errors.New("no pending gateway change")};if time.Now().Unix()>=state.PendingUntil{return errors.New("confirmation window expired")};state.PendingUntil=0;if err=writeAtomic(gatewayDir+"/state.json",[]byte(encode(state)),0600);err!=nil{return err};if err=os.Remove(gatewayBackupPath);err!=nil{return err};execCombined(ctx,"/usr/bin/systemctl","stop","yuanyang-gateway-rollback.timer");return json.NewEncoder(os.Stdout).Encode(state)}
 if request.Action!="apply"{return errors.New("invalid gateway operation")};var input GatewayInput;if err=decodeJSON(request.Config,&input);err!=nil{return err};input.Domain=strings.ToLower(strings.TrimSpace(input.Domain));if !domainName.MatchString(input.Domain)||len(input.Domain)>253||input.Port<1||input.Port>65535||input.Port==9090{return errors.New("valid domain and TLS port other than 9090 required")}
 if _,err=os.Stat(gatewayBackupPath);err==nil{return errors.New("confirm or wait for the previous gateway change")}
 cert,key:=input.Certificate,input.PrivateKey;if input.CertificateName!=""{if !siteName.MatchString(input.CertificateName){return errors.New("invalid certificate name")};cert=readFile("/etc/yuanyang/certificates/"+input.CertificateName+".crt");key=readFile("/etc/yuanyang/certificates/"+input.CertificateName+".key")}else if cert==""&&key==""{cert=readFile("/etc/yuanyang/panel.crt");key=readFile("/etc/yuanyang/panel.key")};parsed,err:=validateCertificate(cert,key);if err!=nil{return err};if err=parsed.VerifyHostname(input.Domain);err!=nil{return err}
 if err=os.MkdirAll(gatewayDir,0700);err!=nil{return err};if err=os.MkdirAll("/etc/yuanyang",0700);err!=nil{return err};backup:=gatewayBackup{Files:map[string][]byte{},Exists:map[string]bool{}};for _,p:=range gatewayFiles{b,e:=os.ReadFile(p);if e!=nil&&!os.IsNotExist(e){return e};backup.Exists[p]=e==nil;backup.Files[p]=b};if err=writeAtomic(gatewayBackupPath,[]byte(encode(backup)),0600);err!=nil{return err}
 exe,_:=os.Executable();if _,err=execCombined(ctx,"/usr/bin/systemd-run","--unit=yuanyang-gateway-rollback","--property=Slice=yuanyang.slice","--on-active=60s","--timer-property=AccuracySec=1s",exe,"gateway-rollback");err!=nil{os.Remove(gatewayBackupPath);return err}
 applied:=false;defer func(){if !applied{if restoreGateway()==nil{stopCtx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel();execCombined(stopCtx,"/usr/bin/systemctl","stop","yuanyang-gateway-rollback.timer")}}}()
 state=GatewayState{Domain:input.Domain,Port:input.Port,PendingUntil:time.Now().Add(60*time.Second).Unix(),CertificateName:input.CertificateName}
 if err=writeAtomic("/etc/yuanyang/panel.key",[]byte(key),0600);err!=nil{return err};if err=writeAtomic("/etc/yuanyang/panel.crt",[]byte(cert),0644);err!=nil{return err};if err=writeAtomic(gatewayConfig,[]byte(renderGateway(state)),0644);err!=nil{return err}
 out,err:=execCombined(ctx,"/usr/sbin/nginx","-t");if err!=nil||strings.Contains(out,"conflicting server name"){return fmt.Errorf("gateway configuration rejected: %s",out)};if _,err=execCombined(ctx,"/usr/bin/systemctl","reload","nginx");err!=nil{return err}
 pool:=x509.NewCertPool();pool.AppendCertsFromPEM([]byte(cert));wanted:=sha256.Sum256(parsed.Raw);verified:=false
 for i:=0;i<5;i++{dialer:=&net.Dialer{Timeout:time.Second};connection,e:=tls.DialWithDialer(dialer,"tcp",net.JoinHostPort("127.0.0.1",strconv.Itoa(input.Port)),&tls.Config{ServerName:input.Domain,RootCAs:pool,MinVersion:tls.VersionTLS12});if e==nil{remote:=connection.ConnectionState().PeerCertificates;if len(remote)>0&&sha256.Sum256(remote[0].Raw)==wanted{verified=true};connection.Close()};if verified{break};select{case <-ctx.Done():return ctx.Err();case <-time.After(200*time.Millisecond):}}
 if !verified{return errors.New("new TLS listener did not become ready; restoring previous configuration")};if err=writeAtomic(gatewayDir+"/state.json",[]byte(encode(state)),0600);err!=nil{return err};applied=true;return json.NewEncoder(os.Stdout).Encode(state)
}
func renderGateway(s GatewayState)string{cert,key:="/etc/yuanyang/panel.crt","/etc/yuanyang/panel.key";if s.CertificateName!=""{cert="/etc/yuanyang/certificates/"+s.CertificateName+".crt";key="/etc/yuanyang/certificates/"+s.CertificateName+".key"};return fmt.Sprintf(`server {
 listen %d ssl;
 server_name %s;
 ssl_certificate %s;
 ssl_certificate_key %s;
 ssl_protocols TLSv1.2 TLSv1.3;
 client_max_body_size 100m;
 location / {
  proxy_pass http://127.0.0.1:9090;
  proxy_http_version 1.1;
  proxy_set_header Host $http_host;
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-Proto https;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection "upgrade";
  proxy_buffering off;
  proxy_request_buffering off;
  proxy_read_timeout 241m;
  proxy_send_timeout 241m;
 }
}
`,s.Port,s.Domain,cert,key)}
func restoreGateway()error{b,err:=os.ReadFile(gatewayBackupPath);if os.IsNotExist(err){return nil};if err!=nil{return err};var backup gatewayBackup;if err=decodeJSON(string(b),&backup);err!=nil{return err};for _,p:=range gatewayFiles{if _,ok:=backup.Exists[p];!ok{return errors.New("incomplete gateway snapshot")};if backup.Exists[p]{mode:=os.FileMode(0600);if p==gatewayConfig||filepath.Ext(p)==".crt"{mode=0644};if err=writeAtomic(p,backup.Files[p],mode);err!=nil{return err}}else if err=os.Remove(p);err!=nil&&!os.IsNotExist(err){return err}}
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel();if _,err=execCombined(ctx,"/usr/sbin/nginx","-t");err!=nil{return err};if _,err=execCombined(ctx,"/usr/bin/systemctl","reload-or-restart","nginx");err!=nil{return err};return os.Remove(gatewayBackupPath)
}
func GatewayRollback()error{if os.Geteuid()!=0{return errors.New("root required")};unlock,err:=lockFirewall();if err!=nil{return err};defer unlock();return restoreGateway()}
