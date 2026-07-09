# Adding a new host

1. Set the local `user.signingkey` to the shared keyid: `git config --global user.signingkey <KEYID>`.
2. Confirm the shared SSM params exist: `/coilysiren/gpg-secret-key` and `/coilysiren/gpg-passphrase`.
3. Update `SSM.md` in `<personal-os-repo>/` with the shared rows if they are missing.
4. Upload the public key if the host has never seen it before.
5. Test: `echo test | gpg --clearsign` should round-trip through the wrapper and import the secret key on demand if needed.
