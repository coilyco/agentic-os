// What a brokered channel asks of accounts and the broker: a ticket for each dial and an
// unlock token for each welcome (references/treehouse-broker.md, "The client").
import { AccountsError, type AccountsClient } from "./accounts";
import type { BrokerClient } from "./broker";
import type { Brokered } from "./daemon-host";

export function brokeredFor(accounts: Pick<AccountsClient, "unlockBySession" | "unlockByPasskey">, broker: Pick<BrokerClient, "connect">, daemonId: string): Brokered {
  return {
    dial: async () => (await broker.connect(daemonId)).url,
    unlock: async (id, nonce) => {
      try {
        return (await accounts.unlockBySession(id, nonce)).token;
      } catch (error) {
        // A session no longer user-verified is refused, and a fresh passkey answers it.
        if (error instanceof AccountsError && error.code === "forbidden") return (await accounts.unlockByPasskey(id, nonce)).token;
        throw error;
      }
    },
  };
}
