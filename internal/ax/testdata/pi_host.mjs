import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync, chmodSync, rmSync, existsSync } from 'node:fs';
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
function startSession(reason) {
  // A previous session's connected status does not prove this one is ready.
  statuses.length = 0;
  events.get('session_start')({ reason }, ctx);
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
    "import { Type } from 'typebox';",
    'const Type = { String: x => ({type:"string", ...x}), Integer: x => ({type:"integer", ...x}), Array: (x, opts) => ({type:"array", items:x, ...opts}), Optional: x => x, Object: x => ({type:"object", properties:x}) };');
  const { default: extension } = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));
  extension(pi);
  assert.equal(statuses.length, 0, 'factory started background resources');
  assert.equal(tools.size, JSON.parse(process.env.AX_TEST_PI_TOOLS).length);
  assert.equal(tools.get("ax_spawn_agent").parameters.properties.args.type, "array");
  for (const name of ['ax_send_message', 'ax_reply', 'ax_resend_message']) {
    const ttl = tools.get(name).parameters.properties.ttl_seconds;
    assert.equal(ttl.type, 'integer');
    assert.equal(ttl.minimum, 1);
    assert.equal(ttl.maximum, 604800);
  }
  assert.equal(tools.get('ax_list_pending').parameters.properties.after_seq.type, 'integer');
  assert.equal(events.get('before_agent_start')({ systemPrompt: 'Native prompt' }).systemPrompt, 'Native prompt\n\nScoped AX policy');
  startSession();
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
  assert.ok(existsSync(session + '.initialized'), 'MCP handshake was incomplete');
  const original = native;
  for (const reason of ['new', 'fork']) {
    // Identity can change before the lifecycle callback reaches this extension.
    native = '98765432-1234-4234-a234-123456789012';
    await assert.rejects(tools.get('ax_list_agents').execute('call', {}), /reconnecting/);
    events.get('session_shutdown')();
    startSession(reason);
    await until(() => statuses.at(-1)?.includes('another conversation'));
    await assert.rejects(tools.get('ax_list_agents').execute('call', {}), /reconnecting/);
    assert.equal(JSON.parse(readFileSync(session)).native_session_id, original);
    native = original;
    events.get('session_shutdown')();
    startSession('resume');
    await until(() => statuses.at(-1) === 'AX connected');
  }
  // Replace a session while initialize is outstanding. Its late rejection must
  // neither disconnect the replacement nor run more calls on that connection.
  writeFileSync(session + '.delay', 'yes');
  const before = readFileSync(session + '.calls', 'utf8').trim().split('\n').length;
  startSession('reload');
  await until(() => readFileSync(session + '.calls', 'utf8').trim().split('\n').length > before);
  const stalePID = JSON.parse(readFileSync(session + '.calls', 'utf8').trim().split('\n').at(-1)).pid;
  events.get('session_shutdown')();
  startSession('reload');
  await until(() => statuses.at(-1) === 'AX connected');
  rmSync(session + '.delay');
  assert.notEqual((await discover()).pid, stalePID);
  assert.deepEqual(readFileSync(session + '.calls', 'utf8').trim().split('\n').map(JSON.parse)
    .filter(call => call.pid === stalePID).map(call => call.method), ['initialize']);
  assert.equal(messages.length, 2, 'session replacement replayed old mail');
  console.log('Pi extension contract passed');
} finally {
  events.get('session_shutdown')?.();
  rmSync(dir, { recursive: true, force: true });
}
