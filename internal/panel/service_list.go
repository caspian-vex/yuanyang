package panel

import (
 "context"
 "errors"
 "net/http"
 "sort"
 "strings"
 "time"
)
type Service struct{Name string `json:"name"`;State string `json:"state"`;Sub string `json:"sub"`;Startup string `json:"startup"`;Description string `json:"description"`}
func(a *App) serviceList(w http.ResponseWriter,r *http.Request){
 a.serviceMu.Lock();defer a.serviceMu.Unlock()
 if time.Since(a.serviceTime)>10*time.Second{ctx,cancel:=context.WithTimeout(r.Context(),3*time.Second);defer cancel();out,err:=a.run(ctx,"systemctl","list-units","--type=service","--all","--no-pager","--plain","--no-legend");if err!=nil{problem(w,503,err);return};files,err:=a.run(ctx,"systemctl","list-unit-files","--type=service","--no-pager","--no-legend");if err!=nil{problem(w,503,err);return};if strings.Contains(out,"[output truncated]")||strings.Contains(files,"[output truncated]"){problem(w,503,errors.New("service list exceeds bounded command output"));return}
 items:=map[string]Service{};for _,line:=range strings.Split(out,"\n"){f:=strings.Fields(line);if len(f)>0&&f[0]=="●"{f=f[1:]};if len(f)<4||!serviceName.MatchString(f[0]){continue};items[f[0]]=Service{Name:f[0],State:f[2],Sub:f[3],Startup:"unknown",Description:strings.Join(f[4:]," ")}}
 for _,line:=range strings.Split(files,"\n"){f:=strings.Fields(line);if len(f)<2||!serviceName.MatchString(f[0]){continue};item,ok:=items[f[0]];if !ok{item=Service{Name:f[0],State:"not-loaded",Sub:"-"}};item.Startup=f[1];items[f[0]]=item}
 list:=make([]Service,0,len(items));for _,item:=range items{list=append(list,item)};sort.Slice(list,func(i,j int)bool{return list[i].Name<list[j].Name});a.serviceCache=list;a.serviceTime=time.Now()
 }
 limit,offset:=page(r);query:=r.URL.Query().Get("q");out:=[]Service{};seen:=0;for _,item:=range a.serviceCache{if query!=""&&!strings.Contains(item.Name,query)&&!strings.Contains(item.Description,query){continue};if seen<offset{seen++;continue};out=append(out,item);if len(out)>=limit{break}};reply(w,200,map[string]any{"items":out})
}
