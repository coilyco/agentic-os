package dev.coilyco.treehouse.push

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage

// Runs with or without a page. Nothing here logs the token or the message.
class PushService : FirebaseMessagingService() {
    override fun onNewToken(token: String) {
        PushStore.save(this, token)
        PushPlugin.live?.tokenChanged(token)
    }

    // An app on screen shows the seat itself, as the web worker's focused case does.
    override fun onMessageReceived(message: RemoteMessage) {
        val shown = PushMessage.parse(message.data) ?: return
        if (PushPlugin.inFront) return
        PushNotifier.show(this, shown)
    }
}
