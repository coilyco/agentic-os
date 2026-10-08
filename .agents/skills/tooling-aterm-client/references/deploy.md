# Deploying the client

The client ships two ways, from the same source. The deployment, not this directory, owns where it is served and who may reach it.

## Served by a daemon

`just aterm-client-install-dir` builds with base `/` and syncs `dist/` into the directory the aterm daemon serves, over loopback and over tailnet HTTPS. The page dials the daemon that served it.

## Hosted

`just aterm-client-build-hosted` builds for a path-prefixed static host. The deployment sets `ATERM_CLIENT_BASE` to its base path and publishes `dist/` with its own sign-in. The daemon admits a websocket from a hosted page only when the page's Origin is allowed (`--allow-origins`, [aterm-daemon.md](../../../../docs/aterm-daemon.md)), and `tailscale whois` still decides who gets in.

## Hosts are added per device

Tailnet names are opaque identifiers, so the hosted build ships none. The first time on a device, add each host by its tailnet name, and the device remembers it in local storage. A gate failure would expose no hostnames. Kai chose this on teable:coilyco/website#8256.

## Installed as an app

Both builds are installable. The manifest uses relative paths, so it works at `/` and under a hosted prefix, and the service worker's scope is the build's own base. Chrome installs from https or from localhost, Safari on a Mac from File, Add to Dock.

`sw.js` is written by the build ([`pwa-plugin.ts`](../../../../aterm-client/pwa-plugin.ts)) with every built file listed and a version taken from their bytes, so a new build is a new cache and the old one is dropped when it takes over. It caches the shell only. A launch asks the network first and falls back to the cached shell after 3 seconds or on failure, so a stopped daemon shows the client's "not answering" panel. Nothing else is cached, and a websocket never passes through it.

It refuses to install when a shell file comes back through a redirect, because a hosted build behind a sign-in would otherwise cache the sign-in page as the client. `just aterm-client-install-dir` replaces the directory a daemon serves, and the next launch picks up the new worker.

A page served over plain http from any other address than localhost cannot be installed, and the install card says so. Android installs only from the hosted or tailnet https address. No device has verified that yet.
