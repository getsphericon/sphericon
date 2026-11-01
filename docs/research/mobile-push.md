# Mobile push: direct APNs/FCM or an external push service

Researched November 2026 from primary sources only: Apple's APNs documentation (read from the JSON that backs
developer.apple.com, since the pages render by script), the Firebase Cloud Messaging docs and REST reference, the
docs and API references of OneSignal, Expo, Pusher Beams and WonderPush, and the docs and API references of
Customer.io (including its published OpenAPI file `journeys-track.json`), Braze and Klaviyo. No third-party blog or
comparison site is cited. A claim that could not be read on a primary source is moved to "Unconfirmed" at the end.
sphericon's side is read from [GLOSSARY.md](../../GLOSSARY.md) (Contact, Identify, Destination, Integration,
Delivery notice) and the map issue #916. Nothing was run; no account was opened with any vendor.

## Question

sphericon will not build its own iOS/Android SDKs in this effort (#916, "Out of scope"). Should it:

- **Direct**: store device tokens and call APNs and FCM itself with the customer's credentials, the customer's app
  registering its tokens through a sphericon API; or
- **External service**: hand delivery to a push service the customer already uses (OneSignal, Expo, Pusher Beams,
  Firebase alone, WonderPush), addressing people by the customer's own user id, so sphericon never holds a token?

For each option: the credentials the customer provides, how a device is registered and tied to a person, whether a
send can be addressed by an external user id, the token lifecycle and invalid-token handling, the delivery and open
signals, and what the customer's app has to do without a sphericon SDK. Then how Customer.io, Braze, Klaviyo and Brevo
do it. The note reports facts and trade-offs; the decision is the maintainer's.

## Summary of findings

- **Direct sending needs no vendor between sphericon and the platforms, but makes sphericon the token store.** The
  customer gives an APNs `.p8` key with its Key ID, Team ID and bundle id, and a Firebase service account JSON. The
  app must still embed Apple's and Google's own client code to obtain a token, and must send each token to sphericon
  with enough information to identify the person. Apple says tokens change on restore, new device and OS reinstall,
  and must not be cached by the app.
- **FCM is replacing the registration token with the Firebase Installation ID (FID).** The HTTP v1 `token` target is
  marked deprecated in favour of `fid`; both are "fully co-supported" today and no removal date is published. A
  direct design would store either value as the device's address.
- **iOS can go through FCM.** The customer uploads the APNs key to its Firebase project, and the iOS app uses the
  Firebase Messaging SDK. sphericon would then speak only FCM HTTP v1 for both platforms, at the cost of requiring
  Firebase in every iOS app.
- **Invalid tokens are reported synchronously in both direct APIs.** APNs answers `410 Unregistered` (with a
  `timestamp` of when the token stopped being valid) or `410 ExpiredToken`; FCM answers `404 UNREGISTERED`, and `400
  INVALID_ARGUMENT` when the payload is otherwise valid. Android registrations idle for 270 days are expired by FCM;
  APNs has no such expiry, so Firebase advises pruning stale tokens by a last-seen timestamp.
- **Neither APNs nor FCM tells the sender that a push was delivered or opened.** An APNs 200 only means it was
  accepted. FCM's per-message delivery log goes to the customer's own BigQuery, and its aggregate Data API is Android
  only. A device-side "delivered" or "opened" exists only if the app reports it, which means the push payload must
  carry a sphericon message id. Customer.io's SDK-less contract (`CIO-Delivery-ID` in the payload, a metrics endpoint
  for `delivered`/`opened`/`converted`) is the existing template for that.
- **OneSignal, Pusher Beams and WonderPush can all be addressed by the customer's user id**, which matches the
  Contact's `subject_id`: OneSignal `include_aliases.external_id` (20,000 per request), Beams `publishes/users` (1,000
  per request), WonderPush `targetUserIds` (10,000 per call). The customer's app must call the vendor SDK's
  `login`/`setUserId` with the same id. Beams also needs the customer to host an auth endpoint that issues Beams
  tokens.
- **Expo and "Firebase alone" are not user-id services.** Expo is addressed by `ExponentPushToken[...]` and Firebase
  by FID/token, topic or condition, so sphericon would still hold an address per device; Expo also works only for
  apps built with `expo-notifications`.
- **In the external option, signals come from the vendor and depend on its SDK and plan.** OneSignal's Event Streams
  emit push `sent`, `received`, `clicked`, `failed`, `unsubscribed` with `message.id` and `external_id`; "received"
  needs the OneSignal SDK and a paid plan. Beams sends webhooks for a publish attempt, a device acknowledgement and an
  open. A send to many user ids returns one vendor id (OneSignal `id`, Beams `publishId`), not one per person.
- **Every platform ESP read ships its own SDK, and two of them also accept raw tokens without it.** Customer.io
  documents an SDK-less path end to end (add a device by API, report metrics by API). Braze (`/users/track` token
  import) and Klaviyo (`POST /api/push-tokens`) accept tokens by API but present it for migration and recommend
  their SDKs. Brevo's mobile push reportedly runs on the WonderPush SDK (see "Unconfirmed"). None of the pages read
  describes handing delivery to a push service the customer already runs.

## 1. Direct: APNs

### Credentials

- Token-based authentication uses a signing key from the Apple developer account: "A 10-character string with the
  Key ID" and "An authentication token signing key, specified as a text file (with a `.p8` file extension)"; the JWT
  carries `alg` `ES256`, `kid` (Key ID), `iss` ("the 10-character Team ID") and `iat`
  ([Establishing a token-based connection to APNs](https://developer.apple.com/documentation/usernotifications/establishing-a-token-based-connection-to-apns)).
- A team-scoped key signs for "any topic in a team"; it can be restricted to Sandbox or Production, "Each environment
  can have a maximum of two keys". Topic-specific keys also exist, up to 200 per environment
  ([same page](https://developer.apple.com/documentation/usernotifications/establishing-a-token-based-connection-to-apns)).
- The JWT must be refreshed "no more than once every 20 minutes and no less than once every 60 minutes"; an `iat`
  older than an hour gets `403 ExpiredProviderToken`
  ([same page](https://developer.apple.com/documentation/usernotifications/establishing-a-token-based-connection-to-apns)).
- **Multi-tenant cost**: "You can't map a connection to APNs to multiple teams. If you manage APNs for multiple
  developer accounts, open separate connection pools for each of them." APNs ties a team and its bundle ids to a
  connection on the first push
  ([same page](https://developer.apple.com/documentation/usernotifications/establishing-a-token-based-connection-to-apns)).
  A sphericon instance sending for several Workspaces holds one HTTP/2 connection pool per customer team.
- Requests go over HTTP/2 and TLS 1.2+ to `api.push.apple.com:443` (production) or `api.sandbox.push.apple.com:443`
  (development), `POST /3/device/<device_token>`, with `apns-topic` (the bundle id) and `apns-push-type`; the JSON
  payload is limited to 4 KB (4096 bytes)
  ([Sending notification requests to APNs](https://developer.apple.com/documentation/usernotifications/sending-notification-requests-to-apns)).

### Registration and the app's part

- "Your app communicates with APNs and receives its device token, which you then forward to your provider server."
  A token is per app and device: "You can't use the same device token for more than one app"
  ([Registering your app with APNs](https://developer.apple.com/documentation/usernotifications/registering-your-app-with-apns)).
- The app registers "each time your app launches" and must "Never cache device tokens in local storage. APNs issues a
  new token when the user restores a device from a backup, when the user installs your app on a new device, and when
  the user reinstalls the operating system"
  ([same page](https://developer.apple.com/documentation/usernotifications/registering-your-app-with-apns)).
- The app must "Securely forward the device token and any other information you need to identify the specific user
  to your server", and the server should "store device tokens with the user's account information. Because a user
  can have multiple devices, prepare your app to handle multiple device tokens"
  ([same page](https://developer.apple.com/documentation/usernotifications/registering-your-app-with-apns)).
- A token is valid only for its environment: `400 BadDeviceToken` says "Verify that the request contains a valid
  token and that the token matches the environment"
  ([Handling notification responses from APNs](https://developer.apple.com/documentation/usernotifications/handling-notification-responses-from-apns)).
  So the stored address is (bundle id, environment, token).

### Token lifecycle and errors

From [Handling notification responses from APNs](https://developer.apple.com/documentation/usernotifications/handling-notification-responses-from-apns):

- `410` "The device token is no longer active for the topic", with reasons `Unregistered` ("There is no need to send
  further pushes to the same device token, unless your application retrieves the same device token") and
  `ExpiredToken` ("The device token has expired").
- The error body's `timestamp` is "The time, represented in milliseconds since Epoch, at which APNs confirmed the
  token was no longer valid for the topic", present only on `410`.
- "Don't retry notification responses with the error code `BadDeviceToken`, `DeviceTokenNotForTopic`, `Forbidden`,
  `ExpiredToken`, `Unregistered`, or `PayloadTooLarge`." `TooManyRequests` (429, same device token) may be retried
  with a delay, and 5xx after 15 minutes.
- "An error code beginning with 4XX slows down your ability to send notifications ... APNs disconnects sooner with
  certain error conditions, such as `BadDeviceToken`. Note that status code `410` isn't considered an error
  condition." Dropping dead tokens promptly protects the customer's connection.

### Delivery and open signals

- A 200 is "Success" of the request; an `apns-id` identifies the notification
  ([Handling notification responses](https://developer.apple.com/documentation/usernotifications/handling-notification-responses-from-apns)).
  When the device is offline, APNs stores the notification "depending on the date you specify in the
  `apns-expiration` header" and "attempts to deliver the notification the next time the device activates"
  ([Sending notification requests](https://developer.apple.com/documentation/usernotifications/sending-notification-requests-to-apns)),
  so acceptance is not delivery.
- The only delivery visibility documented is `apns-unique-id`, "only available in the Development environment", to
  query the Delivery Log in Push Notifications Console
  ([Handling notification responses](https://developer.apple.com/documentation/usernotifications/handling-notification-responses-from-apns)).
- No open or delivered callback to the sender is documented. An open is known only if the app reports it.

## 2. Direct: FCM HTTP v1

### Credentials

- `POST https://fcm.googleapis.com/v1/projects/<project_id>/messages:send`, authorized by a short-lived OAuth 2.0
  access token minted from a service account, scope `https://www.googleapis.com/auth/firebase.messaging`
  ([Send with HTTP v1](https://firebase.google.com/docs/cloud-messaging/send/v1-api),
  [Authorize send requests](https://firebase.google.com/docs/cloud-messaging/auth-server)).
- Outside Google Cloud, "you'll need to download a service account JSON file from your Firebase project"
  ([Authorize send requests](https://firebase.google.com/docs/cloud-messaging/auth-server)). That JSON is what the
  customer hands over; it is per Firebase project.
- A sender in another project can send with the target project's id if its service account holds the FCM API Admin
  role there ([Send with HTTP v1](https://firebase.google.com/docs/cloud-messaging/send/v1-api)).

### Targets: token is giving way to FID

- The message's target is one of `token`, `fid`, `topic`, `condition`. `token` is marked "Deprecated: Use fid
  instead. During the transition period, this field also accepts a Firebase Installation ID (FID)"
  ([REST resource projects.messages](https://firebase.google.com/docs/reference/fcm/rest/v1/projects.messages)).
- FCM "is transitioning from registration tokens to a registration system based on Firebase Installation IDs", "both
  patterns are fully co-supported"; no removal date is given
  ([Best practices for FCM registration management](https://firebase.google.com/docs/cloud-messaging/manage-tokens)).
- There is no user-id target. Sending to many devices is one request per device; the Admin SDKs batch "up to 500"
  ([Send with the Admin SDK](https://firebase.google.com/docs/cloud-messaging/send/admin-sdk)).
- Payload limit: "4096 bytes for most messages, or 2048 bytes in the case of messages to topics"; TTL up to
  2,419,200 seconds (4 weeks) ([ErrorCode](https://firebase.google.com/docs/reference/fcm/rest/v1/ErrorCode)).

### Registration and the app's part

- Android: the app extends `FirebaseMessagingService`, receives its FID in `onRegistered(installationId)` and sends
  it to the server; `getToken()`/`onNewToken` are marked deprecated
  ([Set up an FCM client app on Android](https://firebase.google.com/docs/cloud-messaging/android/client)).
- iOS through FCM: the customer uploads an APNs authentication key with its key ID under Project settings → Cloud
  Messaging; the Firebase SDK maps the APNs token to an FID or FCM token, and with swizzling off the app sets
  `Messaging.apnsToken` itself ([Configure APNs with FCM](https://firebase.google.com/docs/cloud-messaging/ios/certs)).
  The raw APNs token is an input to the SDK, not an FCM address.

### Token lifecycle and errors

- `UNREGISTERED` (404): "App instance was unregistered from FCM", including uninstall and, on iOS, "if the APNs
  Feedback Service reported the APNs token as invalid"; "remove this registration token from the app server"
  ([ErrorCode](https://firebase.google.com/docs/reference/fcm/rest/v1/ErrorCode)).
- `INVALID_ARGUMENT` (400) covers an invalid registration and payload faults alike; treat it as an invalid
  registration "only when the payload is valid"
  ([manage-tokens](https://firebase.google.com/docs/cloud-messaging/manage-tokens)).
- `SENDER_ID_MISMATCH` (403): a token is tied to the senders it registered for, so a token from one Firebase project
  cannot be sent with another project's credentials. `QUOTA_EXCEEDED` (429) and `UNAVAILABLE` (503) are retried with
  exponential back-off ([ErrorCode](https://firebase.google.com/docs/reference/fcm/rest/v1/ErrorCode)).
- On Android, after 270 days of inactivity FCM expires a registration and rejects sends to it; for iOS FCM relies on
  APNs, which has no such expiry. FCM considers a registration stale after about a month and advises storing a
  timestamp on every upload and pruning by it
  ([manage-tokens](https://firebase.google.com/docs/cloud-messaging/manage-tokens)).

### Delivery and open signals

- Per-message logs (`MESSAGE_ACCEPTED`, `MESSAGE_DELIVERED`) go to a BigQuery export in the customer's Firebase
  project; the Data API gives aggregates per app and day, Android only, up to 5 days late; console "Received" and
  "Impressions" are Android only, opens only for notifications received in the background. No per-message callback
  to the sender is described
  ([Understanding message delivery](https://firebase.google.com/docs/cloud-messaging/understand-delivery)).

## 3. External services

### OneSignal

- **Credentials** (customer gives them to OneSignal, not sphericon): for iOS the `.p8` file, Key ID, Team ID and App
  Bundle ID ([iOS p8 setup](https://documentation.onesignal.com/docs/ios-p8-token-based-connection-to-apns)).
  sphericon would hold only the OneSignal App API key, sent as `Authorization: Key <key>`
  ([Create notification](https://documentation.onesignal.com/reference/push-notification)).
- **Device to user**: "The External ID is a unique string you assign to users to track them across devices and
  Subscriptions", set with `OneSignal.login(externalId)` early, at signup and on each login; up to 128 characters;
  "Each User can have up to 20 Subscriptions" ([Users](https://documentation.onesignal.com/docs/users)).
- **Send by external id**: `POST https://api.onesignal.com/notifications?c=push` with `include_aliases.external_id`
  (up to 20,000 users) and `target_channel: "push"`; an `idempotency_key` (UUID, valid 30 days) prevents duplicate
  sends ([Create notification](https://documentation.onesignal.com/reference/push-notification)).
- **Invalid tokens**: OneSignal handles them; the response lists `errors.invalid_aliases` (external ids with no
  subscribed device), and when nobody is reachable `id` is empty and `errors` says "All included players are not
  subscribed." ([Create notification](https://documentation.onesignal.com/reference/push-notification)). sphericon
  sees a person-level "not subscribed", never a token.
- **Tokens without the OneSignal SDK**: the Create user API accepts `iOSPush`/`AndroidPush` subscriptions with a raw
  `token`, for work "outside of frontendSDK-based sessions" such as imports
  ([Create user](https://documentation.onesignal.com/reference/create-user)), but confirmed receipt is "Not supported
  for subscriptions created via API only, with no OneSignal SDK on the device"
  ([Confirmed delivery](https://documentation.onesignal.com/docs/confirmed-delivery)).
- **Signals**: Event Streams post push `sent`, `received`, `clicked`, `failed`, `unsubscribed` to a webhook, with
  `message.id`, `event.external_id`, `event.subscription_id`, `event.kind` and a unique `event.id`; the plan sets the
  number of streams and events (Free: 1 stream, 1,000 events a month)
  ([Event Streams](https://documentation.onesignal.com/docs/event-streams)). "Received" is "Available only on paid
  plans" and "only works if the device has the OneSignal SDK installed"; on iOS it needs a Notification Service
  Extension ([Confirmed delivery](https://documentation.onesignal.com/docs/confirmed-delivery)).

### Expo push service

- `POST https://exp.host/--/api/v2/push/send`, up to 100 messages per request; `to` is an Expo push token
  (`ExponentPushToken[...]`); no authentication unless "enhanced push security" is turned on, then a bearer access
  token; 600 notifications per second per project
  ([Sending notifications](https://docs.expo.dev/push-notifications/sending-notifications/)).
- The customer uploads an FCM V1 service account key and an APNs key to EAS
  ([same page](https://docs.expo.dev/push-notifications/sending-notifications/)).
- The token comes from `getExpoPushTokenAsync` of `expo-notifications`; `getDevicePushTokenAsync` returns the native
  token "which you can then use with other services" ([FAQ](https://docs.expo.dev/push-notifications/faq/)). No send
  by user id is documented, so the server maps people to tokens itself.
- **Signals and lifecycle**: a push ticket `ok` means Expo received the message, not that the user got it; receipts
  are fetched from `/--/api/v2/push/getReceipts` (up to 1,000 ids), recommended about 15 minutes after sending, and
  "cleared after 24 hours"; `DeviceNotRegistered` means stop sending to that token
  ([Sending notifications](https://docs.expo.dev/push-notifications/sending-notifications/)).

### Pusher Beams

- **Credentials**: Beams uses "your APNs Key"
  ([Configure APNs](https://pusher.com/docs/beams/getting-started/ios/configure-apns/)); sphericon would hold the
  instance id and secret key (`Authorization: Bearer <secret>`)
  ([Publish API](https://pusher.com/docs/beams/reference/publish-api/)).
- **Device to user**: the app calls `setUserId` with a `TokenProvider` after login and on every launch; the
  customer's server must expose an auth endpoint that returns a Beams Token generated by a Beams server SDK; up to
  100 devices per user; `clearAllState` on logout
  ([Publish to a specific user](https://pusher.com/docs/beams/guides/publish-to-specific-user/)).
- **Send by user id**: `POST .../publish_api/v1/instances/<id>/publishes/users`, 1 to 1,000 user ids, body up to
  200 KiB; the response is a single `publishId`
  ([Publish API](https://pusher.com/docs/beams/reference/publish-api/)).
- **Signals**: webhooks `v1.PublishToUsersAttempt`, `v1.UserNotificationAcknowledgement` (device confirmed delivery)
  and `v1.UserNotificationOpen`, the last two needing iOS SDK 1.3.0+ or Android SDK 1.4.0+
  ([Webhooks](https://pusher.com/docs/beams/concepts/webhooks/)).
- The product page in November 2026 shows sign-up and a free sandbox plan, with no end-of-life notice
  ([pusher.com/beams](https://pusher.com/beams/)).

### WonderPush (Brevo's push engine)

- The SDK's `setUserId` assigns "your own user IDs to installations"; `unsetUserId` on logout is recommended
  ([User IDs](https://docs.wonderpush.com/docs/user-ids)).
- `POST https://management-api.wonderpush.com/v1/deliveries` with `targetUserIds` ("All their installations will be
  notified"), up to 10,000 values per call, authorized by an `accessToken` query parameter
  ([Send notifications](https://docs.wonderpush.com/reference/post-deliveries)).

### Firebase alone

- FCM has no user id addressing: its targets are `fid`/`token`, `topic` and `condition`
  ([projects.messages](https://firebase.google.com/docs/reference/fcm/rest/v1/projects.messages)). Using "the
  customer's Firebase" is therefore the direct option with FCM as the only transport (section 2), not a pass-through.

## 4. How marketing platforms do it

### Customer.io: own SDK, with a documented SDK-less path

- Credentials: for iOS a `.p8` key with its ids and the bundle id, for Android an FCM JSON key; iOS and Android may
  share one FCM credential ([Push getting started](https://docs.customer.io/journeys/push-getting-started/)).
- "A device token cannot be 'anonymous' in Customer.io": devices belong to an identified person
  ([same page](https://docs.customer.io/journeys/push-getting-started/)).
- Without the SDK the app adds a device with `PUT /api/v1/customers/{identifier}/devices`; `device.id` is "an APNs
  token, an FCM registration token, or a Firebase Installation ID (FID)", `platform` is `ios`/`android`, and
  attributes include `app_identifier` (which app's credentials to use), `push_enabled`, and a read-only `_last_status`
  (`sent`, `bounced`, `suppressed`); `last_used` defaults to the request time
  ([Track API OpenAPI](https://docs.customer.io/files/journeys-track.json)). Up to 25 devices per person; the 26th
  evicts the oldest `last_used`
  ([Push developer guide](https://docs.customer.io/messaging/channels/push/developer-guide/)).
- Each push carries `CIO-Delivery-ID` and `CIO-Delivery-Token` in its data; without the SDK the app reports
  `delivered`, `opened` or `converted` with them
  ([Push developer guide](https://docs.customer.io/messaging/channels/push/developer-guide/)) to `POST
  /api/v1/metrics` (push metrics `converted`, `delivered`, `opened`; `recipient` is "The device ID that the message
  was sent to") ([Track API OpenAPI](https://docs.customer.io/files/journeys-track.json)).

### Braze: own SDK, token import by API

- "the SDK communicates this to Braze" when the platform issues a token; the SDK registers and flushes tokens on
  session start ([Push registration](https://www.braze.com/docs/user_guide/message_building_by_channel/push/push_registration/)).
- `/users/track` accepts a `push_tokens` import (`app_id`, `token`, `device_id`); "Braze assumes imported push tokens
  are valid foreground push tokens for your app and environment", and with no stored APNs gateway a token from the
  other environment bounces with `BadDeviceToken`
  ([POST /users/track](https://www.braze.com/docs/api/endpoints/user_data/post_user_track/)).
- Signals: "Push Notification Send ... does not mean the push was delivered to the device"; a bounce is an error
  from APNs or FCM; an open is a direct tap; there is no push delivered event
  ([Currents message engagement events](https://www.braze.com/docs/user_guide/data_and_analytics/braze_currents/event_glossary/message_engagement_events/)).

### Klaviyo: own SDK, token API for migration

- Push requires installing a Klaviyo mobile SDK and releasing a new app version; Android setup uploads a Google
  service account key; the app tracks Opened Push automatically or manually
  ([Klaviyo help: set up push for Android](https://help.klaviyo.com/hc/en-us/articles/14750928993307)).
- `POST https://a.klaviyo.com/api/push-tokens` takes `token`, `platform` (`ios`/`android`), `vendor` (`apns`/`fcm`),
  `enablement_status` (`AUTHORIZED`, `DENIED`, `NOT_DETERMINED`, `PROVISIONAL`, `UNAUTHORIZED`), `background` and
  `device_metadata` (including `environment` `debug`/`release`) plus a required profile; the docs present it for
  migrating tokens and recommend the SDKs "to create push tokens from users' devices"
  ([Create or Update Push Token](https://developers.klaviyo.com/en/reference/create_push_token)).

### Brevo: WonderPush SDK

- See "Unconfirmed": Brevo's own help pages returned 403 to this research. WonderPush's addressing model is above.

## 5. Comparison

|                               | Direct APNs                         | Direct FCM (Android, iOS via FCM)                        | OneSignal                                     | Pusher Beams                                     | WonderPush                   | Expo                                    |
| ----------------------------- | ----------------------------------- | -------------------------------------------------------- | --------------------------------------------- | ------------------------------------------------ | ---------------------------- | --------------------------------------- |
| Customer gives sphericon      | `.p8`, Key ID, Team ID, bundle id   | service account JSON                                     | App API key                                   | instance id, secret key                          | access token                 | access token (if enabled)               |
| Platform credentials live     | sphericon                           | sphericon (+ APNs key in Firebase)                       | OneSignal                                     | Beams                                            | WonderPush                   | EAS                                     |
| sphericon holds device tokens | yes (bundle, env, token)            | yes (FID or token)                                       | no                                            | no                                               | no                           | yes (Expo token)                        |
| Send by customer user id      | no                                  | no                                                       | yes, 20,000 per request                       | yes, 1,000 per request                           | yes, 10,000 per call         | no                                      |
| App-side SDK                  | Apple APIs only                     | Firebase Messaging SDK                                   | OneSignal SDK + `login`                       | Beams SDK + `setUserId` + customer auth endpoint | WonderPush SDK + `setUserId` | `expo-notifications`                    |
| Dead tokens                   | `410` per request, sphericon prunes | `404 UNREGISTERED` per request, 270-day expiry (Android) | vendor prunes; `invalid_aliases` per person   | vendor                                           | vendor                       | `DeviceNotRegistered` in ticket/receipt |
| Delivered signal              | none (accepted only)                | BigQuery in customer's project; aggregates Android only  | `received` via Event Streams, SDK + paid plan | `UserNotificationAcknowledgement` webhook        | not read                     | push receipt, fetched within 24 h       |
| Open signal                   | only if the app reports it          | only if the app reports it                               | `clicked` via Event Streams                   | `UserNotificationOpen` webhook                   | not read                     | not documented                          |
| Per-person correlation        | sphericon's own message id          | sphericon's own message id                               | `message.id` + `external_id` on each event    | one `publishId` per request                      | not read                     | ticket id per message                   |

## 6. Trade-offs

**Direct (sphericon stores tokens, calls APNs/FCM):**

- One contract for every customer: the Destination is a device address, (app id, platform, APNs environment, token or
  FID), many per Contact, as Apple and Customer.io both model it. sphericon owns suppression of dead tokens, fed by
  synchronous `410`/`404` answers, which fits the Delivery notice and Suppression model without a webhook.
- The customer's app must call a sphericon registration API on every launch and login, and on logout detach the
  device; a stored timestamp per device is needed for stale pruning (FCM's advice). Klaviyo's `enablement_status`
  and Customer.io's `push_enabled` show that OS permission state is stored with the device.
- Delivered and opened exist only if the customer's app reports them, so the push payload must carry a sphericon
  message id and the app must call back (the Customer.io pattern). Without that, push has "sent" and "bounced" only.
- Operations: an HTTP/2 pool and JWT refresh per Apple team, an OAuth token per Firebase project, retries and back-off
  per vendor rules. FCM's token-to-FID transition is in progress.
- Device tokens become personal data held by sphericon, which the map's open item on erasure of the new Destinations
  must cover.
- Variant: requiring iOS through FCM reduces sphericon to one transport, at the cost of Firebase in every iOS app.

**External service (sphericon addresses by the customer's user id):**

- sphericon stores no token; the address is the Contact's `subject_id`, which the customer's app must also pass to
  the vendor SDK's `login`/`setUserId`. A Contact without a `subject_id` cannot be reached.
- Token lifecycle, permission prompts and multi-device fan-out are the vendor's. sphericon learns only "this person
  has no subscribed device" (OneSignal `invalid_aliases`), at the level of the person.
- Each vendor is a different Integration provider with its own send API, limits (1,000 to 20,000 ids per request),
  and its own webhook format for signals, which is an inbound callback preset per vendor. Signals depend on the
  vendor's SDK and, for OneSignal "received", on a paid plan.
- A bulk send returns one vendor id for many people; per-Contact attribution relies on the vendor's event carrying
  the user id (OneSignal does; Beams' acknowledgement and open events are per Authenticated User).
- Customers who already run such a vendor can connect it with no app release; customers who do not must adopt one.
- Expo and Firebase alone do not fit this shape: they still need per-device addresses.

**Not either-or**: the platform ESPs read (Customer.io, Braze, Klaviyo) accept raw APNs/FCM tokens by API alongside
their SDKs, and none of the pages read describes passing delivery to a third-party push service.

## Unconfirmed

- **Brevo mobile push.** A web search summary, not a page read here, says Brevo's mobile push "is currently powered
  by WonderPush, a push service acquired by Brevo", is set up under Settings → Campaigns → Push with Apple key ID,
  team ID and bundle ID, is limited to the Professional and Enterprise plans, and links Brevo contacts to the
  WonderPush SDK's user id. The source pages, help.brevo.com articles 26084951366802 and 4604359025682, returned 403.
- **WonderPush delivery and open webhooks, OneSignal's Android credential, Braze's credential setup and Beams' exact
  APNs key fields** were not read.
- **Whether OneSignal delivers to subscriptions created only through the Create user API** is not stated; only the
  absence of confirmed receipt for them is.
