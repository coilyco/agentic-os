# Deploying the client

The client ships two ways, from the same source.

## Served by a daemon

`just install-client` builds with base `/` and syncs `dist/` into the directory the aterm daemon serves, over loopback and over tailnet HTTPS. The page dials the daemon that served it.

## coilyco.dev/aterm

A push to `main` touching the client runs `publish-coilyco-dev`. A `docker` runner builds with `just build-hosted` (base `/aterm/`, hosted mode) after `just gate`. A `deploy` runner holding the project-sites key then syncs `dist/` to `s3://coilyco-dev-site/aterm/`, the gated coilyco.dev bucket, never the public website bucket. The sync is scoped to the `aterm/` prefix, and the script refuses a build without `/aterm/` asset paths, so `--delete` can never empty the rest of the site.

The gate is Cognito with Google, limited to the coilyco.ai Workspace (infrastructure `terraform/aws-gated-sites`). Each daemon admits a websocket whose Origin is exactly `https://coilyco.dev`, and `tailscale whois` still decides who gets in.

## Hosts are added per device

Tailnet names are opaque identifiers, so the hosted build ships none. The first time on a device, add each host by its tailnet name, and the device remembers it in local storage. A gate failure would expose no hostnames. Kai chose this on teable:coilyco/website#8256.
