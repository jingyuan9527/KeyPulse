package report

// pageTemplate 有三种形态，共用同一份 CSS / 渲染逻辑：
//   - 普通静态页：内嵌历史 JSON，60 秒整页刷新；
//   - 实时静态页：额外从同目录 live.js 每秒读今日快照（经典 script 标签）；
//   - 本地看板页：由 127.0.0.1 回环服务直出，用 SSE 收快照与键位位图，
//     页面内可一键开关，今日视图不整页重载。
//
// 三者都不引 CDN、不联网、不暴露隐私；只有看板页会连本机回环。
// 注意：Go 原始字符串用反引号，因此下面的 JS 一律不用模板字符串。
const dataPlaceholder = "__KP_DATA__"
const livePlaceholder = "__KP_LIVE__"
const serverPlaceholder = "__KP_SERVER__"
const tokenPlaceholder = "__KP_TOKEN__"
const refreshPlaceholder = "__KP_REFRESH__"
const netNotePlaceholder = "__KP_NET_NOTE__"
const footerNotePlaceholder = "__KP_FOOTER_NOTE__"

const pageTemplate = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta http-equiv="Cache-Control" content="no-store">
<meta http-equiv="Pragma" content="no-cache">
__KP_REFRESH__
<title>KeyPulse 输入统计</title>
<style>
:root{--bg:#f7f8fa;--card:#fff;--line:#e6e8ec;--text:#1f2328;--muted:#6b7280;--accent:#185fa5;--l1:#e6f1fb;--l2:#b5d4f4;--l3:#85b7eb;--l4:#378add;--l5:#185fa5;--soft:#f0f1f3;--warn:#b45309}
*{box-sizing:border-box}
body{margin:0;padding:26px 32px 44px;background:var(--bg);color:var(--text);font:14px/1.6 -apple-system,"Segoe UI","Microsoft YaHei",sans-serif}
.wrap{max-width:1080px;margin:0 auto}
header{display:flex;align-items:baseline;gap:14px;margin-bottom:16px;flex-wrap:wrap}
h1{font-size:19px;font-weight:500;margin:0}
.sub{color:var(--muted);font-size:13px}
nav{display:flex;align-items:center;gap:8px;margin-bottom:16px;flex-wrap:wrap;background:var(--card);border:1px solid var(--line);border-radius:10px;padding:8px 10px}
nav button{font:inherit;font-size:13px;border:1px solid transparent;background:transparent;color:var(--muted);padding:5px 13px;border-radius:7px;cursor:pointer}
nav button:hover{background:var(--soft)}
nav button.on{background:var(--l1);color:var(--accent);border-color:var(--l2);font-weight:500}
.spacer{flex:1}
.picker{display:flex;align-items:center;gap:7px;font-size:12px;color:var(--muted)}
.picker select{font:inherit;font-size:12px;padding:4px 8px;border:1px solid var(--line);border-radius:6px;background:#fff;color:var(--text);max-width:230px}
.picker .navbtn{font:inherit;font-size:12px;padding:4px 9px;border:1px solid var(--line);border-radius:6px;background:#fff;color:var(--text);cursor:pointer}
.picker .navbtn:hover{background:var(--l1)}
.picker .navbtn:disabled{opacity:.38;cursor:default}
.cards{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-bottom:14px}
.card{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:14px 18px}
.card .k{color:var(--muted);font-size:12px}
.card .v{font-size:26px;font-weight:500;font-variant-numeric:tabular-nums;letter-spacing:-.4px;margin-top:2px}
.card .foot{font-size:11px;color:var(--muted);margin-top:3px;min-height:18px}
.up{color:var(--accent);font-weight:500}
.down{color:var(--warn);font-weight:500}
.flat{color:var(--muted)}
.summary{border-left:3px solid var(--accent)}
.summary h2{margin-bottom:10px}
.sm-title{font-size:18px;font-weight:500;color:var(--accent);margin-bottom:4px}
.sm-lead{font-size:15px;margin-bottom:6px}
.sm-peak{color:var(--muted);font-size:13px;margin-bottom:10px}
.summary ul{margin:0;padding-left:18px;font-size:13px;line-height:2}
.sm-note{color:var(--muted);font-size:11px;margin-top:12px;padding-top:10px;border-top:1px solid var(--line)}
.panel{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:16px 18px;margin-bottom:14px}
.panel h2{font-size:14px;font-weight:500;margin:0 0 14px}
.panel h2 .tip{font-size:11px;color:var(--muted);font-weight:400;margin-left:8px}
.grid2{display:grid;grid-template-columns:1.25fr 1fr;gap:14px}
.bars{display:flex;align-items:flex-end;gap:8px;height:112px}
.bar{flex:1;display:flex;flex-direction:column;justify-content:flex-end;align-items:center;gap:5px;height:100%}
.bar .col{width:100%;background:var(--l3);border-radius:3px 3px 0 0;min-height:1px}
.bar .col.alt{background:var(--l2)}
.bar .col.sel{background:var(--l5)}
.bar .col.future{background:var(--soft);border:1px dashed var(--line)}
.bar .lb{font-size:11px;color:var(--muted);white-space:nowrap}
.bar.click{cursor:pointer}
.bar.click:hover .col{opacity:.8}
.pair{display:flex;gap:3px;align-items:flex-end;width:100%;height:100%}
.pair .col{flex:1;min-width:0}
.legend{font-size:12px;color:var(--muted);margin-top:10px}
.legend i{display:inline-block;width:9px;height:9px;border-radius:2px;background:var(--l3);margin-right:5px;vertical-align:middle}
.legend i.alt{background:var(--l2)}
.top{display:flex;flex-direction:column;gap:9px}
.row{display:flex;align-items:center;gap:9px;font-size:12px}
.row .nm{width:74px;flex:none;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.row .track{flex:1;height:9px;background:var(--soft);border-radius:3px;overflow:hidden}
.row .fill{display:block;height:100%;background:var(--l4);border-radius:3px}
.row .num{width:62px;text-align:right;font-variant-numeric:tabular-nums;color:var(--muted)}
.hours{display:flex;align-items:flex-end;gap:4px;height:80px}
.hours .hb{flex:1;background:var(--l3);border-radius:2px 2px 0 0;min-height:2px}
.hlabels{display:flex;gap:4px;margin-top:5px;font-size:10px;color:var(--muted)}
.hlabels span{flex:1;text-align:center}
.kb{display:flex;flex-direction:column;gap:5px}
.krow{display:flex;gap:5px}
.key{flex:1;height:30px;border-radius:5px;background:var(--soft);display:flex;align-items:center;justify-content:center;font-size:11px;color:var(--muted);white-space:nowrap;overflow:hidden}
.key.wide{flex:5}
.key.lv1{background:var(--l1);color:var(--accent)}
.key.lv2{background:var(--l2);color:#0c447c}
.key.lv3{background:var(--l3);color:#0c447c}
.key.lv4{background:var(--l4);color:#fff}
.key.lv5{background:var(--l5);color:#fff}
.key.flash{animation:kppulse .3s ease-out}
@keyframes kppulse{0%{background:var(--l5);color:#fff;transform:scale(1.12)}100%{background:var(--l4);color:#fff;transform:scale(1)}}
.hidetoggle{display:none}
nav button.live-on{background:var(--l1);color:var(--accent);border-color:var(--l2)}
table.tbl{width:100%;border-collapse:collapse;font-size:13px}
.tbl th,.tbl td{text-align:left;padding:7px 8px;border-bottom:1px solid var(--line);font-weight:400}
.tbl th{color:var(--muted);font-size:12px}
.tbl td.mono,.tbl th.mono{text-align:right;font-variant-numeric:tabular-nums}
.tbl tr:last-child td{border-bottom:none}
.tbl tr.click{cursor:pointer}
.tbl tr.click:hover{background:var(--l1)}
.verdict{background:var(--l1);border-radius:8px;padding:11px 14px;font-size:13px;margin-bottom:14px;color:#0c447c}
.verdict.muted{background:var(--soft);color:var(--muted)}
.minibar{height:7px;background:var(--l3);border-radius:3px;min-width:2px}
.cal{display:flex;gap:3px;overflow-x:auto;padding-bottom:4px}
.cal-col{display:flex;flex-direction:column;gap:3px;flex:none}
.cal-ml{height:14px;font-size:10px;color:var(--muted);line-height:14px;white-space:nowrap}
.cal-cell{width:13px;height:13px;border-radius:2px;background:var(--soft);border:1px solid rgba(0,0,0,.03)}
.cal-cell.lv1{background:var(--l1)}
.cal-cell.lv2{background:var(--l2)}
.cal-cell.lv3{background:var(--l3)}
.cal-cell.lv4{background:var(--l4)}
.cal-cell.lv5{background:var(--l5)}
.cal-cell.today{outline:2px solid var(--accent);outline-offset:1px}
.cal-cell.has{cursor:pointer}
.cal-cell.has:hover{opacity:.75}
.cal-legend{display:flex;align-items:center;gap:5px;font-size:11px;color:var(--muted);margin-top:10px}
.cal-legend .cal-cell{width:11px;height:11px}
.quips{margin:0;padding-left:18px;font-size:13px;line-height:2}
.hint{font-size:12px;color:var(--muted);background:var(--soft);border-radius:8px;padding:12px 14px}
.health{display:none;border:1px solid #f0d9b5;background:#fffbf2;color:#8a5a12;border-radius:10px;padding:10px 14px;font-size:12.5px;line-height:1.7;margin-bottom:14px}
.health.on{display:block}
.health b{font-weight:600;margin-right:6px}
footer{color:var(--muted);font-size:12px;margin-top:18px;line-height:1.9}
code{background:var(--soft);padding:1px 5px;border-radius:4px;font-size:11px}
@media(max-width:780px){.cards{grid-template-columns:repeat(2,1fr)}.grid2{grid-template-columns:1fr}}
</style></head><body><div class="wrap">
<header><h1>输入统计</h1><span class="sub" id="meta"></span></header>
<nav>
<button id="tab-day">单日</button>
<button id="tab-week">周报 · 对比</button>
<button id="tab-all">总览</button>
<button id="livetoggle" class="hidetoggle">实时模式</button>
<span class="spacer"></span>
<span class="picker" id="picker"></span>
</nav>
<div class="health" id="health"></div>
<main id="view"></main>
<footer>
统计口径：按键只计 keydown / syskeydown，长按连发由内存位图过滤；鼠标点击只计左右中及侧键按下，滚轮单独计数；一天按本地 00:00–23:59 归属；活跃分钟按分钟位图统计。周为周一至周日，缺失日期按 0 计入、日均固定除以 7；未完成周的结论改用同期对比。<br>
本工具只保存计数，不保存按键序列、字符文本、窗口标题或剪贴板内容，__KP_NET_NOTE__。数据目录：<code id="datadir"></code><br>
__KP_FOOTER_NOTE__<br>
<span id="livehint"></span>
构建信息：<code id="ver"></code>
</footer>
</div>
<script>window.KP_DATA=__KP_DATA__;window.KP_LIVE_MODE=__KP_LIVE__;window.KP_SERVER=__KP_SERVER__;window.KP_TOKEN="__KP_TOKEN__";</script>
<script>
(function(){
'use strict';
var D=window.KP_DATA,LIVE=(window.KP_LIVE_MODE===1),SERVER=(window.KP_SERVER===1),TOKEN=window.KP_TOKEN||'';
var liveOn=false;
var view=document.getElementById('view');
if(!D||!D.days||!D.days.length){view.innerHTML='<div class="panel hint">暂无统计数据。敲几下键盘、点几下鼠标后再打开报告。</div>';return;}
var days=D.days,weeks=D.weeks||[],ov=D.ov||{},layout=D.layout||[],names=D.names||{};
var byStamp={},weekByStart={};
days.forEach(function(d){byStamp[d.s]=d;});
weeks.forEach(function(w){weekByStart[w.s]=w;});
var latest=days[days.length-1];
document.getElementById('datadir').textContent=D.dir||'';
document.getElementById('ver').textContent=D.ver||'';
function liveHint(t){document.getElementById('livehint').textContent=t;}
function refreshNote(){
 if(SERVER)return liveOn?' · 本地看板实时（每秒更新今日）':' · 本地看板已暂停';
 return LIVE?' · 实时模式（每秒更新今日）':' · 每 60 秒自动刷新';
}
function setLiveUI(on){
 liveOn=!!on;
 var b=document.getElementById('livetoggle');
 if(b){b.textContent=liveOn?'实时模式：开':'实时模式：关';b.className=liveOn?'live-on':'';}
 if(typeof route==='function'&&view.innerHTML)route();
}
var H=D.health||{};
if(H.n){var hel=document.getElementById('health');hel.innerHTML='<b>采集提示</b> '+esc(H.nt||'');hel.className='on';}

function esc(s){return String(s==null?'':s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}
function n(v){return Number(v||0).toLocaleString('en-US');}
function kn(vk){if((vk>=48&&vk<=57)||(vk>=65&&vk<=90))return String.fromCharCode(vk);return names[String(vk)]||('0x'+Number(vk).toString(16).toUpperCase());}
function prevStamp(s){var y=+s.slice(0,4),m=+s.slice(4,6),d=+s.slice(6,8);var t=new Date(y,m-1,d-1);return t.getFullYear()+String(t.getMonth()+1).padStart(2,'0')+String(t.getDate()).padStart(2,'0');}
function badge(m){if(!m||!m.p)return '<span class="flat">上周无数据</span>';if(m.d>0)return '<span class="up">+'+m.d+'%</span>';if(m.d<0)return '<span class="down">'+m.d+'%</span>';return '<span class="flat">持平</span>';}
function dayDelta(cur,prev){if(!prev||!prev.k)return '';var d=Math.round((cur-prev.k)*100/prev.k);if(d>0)return '<span class="up">+'+d+'%</span> 较前一日';if(d<0)return '<span class="down">'+d+'%</span> 较前一日';return '<span class="flat">与前一日持平</span>';}
function setTab(t){['day','week','all'].forEach(function(x){document.getElementById('tab-'+x).className=(x===t?'on':'');});}
function meta(txt){document.getElementById('meta').textContent=txt;}
function picker(html){document.getElementById('picker').innerHTML=html;}
function go(h){location.hash=h;}
window.KP={goDay:function(s){go('#day='+s);},goWeek:function(s){go('#week='+s);},goToday:function(){go('#today');},goAll:function(){go('#all');}};
document.getElementById('tab-day').onclick=function(){go('#today');};
document.getElementById('tab-week').onclick=function(){go('#week='+(weeks.length?weeks[weeks.length-1].s:''));};
document.getElementById('tab-all').onclick=function(){go('#all');};

function card(k,v,foot){return '<div class="card"><div class="k">'+esc(k)+'</div><div class="v">'+v+'</div><div class="foot">'+(foot||'')+'</div></div>';}
function topRows(pairs,total){total=total||1;var mx=1;pairs.forEach(function(p){if(p[1]>mx)mx=p[1];});if(!pairs.length)return '<div class="hint">没有按键记录</div>';return pairs.map(function(p){return '<div class="row"><span class="nm">'+esc(kn(p[0]))+'</span><span class="track"><span class="fill" style="width:'+Math.round(p[1]*100/mx)+'%"></span></span><span class="num">'+n(p[1])+'</span></div>';}).join('');}
function top10(d){var a=(d.e||[]).slice().sort(function(x,y){return y[1]-x[1]||x[0]-y[0];}).slice(0,10);return topRows(a,d.k);}
function hoursHTML(hk){var mx=1;hk.forEach(function(v){if(v>mx)mx=v;});return '<div class="hours">'+hk.map(function(v,h){return '<div class="hb" style="height:'+Math.round(v*100/mx)+'%" title="'+String(h).padStart(2,'0')+':00 共 '+n(v)+' 次"></div>';}).join('')+'</div><div class="hlabels">'+hk.map(function(v,h){return '<span>'+String(h).padStart(2,'0')+'</span>';}).join('')+'</div>';}
function heatHTML(d){var m={},mx=1;(d.e||[]).forEach(function(p){m[p[0]]=p[1];if(p[1]>mx)mx=p[1];});return '<div class="kb">'+layout.map(function(row){return '<div class="krow">'+row.map(function(vk){var c=m[vk]||0,lv=0;if(c>0){lv=Math.ceil(c*5/mx);if(lv<1)lv=1;if(lv>5)lv=5;}return '<div class="key lv'+lv+(vk===32?' wide':'')+'" data-vk="'+vk+'" title="'+esc(kn(vk))+' '+n(c)+' 次">'+esc(kn(vk))+'</div>';}).join('')+'</div>';}).join('')+'</div>';}
function trendHTML(list,sel,unit){var mx=1;list.forEach(function(p){if(p.k>mx)mx=p.k;});return '<div class="bars">'+list.map(function(p){var h=Math.max(1,Math.round(p.k*100/mx));var target=unit==='week'?'#week='+p.s:'#day='+p.s;return '<div class="bar click" onclick="location.hash=\''+target+'\'" title="'+esc(p.l)+' 按键 '+n(p.k)+' / 点击 '+n(p.c)+'"><div class="col'+(p.s===sel?' sel':'')+'" style="height:'+h+'%"></div><div class="lb">'+esc(p.l)+'</div></div>';}).join('')+'</div>';}
function summaryHTML(s,title,note){var q=(s.q||[]).map(function(x){return '<li>'+esc(x)+'</li>';}).join('');return '<div class="panel summary"><h2>'+esc(title)+'</h2><div class="sm-title">'+esc(s.t)+'</div><div class="sm-lead">'+esc(s.e)+'</div><div class="sm-peak">'+esc(s.p)+' · '+esc(s.pe)+'</div><ul>'+q+'</ul><div class="sm-note">'+esc(note||D.note)+'</div></div>';}

function dayPicker(d){var opts=days.slice().reverse().map(function(x){return '<option value="'+x.s+'"'+(x.s===d.s?' selected':'')+'>'+esc(x.d)+'（'+['','一','二','三','四','五','六','日'][x.w]+'）</option>';}).join('');return '<select onchange="KP.goDay(this.value)">'+opts+'</select><button class="navbtn" onclick="KP.goToday()">回到今天</button>';}
function renderDay(stamp,isToday){
 var d=byStamp[stamp]||latest;isToday=!!isToday&&d.s===latest.s;
 setTab('day');meta((isToday?'今日':'回看')+' '+d.d+' · 数据截止 '+D.gen+' · 共 '+days.length+' 天数据'+refreshNote());
 picker(dayPicker(d));
 var prev=byStamp[prevStamp(d.s)]||null;
 if(!prev){var i=days.indexOf(d);if(i>0)prev=days[i-1];}
 var tail=days.filter(function(x){return x.s<=d.s;}).slice(-14);
 var html='<div class="cards">'+
  card('按键次数',n(d.k),dayDelta(d.k,prev))+
  card('鼠标点击',n(d.c),prev&&prev.c?dayDelta(d.c,{k:prev.c}):'')+
  card('滚轮格数',n(d.r),'')+
  card('活跃分钟',n(d.a),Math.round(d.a/60*10)/10+' 小时')+'</div>';
 html+=summaryHTML(d.su,isToday?'今日速写':'当日速写');
 html+='<div class="grid2"><div class="panel"><h2>按天趋势<span class="tip">截至所选日期 · 点柱子回看那天</span></h2>'+trendHTML(tail.map(function(x){return {l:x.s.slice(4,6)+'/'+x.s.slice(6,8),s:x.s,k:x.k,c:x.c};}),d.s,'day')+'<div class="legend"><i></i>按键次数　（悬停查看数值，点击跳转）</div></div>'+
  '<div class="panel"><h2>常用键 Top 10</h2><div class="top">'+top10(d)+'</div></div></div>';
 html+='<div class="panel"><h2>24 小时按键分布</h2>'+hoursHTML(d.hk)+'</div>';
 html+='<div class="panel"><h2>键盘热力（按物理键位）</h2>'+heatHTML(d)+'</div>';
 view.innerHTML=html;
}

function weekPicker(w){var idx=weeks.indexOf(w),opts=weeks.slice().reverse().map(function(x){return '<option value="'+x.s+'"'+(x.s===w.s?' selected':'')+'>'+esc(x.lb)+'</option>';}).join('');var prev=idx>0?weeks[idx-1].s:'',next=idx<weeks.length-1?weeks[idx+1].s:'';return '<button class="navbtn" onclick="KP.goWeek(\''+prev+'\')"'+(prev?'':' disabled')+'>← 上一周</button><select onchange="KP.goWeek(this.value)">'+opts+'</select><button class="navbtn" onclick="KP.goWeek(\''+next+'\')"'+(next?'':' disabled')+'>下一周 →</button>';}
function cmpRow(name,m){return '<tr><td>'+esc(name)+'</td><td class="mono">'+n(m.c)+'</td><td class="mono">'+n(m.p)+'</td><td>'+badge(m)+'</td></tr>';}
function dualHTML(w,cmp){var mx=1;w.d.forEach(function(x,i){mx=Math.max(mx,x.k,cmp&&cmp.pd?cmp.pd[i]:0);});return '<div class="bars">'+w.d.map(function(x,i){var future=w.part&&i>=w.el&&!x.h;var a=future?2:Math.max(1,Math.round(x.k*100/mx)),b=cmp&&cmp.pd?Math.max(1,Math.round(cmp.pd[i]*100/mx)):1;var tip=esc(x.lb)+'：'+(future?'本周尚未到':'本周 '+n(x.k))+' / 上周 '+n(cmp&&cmp.pd?cmp.pd[i]:0);return '<div class="bar" title="'+tip+'"><div class="pair"><div class="col alt" style="height:'+b+'%"></div><div class="col'+(future?' future':'')+'" style="height:'+a+'%"></div></div><div class="lb">'+esc(x.lb.replace('周',''))+'</div></div>';}).join('')+'</div><div class="legend"><i class="alt"></i>上一周　<i></i>本周　虚线柱为本周尚未到来的日期　（按周一至周日对齐，缺失日期按 0 计）</div>';}
function renderWeek(start){
 if(!weeks.length){view.innerHTML='<div class="panel hint">暂无周数据</div>';return;}
 var w=weekByStart[start]||weeks[weeks.length-1],cmp=w.cmp||{ok:false};
 var wmx=1;w.d.forEach(function(y){if(y.k>wmx)wmx=y.k;});
 var km=(w.part&&cmp.sk)?cmp.sk:cmp.k; // 未完成周的卡片徽章改用同期，避免误导
 setTab('week');meta('周报 '+w.lb+(w.part?' · 进行中':'')+' · 数据截止 '+D.gen+' · 共 '+weeks.length+' 周');
 picker(weekPicker(w));
 var html='<div class="cards">'+
  card('本周按键',n(w.k),badge(km)+(w.part?' 同期口径':''))+
  card('日均按键',n(w.da),w.part?'按 7 天计 · 本周未结束':(cmp.da?badge(cmp.da):''))+
  card('鼠标点击',n(w.c),badge(cmp.c))+
  card('活跃时长',(Math.round(w.a/60*10)/10)+' h',cmp.a?badge(cmp.a):'')+'</div>';
 html+='<div class="panel"><h2>与上一周对比<span class="tip">'+esc(cmp.lb||'')+'</span></h2>'+
  '<div class="verdict'+(cmp.ok?'':' muted')+'">'+esc(cmp.v||'')+'</div>';
 if(cmp.part&&cmp.nt){html+='<div class="hint" style="margin-bottom:14px">'+esc(cmp.nt)+'</div>';}
 if(cmp.ok){
  html+='<table class="tbl"><thead><tr><th>指标</th><th class="mono">本周</th><th class="mono">上周</th><th>环比</th></tr></thead><tbody>'+
   (cmp.part&&cmp.sk?cmpRow('同期按键（'+w.pl+'）',cmp.sk):'')+
   cmpRow('按键总量（7 天）',cmp.k)+cmpRow('鼠标点击',cmp.c)+cmpRow('滚轮格数',cmp.r)+cmpRow('活跃分钟',cmp.a)+cmpRow('日均按键',cmp.da)+'</tbody></table>';
 }
 html+='<div style="height:18px"></div>'+dualHTML(w,cmp)+'</div>';
 html+='<div class="panel summary"><h2>本周速写</h2><div class="sm-title">'+esc(w.ti)+'</div><ul class="quips">'+w.q.map(function(x){return '<li>'+esc(x)+'</li>';}).join('')+'</ul><div class="sm-note">环比口径：完整周按 7 天总量比较；本周尚未结束时改用同期口径（只比已过去的对应天），避免未完成周被低估。缺失日期按 0 计入，日均固定除以 7，不因缺记录虚高。</div></div>';
 html+='<div class="grid2"><div class="panel"><h2>本周 24 小时叠加分布</h2>'+hoursHTML(w.hk)+'</div><div class="panel"><h2>本周常用键 Top 10</h2><div class="top">'+topRows(w.top||[],w.k)+'</div></div></div>';
 html+='<div class="panel"><h2>本周逐日明细<span class="tip">点某一行可回看当天</span></h2><table class="tbl"><thead><tr><th>日期</th><th class="mono">按键</th><th class="mono">点击</th><th class="mono">活跃分钟</th><th style="width:34%">相对强度</th></tr></thead><tbody>'+
  w.d.map(function(x){var h=Math.max(1,Math.round(x.k*100/wmx));return '<tr class="click" onclick="location.hash=\'#day='+x.s+'\'"><td>'+esc(x.d)+'　'+esc(x.lb)+(x.h?'':'　<span class="flat">无记录</span>')+'</td><td class="mono">'+n(x.k)+'</td><td class="mono">'+n(x.c)+'</td><td class="mono">'+n(x.a)+'</td><td><div class="minibar" style="width:'+h+'%"></div></td></tr>';}).join('')+'</tbody></table></div>';
 view.innerHTML=html;
}

function calendarHTML(cal){var cols=[],lastMonth=-1;for(var i=0;i<cal.length;i+=7){cols.push({cells:cal.slice(i,i+7),month:(i===0||cal[i].m!==lastMonth)?cal[i].m:0});lastMonth=cal[i].m;}return '<div class="cal">'+cols.map(function(col){var cells=col.cells.slice();while(cells.length<7)cells.push({h:false,lv:0});return '<div class="cal-col"><div class="cal-ml">'+(col.month?col.month+'月':'')+'</div>'+cells.map(function(c){var cls='cal-cell'+(c.lv?' lv'+c.lv:'')+(c.h?' has':'')+(c.s===latest.s?' today':'');var title=(c.m||'')+'月'+(c.dd||'')+'日 · '+(c.h?n(c.k)+' 次按键':'无数据');var onclick=c.h?' onclick="KP.goDay(\''+c.s+'\')"':'';return '<div class="'+cls+'" title="'+esc(title)+'"'+onclick+'></div>';}).join('')+'</div>';}).join('')+'</div><div class="cal-legend">少 <span class="cal-cell"></span><span class="cal-cell lv1"></span><span class="cal-cell lv2"></span><span class="cal-cell lv3"></span><span class="cal-cell lv4"></span><span class="cal-cell lv5"></span> 多　· 点方格回看当天，蓝框为最新一天</div>';}
function renderAll(){
 setTab('all');meta('全期总览 · 数据截止 '+D.gen+' · 共 '+days.length+' 天 / '+weeks.length+' 周');
 picker('<span class="flat">'+days.length+' 天历史全部内嵌在本页</span>');
 var html=(D.trunc?'<div class="hint" style="margin-bottom:14px">历史超过 '+D.maxd+' 天，本页只内嵌最近 '+days.length+' 天；完整的全部历史请用托盘「导出 CSV」查看。</div>':'')+'<div class="cards">'+
  card('累计按键',n(ov.k),'')+card('累计点击',n(ov.c),'')+
  card('累计活跃',(Math.round((ov.a||0)/60*10)/10)+' h','')+
  card('有记录天数',n(ov.n),'最长连续 '+n(ov.ms)+' 天')+'</div>';
 html+='<div class="panel"><h2>日历热力<span class="tip">按周排列，最多回溯 53 周</span></h2>'+calendarHTML(ov.cal||[])+'</div>';
 html+='<div class="panel"><h2>全期趋势<span class="tip">'+(ov.tu==='week'?'按自然周聚合，点柱进入周报':'按天，点柱回看当天')+'</span></h2>'+trendHTML(ov.tr||[],'',ov.tu)+'<div class="legend"><i></i>按键次数</div></div>';
 html+='<div class="panel summary"><h2>全期里程碑</h2><ul class="quips">'+(ov.q||[]).map(function(x){return '<li>'+esc(x)+'</li>';}).join('')+'</ul><div class="sm-note">累计口径为各日快照直接求和；若曾手动删除某日 .bin 文件，该日不参与累计。</div></div>';
 view.innerHTML=html;
}

function route(){
 var h=location.hash||'#today';
 if(h==='#all'){renderAll();return;}
 if(h.indexOf('#week=')===0){renderWeek(h.slice(6));return;}
 if(h.indexOf('#day=')===0){renderDay(h.slice(5),false);return;}
 renderDay(latest.s,true);
}
window.addEventListener('hashchange',route);
route();

/* 实时模式：用动态 script 读取同目录 live.js（经典 script 标签不受
   file:// 的 CORS 限制，fetch / XHR 会被拦），只替换今日数据并复用
   renderDay 重渲染，不整页重载，因此不闪烁、不丢滚动位置、不打断浏览。
   周报 / 总览仍靠 60 秒整页刷新更新聚合视图，避免实时数字与预聚合
   的周、总览口径打架。 */
var liveSeq=-1,liveBusy=false,liveErr=0;
function applyLive(L){
 var d=L.d;
 if(!d||!d.s)return;
 var idx=-1;
 for(var i=0;i<days.length;i++){if(days[i].s===d.s){idx=i;break;}}
 if(idx<0){days.push(d);days.sort(function(a,b){return a.s<b.s?-1:(a.s>b.s?1:0);});}
 else days[idx]=d;
 byStamp[d.s]=d;
 latest=days[days.length-1];
 D.gen=L.gen||D.gen;
 var h=location.hash||'#today';
 if(h!=='#today'&&h!=='#day='+latest.s)return;
 if(document.activeElement&&document.activeElement.tagName==='SELECT')return;
 route();
}
function pollLive(){
 if(!LIVE||liveBusy)return;
 liveBusy=true;
 var s=document.createElement('script');
 s.src='live.js?t='+Date.now();
 s.onload=s.onerror=function(ev){
  liveBusy=false;
  if(s.parentNode)s.parentNode.removeChild(s);
  if(ev&&ev.type==='error'){
   liveErr++;
   if(liveErr>=5)liveHint('实时数据未更新，请重新点击托盘「打开统计报告」');
   return;
  }
  liveErr=0;
  var L=window.KP_LIVE;
  if(L&&L.seq!==liveSeq&&L.d){liveSeq=L.seq;applyLive(L);}
 };
 (document.head||document.body).appendChild(s);
}
/* 本地看板：SSE 只收两类数据 —— 当天聚合快照、256-bit 键位位图。
   没有按键序列、字符、时间戳或窗口信息；页面内开关走同源 POST。
   键位位图只有「最近窗口按过哪些键」，因此同一键连敲只闪一次。 */
function flashKeys(words){
 if(!words||words.length!==8)return;
 var keys=document.querySelectorAll('.key[data-vk]');
 for(var i=0;i<keys.length;i++){
  var el=keys[i],vk=Number(el.getAttribute('data-vk'));
  if(((words[vk>>5]>>>(vk&31))&1)!==1)continue;
  el.className=el.className.replace(/\s*flash\b/g,'')+' flash';
  (function(node){setTimeout(function(){node.className=node.className.replace(/\s*flash\b/g,'');},320);})(el);
 }
}
function setLiveRemote(on){
 if(typeof fetch!=='function')return;
 fetch('api/live?t='+encodeURIComponent(TOKEN)+'&on='+(on?1:0),{method:'POST'})
  .then(function(){setLiveUI(on);}).catch(function(){liveHint('切换失败，请重新点开托盘「本地看板」');});
}
function startSSE(){
 if(!SERVER||typeof EventSource==='undefined')return;
 var es=new EventSource('events?t='+encodeURIComponent(TOKEN));
 es.addEventListener('mode',function(ev){try{setLiveUI(JSON.parse(ev.data).on);}catch(e){}});
 es.addEventListener('snapshot',function(ev){try{applyLive(JSON.parse(ev.data));}catch(e){}});
 es.addEventListener('keys',function(ev){try{flashKeys(JSON.parse(ev.data));}catch(e){}});
 es.onerror=function(){liveHint('本地看板连接已断开；关闭全部页面后服务会自动停止并释放端口');};
}
function refreshNonToday(){
 // 今日视图由 SSE / live.js 局部更新，绝不整页重载；
 // 周报 / 总览是预聚合视图，仍需定时重载才能看到新数据。
 var h=location.hash||'#today';
 if(h==='#today'||h==='#day='+days[days.length-1].s)return;
 if(typeof location.reload==='function')location.reload();
}
if(SERVER){
 var tb=document.getElementById('livetoggle');
 if(tb){tb.className='';tb.onclick=function(){setLiveRemote(!liveOn);};}
 liveHint('本地看板已连接：点上方「实时模式」开关可暂停推送；今日数据每秒更新，键位随按键闪动。服务仅绑定 127.0.0.1 回环，关闭全部页面后自动停止。');
 setLiveUI(false);
 startSSE();
 setInterval(refreshNonToday,60000);
}else if(LIVE){
 liveHint('实时模式已开启：本页每秒从数据目录读取 live.js 更新今日视图，仍不联网、不暴露隐私；关闭托盘里的「实时模式」可恢复空闲零写入。');
 pollLive();
 setInterval(pollLive,1000);
 setInterval(refreshNonToday,60000);
}
})();
</script></body></html>`
