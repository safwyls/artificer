#!/usr/bin/env python3
"""Extract per-player party/palbox data from a Palworld Level.sav.

Usage: extract_pals.py /path/to/Level.sav

Prints JSON to stdout:
  {
    "players": [
      {
        "uid": "...", "nickname": "...", "level": 12,
        "party":   [<pal>...],  # pals in the player's party container
        "palbox":  [<pal>...],  # pals in the player's palbox container
        "base":    [<pal>...],  # owned pals working at a base (neither container)
        "storage": [<pal>...],  # Dimensional Pal Storage (Players/<uid>_dps.sav)
                                # plus the player's share of GlobalPalStorage.sav
        "inventory": {          # the player's item containers, keyed by role
          "common": {"size": 42, "slots": [<slot>...]}, ...
        },
        "character": {...}      # level progress, condition, stat-point spend
      }
    ]
  }

Read-only by design: the save file is opened for reading and never written.
Parsing relies on palworld-save-tools (MIT, the community-standard GVAS
implementation) — install with: pip install palworld-save-tools

The save format is community-reverse-engineered and evolves with game
patches, so every field access below is defensive: a missing field degrades
to a default rather than failing the whole extraction.
"""

import contextlib
import json
import os
import struct
import sys

from palworld_save_tools.archive import UUID, FArchiveReader
from palworld_save_tools.gvas import GvasFile, GvasHeader
from palworld_save_tools.palsav import decompress_sav_to_gvas
from palworld_save_tools.paltypes import PALWORLD_TYPE_HINTS

from guilds import decode_guild

ZERO_GUID = "00000000-0000-0000-0000-000000000000"

CHARACTER_PATH = ".worldSaveData.CharacterSaveParameterMap.Value.RawData"


def decode_character(reader, type_name, size, path):
    """Decode one character blob, reading only as far as we actually need.

    palworld-save-tools' own decoder reads the trailing fields after the
    property tree (unknown bytes, group id, ...) and then asserts it landed
    exactly on EOF. Newer saves append more trailing data, so that assertion
    fires ("Warning: EOF not reached") and takes the whole extraction with
    it — even though the pal data itself parsed perfectly.

    We only ever read `object.SaveParameter`, and being read-only we never
    have to re-encode the tail, so we stop once the property tree is out and
    ignore whatever follows. Trailing-layout changes can't break us.
    """
    if type_name != "ArrayProperty":
        raise Exception(f"expected ArrayProperty, got {type_name}")
    value = reader.property(type_name, size, path, nested_caller_path=path)
    inner = reader.internal_copy(bytes(value["value"]["values"]), debug=False)
    value["value"] = {"object": inner.properties_until_end()}
    return value


def _unused_encode(*_args, **_kwargs):
    raise NotImplementedError("palcon never writes save files")


# Deliberately the *only* custom decoder we register. palworld-save-tools
# ships decoders for group/guild data, item containers, foliage, base camps,
# map objects and more; every one is both dead weight (we read none of them)
# and a way for an unrelated format change to break the Pal viewer. Left
# unregistered, those blobs stay unparsed byte arrays that nothing can choke
# on, and a big world parses substantially faster.
CUSTOM_PROPERTIES = {CHARACTER_PATH: (decode_character, _unused_encode)}


def decompress_sav(raw):
    """Unwrap a .sav container down to its raw GVAS bytes.

    Palworld has shipped two compression containers, distinguished by the
    magic bytes in the header (NOT by save_type, whose values overlap
    between them):

      PlZ - zlib, the original format; palworld-save-tools handles it.
      PlM - Oodle Kraken, used by newer builds (0.6+). No released version
            of palworld-save-tools reads this yet (upstream PR #215 is
            still open), so we unwrap it here via pyooz, an open-source
            Kraken decompressor. Decompress-only: the published pyooz
            wheel exposes no compressor, which suits the read-only rule.

    A "CNK" magic marks an Xbox-style chunked header, where the real header
    starts 12 bytes further in.
    """
    header_offset = 12 if raw[8:11] == b"CNK" else 0
    magic = raw[header_offset + 8 : header_offset + 11]

    if magic != b"PlM":
        gvas_data, _ = decompress_sav_to_gvas(raw)
        return gvas_data

    try:
        import ooz
    except ImportError:
        raise SystemExit(
            "this save uses the newer Oodle-compressed (PlM) format, which needs "
            "the 'pyooz' package: pip install pyooz"
        )

    uncompressed_len = int.from_bytes(raw[header_offset : header_offset + 4], "little")
    compressed_len = int.from_bytes(raw[header_offset + 4 : header_offset + 8], "little")
    body = raw[header_offset + 12 : header_offset + 12 + compressed_len]
    if len(body) != compressed_len:
        raise SystemExit(
            f"truncated save: header claims {compressed_len} compressed bytes, found {len(body)}"
        )
    return ooz.decompress(body, uncompressed_len)


def unwrap(val):
    """Strip gvas value wrappers down to the scalar inside.

    Property values nest to different depths by type — a ByteProperty holds
    {"value": {"type": ..., "value": 5}} where an IntProperty holds
    {"value": 5} — so unwrap until there's no wrapper left rather than
    assuming a fixed depth.
    """
    while isinstance(val, dict) and "value" in val:
        val = val["value"]
    return val


def v(node, *path, default=None):
    """Walk nested gvas property dicts, unwrapping {"value": ...} at each step."""
    cur = node
    for key in path:
        if not isinstance(cur, dict):
            return default
        cur = cur.get(key)
        if isinstance(cur, dict) and "value" in cur:
            cur = cur["value"]
        if cur is None:
            return default
    return cur


def num(node, *path, default=0):
    val = unwrap(v(node, *path, default=None))
    return val if isinstance(val, (int, float)) and not isinstance(val, bool) else default


def text(node, *path, default=""):
    val = unwrap(v(node, *path, default=None))
    return val if isinstance(val, str) else default


def container_id(node, *path):
    """A container reference is a struct holding a single guid at .ID."""
    raw = unwrap(v(node, *path, "ID", default=None))
    return str(raw) if raw is not None else None


# Soul/condenser and player stat names come back as Japanese labels regardless
# of the server's language, so they're mapped here rather than shown raw.
#
# The full set is vendored from palworld-save-pal's STATUS_NAME_MAP
# (psp-core/src/domain/player.rs) rather than collected from whatever a sample
# save happened to contain — an earlier list built that way was missing
# スタミナ消費軽減 simply because nobody in the sample had spent a point on it,
# and it reached the UI untranslated. Refresh alongside the other vendored
# catalogs (see docs/vendored-game-data.md).
#
# 防御力 is ours rather than theirs: it's a pal soul stat, and this table is
# shared with soul_ranks.
STATUS_NAMES = {
    # The six the game's own stat panel shows.
    "最大HP": "Max HP",
    "最大SP": "Max SP",
    "攻撃力": "Attack",
    "所持重量": "Carry Weight",
    "捕獲率": "Capture Rate",
    "作業速度": "Work Speed",
    "防御力": "Defense",
    # The relic-granted "adventure" stats, in the order upstream lists them.
    "空腹率低減": "Hunger Reduction",
    "泳ぎ速度": "Swim Speed",
    "食料腐敗低減": "Food Spoilage Reduction",
    "ジャンプ力": "Jump Power",
    "滑空速度": "Glide Speed",
    "崖登り速度": "Climb Speed",
    "状態異常耐性": "Ailment Resistance",
    "経験値ボーナス": "EXP Bonus",
    "虹パッシブ率": "Rainbow Passive Rate",
    "移動速度アップ": "Movement Speed",
    "パルスフィアホーミング": "Sphere Homing",
    "スタミナ消費軽減": "Stamina Cost Reduction",
}


def status_points(param, key):
    """Soul upgrades, as {stat: points}, skipping the zeroes that pad the list."""
    out = {}
    for entry in v(param, key, "values", default=None) or []:
        name = text(entry, "StatusName")
        points = num(entry, "StatusPoint")
        if name and points:
            out[STATUS_NAMES.get(name, name)] = points
    return out


def soul_ranks(param):
    """Pal Soul upgrades (+3% per rank), as {stat: rank}. Current saves store
    them as per-stat Rank_* ints; the extractor reads whichever spelling is
    present and falls back to the older Japanese-labelled list."""
    out = {}
    for field, name in (
        ("Rank_HP", "Max HP"),
        ("Rank_Attack", "Attack"),
        ("Rank_Defense", "Defense"),
        ("Rank_CraftSpeed", "Work Speed"),
    ):
        pts = num(param, field, default=0)
        if pts:
            out[name] = pts
    # Palworld's internal names use the British "Defence" in some builds.
    if "Defense" not in out:
        d = num(param, "Rank_Defence", default=0)
        if d:
            out["Defense"] = d
    return out or status_points(param, "GotExStatusPointList")


def parse_player_character(param):
    """The player's own character entry: level progress, condition and how they
    spent their stat points.

    Deliberately no derived totals. The Health/Attack/Defense/Work Speed
    numbers the game's character screen shows are computed at runtime from
    base values, level and equipment, and the save stores none of them — so
    what's reported here is what was actually written down.
    """
    return {
        "exp": num(param, "Exp"),
        # Hp and ShieldHP are FixedPoint64, scaled by 1000. Current values:
        # the maximum is a runtime figure the save never records.
        "hp": round(num(param, "Hp", "Value") / 1000),
        "shield": round(num(param, "ShieldHP", "Value") / 1000),
        # Genuinely a percentage, unlike a pal's species-dependent stomach.
        "stomach": round(num(param, "FullStomach", default=0.0), 1),
        "unusedStatusPoints": num(param, "UnusedStatusPoint"),
        # The two point pools the game tracks separately: the ones spent on
        # level-up, and the scarcer "Ex" ones.
        "statusPoints": status_points(param, "GotStatusPointList"),
        "exStatusPoints": status_points(param, "GotExStatusPointList"),
        # The food buff currently running, and its seconds remaining. The
        # field name really is misspelled in the save format.
        "foodBuff": text(param, "FoodWithStatusEffect"),
        "foodBuffSeconds": num(param, "Tiemr_FoodWithStatusEffect"),
    }


def parse_pal(param, instance_id, slot_index=None):
    char_id = text(param, "CharacterID")
    gender = text(param, "Gender")
    passives = v(param, "PassiveSkillList", "values", default=None) or []
    skills = v(param, "EquipWaza", "values", default=None) or []

    # EPalBaseCampWorkerSickType::None means healthy; anything else is an
    # ailment worth surfacing, since a sick pal stops working at a base.
    sick = text(param, "WorkerSick").split("::")[-1]
    if sick in ("None", ""):
        sick = ""

    # Hp is a FixedPoint64 holding the value scaled by 1000.
    hp_raw = num(param, "Hp", "Value", default=0)

    return {
        "instanceId": instance_id,
        "characterId": char_id,
        "nickname": text(param, "NickName"),
        "level": num(param, "Level", default=1) or 1,
        "exp": num(param, "Exp"),
        "gender": "female" if "Female" in gender else ("male" if "Male" in gender else ""),
        "isBoss": char_id.upper().startswith("BOSS_"),
        "isLucky": bool(unwrap(v(param, "IsRarePal", default=False))),
        "rank": num(param, "Rank", default=1) or 1,
        "talentHp": num(param, "Talent_HP"),
        "talentShot": num(param, "Talent_Shot"),
        "talentDefense": num(param, "Talent_Defense"),
        "passives": [str(p) for p in passives],
        "skills": [str(s).split("::")[-1] for s in skills],
        "hp": round(hp_raw / 1000) if hp_raw else 0,
        # The game omits properties sitting at their defaults, so a healthy
        # pal carries no SanityValue at all — absence means full (100), not
        # zero; a drained pal stores the real number. FullStomach works the
        # same way but its maximum varies by species, which the extractor
        # doesn't know — -1 tells the UI "full" without guessing the cap.
        "sanity": round(num(param, "SanityValue", default=100.0), 1),
        "stomach": round(num(param, "FullStomach", default=-1.0), 1),
        "friendship": num(param, "FriendshipPoint"),
        "sick": sick,
        "souls": soul_ranks(param),
        # Storage sidecars pass their own slot; see storage_slots.
        "slotIndex": num(param, "SlotId", "SlotIndex", default=-1) if slot_index is None else slot_index,
    }


def skip_property(reader, type_name, size):
    """Seek past a property we don't need.

    `size` counts only the payload, not the per-type header that precedes
    it (an IntProperty writes a guid flag but reports size 4), so the
    header has to be consumed before the skip. Layouts mirror
    FArchiveWriter.property_inner.
    """
    if type_name == "StructProperty":
        reader.fstring()  # struct type
        reader.guid()
        reader.optional_guid()
    elif type_name in ("ArrayProperty", "SetProperty"):
        reader.fstring()  # element type
        reader.optional_guid()
    elif type_name == "MapProperty":
        reader.fstring()  # key type
        reader.fstring()  # value type
        reader.optional_guid()
    elif type_name in ("EnumProperty", "ByteProperty"):
        reader.fstring()  # enum / byte subtype
        reader.optional_guid()
    elif type_name == "BoolProperty":
        # The value lives in the header and size is 0.
        reader.bool()
        reader.optional_guid()
    else:
        reader.optional_guid()
    reader.skip(size)


def read_sections(gvas_data, wanted, handlers=None):
    """Pull just the named worldSaveData sections, skipping everything else.

    A world save holds ~22 sections, and the ones we never look at —
    foliage instances, every placed structure, every container slot — are
    the enormous ones. Parsing them costs minutes and gigabytes on an
    established world (byte arrays deserialize into Python lists of ints),
    purely to be discarded. Properties are length-prefixed, so we walk the
    top level, seek past anything unwanted, and stop as soon as everything
    asked for has been read.

    `handlers` maps a section name to a reader that consumes the property
    itself, for sections where parsing the whole thing is exactly what we're
    trying to avoid (see read_item_containers).
    """
    handlers = handlers or {}
    found = {}
    with FArchiveReader(
        gvas_data, PALWORLD_TYPE_HINTS, CUSTOM_PROPERTIES, allow_nan=True
    ) as reader:
        GvasHeader.read(reader)
        while True:
            name = reader.fstring()
            if name == "None":
                break
            type_name = reader.fstring()
            size = reader.u64()
            if name != "worldSaveData" or type_name != "StructProperty":
                skip_property(reader, type_name, size)
                continue

            # Descend into worldSaveData rather than skipping it.
            reader.fstring()
            reader.guid()
            reader.optional_guid()
            while True:
                inner = reader.fstring()
                if inner == "None":
                    break
                inner_type = reader.fstring()
                inner_size = reader.u64()
                if inner in handlers:
                    found[inner] = handlers[inner](reader, inner_type, inner_size)
                elif inner in wanted:
                    prop = reader.property(inner_type, inner_size, f".worldSaveData.{inner}")
                    found[inner] = prop.get("value", [])
                else:
                    skip_property(reader, inner_type, inner_size)
                    continue
                if len(found) == len(wanted):
                    return found
            break
    return found


def read_character_entries(gvas_data):
    return read_sections(gvas_data, {"CharacterSaveParameterMap"}).get(
        "CharacterSaveParameterMap", []
    )


def read_gvas(path, custom_properties):
    """Parse one .sav file whole. Library progress/warning chatter goes to
    stderr: it prints to stdout by default, which would corrupt our JSON."""
    with open(path, "rb") as f:
        raw = f.read()
    with contextlib.redirect_stdout(sys.stderr):
        return GvasFile.read(
            decompress_sav(raw), PALWORLD_TYPE_HINTS, custom_properties, allow_nan=True
        )


def parse_base_camps(entries, reader_source):
    """Base camps as {guild id: [{id, x, y, containerId}]}.

    A camp's own name is an untranslated internal placeholder, so camps are
    labelled by the guild that owns them instead. Coordinates come out in
    the same world space the live map already plots players in.

    containerId is the camp's worker container (from its WorkerDirector),
    which is what ties the pals working at a base to that specific base.
    """
    by_guild = {}
    for entry in entries or []:
        try:
            raw = bytes(entry["value"]["RawData"]["value"]["values"])
            r = reader_source.internal_copy(raw, debug=False)
            camp_id = str(r.guid())
            r.fstring()       # placeholder name
            r.byte()          # state
            transform = r.ftransform()
            r.float()         # area range
            guild_id = str(r.guid())
        except Exception as exc:
            print(f"warning: skipping a base camp: {exc}", file=sys.stderr)
            continue
        # The WorkerDirector blob grew trailing fields in newer saves, so
        # read just past the container guid and ignore the rest. A camp
        # whose blob fails still shows — its workers just won't attribute.
        container = ""
        try:
            wd = bytes(entry["value"]["WorkerDirector"]["value"]["RawData"]["value"]["values"])
            rw = reader_source.internal_copy(wd, debug=False)
            rw.guid()         # director id
            rw.ftransform()   # spawn transform
            rw.byte()         # current order type
            rw.byte()         # current battle type
            container = str(rw.guid())
        except Exception as exc:
            print(f"warning: no worker container for a base camp: {exc}", file=sys.stderr)
        t = transform.get("translation", {})
        by_guild.setdefault(guild_id, []).append(
            {"id": camp_id, "x": t.get("x", 0.0), "y": t.get("y", 0.0), "containerId": container}
        )
    return by_guild


def parse_guilds(entries, base_camps, player_names):
    """Assemble guilds, naming members from the player saves.

    Membership comes from the guild's character handles (reliable across
    versions); names come from player_names, which is built from
    Players/<uid>.sav. Anyone missing there falls back to a name carried in
    the guild record itself, and finally to the bare uid.
    """
    out = []
    for entry in entries or []:
        group_type = text(entry.get("value", {}), "GroupType")
        if "Guild" not in group_type:
            continue  # organizations and parties aren't player guilds
        raw = v(entry.get("value", {}), "RawData", "values", default=None)
        if raw is None:
            continue
        guild = decode_guild(raw)
        if not guild:
            continue

        spare = [n for n in guild.pop("spareNames", []) if n != guild["name"]]
        members = []
        for uid in guild.pop("memberUids", []):
            name = player_names.get(uid, "")
            if not name and spare:
                name = spare.pop(0)
            members.append({"uid": uid, "name": name or uid[:8]})

        guild["members"] = members
        guild["memberCount"] = len(members)
        guild["bases"] = base_camps.get(guild["id"], [])
        out.append(guild)
    out.sort(key=lambda g: (-len(g["members"]), g["name"].lower()))
    return out


# Unreal FDateTime counts 100ns ticks from 0001-01-01; Unix time starts here.
FDATETIME_EPOCH_OFFSET = 62_135_596_800


def ticks_to_unix(ticks):
    if not ticks:
        return 0
    seconds = ticks / 10_000_000 - FDATETIME_EPOCH_OFFSET
    # Reject anything not in living memory: the field is absent or holds
    # something else entirely on some saves, and a bogus date is worse
    # than none.
    return round(seconds) if 1_500_000_000 < seconds < 4_000_000_000 else 0


def player_containers_from_dir(players_dir):
    """Map each player's pal containers from Players/<uid>.sav.

    Newer saves moved OtomoCharacterContainerId (party) and
    PalStorageContainerId (palbox) out of the character entry and into
    per-player files, and dropped OwnerPlayerUId from pals entirely — so
    a pal's owner is now established by which container holds it.

    Returns ({container_guid: (player_uid, bucket)}, {uid: player metadata},
    {container_guid: (player_uid, inventory role)}).
    """
    index, meta, inventory = {}, {}, {}
    if not os.path.isdir(players_dir):
        return index, meta, inventory
    for name in sorted(os.listdir(players_dir)):
        # _dps.sav sidecars are pal storage, handled separately in main().
        if not name.lower().endswith(".sav") or name.lower().endswith("_dps.sav"):
            continue
        try:
            save_data = read_gvas(os.path.join(players_dir, name), {}).properties["SaveData"]["value"]
        except Exception as exc:  # one unreadable player shouldn't sink the rest
            print(f"warning: skipping {name}: {exc}", file=sys.stderr)
            continue
        uid = str(unwrap(save_data.get("PlayerUId")) or "")
        if not uid:
            continue
        translation = v(save_data, "LastTransform", "Translation", default=None) or {}
        meta[uid] = {
            "lastOnline": ticks_to_unix(num(save_data, "LastOnlineDateTime")),
            "lastX": unwrap(translation.get("x")) if "x" in translation else None,
            "lastY": unwrap(translation.get("y")) if "y" in translation else None,
            "platform": text(save_data, "PlayerPlatform").split("::")[-1],
            "technologyPoints": num(save_data, "TechnologyPoint"),
            **paldeck_records(save_data),
        }
        for key, bucket in (
            ("OtomoCharacterContainerId", "party"),
            ("PalStorageContainerId", "palbox"),
        ):
            cid = container_id(save_data, key)
            if cid:
                index[cid] = (uid, bucket)
        for key, role in INVENTORY_CONTAINERS.items():
            cid = container_id(save_data, "InventoryInfo", key)
            if cid:
                inventory[cid] = (uid, role)
    return index, meta, inventory


def paldeck_records(save_data):
    """Per-player Paldex progress, from the player save's RecordData.

    PaldeckUnlockFlag is the dex itself — species register on first
    acquisition however it happened (capture, hatch, trade), and stay
    registered after the pal is gone, which is what "completion" means.
    PalCaptureCount only counts sphere captures, but it's the number the
    game shows per species, so it rides along for the records views.
    """
    record = v(save_data, "RecordData", default=None) or {}
    deck = []
    for pair in v(record, "PaldeckUnlockFlag", default=None) or []:
        if isinstance(pair, dict) and pair.get("value") and pair.get("key"):
            deck.append(str(pair["key"]))
    captures = {}
    for pair in v(record, "PalCaptureCount", default=None) or []:
        if not isinstance(pair, dict):
            continue
        key, count = pair.get("key"), pair.get("value")
        if key and isinstance(count, int) and count > 0:
            captures[str(key)] = count
    return {"paldeck": deck, "captures": captures}


def dashed_guid(hex32):
    """Players/ filenames carry the uid without dashes; records use dashed."""
    h = hex32.lower()
    if len(h) != 32 or any(c not in "0123456789abcdef" for c in h):
        return None
    return f"{h[0:8]}-{h[8:12]}-{h[12:16]}-{h[16:20]}-{h[20:32]}"


def storage_slots(path):
    """Yield (save_parameter, instance_id, slot_index) for each occupied slot
    of a pal storage file — a player's Dimensional Pal Storage sidecar
    (Players/<uid>_dps.sav) or the world's GlobalPalStorage.sav. Both hold a
    root SaveParameterArray whose slots carry the same SaveParameter shape as
    Level.sav's character entries, just as plain properties rather than
    behind the RawData decoder. Empty slots hold CharacterID "None".

    The slot index is the pal's *position in the array*, not its own
    SlotId.SlotIndex: in storage the latter is stale, still holding whatever
    palbox slot the pal came from (0-959, and duplicated across pals), while
    the array position is where the game actually draws it — occupied runs
    break on multiples of 30, the storage page size."""
    arr = read_gvas(path, {}).properties.get("SaveParameterArray")
    for index, slot in enumerate(v(arr, "value", "values", default=None) or []):
        param = v(slot, "SaveParameter")
        if not isinstance(param, dict):
            continue
        if text(param, "CharacterID") in ("", "None"):
            continue
        iid = str(unwrap(v(slot, "InstanceId", "InstanceId", default="")) or "")
        yield param, iid, index


# ---------------------------------------------------------------------------
# Player inventories.
#
# A player's items live in six containers in Level.sav's ItemContainerSaveData,
# referenced by guid from InventoryInfo in Players/<uid>.sav. The keys below are
# those InventoryInfo fields; the values are the names the API serves.
# ---------------------------------------------------------------------------

INVENTORY_CONTAINERS = {
    "CommonContainerId": "common",              # the backpack
    "EssentialContainerId": "essential",        # key items
    "WeaponLoadOutContainerId": "weapons",      # the four weapon slots
    "PlayerEquipArmorContainerId": "equipment", # head, body, accessories, glider
    "FoodEquipContainerId": "food",             # food pouch
    "DropSlotContainerId": "drop",              # what a death would drop
}

ICSD_PATH = ".worldSaveData.ItemContainerSaveData"


def decode_item_slot(raw):
    """Decode one packed item slot.

    Current saves pack the slot into its RawData bytes rather than writing
    the properties out:

        i32   slot index          (its position in the container's grid)
        i32   stack count         (0 for an empty slot)
        str   item id             ("PalSphere_Mega")
        guid  created world id  }  zero unless the item is a "dynamic" one
        guid  local id          }  (gear, eggs) with per-instance state

    Anything after that is left alone — the trailing bytes have grown between
    game versions, and nothing we show comes out of them.
    """
    if len(raw) < 12:
        return None
    slot_index, count, name_len = struct.unpack_from("<iiI", raw, 0)
    off = 12
    # A slot's own length prefix is the only bound-check that matters here;
    # a nonsense one means the layout moved and the rest is not ours to read.
    if name_len < 1 or off + name_len > len(raw):
        return None
    item_id = raw[off : off + name_len - 1].decode("utf-8", "replace")
    off += name_len
    dynamic_id = ""
    if off + 32 <= len(raw):
        local = str(UUID(raw[off + 16 : off + 32]))
        if local != ZERO_GUID:
            dynamic_id = local
    if not item_id or item_id == "None" or count <= 0:
        return None
    return {"slot": slot_index, "itemId": item_id, "count": count, "dynamicId": dynamic_id}


def decode_dynamic_item(raw):
    """Decode one DynamicItemSaveData entry — the per-instance state of a
    piece of gear or an egg, joined to a slot by its local id.

    Layout after the two guids and the item id is version-dependent, so it's
    identified rather than assumed: current saves write a zero i32 first, and
    older ones start straight at the payload.

        gear: f32 durability, i32 rounds left, then the passive list
        egg:  str the species that will hatch

    Eggs also carry the unhatched pal's full parameters, which we skip.
    """
    if len(raw) < 36:
        return None
    name_len = struct.unpack_from("<I", raw, 32)[0]
    if name_len < 1 or 36 + name_len > len(raw):
        return None
    item_id = raw[36 : 36 + name_len - 1].decode("utf-8", "replace")
    local = str(UUID(raw[16:32]))
    rest = raw[36 + name_len :]
    # A leading zero i32 marks the current layout; without it the payload
    # starts immediately, as it did before the field was added.
    off = 4 if len(rest) >= 4 and struct.unpack_from("<I", rest, 0)[0] == 0 else 0
    out = {"dynamicId": local, "itemId": item_id}

    if item_id.startswith("PalEgg"):
        if off + 4 <= len(rest):
            species_len = struct.unpack_from("<I", rest, off)[0]
            if 1 <= species_len <= 64 and off + 4 + species_len <= len(rest):
                species = rest[off + 4 : off + 4 + species_len - 1].decode("utf-8", "replace")
                if species and species != "None":
                    out["eggSpecies"] = species
        return out

    if off + 4 <= len(rest):
        durability = struct.unpack_from("<f", rest, off)[0]
        # NaN and negatives mean we're reading the wrong bytes, not a broken
        # sword; the UI would rather show nothing than a nonsense number.
        if durability == durability and durability >= 0:
            out["durability"] = round(durability, 1)
    if off + 8 <= len(rest):
        rounds = struct.unpack_from("<i", rest, off + 4)[0]
        if rounds > 0:
            out["ammo"] = rounds
    if off + 12 <= len(rest):
        passives = read_item_passives(rest, off + 8)
        if passives:
            out["passives"] = passives
    return out


def read_item_passives(rest, off):
    """The passive list a dropped weapon rolled, as a length-prefixed array of
    strings. Empty on everything crafted, so a misread here costs nothing and
    a wrong guess would invent skills — hence the bail-outs."""
    count = struct.unpack_from("<I", rest, off)[0]
    if not 0 < count <= 8:
        return []
    out, cursor = [], off + 4
    for _ in range(count):
        if cursor + 4 > len(rest):
            return []
        size = struct.unpack_from("<I", rest, cursor)[0]
        cursor += 4
        if size < 1 or cursor + size > len(rest):
            return []
        out.append(rest[cursor : cursor + size - 1].decode("utf-8", "replace"))
        cursor += size
    return out


def read_item_containers(reader, type_name, size, wanted):
    """Read the item containers named in `wanted`, walking past the rest.

    ItemContainerSaveData holds every container in the world — each chest,
    each base, each pal's own bag — which on an established world is thousands
    of containers and hundreds of thousands of slots. Fully parsing it takes
    minutes; the couple of dozen containers belonging to players take a tenth
    of a second. The map's entries aren't individually length-prefixed, so
    every entry is still walked, but only wanted ones are parsed.
    """
    if type_name != "MapProperty":
        raise Exception(f"expected MapProperty, got {type_name}")
    reader.fstring()  # key type
    reader.fstring()  # value type
    reader.optional_guid()
    reader.u32()
    count = reader.u32()

    found = {}
    for _ in range(count):
        key = reader.properties_until_end(f"{ICSD_PATH}.Key")
        container_id = str(unwrap(v(key, "ID", default="")) or "")
        keep = container_id in wanted
        entry = {"size": 0, "slots": []} if keep else None
        while True:
            name = reader.fstring()
            if name == "None":
                break
            prop_type = reader.fstring()
            prop_size = reader.u64()
            if keep and name == "Slots":
                prop = reader.property(prop_type, prop_size, f"{ICSD_PATH}.Value.Slots")
                for slot in v(prop, "value", "values", default=None) or []:
                    raw = v(slot, "RawData", "values", default=None)
                    decoded = decode_item_slot(bytes(raw)) if raw else None
                    if decoded:
                        entry["slots"].append(decoded)
            elif keep and name == "SlotNum":
                # The container's real capacity, so the UI can draw the empty
                # slots too rather than implying a full bag.
                entry["size"] = num(reader.property(prop_type, prop_size, f"{ICSD_PATH}.Value.SlotNum"))
            else:
                skip_property(reader, prop_type, prop_size)
        if entry is not None:
            found[container_id] = entry
    return found


def build_inventories(containers, dynamic_entries, index):
    """Assemble {player uid: {role: container}} from the parsed containers,
    folding each dynamic item's state into the slot that references it."""
    dynamic = {}
    for entry in dynamic_entries or []:
        raw = v(entry, "RawData", "values", default=None)
        decoded = decode_dynamic_item(bytes(raw)) if raw else None
        if decoded:
            dynamic[decoded.pop("dynamicId")] = decoded

    out = {}
    for container_id, container in containers.items():
        uid, role = index[container_id]
        slots = []
        for slot in sorted(container["slots"], key=lambda s: s["slot"]):
            state = dynamic.get(slot.pop("dynamicId"))
            if state:
                # itemId is already on the slot and agrees; the rest is the
                # per-instance state the slot itself doesn't carry.
                slots.append({**slot, **{k: x for k, x in state.items() if k != "itemId"}})
            else:
                slots.append(slot)
        out.setdefault(uid, {})[role] = {"size": container["size"], "slots": slots}
    return out


def main():
    if len(sys.argv) != 2:
        print("usage: extract_pals.py <Level.sav>", file=sys.stderr)
        return 2

    level_path = sys.argv[1]
    if os.path.isdir(level_path):
        level_path = os.path.join(level_path, "Level.sav")

    # container guid -> (player uid, "party" | "palbox"), and the same for the
    # player's six item containers
    containers, player_meta, item_containers = player_containers_from_dir(
        os.path.join(os.path.dirname(level_path), "Players")
    )

    with open(level_path, "rb") as f:
        gvas_data = decompress_sav(f.read())
    guilds = []
    guild_entries, camps, inventories = None, {}, {}
    try:
        with contextlib.redirect_stdout(sys.stderr):
            sections = read_sections(
                gvas_data,
                {
                    "CharacterSaveParameterMap",
                    "BaseCampSaveData",
                    "GroupSaveDataMap",
                    "ItemContainerSaveData",
                    "DynamicItemSaveData",
                },
                handlers={
                    "ItemContainerSaveData": lambda r, t, s: read_item_containers(
                        r, t, s, set(item_containers)
                    )
                },
            )
        char_map = sections.get("CharacterSaveParameterMap", [])
        inventories = build_inventories(
            sections.get("ItemContainerSaveData") or {},
            v(sections.get("DynamicItemSaveData"), "values", default=None),
            item_containers,
        )
        with contextlib.redirect_stdout(sys.stderr), FArchiveReader(
            b"", PALWORLD_TYPE_HINTS, {}, allow_nan=True
        ) as helper:
            camps = parse_base_camps(sections.get("BaseCampSaveData"), helper)
            guild_entries = sections.get("GroupSaveDataMap")
    except Exception as exc:
        # The targeted walk depends on save layout; if a future format
        # shift breaks it, fall back to parsing everything rather than
        # reporting no pals at all.
        print(f"warning: fast path failed ({exc}); parsing whole save", file=sys.stderr)
        with contextlib.redirect_stdout(sys.stderr):
            gvas = GvasFile.read(gvas_data, PALWORLD_TYPE_HINTS, CUSTOM_PROPERTIES, allow_nan=True)
        world = gvas.properties.get("worldSaveData", {}).get("value", {})
        char_map = world.get("CharacterSaveParameterMap", {}).get("value", [])

    players = {}  # uid -> record
    pals = []     # (container_guid, old_owner_uids, pal)

    def record_for(uid):
        return players.setdefault(
            uid,
            {"uid": uid, "nickname": "", "level": 1, "party": [], "palbox": [],
             "base": [], "storage": [], "inventory": {}, "character": None},
        )

    for entry in char_map:
        key = entry.get("key", {})
        uid = str(unwrap(v(key, "PlayerUId", default=ZERO_GUID)) or ZERO_GUID)
        instance_id = str(unwrap(v(key, "InstanceId", default="")) or "")
        param = v(entry.get("value", {}), "RawData", "object", "SaveParameter", default=None)
        if not isinstance(param, dict):
            continue

        if unwrap(v(param, "IsPlayer", default=False)):
            rec = record_for(uid)
            rec["nickname"] = text(param, "NickName")
            rec["level"] = num(param, "Level", default=1) or 1
            rec["character"] = parse_player_character(param)
            # Older saves keep the player's containers on the character entry
            # itself; newer ones only in Players/<uid>.sav (already indexed).
            for prop, bucket in (
                ("OtomoCharacterContainerId", "party"),
                ("PalStorageContainerId", "palbox"),
            ):
                cid = container_id(param, prop)
                if cid:
                    containers.setdefault(cid, (uid, bucket))
        else:
            cid = container_id(param, "SlotId", "ContainerId") or container_id(param, "SlotID", "ContainerId")
            # OwnerPlayerUId was dropped in newer saves; OldOwnerPlayerUIds is
            # what remains to attribute a pal sitting in a base container.
            old_owners = [
                str(o) for o in (v(param, "OldOwnerPlayerUIds", "values", default=None) or [])
            ]
            owner = str(unwrap(v(param, "OwnerPlayerUId", default="")) or "")
            if owner:
                old_owners.insert(0, owner)
            pals.append((cid, old_owners, parse_pal(param, instance_id)))

    # Worker container → camp id, so base pals can say WHICH base they
    # work at, not just that they work at one.
    base_by_container = {}
    for camp_list in camps.values():
        for c in camp_list:
            if c.get("containerId"):
                base_by_container[c["containerId"]] = c["id"]

    for cid, old_owners, pal in pals:
        owner_bucket = containers.get(cid) if cid else None
        if owner_bucket is not None:
            uid, bucket = owner_bucket
            record_for(uid)[bucket].append(pal)
            continue
        # Not in anyone's party or palbox: it's working at a base (or was
        # otherwise released from a container). Attribute it to its most
        # recent owner if we know one; a pal with no owner at all is wild.
        for uid in old_owners:
            if uid and uid != ZERO_GUID:
                if cid and cid in base_by_container:
                    pal["baseId"] = base_by_container[cid]
                record_for(uid)["base"].append(pal)
                break

    # Pal storage buildings keep their pals outside Level.sav entirely.
    # Per-player Dimensional Pal Storage sits beside the player's own sav:
    players_dir = os.path.join(os.path.dirname(level_path), "Players")
    if os.path.isdir(players_dir):
        for fname in sorted(os.listdir(players_dir)):
            if not fname.lower().endswith("_dps.sav"):
                continue
            uid = dashed_guid(fname[: -len("_dps.sav")])
            if not uid:
                continue
            try:
                for param, iid, slot in storage_slots(os.path.join(players_dir, fname)):
                    record_for(uid)["storage"].append(parse_pal(param, iid, slot))
            except Exception as exc:  # a broken sidecar shouldn't sink the rest
                print(f"warning: skipping {fname}: {exc}", file=sys.stderr)

    # The guild-shared variant is one world-level file; attribute each pal to
    # its most recent owner, like base pals.
    gps_path = os.path.join(os.path.dirname(level_path), "GlobalPalStorage.sav")
    if os.path.isfile(gps_path):
        try:
            for param, iid, slot in storage_slots(gps_path):
                owner = str(unwrap(v(param, "OwnerPlayerUId", default="")) or "")
                olds = [str(o) for o in (v(param, "OldOwnerPlayerUIds", "values", default=None) or [])]
                if owner:
                    olds.insert(0, owner)
                for uid in olds:
                    if uid and uid != ZERO_GUID:
                        record_for(uid)["storage"].append(parse_pal(param, iid, slot))
                        break
        except Exception as exc:
            print(f"warning: skipping GlobalPalStorage.sav: {exc}", file=sys.stderr)

    # A player who has items but no character entry (never spawned in) still
    # gets a record, so their bags aren't silently dropped.
    for uid in inventories:
        record_for(uid)

    for uid, rec in players.items():
        rec.update(player_meta.get(uid, {}))
        rec["inventory"] = inventories.get(uid, {})
        rec.setdefault("lastOnline", 0)
        rec.setdefault("lastX", None)
        rec.setdefault("lastY", None)
        rec.setdefault("platform", "")
        rec.setdefault("paldeck", [])
        rec.setdefault("captures", {})

    player_names = {uid: rec["nickname"] for uid, rec in players.items() if rec["nickname"]}
    if guild_entries:
        with contextlib.redirect_stdout(sys.stderr):
            guilds = parse_guilds(guild_entries, camps, player_names)
    # containerId was only needed for the worker join above; the API
    # payload carries the camp id instead, which base pals reference.
    for g in guilds:
        for b in g.get("bases", []):
            b.pop("containerId", None)

    out = {
        "players": sorted(players.values(), key=lambda r: (r["nickname"].lower(), r["uid"])),
        "guilds": guilds,
    }
    json.dump(out, sys.stdout, separators=(",", ":"))
    return 0


if __name__ == "__main__":
    sys.exit(main())
