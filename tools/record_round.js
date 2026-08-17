#!/usr/bin/env node
// tools/record_round.js — 每轮 AI 交互完成后运行一次。
// 自动完成：取上一 commit → git add → git commit → 计算本轮 diff → 追加一条 JSONL。
// 保证 round_id / commit_hash / modify_diff 严格一一对应，diff 换行按 JSON 规范转义为 \n。
const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const prompt = process.argv[2];
if (!prompt) {
  console.error('用法: node tools/record_round.js "<本轮prompt内容>"');
  process.exit(1);
}

// ★ 按实际情况修改
const AGENT_TYPE = 'Kilo Code';  // Kilo Code / PI / Cine，填你实际用的
const DEV_LANG = 'Go';
const NAME = '姓名';             // 改成你的名字
const JSONL_FILE = path.join(__dirname, '..', `AI开发考核_${NAME}_csvstats.jsonl`);

const pad = (n) => String(n).padStart(2, '0');
const d = new Date();
const modify_time = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;

// 上一轮 commit（首轮为空）
let before = '';
try { before = execSync('git rev-parse HEAD').toString().trim(); } catch (e) {}

// 轮次号：从 JSONL 已有行数推断，实现 round_id 自增且唯一
let roundId = 1;
if (fs.existsSync(JSONL_FILE)) {
  const lines = fs.readFileSync(JSONL_FILE, 'utf8').trim().split('\n').filter(Boolean);
  if (lines.length) roundId = JSON.parse(lines[lines.length - 1]).round_id + 1;
}

// add + commit（commit message 自动含轮次，保证可追溯）
execSync('git add -A', { stdio: 'inherit' });
execSync(`git commit -m "round ${roundId}: ${prompt.slice(0, 40)}"`, { stdio: 'inherit' });
const after = execSync('git rev-parse HEAD').toString().trim();

// 本轮完整 diff（首轮用 git show 取首个 commit 的 patch）
const diff = before
  ? execSync(`git diff ${before} ${after}`).toString()
  : execSync(`git show --format= --no-ext-diff ${after}`).toString();

// 组装并追加 JSONL（JSON.stringify 自动把 diff/prompt 里的换行转义为 \n）
const record = {
  round_id: roundId,
  prompt_content: prompt,
  modify_diff: diff,
  commit_hash: after,
  modify_time,
  agent_type: AGENT_TYPE,
  dev_language: DEV_LANG,
};
fs.appendFileSync(JSONL_FILE, JSON.stringify(record) + '\n');
console.log(`✔ 已记录第 ${roundId} 轮 → commit ${after.slice(0, 8)}`);
