package events

// WebhooksConsumer exposes the consumer that serializes the body a queued webhook
// delivery carries, so a test can build that body the way dispatch does.
var WebhooksConsumer = webhooksConsumer

// AutomationsConsumer exposes the consumer that enrolls a Contact on an event's action.
var AutomationsConsumer = automationsConsumer

// RulesConsumer exposes the consumer that queues an event's evaluation against the Rules.
var RulesConsumer = rulesConsumer
