package panel

import (
 "context"
 _ "embed"
 "database/sql"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "sync"
 "sync/atomic"
 "time"

 _ "modernc.org/sqlite"
)

//go:embed migrations/001.sql
var initialSchema string

type App struct {
 db *sql.DB
 writeQueue chan writeRequest
 mutations sync.Mutex
 root *os.Root
 rootPath, web, data string
 ctx context.Context
 stop context.CancelFunc
 workers sync.WaitGroup
 commands chan struct{}
 uploads chan struct{}
 terminals chan struct{}
 logs chan struct{}
 requests chan struct{}
 logins chan struct{}
 configMu sync.RWMutex
 config map[string]string
 fileMu sync.RWMutex
 fileWrites sync.Mutex
 trustProxy bool
 serviceMu sync.Mutex
 serviceCache []Service
 serviceTime time.Time
 portMu sync.Mutex
 portCache []Port
 portTime time.Time
 overviewMu sync.Mutex
 overviewCache map[string]int
 overviewTime time.Time
 jobs chan Task
 jobMu sync.Mutex
 running map[int64]bool
 alertSince map[string]time.Time
 notifications chan Notice
 mu sync.RWMutex
 metric Metric
 processes []Process
 history []Metric
 failures map[string]*failure
}
type failure struct { Count int; Until time.Time }

func New(ctx context.Context,data,root,web string)(*App,error){
 if err:=os.MkdirAll(data,0700);err!=nil{return nil,err}
 db,err:=sql.Open("sqlite",filepath.Join(data,"panel.db"));if err!=nil{return nil,err}
 db.SetMaxOpenConns(1)
 a:=&App{db:db,writeQueue:make(chan writeRequest,64),ctx:ctx,data:data,web:web,rootPath:root,commands:make(chan struct{},1),uploads:make(chan struct{},1),terminals:make(chan struct{},2),logs:make(chan struct{},2),requests:make(chan struct{},16),logins:make(chan struct{},2),failures:map[string]*failure{},history:make([]Metric,0,360),jobs:make(chan Task,32),running:map[int64]bool{},alertSince:map[string]time.Time{},notifications:make(chan Notice,64)}
 _,err=db.Exec(initialSchema)
 if err!=nil{db.Close();return nil,err}
 if err=os.Chmod(filepath.Join(data,"panel.db"),0600);err!=nil{db.Close();return nil,err}
 if _,err=a.key();err!=nil{db.Close();return nil,err}
 a.config=map[string]string{};rows,e:=db.Query("SELECT key,value FROM settings WHERE key NOT LIKE 'totp-%'");if e!=nil{db.Close();return nil,e};for rows.Next(){var k,v string;if e=rows.Scan(&k,&v);e!=nil{rows.Close();db.Close();return nil,e};a.config[k]=v};rows.Close()
 root=a.setting("files.root",root);a.rootPath=root
 a.root,err=os.OpenRoot(root)
 if err!=nil{db.Close();return nil,errors.New("file root must exist: "+root)}
 a.ctx,a.stop=context.WithCancel(ctx)
 for _,work:=range []func(){a.writeWorker,a.collect,a.schedule,a.worker,a.mailer,a.renewCertificates,a.watchServices}{a.workers.Add(1);go func(fn func()){defer a.workers.Done();fn()}(work)}
 return a,nil
}
func(a *App) Close(){if a.stop!=nil{a.stop()};a.workers.Wait();a.fileMu.Lock();if a.root!=nil{a.root.Close()};a.fileMu.Unlock();a.db.Close()}
type writeResult struct{id int64;err error}
type writeRequest struct{run func()writeResult;done chan writeResult;state *atomic.Int32}
func(a *App) write(run func()writeResult)writeResult{if a.ctx.Err()!=nil{return writeResult{err:a.ctx.Err()}};request:=writeRequest{run:run,done:make(chan writeResult,1),state:new(atomic.Int32)};select{case a.writeQueue<-request:case <-a.ctx.Done():return writeResult{err:a.ctx.Err()};default:return writeResult{err:errors.New("database write queue full")}};select{case result:=<-request.done:return result;case <-a.ctx.Done():if request.state.CompareAndSwap(0,3){return writeResult{err:a.ctx.Err()}};return <-request.done}}
func(a *App) writeWorker(){for{select{case <-a.ctx.Done():return;case request:=<-a.writeQueue:if request.state.CompareAndSwap(0,1){result:=request.run();request.state.Store(2);request.done<-result}}}}
func(a *App) exec(q string,args ...any)error{return a.write(func()writeResult{_,err:=a.db.ExecContext(a.ctx,q,args...);return writeResult{err:err}}).err}
func(a *App) insert(q string,args ...any)(int64,error){result:=a.write(func()writeResult{res,err:=a.db.ExecContext(a.ctx,q,args...);if err!=nil{return writeResult{err:err}};id,err:=res.LastInsertId();return writeResult{id:id,err:err}});return result.id,result.err}
func(a *App) transaction(fn func(*sql.Tx)error)error{return a.write(func()writeResult{tx,err:=a.db.BeginTx(a.ctx,nil);if err!=nil{return writeResult{err:err}};defer tx.Rollback();if err=fn(tx);err==nil{err=tx.Commit()};return writeResult{err:err}}).err}
func(a *App) audit(user,action,target,result string){a.exec("INSERT INTO audit(time,user,action,target,result) VALUES(?,?,?,?,?)",time.Now().Unix(),user,action,target,result)}
func encode(v any)string{b,_:=json.Marshal(v);return string(b)}
func(a *App) TrustLocalProxy(enabled bool){a.trustProxy=enabled}
