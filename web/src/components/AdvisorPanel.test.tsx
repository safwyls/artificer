import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api, type AdvisorStatus, type Guild, type Pal, type PalsResult, type PlayerPals } from "../lib/api";
import { renderWithProviders } from "../test/utils";
import { AdvisorPanel } from "./AdvisorPanel";

function makePal(characterId: string, over: Partial<Pal> = {}): Pal {
  return {
    instanceId: `${characterId}-${over.nickname ?? ""}`,
    characterId,
    nickname: "",
    level: 30,
    gender: "male",
    isBoss: false,
    isLucky: false,
    rank: 1,
    talentHp: 50,
    talentShot: 50,
    talentDefense: 50,
    passives: [],
    exp: 0,
    skills: [],
    hp: 1,
    sanity: 100,
    stomach: 100,
    friendship: 0,
    sick: "",
    souls: {},
    slotIndex: 0,
    baseId: "",
    ...over,
  };
}

const GUILDS: Guild[] = [
  {
    id: "g1",
    name: "Palhalla",
    baseCampLevel: 10,
    members: [{ uid: "u1", name: "Aster" }],
    memberCount: 1,
    bases: [{ id: "base-1", x: 0, y: 0 }],
  },
];

// A crew with a sick pal (so the sick-crew suggestion chip has a reason to
// exist) plus spare duplicates in the palbox.
const PLAYERS = [
  {
    uid: "u1",
    nickname: "Aster",
    level: 30,
    party: [],
    palbox: [makePal("Penguin", { nickname: "SpareA" }), makePal("Penguin", { nickname: "SpareB" })],
    base: [
      makePal("Anubis", { baseId: "base-1", nickname: "Digger" }),
      makePal("Anubis", { baseId: "base-1", sick: "ulcer", nickname: "Poorly" }),
    ],
    storage: [],
  },
] as unknown as PlayerPals[];

const PALS_RESULT = { players: PLAYERS, guilds: GUILDS, parsedAt: "", saveModTime: "" } as PalsResult;

const MODEL_OPTIONS = {
  anthropic: [
    { id: "claude-opus-5", label: "Claude Opus 5 · most capable" },
    { id: "claude-haiku-4-5", label: "Claude Haiku 4.5 · fastest" },
  ],
  gemini: [
    { id: "gemini-3.5-flash", label: "Gemini 3.5 Flash · most capable" },
    { id: "gemini-3.6-flash", label: "Gemini 3.6 Flash · efficient" },
  ],
};

function open(status: Partial<AdvisorStatus> = {}, onStatusChange = () => {}) {
  const full: AdvisorStatus = {
    enabled: true,
    provider: "anthropic",
    model: "claude-opus-5",
    source: "env",
    canConfigure: false,
    hasPersonalKey: false,
    maxToolRounds: 8,
    modelOptions: MODEL_OPTIONS,
    ...status,
  };
  return renderWithProviders(
    <AdvisorPanel serverId={1} status={full} onStatusChange={onStatusChange} onClose={() => {}} />,
  );
}

describe("AdvisorPanel", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(api, "serverPals").mockResolvedValue(PALS_RESULT);
  });

  it("counts what it sees and suggests save-derived questions", async () => {
    open();
    await waitFor(() => expect(screen.getByText(/4 pals in view/)).toBeInTheDocument());
    // The sick crew member turns into an invitation, not a canned example.
    expect(screen.getByRole("button", { name: /Why have pals stopped working/ })).toBeInTheDocument();
  });

  it("sends the conversation and renders the reply", async () => {
    const chat = vi.spyOn(api, "advisorChat").mockResolvedValue({ reply: "Condense SpareA into SpareB." });
    const user = userEvent.setup();
    open();
    await waitFor(() => expect(screen.getByText(/4 pals in view/)).toBeInTheDocument());

    await user.type(screen.getByRole("textbox", { name: /Ask the advisor/ }), "What should I condense?");
    await user.click(screen.getByRole("button", { name: "Ask" }));

    await waitFor(() => expect(screen.getByText("Condense SpareA into SpareB.")).toBeInTheDocument());
    const [serverId, context, messages] = chat.mock.calls[0];
    expect(serverId).toBe(1);
    expect(messages).toEqual([{ role: "user", content: "What should I condense?" }]);
    // The context is the summary the boards show: the sick pal is in it.
    expect(context).toContain("Poorly");
    expect(context).toContain("ulcer");
  });

  it("runs the model's tool calls through the real calculators and resubmits", async () => {
    const chat = vi
      .spyOn(api, "advisorChat")
      .mockResolvedValueOnce({
        content: "",
        toolCalls: [{ id: "call-1", name: "breed_child", args: { parentA: "Relaxaurus", parentB: "Sparkit" } }],
      })
      .mockResolvedValueOnce({ reply: "Breed Relaxaurus with Sparkit." });
    const user = userEvent.setup();
    open();

    await user.type(screen.getByRole("textbox", { name: /Ask the advisor/ }), "How do I get Relaxaurus Lux?");
    await user.click(screen.getByRole("button", { name: "Ask" }));

    await waitFor(() => expect(screen.getByText("Breed Relaxaurus with Sparkit.")).toBeInTheDocument());
    // The transcript narrates the lookup in player terms.
    expect(screen.getByText(/checked Relaxaurus × Sparkit/)).toBeInTheDocument();

    expect(chat).toHaveBeenCalledTimes(2);
    const secondMessages = chat.mock.calls[1][2];
    const toolTurn = secondMessages.find((m) => m.role === "tool");
    expect(toolTurn?.toolResults?.[0]).toMatchObject({ id: "call-1", name: "breed_child" });
    expect(toolTurn?.toolResults?.[0].content).toContain("Relaxaurus Lux");
    // Tool definitions ride with every request — wiki and docs included.
    const toolNames = chat.mock.calls[0][3]?.map((t) => t.name);
    expect(toolNames).toContain("breed_child");
    expect(toolNames).toContain("palworld_wiki");
    expect(toolNames).toContain("search_palcon_docs");
  });

  it("nudges the model to answer on the last round and salvages its preamble", async () => {
    const chat = vi
      .spyOn(api, "advisorChat")
      .mockResolvedValueOnce({
        content: "",
        toolCalls: [{ id: "1", name: "breed_child", args: { parentA: "Relaxaurus", parentB: "Sparkit" } }],
      })
      // The model ignores the nudge and asks for more tools — but wrote a
      // preamble the loop can salvage as the answer.
      .mockResolvedValueOnce({
        content: "So far: Relaxaurus × Sparkit gives Relaxaurus Lux.",
        toolCalls: [{ id: "2", name: "parents_for", args: { child: "Relaxaurus Lux" } }],
      });
    const user = userEvent.setup();
    open({ maxToolRounds: 1 });

    await user.type(screen.getByRole("textbox", { name: /Ask the advisor/ }), "Deep breeding question");
    await user.click(screen.getByRole("button", { name: "Ask" }));

    await waitFor(() =>
      expect(screen.getByText(/So far: Relaxaurus × Sparkit gives Relaxaurus Lux\./)).toBeInTheDocument(),
    );
    // Exactly maxRounds tool rounds + the salvage stop — no third request.
    expect(chat).toHaveBeenCalledTimes(2);
    // The last tool turn carried the answer-now note.
    const toolTurn = chat.mock.calls[1][2].find((m) => m.role === "tool");
    expect(toolTurn?.content).toMatch(/Answer now/);
  });

  it("names the provider questions are sent to", async () => {
    open({ provider: "gemini" });
    await waitFor(() => expect(screen.getByText(/go to the Google Gemini API/)).toBeInTheDocument());
  });

  it("offers shared-key setup to admins when no key is configured", async () => {
    const setKey = vi.spyOn(api, "setAdvisorKey").mockResolvedValue({
      enabled: true,
      provider: "gemini",
      model: "gemini-3.6-flash",
      source: "ui",
      canConfigure: true,
      hasPersonalKey: false,
      maxToolRounds: 8,
      modelOptions: MODEL_OPTIONS,
    });
    const onStatusChange = vi.fn();
    const user = userEvent.setup();
    open({ enabled: false, provider: "", source: "", canConfigure: true }, onStatusChange);

    await user.selectOptions(screen.getByRole("combobox", { name: /Model provider/ }), "gemini");
    // Switching provider resets the model list; a non-default pick sticks.
    await user.selectOptions(screen.getByRole("combobox", { name: "Model" }), "gemini-3.6-flash");
    await user.type(screen.getByLabelText("API key"), "AIza-test");
    await user.click(screen.getByRole("button", { name: /Turn on the advisor/ }));

    await waitFor(() => expect(onStatusChange).toHaveBeenCalledWith(expect.objectContaining({ enabled: true })));
    expect(setKey).toHaveBeenCalledWith("gemini", "AIza-test", "gemini-3.6-flash");
  });

  it("lets non-admins bring their own key when there's no shared one", async () => {
    const setMyKey = vi.spyOn(api, "setMyAdvisorKey").mockResolvedValue({
      enabled: true,
      provider: "anthropic",
      model: "claude-opus-5",
      source: "personal",
      canConfigure: false,
      hasPersonalKey: true,
      maxToolRounds: 8,
      modelOptions: MODEL_OPTIONS,
    });
    const onStatusChange = vi.fn();
    const user = userEvent.setup();
    open({ enabled: false, provider: "", source: "", canConfigure: false }, onStatusChange);

    expect(screen.getByText(/bring your own/)).toBeInTheDocument();
    await user.type(screen.getByLabelText("API key"), "sk-mine");
    await user.click(screen.getByRole("button", { name: /Use my key/ }));

    await waitFor(() =>
      expect(onStatusChange).toHaveBeenCalledWith(expect.objectContaining({ source: "personal" })),
    );
    expect(setMyKey).toHaveBeenCalledWith("anthropic", "sk-mine", "claude-opus-5");
  });

  it("changes a saved personal key's model without re-entering the key", async () => {
    const setModel = vi.spyOn(api, "setMyAdvisorModel").mockResolvedValue({
      enabled: true,
      provider: "gemini",
      model: "gemini-3.6-flash",
      source: "personal",
      canConfigure: false,
      hasPersonalKey: true,
      maxToolRounds: 8,
      modelOptions: MODEL_OPTIONS,
    });
    const onStatusChange = vi.fn();
    const user = userEvent.setup();
    open({ provider: "gemini", model: "gemini-3.5-flash", source: "personal", hasPersonalKey: true }, onStatusChange);

    await user.click(screen.getByText("Your key"));
    // One form serves both jobs: with the key field blank, picking a
    // different model turns the submit into a model-only change.
    await user.selectOptions(screen.getByRole("combobox", { name: "Model" }), "gemini-3.6-flash");
    await user.click(screen.getByRole("button", { name: "Change model" }));

    await waitFor(() =>
      expect(onStatusChange).toHaveBeenCalledWith(expect.objectContaining({ model: "gemini-3.6-flash" })),
    );
    expect(setModel).toHaveBeenCalledWith("gemini-3.6-flash");
  });

  it("shows a personal key as in use and lets its owner remove it", async () => {
    const del = vi.spyOn(api, "deleteMyAdvisorKey").mockResolvedValue({
      enabled: true,
      provider: "anthropic",
      model: "claude-opus-5",
      source: "env",
      canConfigure: false,
      hasPersonalKey: false,
      maxToolRounds: 8,
      modelOptions: MODEL_OPTIONS,
    });
    const onStatusChange = vi.fn();
    const user = userEvent.setup();
    open({ provider: "gemini", source: "personal", hasPersonalKey: true }, onStatusChange);

    await waitFor(() => expect(screen.getByText(/on your own key/)).toBeInTheDocument());
    await user.click(screen.getByText("Your key"));
    await user.click(screen.getByRole("button", { name: "Remove my key" }));
    await waitFor(() =>
      expect(onStatusChange).toHaveBeenCalledWith(expect.objectContaining({ hasPersonalKey: false })),
    );
    expect(del).toHaveBeenCalled();
  });

  it("still chats when the save can't be read", async () => {
    vi.spyOn(api, "serverPals").mockRejectedValue(new Error("no save"));
    vi.spyOn(api, "advisorChat").mockResolvedValue({ reply: "Lamball drops wool and lamball mutton." });
    const user = userEvent.setup();
    open();
    await waitFor(() => expect(screen.getByText(/answering without roster data/)).toBeInTheDocument());

    await user.type(screen.getByRole("textbox", { name: /Ask the advisor/ }), "What does Lamball drop?");
    await user.click(screen.getByRole("button", { name: "Ask" }));
    await waitFor(() => expect(screen.getByText(/drops wool/)).toBeInTheDocument());
  });
});
