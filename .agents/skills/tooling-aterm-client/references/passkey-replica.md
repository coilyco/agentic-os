# One enrollment for every daemon of Kai's (COI-3542)

A passkey or device key enrolled on one daemon unlocks every daemon of hers, and adding a host asks her to enroll nothing. Jev picked this shape over Treehouse accounts and a peer pull on 2026-11-11, 0.98 (the record has the evidence).

**The primary** is the daemon she enrolls on in her own terminal. `aterm passkey enroll` prints the code as before, then waits for the enrollment and publishes the store to the SSM SecureString `/coilysiren/aterm/passkeys` from that terminal, which holds her AWS session. `--no-publish` returns at once, `aterm passkey publish` publishes by hand, and `ATERM_PASSKEYS_SSM=off` publishes nowhere. `aterm passkey revoke` revokes here and publishes an empty store.

**A replica** starts with `--passkeys-from <file>` (`ATERM_PASSKEYS_FROM`), the file an ExternalSecret mounts from that parameter. It re-reads the file when it changes, writes nothing, and answers mint, revoke, enroll and a device key add with `errFollowing`. A file that is gone publishes nothing, so no passkey is honored, and one that will not parse keeps what is held. A passkey's counter advances in memory only. A device key has no counter.

**What is published** is the store without attestation, which a login never reads, so a few devices fit a parameter. It holds public keys and the user handle, never a secret.

**Revocation** reaches a replica after the ExternalSecret refresh interval plus a reload, which is the lag to size for.

**The rule that a seat cannot add a device** (COI-3538) holds in the daemon, which refuses mint inside a session. A seat with `aosguard ops aws ssm put-parameter` could still write the parameter, so that path needs a deny for seats.

The test of the whole path is `TestAPasskeyEnrolledOnOneDaemonUnlocksAReplicaOverTheProtocol`.
