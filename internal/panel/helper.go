package panel

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/url"
 "os"
 "os/exec"
 "os/user"
 "path/filepath"
 "regexp"
 "strings"
 "strconv"
 "time"
)
type HelperRequest struct{Operation,Name,Action,Config string;Timeout int;Certificate,PrivateKey string;Port int;Protocol,Source,Policy,Workdir string;TaskID int64}
var domainName=regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$`)
var siteName=regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
type Site struct{ID int64 `json:"id"`;Name string `json:"name"`;Domains []string `json:"domains"`;Kind string `json:"kind"`;Root string `json:"root"`;Upstream string `json:"upstream"`;Enabled bool `json:"enabled"`;HTTPS bool `json:"https"`;Redirect bool `json:"redirect"`;NotFound string `json:"notFound"`}
func validateSite(s Site)error{
 if !siteName.MatchString(s.Name){return errors.New("invalid site name")};if len(s.Domains)==0||len(s.Domains)>20{return errors.New("1–20 domains required")};seen:=map[string]bool{};for _,d:=range s.Domains{if !domainName.MatchString(d)||len(d)>253{return errors.New("invalid domain: "+d)};lower:=strings.ToLower(d);if seen[lower]{return errors.New("duplicate domain: "+d)};seen[lower]=true}
 if s.NotFound!=""&&(!regexp.MustCompile(`^/[a-zA-Z0-9_./-]{1,200}$`).MatchString(s.NotFound)||strings.Contains(s.NotFound,"..")){return errors.New("invalid 404 URI")}
 if s.Kind=="static"{if !strings.HasPrefix(s.Root,"/var/www/")||strings.Contains(s.Root,"..")||strings.ContainsAny(s.Root,"\n\r\";{}$\\ \t"){return errors.New("root must be a plain path under /var/www")}}else if s.Kind=="proxy"{u,err:=url.Parse(s.Upstream);if err!=nil||u.User!=nil||u.Hostname()==""||(u.Scheme!="http"&&u.Scheme!="https")||u.RawQuery!=""||u.Fragment!=""||strings.ContainsAny(s.Upstream,"\n\r\";{}$\\ \t"){return errors.New("invalid HTTP upstream")};if u.Port()!=""{if _,err=net.LookupPort("tcp",u.Port());err!=nil{return err}}}else{return errors.New("invalid site type")};return nil
}
func renderSite(s Site)string{
 if !s.Enabled{return "# disabled\n"};conf:="server {\n listen 80;\n server_name "+strings.Join(s.Domains," ")+";\n access_log /var/log/nginx/yuanyang-"+s.Name+".access.log;\n error_log /var/log/nginx/yuanyang-"+s.Name+".error.log;\n location ^~ /.well-known/acme-challenge/ { root /var/lib/yuanyang/acme; }\n"
 if s.HTTPS{if s.Redirect{conf+=" location / { return 301 https://$host$request_uri; }\n}\nserver {\n server_name "+strings.Join(s.Domains," ")+";\n"};conf+=" listen 443 ssl;\n ssl_certificate /etc/yuanyang/certificates/"+s.Name+".crt;\n ssl_certificate_key /etc/yuanyang/certificates/"+s.Name+".key;\n ssl_protocols TLSv1.2 TLSv1.3;\n"}
 if s.NotFound!=""{conf+=" error_page 404 "+s.NotFound+";\n"};if s.Kind=="static"{conf+=" root "+s.Root+";\n index index.html index.htm;\n location / { try_files $uri $uri/ =404; }\n"}else{conf+=" location / { proxy_pass "+s.Upstream+"; proxy_set_header Host $http_host; proxy_set_header X-Real-IP $remote_addr; proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for; proxy_set_header X-Forwarded-Proto $scheme; proxy_http_version 1.1; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection \"upgrade\"; }\n"};return conf+"}\n"
}
func Helper(stdin io.Reader)error{
 if os.Geteuid()!=0{return errors.New("helper requires root")}
 var in HelperRequest;d:=json.NewDecoder(io.LimitReader(stdin,64<<10));d.DisallowUnknownFields();if err:=d.Decode(&in);err!=nil{return err}
 timeout:=45*time.Second;if in.Operation=="ports"{timeout=3*time.Second};if in.Operation=="task"&&in.Timeout>0&&in.Timeout<=900{timeout=time.Duration(in.Timeout)*time.Second};ctx,cancel:=context.WithTimeout(context.Background(),timeout);defer cancel()
 run:=func(name string,args ...string)error{c:=exec.CommandContext(ctx,name,args...);c.Env=append(os.Environ(),"LC_ALL=C","LANG=C");c.Stdout=os.Stdout;c.Stderr=os.Stderr;return c.Run()}
 switch in.Operation{
 case "gateway":return gatewayHelper(ctx,in)
 case "ports":return json.NewEncoder(os.Stdout).Encode(readPorts(ctx))
 case "reboot":return run("/usr/bin/systemd-run","--unit=yuanyang-reboot","--on-active=5s","/usr/bin/systemctl","reboot")
 case "terminate":pid,err:=strconv.Atoi(in.Name);if err!=nil||pid<=1{return errors.New("invalid process")};name:=readFile("/proc/"+in.Name+"/comm");if name==""{return errors.New("process not found")};for _,v:=range []string{"sshd","systemd","nginx","yuanyang","sudo"}{if strings.Contains(name,v){return errors.New("protected process")}};return terminateProcess(pid)
 case "firewall":return firewallHelper(ctx,in)
 case "certificate":return installCertificate(in)
 case "task":
 if !accountName.MatchString(in.Name)||in.Name=="root"||in.Name=="yuanyang"||in.TaskID<=0||len(in.Config)>8192{return errors.New("invalid task account or command")};account,err:=user.Lookup(in.Name);if err!=nil||account.Uid=="0"{return errors.New("task account must exist and cannot be root")};working:=account.HomeDir;if in.Workdir!=""{if !filepath.IsAbs(in.Workdir)||strings.ContainsRune(in.Workdir,0){return errors.New("invalid task working directory")};working=in.Workdir};if st,e:=os.Stat(working);e!=nil||!st.IsDir(){if in.Workdir!=""{return errors.New("working directory does not exist")};working="/tmp"};cmd:=exec.CommandContext(ctx,"/usr/bin/systemd-run","--quiet","--wait","--pipe","--collect","--unit=yuanyang-task-"+strconv.FormatInt(in.TaskID,10),"--property=User="+in.Name,"--property=WorkingDirectory="+working,"--property=MemoryMax=128M","--property=CPUQuota=50%","--property=TasksMax=32","--property=RuntimeMaxSec="+strconv.Itoa(in.Timeout),"--property=NoNewPrivileges=yes","--property=KillMode=control-group","/bin/sh","-c",in.Config);cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;configureProcessGroup(cmd);return cmd.Run()
 case "service":if !serviceName.MatchString(in.Name)||protectedService(in.Name){return errors.New("protected or invalid service")};switch in.Action{case "start","stop","restart","enable","disable":return run("/usr/bin/systemctl",in.Action,in.Name);default:return errors.New("invalid action")}
 case "site":
 var s Site;if err:=json.Unmarshal([]byte(in.Config),&s);err!=nil{return err};if err:=validateSite(s);err!=nil{return err}
 if s.Enabled&&s.HTTPS&&in.Action!="delete"{cert,err:=os.ReadFile("/etc/yuanyang/certificates/"+s.Name+".crt");if err!=nil{return err};key,err:=os.ReadFile("/etc/yuanyang/certificates/"+s.Name+".key");if err!=nil{return err};c,err:=validateCertificate(string(cert),string(key));if err!=nil{return err};for _,d:=range s.Domains{if err=c.VerifyHostname(d);err!=nil{return err}}}
 dir:="/etc/nginx/conf.d";path:=filepath.Join(dir,"yuanyang-"+s.Name+".conf");old,oldErr:=os.ReadFile(path);if oldErr!=nil&&!os.IsNotExist(oldErr){return oldErr};if in.Action=="create"&&oldErr==nil{return errors.New("configuration already exists; refusing to overwrite")};if in.Action=="delete"{if err:=os.Remove(path);err!=nil&&!os.IsNotExist(err){return err}}else{if err:=writeAtomic(path,[]byte(renderSite(s)),0644);err!=nil{return err}}
 rollback:=func(){if oldErr==nil{writeAtomic(path,old,0644)}else{os.Remove(path)}}
 validation,err:=execCombined(ctx,"/usr/sbin/nginx","-t");if err!=nil||strings.Contains(validation,"conflicting server name"){rollback();return fmt.Errorf("configuration rejected: %s",validation)}
 if err:=run("/usr/bin/systemctl","reload","nginx");err!=nil{rollback();run("/usr/bin/systemctl","reload","nginx");return err};return nil
 default:return errors.New("unsupported privileged operation")
 }
}
