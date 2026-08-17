# Item icons

Sprite icons for every item, shown on the Inventory view.

**These are Palworld game assets — copyright Pocketpair, Inc.**, not part of
this project. They're vendored here so a self-hosted deployment works without
extra setup, and the Inventory view credits Pocketpair on screen. A fork that
redistributes Palcon should make that call deliberately rather than inherit it.
The same considerations as `../pal-icons/README.md` apply.

## Source

Vendored from [palworld-save-pal](https://github.com/oMaN-Rod/palworld-save-pal)
(MIT), path `ui/src/lib/assets/img/t_itemicon_*.webp`. The accompanying
catalog, `web/src/data/items.json`, is built from the same repo:

| Upstream file | Contributes |
| --- | --- |
| `data/json/items.json` | category (`type_a`), rarity, weight, max stack, icon name, and the `dynamic` block (max durability, magazine size, built-in passives) plus `damage` / `defense` |
| `data/json/l10n/en/items.json` | English name and description |

Both are static id → data lookups; no code was copied from either project.

## Naming and trimming

A file is named for the catalog's `icon` value with the `t_itemicon_` prefix
stripped — `t_itemicon_weapon_katana.webp` becomes `weapon_katana.webp`, which
is what `items.json` stores in its `i` field. Several items share one icon
(the two Attack Pendant tiers, for instance), so 2,372 items resolve to 884
files.

Upstream ships them at 256×256; they're downscaled to 64×64 here, matching the
pal icons — 6.8 MB becomes 1.7 MB, and nothing in the UI draws them larger than
48px. Seven catalog entries reference an icon upstream doesn't ship (test and
dummy items, plus a few unreleased ones); `web/src/lib/items.ts` falls back to
the empty frame, the same way an unknown pal renders.

## Refreshing

```sh
REPO=https://raw.githubusercontent.com/oMaN-Rod/palworld-save-pal/main
curl -sL -o /tmp/items.json      "$REPO/data/json/items.json"
curl -sL -o /tmp/items_en.json   "$REPO/data/json/l10n/en/items.json"
```

Then rebuild `web/src/data/items.json` by joining the two on item id, keeping
only the fields `web/src/lib/items.ts` declares (`n c r w i d s dur mag dmg def
ps`), and re-fetch any newly-referenced icons, resizing each to fit 64×64:

```python
img = Image.open(io.BytesIO(data)).convert("RGBA")
img.thumbnail((64, 64), Image.LANCZOS)
img.save(dest, "WEBP", quality=82, method=6)
```

See `docs/vendored-game-data.md` for the refresh cadence and the other
catalogs that drift with the same game patches.
