#!/usr/bin/env node
// A pipe-only bridge fixture; this does not simulate a model or claim a live Pi exchange.
import { readFileSync, writeFileSync, appendFileSync, existsSync } from 'node:fs';
import { createInterface } from 'node:readline';
const sessionFile = process.env.AX_SESSION_FILE;
const lines = createInterface({ input: process.stdin });
if (process.argv[2] === 'hook') {
  let input = '';
  for await (const line of lines) input += line;
  const hook = JSON.parse(input);
  const saved = JSON.parse(readFileSync(sessionFile));
  saved.native_session_id = hook.session_id;
  writeFileSync(sessionFile, JSON.stringify(saved));
  appendFileSync(sessionFile + '.hooks', hook.hook_event_name + '\n');
} else {
  const emit = packet => console.log(JSON.stringify({ jsonrpc: '2.0', ...packet }));
  let initialized = false;
  for await (const line of lines) {
    const request = JSON.parse(line);
    appendFileSync(sessionFile + '.calls', JSON.stringify({pid:process.pid,method:request.method}) + '\n');
    if (request.method === 'initialize' && existsSync(sessionFile + '.delay')) await new Promise(r => setTimeout(r, 250));
    if (request.method === 'notifications/initialized') {
      initialized = true;
      appendFileSync(sessionFile + '.initialized', 'yes\n');
      continue;
    }
    if (request.method !== 'initialize' && !initialized) {
      emit({id:request.id,error:{message:'missing initialized notification'}});
      continue;
    }
    let result = {};
    if (request.method === 'tools/call') {
      if (request.params.name === 'send_message') {
        const params = { native_session_id: JSON.parse(readFileSync(sessionFile)).native_session_id,
          content: request.params.arguments.text, meta: { message_id: 'msg_fixture' } };
        emit({ method: 'notifications/ax/message', params: { ...params, native_session_id: 'wrong-session' } });
        emit({ method: 'notifications/ax/message', params });
      }
      result = { content: [{ type: 'text', text: JSON.stringify({ pid: process.pid, name: request.params.name }) }] };
    }
    emit({ id: request.id, result });
  }
}
