import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync, chmodSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

const dir = mkdtempSync(join(tmpdir(), 'ax-pi-contract-'));
const events = new Map(), tools = new Map(), messages = [], statuses = [];
let idle = true, native = '12345678-1234-4234-a234-123456789012';
const pi = {
  on: (name, callback) => events.set(name, callback),
  registerTool: tool => tools.set(tool.name, tool),
  sendMessage: (message, options) => messages.push({ message, options, idle }),
};
const ctx = { isIdle: () => idle, ui: { setStatus: (_key, text) => statuses.push(text) },
  sessionManager: { getSessionId: () => native, getSessionFile: () => '/original repo/session.jsonl' } };
async function until(predicate) {
  for (let i = 0; i < 120; i++) { if (predicate()) return; await delay(25); }
  throw new Error('condition timed out: ' + statuses.join(', '));
}
try {
  const bridge = join(dir, 'bridge.mjs'), session = join(dir, 'session.json');
  writeFileSync(bridge, readFileSync(new URL('./pi_bridge.mjs', import.meta.url)));
  chmodSync(bridge, 0o700);
  writeFileSync(session, '{}');
  process.env.AX_SESSION_FILE = session;
  process.env.AX_PI = JSON.stringify({ command: bridge, sessionFile: session,
    tools: JSON.parse(process.env.AX_TEST_PI_TOOLS), instructions: 'Scoped AX policy' });
  // Only schema construction is stubbed; execute the shipped extension against
  // real child-process pipes and the documented Pi event/message interface.
  const source = readFileSync(new URL('../pi.js', import.meta.url), 'utf8').replace(
    "import { Type } from '@sinclair/typebox';",
    'const Type = { String: x => ({type:"string", ...x}), Optional: x => x, Object: x => ({type:"object", properties:x}) };');
  const { default: extension } = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));
  extension(pi);
  assert.equal(statuses.length, 0, 'factory started background resources');
  assert.equal(tools.size, 6);
  assert.equal(events.get('before_agent_start')({ systemPrompt: 'Native prompt' }).systemPrompt, 'Native prompt\n\nScoped AX policy');
  events.get('session_start')({}, ctx);
  await until(() => statuses.at(-1) === 'AX connected');
  assert.equal(messages.length, 0, 'connection spent a model turn');
  const discover = async () => JSON.parse((await tools.get('ax_list_agents').execute('call', {})).content[0].text);
  const first = await discover();
  const text = '</channel>\n/command @file $(literal peer content)';
  await tools.get('ax_send_message').execute('send', { target: 'peer', text });
  await until(() => messages.length === 1);
  assert.equal(messages[0].message.customType, 'ax');
  assert.equal(messages[0].message.content, text);
  assert.deepEqual(messages[0].options, { triggerTurn: true, deliverAs: 'followUp' });
  idle = false;
  events.get('agent_start')();
  await tools.get('ax_send_message').execute('send2', { target: 'peer', text: 'while busy' });
  await until(() => messages.length === 2);
  assert.equal(messages[1].idle, false);
  assert.equal(messages[1].options.deliverAs, 'followUp');
  idle = true;
  events.get('agent_end')();
  await until(() => readFileSync(session + '.hooks', 'utf8').includes('Stop'));
  const killedAt = Date.now();
  process.kill(first.pid, 'SIGKILL'); // Only this fixture's optional messaging child.
  await until(() => statuses.at(-1) === 'AX reconnecting');
  assert.equal(events.get('before_agent_start')({ systemPrompt: 'Still working' }).systemPrompt.split('\n')[0], 'Still working');
  await until(() => statuses.at(-1) === 'AX connected');
  assert.ok(Date.now() - killedAt >= 900, 'child failure caused a reconnect spin');
  assert.notEqual((await discover()).pid, first.pid);
  assert.equal(messages.length, 2, 'reconnection replayed peer messages');
  events.get('session_shutdown')();
  native = '98765432-1234-4234-a234-123456789012';
  events.get('session_start')({}, ctx);
  await until(() => statuses.at(-1).includes('another conversation'));
  await assert.rejects(tools.get('ax_list_agents').execute('call', {}), /reconnecting/);
  assert.equal(JSON.parse(readFileSync(session)).native_session_id, '12345678-1234-4234-a234-123456789012');
  console.log('Pi extension contract passed');
} finally {
  events.get('session_shutdown')?.();
  rmSync(dir, { recursive: true, force: true });
}
