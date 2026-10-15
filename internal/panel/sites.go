package panel

import (
 "errors"
 "fmt"
 "net/http"
 "strings"
)
func(a *App) sites(w http.ResponseWriter,r *http.Request,u User){
 if r.Method=="GET"{limit,offset:=page(r);rows,err:=a.db.Query("SELECT id,name,domains,kind,root,upstream,enabled,https,redirect,not_found FROM sites ORDER BY id DESC LIMIT ? OFFSET ?",limit,offset);if err!=nil{problem(w,500,err);return};defer rows.Close();items:=[]Site{};for rows.Next(){var s Site;var domains string;if rows.Scan(&s.ID,&s.Name,&domains,&s.Kind,&s.Root,&s.Upstream,&s.Enabled,&s.HTTPS,&s.Redirect,&s.NotFound)==nil{decodeJSON(domains,&s.Domains);items=append(items,s)}};reply(w,200,items);return}
 if r.Method!="POST"&&r.Method!="DELETE"{w.WriteHeader(405);return}
 var s Site;if !body(w,r,&s){return};if err:=validateSite(s);err!=nil{problem(w,400,err);return}
 a.mutations.Lock();defer a.mutations.Unlock()
 if s.ID==0{var count int;a.db.QueryRow("SELECT count(*) FROM sites").Scan(&count);if count>=200{problem(w,409,errors.New("single-machine site limit is 200"));return};var same int;a.db.QueryRow("SELECT count(*) FROM sites WHERE name=?",s.Name).Scan(&same);if same>0{problem(w,409,errors.New("site name already exists"));return}}
 var original Site;var domains string
 if s.ID!=0{err:=a.db.QueryRow("SELECT id,name,domains,kind,root,upstream,enabled,https,redirect,not_found FROM sites WHERE id=?",s.ID).Scan(&original.ID,&original.Name,&domains,&original.Kind,&original.Root,&original.Upstream,&original.Enabled,&original.HTTPS,&original.Redirect,&original.NotFound);if err!=nil{problem(w,404,errors.New("site not found"));return};decodeJSON(domains,&original.Domains);if s.Name!=original.Name{problem(w,400,errors.New("site name is immutable"));return}}
 if r.Method=="DELETE"&&s.ID==0{problem(w,400,errors.New("site id required"));return}
 if r.Method!="DELETE"{
 rows,err:=a.db.Query("SELECT name,domains FROM sites WHERE id<>?",s.ID);if err!=nil{problem(w,500,err);return};for rows.Next(){var name,raw string;rows.Scan(&name,&raw);var ds []string;decodeJSON(raw,&ds);for _,d:=range ds{for _,n:=range s.Domains{if strings.EqualFold(d,n){rows.Close();problem(w,409,errors.New("domain already used by "+name));return}}}};rows.Close()
 }
 action:="apply";if s.ID==0{action="create"};if r.Method=="DELETE"{action="delete"}
 out,err:=a.privileged(r.Context(),HelperRequest{Operation:"site",Action:action,Config:encode(s)})
 if err!=nil{a.audit(u.Name,"site."+action,s.Name,result(err));problem(w,400,fmt.Errorf("%s: %w",out,err));return}
 if r.Method=="DELETE"{err=a.exec("DELETE FROM sites WHERE id=?",s.ID)}else if s.ID==0{err=a.exec("INSERT INTO sites(name,domains,kind,root,upstream,enabled,https,redirect,not_found) VALUES(?,?,?,?,?,?,?,?,?)",s.Name,encode(s.Domains),s.Kind,s.Root,s.Upstream,s.Enabled,s.HTTPS,s.Redirect,s.NotFound)}else{err=a.exec("UPDATE sites SET domains=?,kind=?,root=?,upstream=?,enabled=?,https=?,redirect=?,not_found=? WHERE id=?",encode(s.Domains),s.Kind,s.Root,s.Upstream,s.Enabled,s.HTTPS,s.Redirect,s.NotFound,s.ID)}
 if err!=nil{rollback:=original;rollbackAction:="apply";if s.ID==0{rollback=s;rollbackAction="delete"};a.privileged(r.Context(),HelperRequest{Operation:"site",Action:rollbackAction,Config:encode(rollback)})};a.audit(u.Name,"site."+action,s.Name,result(err));if err!=nil{problem(w,500,err);return};reply(w,200,map[string]bool{"ok":true})
}
