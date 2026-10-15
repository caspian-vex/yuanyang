import { ref } from 'vue'
export interface User { id:number;name:string;role:'admin'|'operator'|'viewer';csrf:string;terminal:boolean }
export const user=ref<User|null>(null)
export async function api<T=any>(path:string,options:RequestInit={}):Promise<T>{
 const headers=new Headers(options.headers)
 if(options.body&&typeof options.body==='string')headers.set('Content-Type','application/json')
 if(user.value)headers.set('X-CSRF-Token',user.value.csrf)
 const r=await fetch('/api/'+path,{...options,headers,credentials:'same-origin'})
 const data=await r.json().catch(()=>({error:`HTTP ${r.status}`}))
 if(!r.ok){if(r.status===401)user.value=null;throw new Error(data.error||`HTTP ${r.status}`)}
 return data
}
export function post<T=any>(path:string,data:unknown){return api<T>(path,{method:'POST',body:JSON.stringify(data)})}
export const bytes=(n:number)=>n>=1073741824?(n/1073741824).toFixed(1)+' GB':n>=1048576?(n/1048576).toFixed(1)+' MB':n>=1024?(n/1024).toFixed(1)+' KB':n+' B'
