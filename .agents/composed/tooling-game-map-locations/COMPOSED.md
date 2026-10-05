---
name: tooling-game-map-locations
description: Find where named places, camps, bosses or fast-travel points sit in a game world and draw them on a real map, using MapGenie's pin data instead of guessing from guide prose. Use when asked where something is in Enshrouded or another MapGenie-mapped game, or for a marked-up map, farm route or location screenshot.
---

# Game map locations

Guides and wikis name places but rarely give coordinates. A map drawn from
names alone is invented geography, and it reads as fact. MapGenie keeps every
pin for a game map in the page itself, so read positions from there and
project them. Never place a marker you did not project.

Enshrouded's world map is `https://mapgenie.io/enshrouded/maps/embervale`.
Other games follow `https://mapgenie.io/<game>/maps/<map>`.

## Read the pins

Open the map with the browser tool, then evaluate in the page. The pin data
is `window.mapData`, with `locations` (each has `title`, `description`,
`category_id`, `latitude`, `longitude`) and `categories` keyed by id.

```js
() => { const md = window.mapData; const cats = {};
  Object.values(md.categories).forEach(c => cats[c.id] = c.title);
  const want = /willow crush|glenn?wood|ancient spire/i;
  return md.locations
    .filter(l => want.test(l.title || '') || want.test(l.description || ''))
    .map(l => ({ title: l.title, cat: cats[l.category_id], lng: l.longitude, lat: l.latitude })); }
```

* Search descriptions too. A chest or lore page names its parent place in
  its description, which confirms an area when the place pin is missing.
* Match loosely. Guides misspell place names. The map has Glennwood's End
  where the guides wrote Glenwood's End.
* Filter by category for whole classes. In Enshrouded, `Ancient Spire` is
  fast travel. `Enemy Camp`, `Settlement` and `Boss` locate camps.

These globals are undocumented. Check they exist before relying on them. If
they are gone, say so and link the map rather than drawing anything.

## Frame and project

`window.map` is the map engine. Fit the view to the pins, wait for tiles,
then project each `[lng, lat]` to screen pixels in the same call.

```js
async () => { const m = window.map;
  m.fitBounds([[minLng, minLat], [maxLng, maxLat]], { padding: 140, duration: 0 });
  await new Promise(r => setTimeout(r, 3500));
  return pts.map(p => { const s = m.project(p); return [Math.round(s.x), Math.round(s.y)]; }); }
```

* Hide the pin clutter before the clean screenshot:
  `m.setLayoutProperty('locations', 'visibility', 'none')`, and the same for
  `location-titles` on a close-up.
* Turn `location-titles` back on for a zoomed-out view. The place names are
  what lets a player find their way to the area.
* Use `jumpTo({ center, zoom })` for a set zoom. Reproject after every camera
  change, since pixel positions are only valid for the view that made them.

## Prove positions before drawing

Take one screenshot with the pins still visible. Each projected point must sit
on its place's cluster of pins. That one look catches a wrong layer or a
stale projection.

Between two zoom levels the scale is `2^(z1 - z2)`. Predict where a base-map
label from one view lands in the other, and compare. A miss of more than a few
pixels means a view changed underneath you.

## Annotate

Draw with ImageMagick from one argument list. Numbered circles carry an order
only when the order is real, for example camps in the order to try them. Colour
encodes evidence, such as confirmed by players, guide pick, or mixed reports.

Crop offsets are the trap. Subtract a crop offset from coordinates exactly
once. A frame box derived from already-cropped coordinates lands one offset
off and silently leaves pins outside it. A frame for a close-up should contain
every pin the close-up shows, so check that by eye.

## Hover labels on a page

For names shown only on hover, overlay buttons on the image instead of drawing
text into it. Position each one in percent of the image: `(x - cropX) / width`
and `y / height`. Show the tooltip on `:hover` and `:focus`, so a tap works on
a phone. Flip the tooltip's anchor to the right edge for points past about
75% across, so names near the edge stay on screen.

## Provenance

Name MapGenie as the pin source in the result. The screenshots are MapGenie's
imagery, so keep pages that carry them private and keep them out of tracked
repositories. Commit the method, not the map.
