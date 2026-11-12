package dev.coilyco.treehouse.systembrowser

import java.net.URI

/** Which intents are the app's own sign-in return. A Mac's rule is in rules.rs. */
object SignInRules {
    const val SCHEME = "dev.coilyco.treehouse"
    private const val PATH = "/signin"

    /** `dev.coilyco.treehouse:/signin?...` with no host. Anything else is ignored. */
    fun isReturn(address: String?): Boolean {
        val uri = try {
            URI(address ?: return false)
        } catch (e: Exception) {
            return false
        }
        return uri.scheme == SCHEME && uri.rawAuthority == null && uri.path == PATH
    }
}
