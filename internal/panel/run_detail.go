package panel

import (
 "net/http"
 "strconv"
)
func(a *App) runDetail(w http.ResponseWriter,r *http.Request){if r.Method!="GET"{w.WriteHeader(405);return};id,err:=strconv.ParseInt(r.URL.Query().Get("id"),10,64);if err!=nil{problem(w,400,err);return};var output,status string;if err=a.db.QueryRow("SELECT output,status FROM runs WHERE id=?",id).Scan(&output,&status);err!=nil{problem(w,404,err);return};reply(w,200,map[string]any{"id":id,"output":output,"status":status})}
