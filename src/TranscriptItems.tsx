import { memo, useState } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { ToolContent, ToolLocation, TranscriptItem } from "./model";
import { request } from "./socket";

type Permission = Extract<TranscriptItem, { kind: "permission" }>;
type Tool = Extract<TranscriptItem, { kind: "tool" }>;

function PermissionRequest({ agentId, item }: { agentId: string; item: Permission }) {
  const [error, setError] = useState<string | null>(null);
  const answer = (optionId: string) => {
    setError(null);
    request({ t: "answerPermission", agentId, requestId: item.requestId, optionId }).catch((reason: Error) => setError(reason.message));
  };

  return (
    <div className="rounded border border-amber-700/60 bg-amber-950/30 p-2">
      <div className="break-words">{item.title}</div>
      <div className="mt-2 flex flex-wrap gap-2">
        {item.options.map(option => (
          <button
            key={option.optionId}
            type="button"
            disabled={item.answer !== undefined}
            onClick={() => answer(option.optionId)}
            className={`rounded px-2 py-1 text-xs disabled:opacity-50 ${item.answer === option.optionId ? "bg-sky-700" : "bg-neutral-700 enabled:hover:bg-neutral-600"}`}
          >
            {option.name}
          </button>
        ))}
        {item.answer === "cancelled" && <span className="self-center text-xs text-neutral-400">Cancelled</span>}
      </div>
      {error && <div className="mt-2 text-xs text-red-300">{error}</div>}
    </div>
  );
}

/** A path inside the session's clone is shown relative to it. */
function shortPath(path: string, root: string) {
  return path.startsWith(`${root}/`) ? path.slice(root.length + 1) : path;
}

function place({ path, line }: ToolLocation, root: string) {
  // The clone itself (a shell's directory, say) says nothing.
  if (path === root) return "";
  return line ? `${shortPath(path, root)}:${line}` : shortPath(path, root);
}

function lineClass(line: string) {
  if (line.startsWith("+")) return "bg-green-950/60 text-green-300";
  if (line.startsWith("-")) return "bg-red-950/60 text-red-300";
  if (line.startsWith("@@")) return "text-sky-400";
  return "text-neutral-400";
}

function ToolOutput({ content, root }: { content: ToolContent; root: string }) {
  if (content.type === "text") {
    return <pre className="max-h-80 overflow-auto rounded bg-[#151515] p-2 break-words whitespace-pre-wrap">{content.text}</pre>;
  }
  return (
    <div className="rounded bg-[#151515]">
      <div className="border-b border-neutral-700 px-2 py-1 text-neutral-400">{shortPath(content.path, root)}</div>
      {content.patch ? (
        <div className="max-h-96 overflow-auto py-1">
          <div className="w-max min-w-full whitespace-pre">
            {content.patch.split("\n").map((line, index) => (
              <div key={index} className={`px-2 ${lineClass(line)}`}>
                {line || " "}
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className="px-2 py-1 text-neutral-500">No changes</div>
      )}
    </div>
  );
}

function ToolCall({ item, root }: { item: Tool; root: string }) {
  const places = item.locations.map(location => place(location, root)).filter(Boolean);
  const summary = (
    <span className="font-mono text-xs break-words text-neutral-400">
      {item.toolKind} · {item.title}
      {places[0] && ` · ${places[0]}${places.length > 1 ? ` +${places.length - 1}` : ""}`} · {item.status}
    </span>
  );
  if (item.content.length === 0 && places.length < 2) return <div>{summary}</div>;
  return (
    <details>
      <summary className="cursor-pointer select-none">{summary}</summary>
      <div className="mt-1 space-y-2 font-mono text-xs text-neutral-300">
        {places.length > 1 && (
          <ul>
            {places.map(path => (
              <li key={path}>{path}</li>
            ))}
          </ul>
        )}
        {item.content.map((content, index) => (
          <ToolOutput key={index} content={content} root={root} />
        ))}
      </div>
    </details>
  );
}

function ExternalLink(props: React.ComponentProps<"a">) {
  return <a {...props} target="_blank" rel="noreferrer noopener" />;
}

/** One transcript entry. Memoized so a streaming message doesn't re-render the markdown above it. */
export const TranscriptEntry = memo(function TranscriptEntry({ agentId, item, root }: { agentId: string; item: TranscriptItem; root: string }) {
  switch (item.kind) {
    case "user":
      if (item.from) {
        return (
          <div className="mr-8 rounded border border-sky-800/60 bg-sky-950/30 px-3 py-2 break-words whitespace-pre-wrap">
            <div className="text-[11px] tracking-wide text-sky-300 uppercase">
              From {item.from}
              {item.steered && " (steer)"}
            </div>
            {item.text}
          </div>
        );
      }
      return (
        <div className="ml-8 rounded bg-[#2a2d2e] px-3 py-2 break-words whitespace-pre-wrap">
          {item.steered && <div className="text-[11px] tracking-wide text-amber-300 uppercase">Steer</div>}
          {item.text}
        </div>
      );
    case "agent":
      return (
        <div className="markdown">
          <Markdown remarkPlugins={[remarkGfm]} components={{ a: ExternalLink }}>{item.text}</Markdown>
        </div>
      );
    case "thought":
      return (
        <details className="text-neutral-400">
          <summary className="cursor-pointer select-none">Thinking</summary>
          <div className="pt-1 break-words whitespace-pre-wrap">{item.text}</div>
        </details>
      );
    case "tool":
      return <ToolCall item={item} root={root} />;
    case "permission":
      return <PermissionRequest agentId={agentId} item={item} />;
    case "notice":
      return <div className="text-xs break-words whitespace-pre-wrap text-amber-300">{item.text}</div>;
  }
});
