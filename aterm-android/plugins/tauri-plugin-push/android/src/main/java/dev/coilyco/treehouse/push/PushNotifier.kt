package dev.coilyco.treehouse.push

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat

/** Shows a push, and carries what a tap needs back to the app in the launch intent. */
object PushNotifier {
    private const val ALERT = "treehouse-push-alert"
    private const val QUIET = "treehouse-push-quiet"
    // One notification per tag, so a second push for a seat replaces the first.
    private const val ID = 1
    private const val EXTRA = "dev.coilyco.treehouse.push."
    const val OPEN = EXTRA + "OPEN"
    const val TAP = EXTRA + "tap"
    const val KIND = EXTRA + "kind"
    const val DAEMON_ID = EXTRA + "daemon_id"
    const val SESSION = EXTRA + "session"
    const val TAG = EXTRA + "tag"

    fun show(context: Context, message: PushMessage) {
        val manager = NotificationManagerCompat.from(context)
        // No permission: the person said no to the page's one ask.
        if (!manager.areNotificationsEnabled()) return
        channels(context)
        val notification = NotificationCompat.Builder(context, if (message.quiet) QUIET else ALERT)
            .setSmallIcon(smallIcon(context))
            .setContentTitle(message.title)
            .setContentText(message.body)
            .setStyle(NotificationCompat.BigTextStyle().bigText(message.body))
            .setCategory(NotificationCompat.CATEGORY_MESSAGE)
            .setPriority(if (message.quiet) NotificationCompat.PRIORITY_LOW else NotificationCompat.PRIORITY_HIGH)
            .setSilent(message.quiet)
            .setAutoCancel(true)
            .setContentIntent(tapIntent(context, message))
            .build()
        try {
            manager.notify(message.tag, ID, notification)
        } catch (e: SecurityException) {
            // The permission went away between the check and the post.
        }
    }

    private fun channels(context: Context) {
        val system = context.getSystemService(NotificationManager::class.java) ?: return
        system.createNotificationChannel(
            NotificationChannel(ALERT, "Seats asking", NotificationManager.IMPORTANCE_HIGH),
        )
        system.createNotificationChannel(
            NotificationChannel(QUIET, "Seats done", NotificationManager.IMPORTANCE_LOW),
        )
    }

    // The flat canopy the build copies in (COI-3613), else the launcher icon.
    private fun smallIcon(context: Context): Int {
        val canopy = context.resources.getIdentifier("ic_stat_treehouse", "drawable", context.packageName)
        return if (canopy != 0) canopy else context.applicationInfo.icon
    }

    private fun tapIntent(context: Context, message: PushMessage): PendingIntent? {
        val launch = context.packageManager.getLaunchIntentForPackage(context.packageName) ?: return null
        // Its own action, or Android resumes the launcher's task and drops the extras.
        launch.action = OPEN
        launch.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        launch.putExtra(TAP, true)
        launch.putExtra(KIND, message.kind)
        launch.putExtra(DAEMON_ID, message.daemonId)
        launch.putExtra(SESSION, message.session)
        launch.putExtra(TAG, message.tag)
        return PendingIntent.getActivity(
            context,
            message.tag.hashCode(),
            launch,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
    }
}
