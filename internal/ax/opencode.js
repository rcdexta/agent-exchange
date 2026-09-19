// Loaded only by the TUI launched by AX. Native configuration and permissions
// remain owned by OpenCode; peer bodies never enter its user-prompt API.
export default {
  id: "agent-exchange",
  async tui(api) {
    const options = JSON.parse(process.env.AX_OPENCODE);
    const abort = new AbortController();
    const request = async (path, body) => {
      const response = await fetch(`http://ax${path}`, {
        unix: options.socket,
        timeout: false,
        signal: abort.signal,
        ...(body ? { method: "POST", body: JSON.stringify(body) } : {}),
      });
      if (!response.ok) throw new Error(await response.text());
      return response.status === 204 ? undefined : response.json();
    };
    let selected, creating = false, checking = false, previous;
    const report = (error) => api.ui.toast({title: "AX", message: String(error), variant: "error", duration: 10000});
    const check = async () => {
      if (checking || !api.state.ready || abort.signal.aborted) return;
      checking = true;
      try {
        let route = api.route.current;
        if (!selected && route.name === "home" && !options.waitForSession && !creating) {
          creating = true;
          const result = await api.client.session.create();
          if (result.error || !result.data) throw new Error("OpenCode could not create the AX conversation");
          api.route.navigate("session", {sessionID: result.data.id});
          route = api.route.current;
        }
        if (route.name !== "session") return;
        const native = route.params.sessionID;
        const session = api.state.session.get(native);
        if (!session || session.parentID) return;
        const state = api.state.session.permission(native).length || api.state.session.question(native).length
          ? "blocked" : api.state.session.status(native)?.type === "busy" ? "busy" : "ready";
        const permission = options.auto ? "auto" : api.state.config.permission === "allow" || api.state.config.permission?.["*"] === "allow" ? "bypassPermissions" : "default";
        const key = `${native}:${state}:${permission}`;
        if (key === previous) return;
        selected = native;
        await request("/bind", {native, state, permission});
        previous = key;
      } finally { checking = false; }
    };
    // Session selection is TUI state, not a model turn or a transcript lookup.
    const timer = setInterval(() => check().catch(report), 200);
    const receive = async () => {
      while (!abort.signal.aborted) {
        const wake = await request("/next");
        let error;
        try {
          if (wake.native !== selected) throw new Error("AX wake belongs to another conversation");
          const messages = api.state.session.messages(selected);
          const last = [...messages].reverse().find((message) => message.role === "user");
          const model = last?.model || (options.model ? {providerID: options.model.split("/")[0], modelID: options.model.split("/").slice(1).join("/")} : undefined);
          const result = await api.client.session.promptAsync({sessionID: selected, model, agent: last?.agent || options.agent, parts: [{type: "text", text: wake.text}]});
          if (result.error) throw new Error(JSON.stringify(result.error));
        } catch (cause) { error = String(cause); report(cause); }
        await request("/receipt", {id: wake.id, error});
      }
    };
    receive().catch((error) => { if (!abort.signal.aborted) report(error); });
    api.lifecycle.onDispose(() => { clearInterval(timer); abort.abort(); });
  },
};
