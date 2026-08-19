import { useState } from "react";
import type { SaveCharacter, SaveItem } from "../../lib/api";
import { agoLabel } from "../../lib/time";
import { levelForXp } from "../../lib/xp";
import { WkNote, WkPanel } from "./WkPanel";
import skillNames from "../../data/skillNames.json";
import itemNames from "../../data/itemNames.json";

/**
 * The characters the world save knows — everyone who has played, not just
 * who is online now. Read-only by nature: the save is the source, and it
 * moves when the game saves (~5 minutes), so this is the ledger next to
 * the Adventurers table's live roster.
 *
 * Skill and item ids resolve through vendored id → name maps
 * (src/data, see games/dragonwilds/docs/vendored-game-data.md); an id the
 * maps don't know renders as itself, so game updates degrade to ugly
 * rather than wrong.
 */

const SKILL_NAMES: Record<string, string> = skillNames;
const ITEM_NAMES: Record<string, string> = itemNames;

function skillName(id: string): string {
  return SKILL_NAMES[id] ?? id;
}

function itemName(id: string): string {
  return ITEM_NAMES[id] ?? id;
}

/** UE units are centimetres; metres read better at a glance. */
function positionLabel(p: NonNullable<SaveCharacter["position"]>): string {
  return `${Math.round(p.x / 100).toLocaleString()}, ${Math.round(p.y / 100).toLocaleString()} m`;
}

function playtimeLabel(hours: number): string {
  if (hours >= 10) return `${Math.round(hours)} h`;
  return `${hours.toFixed(1)} h`;
}

/** One fact in a character's header strip. */
function CharFact({ label, value, title }: { label: string; value: React.ReactNode; title?: string }) {
  return (
    <div>
      <div className="text-[11px] uppercase tracking-[0.14em] text-wk-mist">{label}</div>
      <div className="mt-0.5 text-sm text-wk-parchment" title={title}>
        {value}
      </div>
    </div>
  );
}

function ItemRows({ items, kind }: { items: SaveItem[]; kind: string }) {
  if (items.length === 0) return null;
  return (
    <>
      {items.map((it) => (
        <tr key={`${kind}-${it.slot}`}>
          <td className="border-t border-wk-edge px-2.5 py-1.5 text-xs text-wk-mist">{kind}</td>
          <td className="border-t border-wk-edge px-2.5 py-1.5">
            {ITEM_NAMES[it.id] ? (
              <span className="text-wk-parchment">{itemName(it.id)}</span>
            ) : (
              <span className="font-mono text-xs text-wk-mist" title="No name vendored for this item id yet">
                {it.id}
              </span>
            )}
          </td>
          <td className="border-t border-wk-edge px-2.5 py-1.5 text-right font-mono text-xs text-wk-parchment">
            {it.count > 1 ? `×${it.count}` : ""}
          </td>
          <td className="border-t border-wk-edge px-2.5 py-1.5 text-right font-mono text-xs text-wk-mist">
            {it.durability !== undefined ? Math.round(it.durability).toLocaleString() : ""}
          </td>
        </tr>
      ))}
    </>
  );
}

function CharacterCard({ c }: { c: SaveCharacter }) {
  const [open, setOpen] = useState(false);
  const itemCount = c.inventory.length + c.equipment.length;
  // A full record arrives by a player sharing it (or from an older-build
  // save); a transform-only entry is position and guid, honestly presented
  // rather than padded with zero vitals. Skills present = record present.
  const hasRecord = c.skills.length > 0 || c.sharedAt !== undefined || (c.charName !== "" && c.saveCount > 0);
  // Highest XP first: the skills someone actually plays lead.
  const skills = [...c.skills].sort((a, b) => b.xp - a.xp);
  return (
    <div className="rounded-md border border-wk-edge bg-wk-ink/40 px-3.5 py-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="font-wkdisplay text-lg font-semibold text-wk-parchment">
            {c.charName || "Unnamed adventurer"}
            {c.sharedAt && (
              <span
                className="ml-2 align-middle text-[11px] font-normal uppercase tracking-[0.12em] text-wk-rune"
                title={`Full sheet relayed by this player's companion app ${new Date(c.sharedAt).toLocaleString()}`}
              >
                shared {agoLabel(c.sharedAt)}
              </span>
            )}
          </div>
          <div
            className="mt-0.5 font-mono text-[11px] tracking-[0.06em] text-wk-mist"
            title="Character guid — the id the server logs on disconnect; the save never holds the EOS player id"
          >
            {c.charGuid}
          </div>
        </div>
        <div className="grid shrink-0 grid-cols-2 gap-x-7 gap-y-2.5 sm:grid-cols-4 sm:text-right">
          {c.position && (
            <CharFact
              label="Last stood at"
              value={<span className="font-mono text-xs">{positionLabel(c.position)}</span>}
              title={`UE units: X=${Math.round(c.position.x)} Y=${Math.round(c.position.y)} Z=${Math.round(c.position.z)}`}
            />
          )}
          {c.playtimeHours > 0 && <CharFact label="Time in world" value={playtimeLabel(c.playtimeHours)} />}
          {hasRecord && <CharFact label="Health" value={Math.round(c.health)} />}
          {hasRecord && (
            <CharFact label="Saves" value={c.saveCount} title="How many times this character's state has been written" />
          )}
        </div>
      </div>

      {skills.length > 0 && (
        <div className="mt-3 grid grid-cols-2 gap-x-6 gap-y-1.5 sm:grid-cols-5">
          {skills.map((s) => (
            <div key={s.id} className="flex items-baseline justify-between gap-2 border-t border-wk-edge pt-1.5">
              <span className="text-xs text-wk-mist">{skillName(s.id)}</span>
              <span
                className="font-mono text-xs text-wk-parchment"
                title={`${s.xp.toLocaleString()} XP — level derived on the RuneScape curve`}
              >
                {levelForXp(s.xp)}
              </span>
            </div>
          ))}
        </div>
      )}

      {itemCount > 0 && (
        <div className="mt-3">
          <button
            onClick={() => setOpen(!open)}
            className="rounded-sm border border-wk-edge px-2.5 py-0.5 text-xs text-wk-mist transition hover:border-wk-brass hover:text-wk-brasshi"
          >
            {open ? "Hide" : "Show"} carried items ({itemCount})
          </button>
          {open && (
            <table className="mt-2 w-full border-collapse text-sm">
              <thead>
                <tr>
                  <th className="px-2.5 pb-1.5 text-left text-[11px] font-medium uppercase tracking-[0.12em] text-wk-mist">
                    Where
                  </th>
                  <th className="px-2.5 pb-1.5 text-left text-[11px] font-medium uppercase tracking-[0.12em] text-wk-mist">
                    Item
                  </th>
                  <th className="px-2.5 pb-1.5 text-right text-[11px] font-medium uppercase tracking-[0.12em] text-wk-mist">
                    Count
                  </th>
                  <th className="px-2.5 pb-1.5 text-right text-[11px] font-medium uppercase tracking-[0.12em] text-wk-mist">
                    Durability
                  </th>
                </tr>
              </thead>
              <tbody>
                <ItemRows items={c.equipment} kind="Worn" />
                <ItemRows items={c.inventory} kind="Pack" />
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}

export function WkCharacters({
  players,
  available,
  loading,
  error,
}: {
  players: SaveCharacter[];
  /** False when there is no save to read (no path configured, or none yet). */
  available: boolean;
  loading: boolean;
  error?: string;
}) {
  return (
    <WkPanel title="Characters of this world" meta="from the world save · moves when the game saves (~5 min)">
      {loading && <p className="text-sm text-wk-mist">Reading the save…</p>}
      {!loading && error && (
        <p className="text-sm text-wk-mist">The save could not be read — {error}</p>
      )}
      {!loading && !error && !available && (
        <p className="text-sm text-wk-mist">
          No save to read — set a save path in Settings and the characters appear here.
        </p>
      )}
      {!loading && !error && available && players.length === 0 && (
        <p className="text-sm text-wk-mist">The save holds no characters yet — nobody has joined this world.</p>
      )}
      {!loading && !error && players.length > 0 && (
        <div className="space-y-3">
          {players.map((c) => (
            <CharacterCard key={c.charGuid || c.charName} c={c} />
          ))}
        </div>
      )}
      <WkNote>
        Everyone who has played this world is listed, online or not. The game keeps each character's skills
        and inventory on that player's own machine — the server only holds positions — so full sheets appear
        here when players share them through the companion app (see the panel below). Names are also learned
        from the server log when a player leaves. Skill levels derive from the RuneScape 1–99 curve — hover
        for exact XP.
      </WkNote>
    </WkPanel>
  );
}
