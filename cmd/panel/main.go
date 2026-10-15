package main

import (
 "context"
 "errors"
 "flag"
 "log"
 "net/http"
 "os"
 "os/exec"
 "os/signal"
 "runtime"
 "runtime/debug"
 "syscall"
 "time"

 "yuanyang/internal/panel"
)

func main() {
 runtime.GOMAXPROCS(1)
 debug.SetMemoryLimit(64 << 20)
 if len(os.Args)>1&&os.Args[1]=="helper"{if err:=panel.Helper(os.Stdin);err!=nil{log.Print(err);var exit *exec.ExitError;if errors.As(err,&exit)&&exit.ExitCode()>0{os.Exit(exit.ExitCode())};os.Exit(1)};return}
 if len(os.Args)>1&&os.Args[1]=="firewall-rollback"{if err:=panel.FirewallRollback();err!=nil{log.Fatal(err)};return}
 if len(os.Args)>1&&os.Args[1]=="gateway-rollback"{if err:=panel.GatewayRollback();err!=nil{log.Fatal(err)};return}
 if len(os.Args)>1&&os.Args[1]=="safety-recover"{if err:=panel.FirewallRollback();err!=nil{log.Fatal(err)};if err:=panel.GatewayRollback();err!=nil{log.Fatal(err)};return}
 if len(os.Args)>1&&os.Args[1]=="recover"{if len(os.Args)!=3{log.Fatal("usage: yuanyang recover DATA_DIRECTORY (password from stdin)")};if err:=panel.Recover(os.Args[2],os.Stdin);err!=nil{log.Fatal(err)};return}
 addr := flag.String("listen", "127.0.0.1:9090", "HTTP listen address; use a TLS reverse proxy")
 data := flag.String("data", "./data", "private data directory")
 root := flag.String("files", "/var/www", "file manager root")
 web := flag.String("web", "./web/dist", "built frontend directory")
 trustProxy:=flag.Bool("trust-local-proxy",false,"trust X-Real-IP from a loopback reverse proxy")
 flag.Parse()
 ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
 defer stop()
 app, err := panel.New(ctx, *data, *root, *web)
 if err != nil { log.Fatal(err) }
 app.TrustLocalProxy(*trustProxy)
 defer app.Close()
 server := &http.Server{Addr:*addr, Handler:app.Handler(), ReadHeaderTimeout:5*time.Second, IdleTimeout:30*time.Second, MaxHeaderBytes:16<<10}
 shutdownDone:=make(chan struct{})
 go func(){ <-ctx.Done(); c,cancel:=context.WithTimeout(context.Background(),5*time.Second); defer cancel();if server.Shutdown(c)!=nil{server.Close()};close(shutdownDone) }()
 log.Printf("Yuanyang listening on %s", *addr)
 if err:=server.ListenAndServe(); err!=nil && err!=http.ErrServerClosed { log.Fatal(err) }
 if ctx.Err()!=nil{<-shutdownDone}
}
