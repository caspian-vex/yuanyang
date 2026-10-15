<script setup lang="ts">
// Each mounted component owns one SSH connection.
import { ref,onUnmounted,nextTick } from 'vue'
import { user } from '../api'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
const passphrase=ref('')
const username=ref(''),password=ref(''),privateKey=ref(''),element=ref<HTMLElement>(),connected=ref(false),error=ref('')
let ws:WebSocket|undefined,term:Terminal|undefined,fit:FitAddon|undefined,observer:ResizeObserver|undefined
async function connect(){disconnect();error.value='';await nextTick();term=new Terminal({cursorBlink:true,scrollback:1000,fontFamily:'Consolas, monospace',fontSize:14,theme:{background:'#111b2e',foreground:'#dfe8f5'}});fit=new FitAddon();term.loadAddon(fit);term.open(element.value!);fit.fit();ws=new WebSocket(`${location.protocol==='https:'?'wss':'ws'}://${location.host}/api/terminal`);ws.binaryType='arraybuffer';ws.onopen=()=>{ws!.send(JSON.stringify({Username:username.value,Password:password.value,PrivateKey:privateKey.value,Passphrase:passphrase.value,CSRF:user.value!.csrf,Cols:term!.cols,Rows:term!.rows}));password.value='';privateKey.value='';passphrase.value='';connected.value=true};ws.onmessage=e=>{if(typeof e.data==='string')term?.writeln(e.data);else term?.write(new Uint8Array(e.data))};ws.onclose=()=>{connected.value=false;term?.writeln('\r\n[连接已关闭]')};ws.onerror=()=>error.value='终端连接失败，请检查 SSH 服务及权限';term.onData(d=>{if(ws?.readyState===WebSocket.OPEN)ws.send(new TextEncoder().encode(d))});observer=new ResizeObserver(()=>{fit?.fit();if(ws?.readyState===WebSocket.OPEN)ws.send(JSON.stringify({Cols:term!.cols,Rows:term!.rows}))});observer.observe(element.value!)}
function disconnect(){observer?.disconnect();if(ws){ws.onclose=null;ws.close();ws=undefined}term?.dispose();term=undefined;connected.value=false}
onUnmounted(disconnect)
</script>
<template><section class="card"><div class="notice">终端使用系统账号实际权限。密码与私钥仅用于本次连接，不持久保存；空闲 30 分钟断开。</div><form class="toolbar" @submit.prevent="connect"><input v-model="username" placeholder="SSH 系统用户名" required autocomplete="off"><input v-model="password" placeholder="SSH 密码" type="password" autocomplete="off"><button class="primary" :disabled="connected">连接本机</button><button type="button" @click="disconnect">断开</button></form><details><summary>使用私钥</summary><textarea v-model="privateKey" rows="5" placeholder="粘贴 PEM / OpenSSH 私钥；连接后清空"></textarea><label>私钥口令（加密私钥需要）<input v-model="passphrase" type="password" autocomplete="off"></label></details><p v-if="error" class="error">{{error}}</p><div ref="element" class="terminal"></div></section></template>
