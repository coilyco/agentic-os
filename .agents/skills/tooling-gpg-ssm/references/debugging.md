# Debugging gpg-ssm

- **`user.signingkey is unset`** - run `git config --global user.signingkey <KEYID>`. The keyid comes from `gpg --list-secret-keys --keyid-format LONG`.
- **`AWS credentials expired or missing`** - the message says it: `aws sso login` (or `ward ops aws sso login`), retry.
- **`failed to fetch /coilysiren/gpg-secret-key` or `/coilysiren/gpg-passphrase`** - the shared param doesn't exist or IAM denies. Check `ward ops aws ssm get-parameter --name /coilysiren/gpg-secret-key --with-decryption` or `... /coilysiren/gpg-passphrase --with-decryption` directly. If 404, the shared SSM row is missing.
- **Sign succeeds but GitHub shows "Unverified"** - public key not uploaded, or wrong email on the GPG uid vs `user.email`. Check `gpg --list-keys` against the GitHub signing-keys page.
- **Hangs forever** - gpg-agent prompting for the passphrase via pinentry, meaning `--pinentry-mode loopback` got dropped or gpg-agent is in a weird state. Restart: `gpgconf --kill gpg-agent`.

## Never bypass

The temptation is real: "I'm in a hurry, let me just `git commit --no-gpg-sign`" or `git config --global commit.gpgsign false` for a session. Don't. Signed commits are a <personal-os-repo> pre-commit hook expectation across repos. If `gpg-ssm` is genuinely broken, fix it (or temporarily comment out the `gpg.program` line and remember to put it back). Bypassing leaves unsigned commits in history that look identical to spoofed ones.
