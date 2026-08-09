import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Maximize2, Minimize2, RotateCcw, X } from "lucide-react";
import { api, ApiError, type AdvisorMessage, type AdvisorStatus } from "../lib/api";
import { buildAdvisorContext } from "../lib/advisor";
import { ADVISOR_TOOLS, describeToolCall, runAdvisorTool } from "../lib/advisor-tools";
import { toSavePals } from "../lib/savepals";
import { cn } from "../lib/utils";
import { Select } from "./ui/select";
import { ADVISOR_AVATAR } from "./AdvisorOverlay";

/**
 * The advisor's chat window — the panel behind the floating bubble
 * (AdvisorOverlay). Split from the bubble so this file's heavy imports
 * (breeding table, stat catalogs, the context builder) load only when
 * someone actually opens the chat.
 *
 * The browser sends the same derived numbers the calculator pages render
 * with every question; the server holds the API key and the prompt.
 * Tools — the calculators, the palcon docs search, the Palworld wiki —
 * execute here too (lib/advisor-tools.ts). Conversation state lives in this
 * component: closing the bubble keeps it (the overlay keeps the panel
 * mounted), switching servers resets it (the overlay keys the panel by
 * server).
 *
 * Replies render as plain text on purpose: the system prompt forbids
 * markdown, which keeps a rendering dependency out of the bundle and keeps
 * answers in the app's own voice rather than a formatted document's.
 */

// Request limits belong to the KEY (its account/project and tier), not the
// model — Google stopped publishing per-model numbers entirely, and
// Anthropic's vary by usage tier — so the picker shows an honest per-
// provider note and links to the dashboard that knows the real numbers,
// rather than figures that would be wrong for most keys.
const PROVIDERS = [
  {
    id: "anthropic",
    label: "Anthropic Claude",
    apiLabel: "the Anthropic API",
    limitsNote: "Requests/min depend on the key's usage tier (entry tier: 1,000/min per model, no daily cap).",
    limitsUrl: "https://platform.claude.com/settings/limits",
  },
  {
    id: "gemini",
    label: "Google Gemini",
    apiLabel: "the Google Gemini API",
    limitsNote: "Quotas are per Google project — free-tier keys have small per-model daily caps.",
    limitsUrl: "https://aistudio.google.com/rate-limit",
  },
];

function apiLabel(provider?: string): string {
  return PROVIDERS.find((p) => p.id === provider)?.apiLabel ?? "the model provider's API";
}

/** The active model's short display name ("Claude Opus 5"), falling back
 * to the raw id for anything the options list doesn't know. */
function modelLabel(status: AdvisorStatus): string {
  const label = status.modelOptions?.[status.provider]?.find((m) => m.id === status.model)?.label;
  return label ? label.split(" · ")[0] : status.model;
}

/** One form for everything about a key: provider, model, and the key
 * itself, shared by shared-key and personal-key flows. When a key is
 * already saved (`current`), the pickers prefill from it and the key field
 * may stay blank — submitting then changes only the model, via the
 * model-only endpoint, and the button says so. Typing a key saves a full
 * replacement. The key field starts empty either way: there is nothing to
 * show; the server never returns it. */
function AdvisorKeyForm({
  submitLabel,
  current,
  modelOptions,
  saveKey,
  saveModel,
  onStatusChange,
}: {
  submitLabel: string;
  /** The saved key's provider and model, when one exists for this scope. */
  current?: { provider: string; model: string };
  modelOptions: Record<string, { id: string; label: string }[]>;
  saveKey: (provider: string, apiKey: string, model: string) => Promise<AdvisorStatus>;
  saveModel?: (model: string) => Promise<AdvisorStatus>;
  onStatusChange: (status: AdvisorStatus) => void;
}) {
  const [provider, setProvider] = useState(current?.provider ?? PROVIDERS[0].id);
  const [model, setModel] = useState(
    current?.model ?? modelOptions[current?.provider ?? PROVIDERS[0].id]?.[0]?.id ?? "",
  );
  const [key, setKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const options = modelOptions[provider] ?? [];

  const keyTrim = key.trim();
  const providerChanged = current ? provider !== current.provider : true;
  // Blank key + same provider + different model = a model-only change.
  const modelOnly = !!current && !!saveModel && !providerChanged && !keyTrim && model !== current.model;
  const submittable = !!keyTrim || modelOnly;

  return (
    <form
      onSubmit={async (e) => {
        e.preventDefault();
        if (!submittable || saving) return;
        setSaving(true);
        setError(null);
        try {
          onStatusChange(keyTrim ? await saveKey(provider, keyTrim, model) : await saveModel!(model));
          setKey("");
        } catch (err) {
          setError(err instanceof ApiError ? err.message : "Couldn't save the change.");
        } finally {
          setSaving(false);
        }
      }}
      className="space-y-2"
    >
      <Select
        value={provider}
        onChange={(e) => {
          setProvider(e.target.value);
          // The lists don't overlap between providers; reset to the new
          // provider's default rather than carrying a stale choice.
          setModel(modelOptions[e.target.value]?.[0]?.id ?? "");
        }}
        aria-label="Model provider"
      >
        {PROVIDERS.map((p) => (
          <option key={p.id} value={p.id}>
            {p.label}
          </option>
        ))}
      </Select>
      {options.length > 0 && (
        <Select value={model} onChange={(e) => setModel(e.target.value)} aria-label="Model">
          {options.map((m) => (
            <option key={m.id} value={m.id}>
              {m.label}
            </option>
          ))}
        </Select>
      )}
      {(() => {
        const p = PROVIDERS.find((x) => x.id === provider);
        return p ? (
          <p className="text-[10px] leading-relaxed text-ink/40">
            {p.limitsNote}{" "}
            <a
              href={p.limitsUrl}
              target="_blank"
              rel="noreferrer"
              className="font-semibold text-ink/55 underline decoration-ink/20 transition hover:text-ink"
            >
              Check this key's limits
            </a>
          </p>
        ) : null;
      })()}
      <input
        type="password"
        value={key}
        onChange={(e) => setKey(e.target.value)}
        placeholder={current ? "API key (leave blank to keep yours)" : "API key"}
        aria-label="API key"
        autoComplete="off"
        maxLength={512}
        className="w-full rounded-xl border border-ink/10 bg-white px-3.5 py-2 text-sm outline-none transition focus:border-brand-red/50"
      />
      {current && providerChanged && !keyTrim && (
        <p className="text-[10px] text-ink/40">Switching provider needs that provider's API key.</p>
      )}
      <button
        type="submit"
        disabled={saving || !submittable}
        className={cn(
          "rounded-xl bg-brand-red px-4 py-2 text-sm font-bold text-paper transition",
          (saving || !submittable) && "opacity-40",
        )}
      >
        {saving ? "Saving…" : modelOnly ? "Change model" : submitLabel}
      </button>
      {error && <p className="text-xs text-destructive">{error}</p>}
    </form>
  );
}

// Mirrors the server's conversation cap. Checked here, before sending, so
// a full conversation reads as "context is full, start fresh" rather than
// the server's raw validation error.
const MAX_TURNS = 40;
const CONTEXT_FULL =
  "This conversation has reached its maximum context. Start a new chat to keep asking — the advisor reads the save fresh every question, so nothing is lost.";

export function AdvisorPanel({
  serverId,
  status,
  onStatusChange,
  onClose,
  expanded,
  onToggleExpand,
}: {
  serverId: number;
  status: AdvisorStatus;
  onStatusChange: (status: AdvisorStatus) => void;
  onClose: () => void;
  expanded: boolean;
  onToggleExpand: () => void;
}) {
  const [turns, setTurns] = useState<AdvisorMessage[]>([]);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [removing, setRemoving] = useState(false);
  const [rounds, setRounds] = useState(String(status.maxToolRounds || 8));
  const scrollRef = useRef<HTMLDivElement>(null);

  // Same read (and same cache entry) as the pal pages; the advisor works
  // without it — a save-less server still gets game and console answers.
  const palsQuery = useQuery({
    queryKey: ["server-pals", serverId],
    queryFn: () => api.serverPals(serverId),
    enabled: status.enabled,
    retry: false,
    gcTime: 60 * 60_000,
    staleTime: 60_000,
  });
  const savePals = useMemo(
    () => (palsQuery.data ? toSavePals(palsQuery.data.players) : []),
    [palsQuery.data],
  );
  const ctx = useMemo(
    () => buildAdvisorContext(savePals, palsQuery.data?.guilds ?? []),
    [savePals, palsQuery.data],
  );

  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [turns, sending]);

  // One question can take several model turns: when the model asks for tool
  // calls, run them here (the calculators, docs search and wiki live in
  // this bundle) and re-submit until it answers in words. The cap is the
  // admin's setting; on the last permitted round the results carry a note
  // telling the model to answer with what it has, so hitting the cap ends
  // in a best-effort answer instead of five rounds of discarded work.
  const maxRounds = Math.max(1, status.maxToolRounds || 8);
  const NUDGE =
    "[palcon] That was the last tool round for this question. Answer now with what you already have — do not request more tools.";
  const newChat = () => {
    setTurns([]);
    setError(null);
  };

  const ask = async (question: string) => {
    const q = question.trim();
    if (!q || sending) return;
    if (turns.length + 1 > MAX_TURNS) {
      setError(CONTEXT_FULL);
      return;
    }
    let next: AdvisorMessage[] = [...turns, { role: "user", content: q }];
    setTurns(next);
    setInput("");
    setSending(true);
    setError(null);
    try {
      for (let round = 0; ; round++) {
        // Tool rounds add two turns each, so a long question can fill the
        // context mid-loop — stop with the same message either way.
        if (next.length > MAX_TURNS) {
          setError(CONTEXT_FULL);
          break;
        }
        const res = await api.advisorChat(serverId, ctx.json, next, ADVISOR_TOOLS);
        if (!res.toolCalls?.length) {
          setTurns([...next, { role: "assistant", content: res.reply ?? "" }]);
          break;
        }
        if (round >= maxRounds) {
          // The nudge was ignored. Salvage any preamble the model wrote
          // before its calls; only a truly wordless turn becomes an error.
          if (res.content) {
            setTurns([...next, { role: "assistant", content: res.content }]);
          } else {
            setError("The advisor couldn't finish inside its tool budget — try a more specific question.");
          }
          break;
        }
        // The assistant turn is echoed verbatim (ids and signatures
        // included) so the provider can pair the results with its calls.
        const results = await Promise.all(
          res.toolCalls.map(async (c) => ({ id: c.id, name: c.name, content: await runAdvisorTool(c.name, c.args) })),
        );
        next = [
          ...next,
          { role: "assistant", content: res.content ?? "", toolCalls: res.toolCalls },
          { role: "tool", toolResults: results, content: round === maxRounds - 1 ? NUDGE : undefined },
        ];
        setTurns(next);
      }
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "The advisor is unreachable — ask again in a moment.");
    } finally {
      setSending(false);
    }
  };

  return (
    <section
      aria-label="Advisor chat"
      className={cn(
        "flex flex-col overflow-hidden rounded-2xl border border-ink/15 bg-paper shadow-2xl",
        expanded ? "h-full w-full max-w-3xl" : "h-[min(38rem,calc(100dvh-10rem))] w-[calc(100vw-2rem)] max-w-sm",
      )}
    >
      <header className="flex items-center gap-2.5 border-b border-ink/10 bg-white/70 px-4 py-3">
        <img
          src={ADVISOR_AVATAR}
          alt=""
          className="h-9 w-9 rounded-full border border-brand-amber/50 bg-brand-amber/15 object-contain p-0.5"
        />
        <div className="min-w-0 flex-1">
          <p className="font-display text-sm font-bold leading-tight">Ask Anubis</p>
          <p className="truncate text-[11px] text-ink/45">
            {status.enabled
              ? `${ctx.counts.pals} pals in view · pals, the game & palcon itself`
              : "not set up yet"}
          </p>
        </div>
        {turns.length > 0 && (
          <button
            onClick={newChat}
            aria-label="Start a new chat"
            title="Start a new chat"
            className="rounded-lg p-1.5 text-ink/40 transition hover:bg-ink/5 hover:text-ink"
          >
            <RotateCcw className="h-4 w-4" />
          </button>
        )}
        <button
          onClick={onToggleExpand}
          aria-label={expanded ? "Shrink the chat" : "Expand the chat"}
          title={expanded ? "Shrink the chat" : "Expand the chat"}
          className="rounded-lg p-1.5 text-ink/40 transition hover:bg-ink/5 hover:text-ink"
        >
          {expanded ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
        </button>
        <button onClick={onClose} aria-label="Close the advisor" className="rounded-lg p-1.5 text-ink/40 transition hover:bg-ink/5 hover:text-ink">
          <X className="h-4 w-4" />
        </button>
      </header>

      {!status.enabled ? (
        <div className="flex-1 overflow-y-auto p-4">
          {status.canConfigure ? (
            <>
              <p className="text-sm text-muted-foreground">
                A chat that reads this server's save, the console's calculators, palcon's docs and the Palworld
                wiki. Add a model API key to turn it on for everyone. The key is stored encrypted in palcon's
                database and never shown again; each question sends the asking player's visible pal roster to the
                provider's API.
              </p>
              <div className="mt-4">
                <AdvisorKeyForm submitLabel="Turn on the advisor" modelOptions={status.modelOptions ?? {}} saveKey={api.setAdvisorKey} onStatusChange={onStatusChange} />
              </div>
            </>
          ) : (
            <>
              <p className="text-sm text-muted-foreground">
                No shared key is set up on this server, but you can bring your own: add a model API key and the
                advisor works for your account alone, on your own billing. It's stored encrypted against your
                palcon user and never shown again.
              </p>
              <div className="mt-4">
                <AdvisorKeyForm submitLabel="Use my key" modelOptions={status.modelOptions ?? {}} saveKey={api.setMyAdvisorKey} onStatusChange={onStatusChange} />
              </div>
            </>
          )}
        </div>
      ) : (
        <>
          <div ref={scrollRef} className={cn("min-h-0 flex-1 space-y-3 overflow-y-auto px-4 py-3", !expanded && "max-h-[55vh]")}>
            {turns.length === 0 && (
              <div className="py-2">
                <p className="text-sm text-muted-foreground">
                  Ask about your pals, the game, or palcon itself — or start from what the save shows:
                </p>
                <div className="mt-2.5 flex flex-wrap gap-1.5">
                  {ctx.suggestions.map((q) => (
                    <button
                      key={q}
                      onClick={() => ask(q)}
                      className="rounded-full border border-brand-amber/40 bg-brand-amber/10 px-3 py-1 text-left text-xs font-semibold text-brand-amber transition hover:bg-brand-amber/20"
                    >
                      {q}
                    </button>
                  ))}
                </div>
                {palsQuery.isError && (
                  <p className="mt-3 text-[11px] text-ink/40">
                    Couldn't read this server's save — answering without roster data.
                  </p>
                )}
              </div>
            )}
            {turns.map((t, i) => {
              if (t.role === "user") {
                return (
                  <p
                    key={i}
                    className="ml-auto w-fit max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-br-md bg-brand-red px-3 py-1.5 text-sm text-paper"
                  >
                    {t.content}
                  </p>
                );
              }
              // Tool turns carry data for the model, not prose for the
              // player; the calls are narrated on the turn that asked.
              if (t.role === "tool") return null;
              return (
                <div key={i} className="border-l-2 border-brand-amber pl-2.5">
                  {t.content && (
                    <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground">{t.content}</p>
                  )}
                  {t.toolCalls && t.toolCalls.length > 0 && (
                    <p className="mt-0.5 text-xs italic text-ink/40">{t.toolCalls.map(describeToolCall).join(" · ")}</p>
                  )}
                </div>
              );
            })}
            {sending && (
              <div className="flex items-center gap-2 border-l-2 border-brand-amber/50 pl-2.5 text-sm text-ink/40">
                <img src={ADVISOR_AVATAR} alt="" className="h-4 w-4 animate-pulse motion-reduce:animate-none" />
                Thinking…
              </div>
            )}
            {error && <p className="text-sm text-destructive">{error}</p>}
            {error === CONTEXT_FULL && (
              <button
                onClick={newChat}
                className="rounded-xl bg-brand-red px-3.5 py-1.5 text-sm font-bold text-paper transition hover:bg-brand-red/90"
              >
                Start a new chat
              </button>
            )}
          </div>

          <form
            onSubmit={(e) => {
              e.preventDefault();
              ask(input);
            }}
            className="flex gap-2 border-t border-ink/10 bg-white/60 p-2.5"
          >
            <input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              maxLength={4000}
              placeholder="Ask about pals, the game, or palcon…"
              aria-label="Ask the advisor"
              className="min-w-0 flex-1 rounded-xl border border-ink/10 bg-white px-3 py-1.5 text-sm outline-none transition focus:border-brand-red/50"
            />
            <button
              type="submit"
              disabled={sending || !input.trim()}
              className={cn(
                "rounded-xl bg-brand-red px-3.5 py-1.5 text-sm font-bold text-paper transition",
                (sending || !input.trim()) && "opacity-40",
              )}
            >
              Ask
            </button>
          </form>

          <div className="border-t border-ink/5 bg-white/40 px-4 py-2">
            <p className="text-[10px] leading-relaxed text-ink/35">
              Roster summaries and questions go to {apiLabel(status.provider)}
              {status.hasPersonalKey ? " on your own key" : ""}. Players hidden by an admin stay out. Wiki lookups
              query palworld.fandom.com directly.
            </p>
            <details className="mt-1">
              <summary className="cursor-pointer text-[10px] font-semibold text-ink/45">
                {status.hasPersonalKey ? "Your key" : "Use your own key"}
              </summary>
              <p className="mt-1 text-[10px] text-ink/40">
                {status.hasPersonalKey
                  ? `Your ${PROVIDERS.find((p) => p.id === status.provider)?.label ?? status.provider} key answers your questions on ${modelLabel(status)} — it applies only to your account.`
                  : "Add your own API key and your questions ride it instead of the server's — stored encrypted, only for your account, with your choice of model."}
              </p>
              <div className="mt-2">
                <AdvisorKeyForm
                  submitLabel={status.hasPersonalKey ? "Replace my key" : "Use my key"}
                  current={
                    status.hasPersonalKey ? { provider: status.provider, model: status.model } : undefined
                  }
                  modelOptions={status.modelOptions ?? {}}
                  saveKey={api.setMyAdvisorKey}
                  saveModel={api.setMyAdvisorModel}
                  onStatusChange={onStatusChange}
                />
              </div>
              {status.hasPersonalKey && (
                <button
                  disabled={removing}
                  onClick={async () => {
                    setRemoving(true);
                    try {
                      onStatusChange(await api.deleteMyAdvisorKey());
                    } catch {
                      // The next status fetch tells the truth either way.
                    } finally {
                      setRemoving(false);
                    }
                  }}
                  className="mt-2 text-[10px] font-semibold text-destructive/80 transition hover:text-destructive"
                >
                  Remove my key
                </button>
              )}
            </details>
            {status.canConfigure && (
              <details className="mt-1">
                <summary className="cursor-pointer text-[10px] font-semibold text-ink/45">Server settings</summary>
                <form
                  onSubmit={async (e) => {
                    e.preventDefault();
                    const n = Number(rounds);
                    if (!Number.isInteger(n) || n < 1 || n > 20) return;
                    try {
                      onStatusChange(await api.setAdvisorSettings(n));
                    } catch {
                      // The next status fetch tells the truth either way.
                    }
                  }}
                  className="mt-2 flex items-end gap-2"
                >
                  <label className="text-[10px] font-semibold text-ink/45">
                    Tool rounds per question
                    <input
                      type="number"
                      min={1}
                      max={20}
                      value={rounds}
                      onChange={(e) => setRounds(e.target.value)}
                      aria-label="Tool rounds per question"
                      className="mt-1 block w-20 rounded-lg border border-ink/10 bg-white px-2 py-1 text-xs outline-none transition focus:border-brand-red/50"
                    />
                  </label>
                  <button
                    type="submit"
                    className="rounded-lg bg-ink/10 px-2.5 py-1 text-[10px] font-bold text-ink/70 transition hover:bg-ink/15"
                  >
                    Save
                  </button>
                </form>
                {status.source !== "personal" && (
                  <p className="mt-1 text-[10px] text-ink/40">
                    {PROVIDERS.find((p) => p.id === status.provider)?.label ?? status.provider} · {modelLabel(status)}
                    {status.source === "ui" ? " · key saved in palcon" : " · key from the environment"}
                  </p>
                )}
                <div className="mt-2">
                  <AdvisorKeyForm
                    submitLabel="Save key"
                    current={
                      status.source === "ui" ? { provider: status.provider, model: status.model } : undefined
                    }
                    modelOptions={status.modelOptions ?? {}}
                    saveKey={api.setAdvisorKey}
                    saveModel={api.setAdvisorModel}
                    onStatusChange={onStatusChange}
                  />
                </div>
                {status.source === "ui" && (
                  <button
                    onClick={async () => {
                      try {
                        onStatusChange(await api.deleteAdvisorKey());
                      } catch {
                        // The next status fetch tells the truth either way.
                      }
                    }}
                    className="mt-2 text-[10px] font-semibold text-destructive/80 transition hover:text-destructive"
                  >
                    Remove saved key
                  </button>
                )}
              </details>
            )}
          </div>
        </>
      )}
    </section>
  );
}
