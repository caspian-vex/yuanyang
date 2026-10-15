package panel

import (
 "net/http"
 "runtime"
 "time"
)
func(a *App) diagnostics(w http.ResponseWriter,r *http.Request,u User){if r.Method!="GET"{w.WriteHeader(405);return};if !admin(w,u){return};var memory runtime.MemStats;runtime.ReadMemStats(&memory);a.mu.RLock();metric:=a.metric;points:=len(a.history);a.mu.RUnlock();a.audit(u.Name,"diagnostics.export","redacted","ok");w.Header().Set("Content-Disposition","attachment; filename=yuanyang-diagnostics.json");reply(w,200,map[string]any{"version":"0.1.0","generated":time.Now().Format(time.RFC3339),"host":hostInfo(),"lastMetric":metric,"resources":map[string]any{"heapAllocated":memory.HeapAlloc,"runtimeReserved":memory.Sys,"goroutines":runtime.NumGoroutine(),"historyPoints":points,"jobQueue":len(a.jobs),"notificationQueue":len(a.notifications)},"limits":map[string]any{"goSoftMemory":64<<20,"aggregateMemory":100000000,"requestConcurrency":16,"terminalConcurrency":2,"logConcurrency":2,"taskQueue":32},"redacted":true})}
