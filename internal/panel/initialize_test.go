package panel

import (
 "bytes"
 "database/sql"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "golang.org/x/crypto/bcrypt"
)

func TestInitializeAdminDoesNotResetExistingAccount(t *testing.T) {
 data:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(data,"setup-token"),[]byte("old-token"),0600);err!=nil{t.Fatal(err)}
 var out bytes.Buffer
 if err:=InitializeAdmin(data,&out);err!=nil{t.Fatal(err)}
 parts:=strings.Split(out.String(),"密码：")
 if len(parts)!=2{t.Fatal("missing password in installation output")}
 password:=strings.SplitN(parts[1],"\n",2)[0]
 if len(password)!=40{t.Fatal("unexpected random password length")}
 db,err:=sql.Open("sqlite",filepath.Join(data,"panel.db"));if err!=nil{t.Fatal(err)};defer db.Close()
 var name,hash,role string
 if err=db.QueryRow("SELECT name,password,role FROM users").Scan(&name,&hash,&role);err!=nil{t.Fatal(err)}
 if name!="admin"||role!="admin"||bcrypt.CompareHashAndPassword([]byte(hash),[]byte(password))!=nil{t.Fatal("administrator not initialized correctly")}
 if _,err=os.Stat(filepath.Join(data,"setup-token"));!os.IsNotExist(err){t.Fatal("legacy setup token retained")}
 out.Reset()
 if err=InitializeAdmin(data,&out);err!=nil{t.Fatal(err)}
 var after string
 if err=db.QueryRow("SELECT password FROM users").Scan(&after);err!=nil{t.Fatal(err)}
 if hash!=after||strings.Contains(out.String(),password){t.Fatal("upgrade reset or disclosed existing password")}
}

func TestBrowserCannotCreateFirstAdministrator(t *testing.T) {
 app:=testApp(t)
 r:=httptest.NewRequest("POST","/api/setup",strings.NewReader(`{"Name":"attacker","Password":"password123456"}`))
 r.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder();app.Handler().ServeHTTP(w,r)
 if w.Code<400{t.Fatal("browser initialization endpoint is still enabled")}
 var count int
 if err:=app.db.QueryRow("SELECT count(*) FROM users").Scan(&count);err!=nil{t.Fatal(err)}
 if count!=0{t.Fatal("browser created an administrator")}
 if _,err:=os.Stat(filepath.Join(app.data,"setup-token"));!os.IsNotExist(err){t.Fatal("server generated a setup token")}
}
