---
name: tooling-blender-mcp
description: Operating knowledge for driving Blender through the blender-mcp addon, and what changes when the Blender instance is on another host. Scene building works remotely, image return does not. Use when building or inspecting a Blender scene over MCP, when get_viewport_screenshot fails, or when choosing between a local and a remote Blender.
---

# Blender over MCP

The `blender_mcp` addon exposes a running GUI Blender to an MCP client:
scene inspection, object and material queries, and `execute_blender_code`
for arbitrary `bpy`. It is the normal way to drive Blender from a session.

## The remote instance cannot hand back an image

**`get_viewport_screenshot` is broken against any Blender that is not on the
same machine as the MCP server, and no amount of retrying fixes it.**

The server allocates the destination path on the **client** host, hands that
string to Blender, and afterwards looks for the file **on the client**.
Blender resolves the path wherever it is actually running. A macOS client
driving Blender on a Windows workstation asks for
`/var/folders/.../blender_screenshot_<pid>.png` and Windows Blender happily
writes `C:\var\folders\...\blender_screenshot_<pid>.png`. The server then
finds nothing and reports `Screenshot failed: Screenshot file was not
created`, which reads like a capture failure and is really a path that was
never on the same filesystem twice.

The capture itself is fine. Driving the addon's own offscreen path by hand
inside `execute_blender_code` produces a correct PNG every time, and orphaned
captures accumulate under `C:\var` on the remote box as proof.

**So the error message is misleading.** Do not chase the GPU, the window
compositing state, or the viewport. Check whether Blender and the MCP server
are on the same host, and if they are not, stop trying.

## What still works remotely

Everything that returns text. Scene construction is fully remote-capable, so
a workstation with the better GPU stays the right place to build and hold a
scene:

* `get_scene_info`, `get_object_info` - inspection
* `execute_blender_code` - mesh, material, lighting, camera, and render work
* rendering to a file **on the remote host**, which is real and correct and
  simply out of reach

## Getting pixels back

The transfer paths mostly do not exist, so budget for that before promising
someone a picture:

* **Taildrop refuses** when the remote node is tagged. `tailscale file cp`
  fails with `peer is owned by a different user`, because a tagged node and a
  user-owned node have no shared owner. Tagged workstations cannot Taildrop
  to a personal laptop.
* **A listener is not an option.** Standing up an HTTP server inside Blender
  to serve the render directory is the obvious move and is denied.
* **Base64 through `execute_blender_code`** works, since the result is text.
  It costs roughly 1.4x the file size in context, so keep the image small
  (a 560px JPEG lands near 15KB) and treat it as a last resort rather than a
  habit.

**The reliable pattern is to build remotely and render locally.** Keep the
scene script deterministic, run the same script against a local Blender, and
read the resulting files directly. Headless works: `blender -b -P scene.py`
renders without a GUI Blender or the addon at all.

## Launching a local Blender headless

The Homebrew wrapper on macOS blocks rather than running, so **call the app
binary directly**:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --factory-startup -noaudio -P scene.py
```

`/opt/homebrew/bin/blender` is a shell wrapper that hangs on `--version`, and
a session that reaches for it appears to have a broken Blender when it does
not.

## Modelling notes that bite

* **Bevel only the sharp rims.** Beveling every edge, including the loop cuts
  added to place material bands, facets the barrel into visible corduroy and
  rounds a flat cap into a dome. Select rim edges by face angle
  (`e.calc_face_angle(0.0) > radians(30)`) instead of passing `bm.edges`.
* **A primitive cylinder has one ring of side quads.** Assigning a material
  band by face height does nothing until the vertical edges are subdivided,
  and the symptom is a suspiciously small face count in the second slot.
* `use_auto_smooth` is gone. Blender 4.1+ wants
  `bpy.ops.object.shade_auto_smooth(angle=...)`.
