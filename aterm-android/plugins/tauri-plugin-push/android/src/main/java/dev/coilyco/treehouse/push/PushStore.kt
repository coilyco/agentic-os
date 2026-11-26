package dev.coilyco.treehouse.push

import android.content.Context

/** The last FCM token, in app-private storage, so `token` answers after a restart. */
object PushStore {
    private const val FILE = "treehouse.push"
    private const val TOKEN = "token"

    fun save(context: Context, token: String) {
        context.getSharedPreferences(FILE, Context.MODE_PRIVATE).edit().putString(TOKEN, token).apply()
    }

    fun load(context: Context): String? =
        context.getSharedPreferences(FILE, Context.MODE_PRIVATE).getString(TOKEN, null)
}
