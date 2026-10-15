<script setup lang="ts">
import { ref } from 'vue'
import TerminalSession from './TerminalSession.vue'
const tabs=ref([1]),active=ref(1)
function add(){if(tabs.value.length<2){const id=Math.max(...tabs.value)+1;tabs.value.push(id);active.value=id}}
function close(id:number){tabs.value=tabs.value.filter(t=>t!==id);if(!tabs.value.length)tabs.value=[id+1];active.value=tabs.value[0]}
</script>
<template><div class="toolbar"><template v-for="tab in tabs" :key="tab"><button :class="{primary:active===tab}" @click="active=tab">终端 {{tab}}</button><button class="quiet" @click="close(tab)">×</button></template><button :disabled="tabs.length>=2" @click="add">＋ 新终端</button></div><TerminalSession v-for="tab in tabs" v-show="active===tab" :key="tab"/></template>
