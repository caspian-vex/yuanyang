// bench measures authenticated read APIs. Run on a separate machine for CPU measurements.
package main

import (
 "bytes"
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "log"
 "net/http"
 "net/http/cookiejar"
 "os"
 "sort"
 "strings"
 "sync"
 "time"
)

func main(){
 base:=flag.String("url","http://127.0.0.1:9090","panel URL")
 count:=flag.Int("requests",2000,"number of requests, max 100000")
 concurrency:=flag.Int("concurrency",2,"concurrent requests, max 8")
 flag.Parse()
 if *count<1||*count>100000||*concurrency<1||*concurrency>8{log.Fatal("invalid bounds")}
 jar,_:=cookiejar.New(nil)
 client:=&http.Client{Timeout:10*time.Second,Jar:jar,Transport:&http.Transport{MaxIdleConns:8,MaxIdleConnsPerHost:*concurrency,MaxConnsPerHost:*concurrency}}
 target:=strings.TrimRight(*base,"/")
 cookie:=os.Getenv("PANEL_COOKIE")
 if cookie==""{
  username,password:=os.Getenv("PANEL_USER"),os.Getenv("PANEL_PASSWORD")
  if username==""||password==""{log.Fatal("set PANEL_COOKIE or PANEL_USER/PANEL_PASSWORD (and PANEL_TOTP if enabled)")}
  payload,_:=json.Marshal(map[string]string{"Name":username,"Password":password,"Code":os.Getenv("PANEL_TOTP")})
  response,err:=client.Post(target+"/api/login","application/json",bytes.NewReader(payload));if err!=nil{log.Fatal(err)}
  io.Copy(io.Discard,io.LimitReader(response.Body,64<<10));response.Body.Close();if response.StatusCode!=200{log.Fatalf("login failed: HTTP %d",response.StatusCode)}
 }
 endpoints:=[]string{"me","metrics","history?range=1h","host","overview","processes?sort=cpu"}
 jobs:=make(chan int,*concurrency)
 var workers sync.WaitGroup
 var mu sync.Mutex
 latencies:=make([]time.Duration,0,*count)
 failures:=0
 start:=time.Now()
 for i:=0;i<*concurrency;i++{workers.Add(1);go func(){defer workers.Done();for n:=range jobs{
  req,err:=http.NewRequest("GET",target+"/api/"+endpoints[n%len(endpoints)],nil);if err!=nil{log.Fatal(err)};if cookie!=""{req.Header.Set("Cookie",cookie)}
  begin:=time.Now();response,err:=client.Do(req);ok:=err==nil
  if err==nil{_,err=io.Copy(io.Discard,io.LimitReader(response.Body,2<<20));response.Body.Close();ok=response.StatusCode==200&&err==nil}
  elapsed:=time.Since(begin);mu.Lock();if ok{latencies=append(latencies,elapsed)}else{failures++};mu.Unlock()
 }}()}
 for n:=0;n<*count;n++{jobs<-n};close(jobs);workers.Wait()
 sort.Slice(latencies,func(i,j int)bool{return latencies[i]<latencies[j]})
 quantile:=func(percent int)string{if len(latencies)==0{return "unavailable"};index:=(len(latencies)*percent+99)/100-1;return fmt.Sprintf("%.2f ms",float64(latencies[index].Microseconds())/1000)}
 fmt.Printf("requests=%d success=%d failures=%d elapsed=%s\nP50=%s P95=%s P99=%s\n",*count,len(latencies),failures,time.Since(start).Round(time.Millisecond),quantile(50),quantile(95),quantile(99))
 if failures>0||len(latencies)==0||latencies[(len(latencies)*95+99)/100-1]>300*time.Millisecond{os.Exit(1)}
}
