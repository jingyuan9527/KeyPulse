// 报告页面回归验收：用极简 DOM stub 在 Node 里执行 report.html 内的 JS，
// 验证单日 / 周报 / 总览三个视图都能渲染、口径分支正确、不抛异常。
// Go 单测覆盖不到前端渲染，这一步用来防止"编译过了但页面一片空白"。
//
// 用法：node scripts/check-page.js <report.html 路径>
// 前置：./keypulse-cli.exe --selftest 生成数据后再跑本脚本。
const fs = require('fs');
const vm = require('vm');
const path = require('path');

const file = process.argv[2] || path.join(__dirname, '..', '.tmp', '_selftest', 'report.html');
const html = fs.readFileSync(file, 'utf8');
const blocks = [];
const re = /<script>([\s\S]*?)<\/script>/g;
let m;
while ((m = re.exec(html)) !== null) blocks.push(m[1]);
if (blocks.length !== 2) {
  console.error('FAIL: 期望 2 个 script，实际 ' + blocks.length);
  process.exit(1);
}

try { new vm.Script(blocks[1]); } catch (e) {
  console.error('FAIL: JS 语法错误: ' + e.message);
  process.exit(1);
}

const nodes = {};
const getEl = (id) => {
  if (!nodes[id]) nodes[id] = { innerHTML: '', textContent: '', className: '', onclick: null };
  return nodes[id];
};
let onHashChange = null;
let intervals = 0;
const sandbox = {
  window: { addEventListener: (ev, fn) => { if (ev === 'hashchange') onHashChange = fn; } },
  document: { getElementById: getEl },
  location: { hash: '#today' },
  console: console,
  setInterval: () => { intervals++; return 0; },
};
vm.createContext(sandbox);
vm.runInContext(blocks[0], sandbox, { filename: 'data.js' });
if (!sandbox.window.KP_DATA || !sandbox.window.KP_DATA.days.length) {
  console.error('FAIL: KP_DATA 未注入或无数据');
  process.exit(1);
}
const D = sandbox.window.KP_DATA;
vm.runInContext(blocks[1], sandbox, { filename: 'page.js' });
if (!onHashChange) {
  console.error('FAIL: hashchange 未注册');
  process.exit(1);
}

function render(hash) {
  sandbox.location.hash = hash;
  onHashChange();
  return nodes.view.innerHTML;
}

const latest = D.days[D.days.length - 1].s;
const oldest = D.days[0].s;
const lastWeek = D.weeks[D.weeks.length - 1].s;
const prevWeek = D.weeks.length > 1 ? D.weeks[D.weeks.length - 2].s : lastWeek;
const cases = [
  { hash: '#today', must: ['今日速写', '键盘热力', '按天趋势'], mustNot: [] },
  { hash: '#day=' + oldest, must: ['当日速写', '常用键 Top 10'], mustNot: ['今日速写'] },
  { hash: '#week=' + lastWeek, must: ['与上一周对比', '本周速写', '本周逐日明细'], mustNot: [] },
  { hash: '#week=' + prevWeek, must: ['与上一周对比', '按键总量（7 天）'], mustNot: ['同期按键'] },
  { hash: '#all', must: ['日历热力', '全期里程碑', '全期趋势'], mustNot: [] },
];
let bad = 0;
for (const c of cases) {
  let out;
  try { out = render(c.hash); } catch (e) {
    console.error('FAIL ' + c.hash + ' 抛异常: ' + e.stack);
    bad++; continue;
  }
  const problems = [];
  if (!out || out.length < 500) problems.push('输出过短(' + (out || '').length + ')');
  for (const s of c.must) if (!out.includes(s)) problems.push('缺少 "' + s + '"');
  for (const s of c.mustNot) if (out.includes(s)) problems.push('不该出现 "' + s + '"');
  for (const token of ['undefined', 'NaN', '[object Object]', '__KP_DATA__', 'Infinity']) {
    if (out.includes(token)) problems.push('含异常标记 ' + token);
  }
  if (problems.length) { console.error('FAIL ' + c.hash + ': ' + problems.join(', ')); bad++; }
  else console.log('PASS ' + c.hash + '  ' + out.length + ' 字符');
}

// 进行中的周：必须启用同期口径，并用虚线柱标出尚未到来的日期。
if (D.weeks[D.weeks.length - 1].part) {
  const out = render('#week=' + lastWeek);
  for (const s of ['同期按键', '本周尚未结束', 'col future', '同期（']) {
    if (!out.includes(s)) { console.error('FAIL 未完成周缺少 "' + s + '"'); bad++; }
  }
  if (!bad) console.log('PASS 未完成周：同期口径 + 未到日期标记');
}

// 健康提示分支：用一份带 health 的数据集重跑页面脚本，验证采集中断
// 提示条会被点亮、文案经过 esc。自检数据本身不含 health.log，
// 因此顺便验证无事件时提示条保持隐藏。
if (nodes.health && nodes.health.className === 'on' && !(D.health && D.health.n)) {
  console.error('FAIL 无恢复事件却点亮了健康提示条'); bad++;
}
const nodes2 = {};
const getEl2 = (id) => {
  if (!nodes2[id]) nodes2[id] = { innerHTML: '', textContent: '', className: '', onclick: null };
  return nodes2[id];
};
const sandbox2 = {
  window: { addEventListener: () => {} },
  document: { getElementById: getEl2 },
  location: { hash: '#today' },
  console: console,
};
vm.createContext(sandbox2);
sandbox2.window.KP_DATA = Object.assign({}, D, {
  health: { n: 2, t: 1, l: '2026-11-06 10:44', nt: '采集曾中断并自动恢复 2 次，最长缺口约 2 分钟。该时段的统计为近似值。' },
});
vm.runInContext(blocks[1], sandbox2, { filename: 'page-health.js' });
if (!nodes2.health || nodes2.health.className !== 'on' || !nodes2.health.innerHTML.includes('近似值')) {
  console.error('FAIL 健康提示条未渲染'); bad++;
} else console.log('PASS 健康提示条');

// 实时模式：伪造一次 live.js 加载，验证页面能局部更新今日视图，
// 且普通模式不会启动任何轮询（保持零额外开销）。
let liveBad = 0;
const liveNodes = {};
const getEl3 = (id) => {
  if (!liveNodes[id]) liveNodes[id] = { innerHTML: '', textContent: '', className: '', onclick: null };
  return liveNodes[id];
};
let livePolls = 0;
const sandbox3 = {
  window: { addEventListener: () => {} },
  document: {
    getElementById: getEl3,
    createElement: (tag) => ({ tagName: tag.toUpperCase(), parentNode: null, src: '', onload: null, onerror: null }),
    head: {
      appendChild: (el) => {
        livePolls++;
        el.parentNode = { removeChild: () => {} };
        const ds3 = sandbox3.window.KP_DATA;
        const base = ds3.days[ds3.days.length - 1];
        sandbox3.window.KP_LIVE = {
          seq: 42,
          gen: '2026-11-06 17:00:00',
          d: Object.assign({}, base, { k: 999999, c: 888888 }),
        };
        el.onload({ type: 'load' });
      },
    },
    activeElement: null,
  },
  location: { hash: '#today' },
  console: console,
  setInterval: () => 0,
};
vm.createContext(sandbox3);
vm.runInContext(blocks[0], sandbox3, { filename: 'data-live.js' });
sandbox3.window.KP_LIVE_MODE = 1;
try {
  vm.runInContext(blocks[1], sandbox3, { filename: 'page-live.js' });
} catch (e) {
  console.error('FAIL 实时模式页面抛异常: ' + e.stack); liveBad++;
}
const liveOut = liveNodes.view ? liveNodes.view.innerHTML : '';
for (const s of ['999,999', '888,888', '今日速写']) {
  if (!liveOut.includes(s)) { console.error('FAIL 实时局部更新缺少 "' + s + '"'); liveBad++; }
}
if (!liveNodes.meta || !liveNodes.meta.textContent.includes('实时模式')) {
  console.error('FAIL 实时模式未在页头标注'); liveBad++;
}
if (!liveNodes.livehint || !liveNodes.livehint.textContent.includes('实时模式已开启')) {
  console.error('FAIL 缺少实时模式页脚说明'); liveBad++;
}
if (livePolls !== 1) { console.error('FAIL 实时模式轮询次数异常: ' + livePolls); liveBad++; }
if (!liveBad) console.log('PASS 实时模式：live.js 局部更新今日视图');
bad += liveBad;
if (intervals !== 0) { console.error('FAIL 普通模式启动了轮询: ' + intervals); bad++; }
else console.log('PASS 普通模式：未启动实时轮询');

// 本地看板（SSE）：模拟 EventSource 事件，验证页面内开关、今日快照
// 局部更新、键位位图闪动，以及「不启动 live.js 轮询」。
let boardBad = 0;
const boardNodes = {};
const getEl4 = (id) => {
  if (!boardNodes[id]) boardNodes[id] = { innerHTML: '', textContent: '', className: '', onclick: null };
  return boardNodes[id];
};
const flashKeys = [
  { vk: 65, className: 'key lv2' },
  { vk: 66, className: 'key lv0' },
].map((k) => ({ className: k.className, getAttribute: () => String(k.vk) }));
const listeners = {};
const fetchCalls = [];
let esUrls = [];
let boardPolls = 0;
class FakeEventSource {
  constructor(url) { esUrls.push(url); this.onerror = null; }
  addEventListener(ev, fn) { listeners[ev] = fn; }
}
const sandbox4 = {
  window: { addEventListener: () => {} },
  document: {
    getElementById: getEl4,
    createElement: (tag) => ({ tagName: tag.toUpperCase(), parentNode: null, src: '', onload: null, onerror: null }),
    head: { appendChild: () => { boardPolls++; } },
    activeElement: null,
    querySelectorAll: (sel) => (sel === '.key[data-vk]' ? flashKeys : []),
  },
  location: { hash: '#today' },
  console: console,
  setInterval: () => 0,
  setTimeout: () => 0,
  EventSource: FakeEventSource,
  fetch: (url) => { fetchCalls.push(url); return Promise.resolve({}); },
};
vm.createContext(sandbox4);
vm.runInContext(blocks[0], sandbox4, { filename: 'data-board.js' });
sandbox4.window.KP_LIVE_MODE = 0;
sandbox4.window.KP_SERVER = 1;
sandbox4.window.KP_TOKEN = 'abc123';
try {
  vm.runInContext(blocks[1], sandbox4, { filename: 'page-board.js' });
} catch (e) {
  console.error('FAIL 本地看板页面抛异常: ' + e.stack); boardBad++;
}
if (esUrls.length !== 1 || !esUrls[0].includes('events?t=abc123')) {
  console.error('FAIL 看板页未建立带 token 的 SSE 连接: ' + JSON.stringify(esUrls)); boardBad++;
}
if (boardPolls !== 0) { console.error('FAIL 看板页仍在轮询 live.js: ' + boardPolls); boardBad++; }
if (!listeners.mode || !listeners.snapshot || !listeners.keys) {
  console.error('FAIL SSE 事件监听未注册齐全'); boardBad++;
} else {
  listeners.mode({ data: '{"on":true}' });
  if (!boardNodes.livetoggle || boardNodes.livetoggle.textContent !== '实时模式：开') {
    console.error('FAIL 页面内开关未随 mode 事件更新'); boardBad++;
  }
  const baseDay = D.days[D.days.length - 1];
  listeners.snapshot({ data: JSON.stringify({ seq: 77, gen: '2026-11-09 13:00:00', d: Object.assign({}, baseDay, { k: 777777, c: 666666 }) }) });
  const boardOut = boardNodes.view ? boardNodes.view.innerHTML : '';
  for (const s of ['777,777', '666,666', '键盘热力']) {
    if (!boardOut.includes(s)) { console.error('FAIL 看板快照局部更新缺少 "' + s + '"'); boardBad++; }
  }
  // VK 65('A') 在第 3 个 uint32 的 bit 1；VK 66 不应闪。
  const words = [0, 0, 2, 0, 0, 0, 0, 0];
  listeners.keys({ data: JSON.stringify(words) });
  if (!flashKeys[0].className.includes('flash') || flashKeys[1].className.includes('flash')) {
    console.error('FAIL 键位闪动位图匹配错误: ' + JSON.stringify(flashKeys.map((k) => k.className))); boardBad++;
  }
  if (!boardNodes.meta || !boardNodes.meta.textContent.includes('本地看板实时')) {
    console.error('FAIL 看板页头未标注实时状态'); boardBad++;
  }
}
if (boardNodes.livetoggle && boardNodes.livetoggle.onclick) {
  boardNodes.livetoggle.onclick();
  if (!fetchCalls.some((u) => u.includes('api/live?t=abc123') && u.includes('on='))) {
    console.error('FAIL 页面内开关未请求 /api/live: ' + JSON.stringify(fetchCalls)); boardBad++;
  }
} else { console.error('FAIL 页面内开关按钮未启用'); boardBad++; }
if (!boardBad) console.log('PASS 本地看板：SSE 开关 / 快照更新 / 键位闪动 / 零 live.js 轮询');
bad += boardBad;

const cmp = D.weeks[D.weeks.length - 1].cmp || {};
console.log('---');
console.log('数据集: ' + D.days.length + ' 天 / ' + D.weeks.length + ' 周 / HTML ' + (html.length / 1024).toFixed(1) + ' KB');
console.log('最新周结论: ' + cmp.v);
if (cmp.nt) console.log('口径提示: ' + cmp.nt);
if (bad) process.exit(1);
console.log('全部视图渲染通过');
