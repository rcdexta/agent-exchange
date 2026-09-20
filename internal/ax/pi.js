import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { readFile } from 'node:fs/promises';
import { Type } from '@sinclair/typebox';

// Pi owns its terminal and this optional messaging child. Nothing here owns or
// terminates Pi, and peer content enters as a custom message, never user input.
export default function ax(pi) {
  const options = JSON.parse(process.env.AX_PI);
  let current, child, retry, connecting, ready = false, sequence = 0;
  const pending = new Map();

  function status(text) {
    current?.ctx.ui.setStatus('ax', text);
  }

  function disconnect(error) {
    ready = false;
    const old = child;
    child = undefined;
    old?.kill();
    for (const request of pending.values()) request.reject(error);
    pending.clear();
  }

  function rpc(method, params) {
    if (!child) return Promise.reject(new Error('AX messaging is reconnecting'));
    const id = ++sequence;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => finish(new Error('AX messaging timed out')), 15000);
      function finish(error, value) {
        clearTimeout(timer);
        pending.delete(id);
        error ? reject(error) : resolve(value);
      }
      pending.set(id, { resolve: value => finish(null, value), reject: finish });
      child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n', error => {
        if (error) finish(error);
      });
    });
  }

  async function tool(name, args) {
    const result = await rpc('tools/call', { name, arguments: args });
    if (result.isError) throw new Error(result.content?.[0]?.text || 'AX tool failed');
    return { content: result.content, details: {} };
  }

  function lifecycle(session, event) {
    return new Promise((resolve, reject) => {
      const hook = spawn(options.command, ['hook'], { stdio: ['pipe', 'ignore', 'pipe'] });
      let message = '';
      const timeout = setTimeout(() => hook.kill(), 3000);
      hook.stderr.on('data', data => { message = (message + data).slice(-2048); });
      hook.stdin.on('error', () => {});
      hook.on('error', error => { clearTimeout(timeout); reject(error); });
      hook.on('close', code => {
        clearTimeout(timeout);
        code === 0 ? resolve() : reject(new Error(message.trim() || 'AX lifecycle unavailable'));
      });
      hook.stdin.end(JSON.stringify({
        session_id: session.id, session_file: session.file,
        hook_event_name: event, permission_mode: 'native',
      }));
    });
  }

  async function connect(session) {
    if (current !== session || child || connecting) return;
    connecting = session;
    try {
      const saved = JSON.parse(await readFile(options.sessionFile, 'utf8'));
      if (current !== session) return;
      if (saved.native_session_id && saved.native_session_id !== session.id) {
        status('AX name belongs to another conversation; launch with a new AX name');
        return;
      }
      const bridge = spawn(options.command, ['bridge'], { stdio: ['pipe', 'pipe', 'ignore'] });
      child = bridge;
      const failed = error => {
        if (child !== bridge) return;
        disconnect(error);
        status('AX reconnecting');
        schedule(session);
      };
      bridge.on('error', failed);
      bridge.on('close', () => failed(new Error('AX messaging disconnected')));
      bridge.stdin.on('error', failed);
      const lines = createInterface({ input: bridge.stdout });
      lines.on('line', line => {
        if (child !== bridge || current !== session) return;
        try {
          const message = JSON.parse(line);
          if (message.id !== undefined) {
            const request = pending.get(message.id);
            if (message.error) request?.reject(new Error(message.error.message));
            else request?.resolve(message.result);
          } else if (message.method === 'notifications/ax/message') {
            const p = message.params;
            if (p.native_session_id !== session.id || typeof p.content !== 'string') return;
            pi.sendMessage({ customType: 'ax', content: p.content, display: true, details: p.meta },
              { triggerTurn: true, deliverAs: 'followUp' });
          }
        } catch (error) {
          failed(error);
        }
      });
      await lifecycle(session, 'SessionStart');
      if (!session.ctx.isIdle()) await lifecycle(session, 'UserPromptSubmit');
      if (current !== session || child !== bridge) return;
      await rpc('initialize', { protocolVersion: '2025-11-25', capabilities: {}, clientInfo: { name: 'ax-pi', version: '1' } });
      await rpc('tools/list', {});
      await tool('list_agents', {}); // Connect without spending a model turn.
      if (current === session && child === bridge) {
        ready = true;
        status('AX connected');
      }
    } catch (error) {
      if (current === session) {
        disconnect(error);
        status('AX unavailable: ' + error.message);
        schedule(session);
      }
    } finally {
      if (connecting === session) connecting = undefined;
    }
  }

  function schedule(session) {
    if (current !== session || retry) return;
    retry = setTimeout(() => {
      retry = undefined;
      if (connecting) schedule(session);
      else void connect(session);
    }, 1000);
  }

  for (const spec of options.tools) {
    const fields = Object.fromEntries(Object.entries(spec.inputSchema.properties).map(([name, value]) => {
      const schema = Type.String({ description: value.description });
      return [name, spec.inputSchema.required.includes(name) ? schema : Type.Optional(schema)];
    }));
    pi.registerTool({
      name: 'ax_' + spec.name, label: 'AX ' + spec.name, description: spec.description,
      parameters: Type.Object(fields, { additionalProperties: false }),
      async execute(_id, args) {
        if (!ready) throw new Error('AX messaging is reconnecting; retry shortly');
        return tool(spec.name, args);
      },
    });
  }

  function stop() {
    current = undefined;
    clearTimeout(retry);
    retry = undefined;
    connecting = undefined;
    disconnect(new Error('AX session closed'));
  }
  pi.on('session_start', (_event, ctx) => {
    stop();
    current = { ctx, id: ctx.sessionManager.getSessionId(), file: ctx.sessionManager.getSessionFile() };
    void connect(current);
  });
  pi.on('session_shutdown', stop);
  pi.on('before_agent_start', event => ({ systemPrompt: event.systemPrompt + '\n\n' + options.instructions }));
  let stateChange = Promise.resolve();
  for (const [event, hook] of [['agent_start', 'UserPromptSubmit'], ['agent_end', 'Stop']]) {
    pi.on(event, () => {
      const session = current;
      if (!session) return;
      stateChange = stateChange.then(() => current === session ? lifecycle(session, hook) : undefined)
        .catch(error => status('AX unavailable: ' + error.message));
    });
  }
}
