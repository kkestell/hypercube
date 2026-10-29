import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import type { IDockviewPanelProps } from "dockview-react";
import type { Command, ConfigOption, PlanEntry, Usage } from "./model";
import { useModel } from "./modelStore";
import { request } from "./socket";
import { TranscriptEntry } from "./TranscriptItems";
import { useAgentInfo, useTranscript } from "./transcriptStore";

function tokens(count: number) {
  return count >= 1000 ? `${(count / 1000).toFixed(count >= 10_000 ? 0 : 1)}k` : String(count);
}

function usageText({ used, size, cost }: Usage) {
  const context = `${tokens(used)} / ${tokens(size)} tokens (${Math.round((used / size) * 100)}%)`;
  if (!cost) return context;
  return `${context} · ${new Intl.NumberFormat(undefined, { style: "currency", currency: cost.currency, maximumFractionDigits: 4 }).format(cost.amount)}`;
}

function ConfigSelect({ option, value, disabled, onChange }: { option: ConfigOption; value: string; disabled: boolean; onChange: (value: string) => void }) {
  return (
    <label className="flex items-center gap-1 text-neutral-400">
      {option.name}
      <select
        value={value}
        disabled={disabled}
        onChange={event => onChange(event.target.value)}
        title={option.options.find(choice => choice.value === value)?.description}
        className="max-w-56 rounded border border-neutral-600 bg-[#252526] px-1 py-0.5 text-neutral-200 disabled:opacity-50"
      >
        {option.options.map(choice => (
          <option key={choice.value} value={choice.value} title={choice.description}>
            {choice.name}
          </option>
        ))}
      </select>
    </label>
  );
}

const statusMarks = { pending: "○", in_progress: "◐", completed: "●" };

function Plan({ entries }: { entries: PlanEntry[] }) {
  return (
    <ul className="max-h-32 space-y-0.5 overflow-auto border-t border-neutral-700 px-3 py-2 text-xs">
      {entries.map((entry, index) => (
        <li key={index} className={entry.status === "completed" ? "text-neutral-500 line-through" : entry.status === "in_progress" ? "text-sky-300" : "text-neutral-300"}>
          <span className="mr-1.5 inline-block no-underline">{statusMarks[entry.status]}</span>
          {entry.content}
        </li>
      ))}
    </ul>
  );
}

/** Commands whose name starts with what's typed after the `/`, while the name is still being typed. */
function matchingCommands(commands: Command[], text: string) {
  const typed = /^\/(\S*)$/.exec(text)?.[1];
  if (typed === undefined) return [];
  return commands.filter(command => command.name.startsWith(typed)).slice(0, 8);
}

/** A dockview panel for one agent: its transcript and a composer. */
export function AgentPanel({ params }: IDockviewPanelProps<{ agentId: string }>) {
  const { agents, sessions } = useModel();
  const items = useTranscript(params.agentId);
  const { plan, usage, configOptions, commands } = useAgentInfo(params.agentId);
  const agent = agents[params.agentId];
  const session = agent && sessions[agent.sessionId];
  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);
  /** Config values chosen but not yet confirmed by the agent. */
  const [chosen, setChosen] = useState<Record<string, string>>({});
  const [highlighted, setHighlighted] = useState(0);
  const scroller = useRef<HTMLDivElement>(null);
  const following = useRef(true);

  // Keep the end in view while the user hasn't scrolled up.
  useEffect(() => {
    const element = scroller.current;
    if (element && following.current) element.scrollTop = element.scrollHeight;
  }, [items, agent?.queue, agent?.steer]);

  if (!agent || !session) return null;

  const matches = matchingCommands(commands ?? [], text);
  const command = matches[Math.min(highlighted, matches.length - 1)];
  const complete = (picked: Command) => setText(`/${picked.name} `);

  /** A busy agent is steered unless `queue`. */
  const send = (queue = false) => {
    const prompt = text.trim();
    if (!prompt) return;
    setText("");
    setError(null);
    following.current = true;
    request({ t: "prompt", agentId: agent.id, text: prompt, queue }).catch((reason: Error) => {
      setText(prompt);
      setError(reason.message);
    });
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    send();
  };
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.nativeEvent.isComposing) return;
    if (command && (event.key === "ArrowDown" || event.key === "ArrowUp")) {
      event.preventDefault();
      setHighlighted((Math.min(highlighted, matches.length - 1) + (event.key === "ArrowDown" ? 1 : matches.length - 1)) % matches.length);
    } else if (command && event.key === "Tab") {
      event.preventDefault();
      complete(command);
    } else if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      // Enter on a half-typed command completes it; on a whole one it sends.
      if (command && text !== `/${command.name}`) complete(command);
      else send(event.metaKey || event.ctrlKey);
    }
  };
  const stop = () => {
    request({ t: "cancel", agentId: agent.id }).catch((reason: Error) => setError(reason.message));
  };
  const interrupt = () => {
    request({ t: "interrupt", agentId: agent.id }).catch((reason: Error) => setError(reason.message));
  };
  const restart = () => {
    setError(null);
    request({ t: "restartAgent", agentId: agent.id }).catch((reason: Error) => setError(reason.message));
  };
  const setConfig = (configId: string, value: string) => {
    setError(null);
    setChosen(before => ({ ...before, [configId]: value }));
    request({ t: "setConfigOption", agentId: agent.id, configId, value })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setChosen(({ [configId]: _, ...rest }) => rest));
  };

  const canPrompt = session.status === "ready" && agent.status !== "exited";
  const working = agent.status === "busy" || agent.status === "waiting";
  const canStop = working || agent.queue.length > 0;

  return (
    <div className="flex h-full flex-col bg-[#1e1e1e] text-sm text-neutral-200">
      {(configOptions?.length || usage) && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-neutral-700 px-3 py-1.5 text-xs">
          {configOptions?.map(option => (
            <ConfigSelect key={option.id} option={option} value={chosen[option.id] ?? option.currentValue} disabled={agent.status === "exited"} onChange={value => setConfig(option.id, value)} />
          ))}
          {usage && <span className="ml-auto text-neutral-400">{usageText(usage)}</span>}
        </div>
      )}
      <div
        ref={scroller}
        onScroll={event => {
          const element = event.currentTarget;
          following.current = element.scrollHeight - element.scrollTop - element.clientHeight < 40;
        }}
        className="min-h-0 flex-1 space-y-3 overflow-auto p-3"
      >
        {session.status === "cloning" && <div className="text-neutral-500">Cloning…</div>}
        {session.status === "failed" && <div className="break-words whitespace-pre-wrap text-red-300">{session.error}</div>}
        {session.status === "ready" && agent.status === "starting" && <div className="text-neutral-500">Starting…</div>}
        {session.status === "stopped" && <div className="text-neutral-500">Stopped. Start the session to resume.</div>}
        {items.map(item => (
          <TranscriptEntry key={item.id} agentId={agent.id} item={item} root={session.clonePath} />
        ))}
        {agent.steer && (
          <div className="ml-8 space-y-2 rounded border border-dashed border-amber-600/70 px-3 py-2 break-words whitespace-pre-wrap text-neutral-300">
            <div className="flex items-center gap-2 text-[11px] tracking-wide text-amber-300 uppercase">
              <span>{agent.steer.waitingFor ? `Steer: waiting for ${agent.steer.waitingFor} to finish` : "Steer: interrupting"}</span>
              {agent.steer.waitingFor && (
                <button type="button" onClick={interrupt} className="ml-auto rounded bg-amber-800 px-2 py-0.5 normal-case hover:bg-amber-700">
                  Interrupt now
                </button>
              )}
            </div>
            {agent.steer.prompts.map((steered, index) => (
              <div key={index}>
                {steered.from && <div className="text-[11px] tracking-wide text-neutral-400 uppercase">From {steered.from}</div>}
                {steered.text}
              </div>
            ))}
          </div>
        )}
        {agent.queue.map((queued, index) => (
          <div key={index} className="ml-8 rounded border border-dashed border-neutral-600 px-3 py-2 break-words whitespace-pre-wrap text-neutral-400">
            <div className="text-[11px] tracking-wide uppercase">{queued.from ? `Queued, from ${queued.from}` : "Queued"}</div>
            {queued.text}
          </div>
        ))}
      </div>
      {plan && plan.length > 0 && <Plan entries={plan} />}
      <form onSubmit={submit} className="relative border-t border-neutral-700 p-2">
        {matches.length > 0 && (
          <ul className="absolute right-2 bottom-full left-2 overflow-hidden rounded border border-neutral-600 bg-[#252526] text-xs shadow-lg">
            {matches.map(match => (
              <li
                key={match.name}
                // Keep focus in the textarea.
                onMouseDown={event => event.preventDefault()}
                onClick={() => complete(match)}
                className={`flex cursor-pointer gap-2 px-2 py-1 ${match === command ? "bg-neutral-700" : "hover:bg-neutral-700/60"}`}
              >
                <span className="font-mono">
                  /{match.name}
                  {match.hint && <span className="text-neutral-500"> {match.hint}</span>}
                </span>
                <span className="truncate text-neutral-400">{match.description}</span>
              </li>
            ))}
          </ul>
        )}
        {error && <div className="pb-2 text-xs break-words text-red-300">{error}</div>}
        <div className="flex items-end gap-2">
          <textarea
            value={text}
            onChange={event => {
              setText(event.target.value);
              setHighlighted(0);
            }}
            onKeyDown={onKeyDown}
            disabled={!canPrompt}
            rows={2}
            placeholder={canPrompt ? (working ? `Steer ${agent.name} (Enter) or queue (⌘/Ctrl+Enter)` : `Message ${agent.name}`) : "Not available"}
            className="min-w-0 flex-1 resize-none rounded border border-neutral-600 bg-[#252526] px-2 py-1 disabled:opacity-50"
          />
          {agent.status === "exited" && session.status === "ready" && (
            <button type="button" onClick={restart} className="rounded bg-neutral-700 px-3 py-1 hover:bg-neutral-600">
              Restart
            </button>
          )}
          {canStop && (
            <button type="button" onClick={stop} className="rounded bg-red-800 px-3 py-1 hover:bg-red-700">
              Stop
            </button>
          )}
          {working && (
            <button type="button" onClick={() => send(true)} disabled={!canPrompt || !text.trim()} className="rounded bg-neutral-700 px-3 py-1 enabled:hover:bg-neutral-600 disabled:opacity-50">
              Queue
            </button>
          )}
          <button type="submit" disabled={!canPrompt || !text.trim()} className="rounded bg-sky-700 px-3 py-1 enabled:hover:bg-sky-600 disabled:opacity-50">
            {working ? "Steer" : "Send"}
          </button>
        </div>
      </form>
    </div>
  );
}
