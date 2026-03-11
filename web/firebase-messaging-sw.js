importScripts('https://www.gstatic.com/firebasejs/10.13.2/firebase-app-compat.js');
importScripts('https://www.gstatic.com/firebasejs/10.13.2/firebase-messaging-compat.js');

self.addEventListener('push', function(event) {
  let payload = {};
  if (event.data) {
    try {
      payload = event.data.json();
    } catch(e) {}
  }

  const data = payload.data || {};
  if (data.event === 'incoming_call') {
    const title = data.caller_id || data.remote_number || 'Incoming Call';
    const body = `Incoming call via ${data.box_name || 'CallFXO'}`;
    event.waitUntil(
      self.registration.showNotification(title, {
        body: body,
        tag: data.invite_id,
        requireInteraction: true,
        data: data
      })
    );
  } else if (data.event === 'incoming_stop' || data.event === 'incoming_answered') {
    event.waitUntil(
      self.registration.getNotifications({ tag: data.invite_id }).then(notifications => {
        notifications.forEach(notification => notification.close());
      })
    );
  }
});

self.addEventListener('notificationclick', function(event) {
  event.notification.close();
  event.waitUntil(
    clients.matchAll({ type: 'window', includeUncontrolled: true }).then(function(clientList) {
      if (clientList.length > 0) {
        let client = clientList[0];
        for (let i = 0; i < clientList.length; i++) {
          if (clientList[i].focused) {
            client = clientList[i];
          }
        }
        return client.focus();
      }
      return clients.openWindow('/');
    })
  );
});
