package boundgen

const (
	site     = "openapi/site.openapi.json"
	external = "openapi/external.openapi.json"
	service  = "openapi/service.openapi.json"
)

// Bounds are the contract's bounds on the input of a domain write, in the order they are written.
// A bound added here is checked by each of its owners: the bound-ownership test refuses one that an
// owner's code does not use.
var Bounds = []Bound{
	{
		Name: "Name", Doc: "is what a person calls a named row (a Tag, a Segment, a Template, ...), trimmed.",
		Spec: site, Schema: "Name", Owners: []string{"internal/kernel/validation"},
	},
	{
		Name: "UserName", Doc: "is what a User is called, trimmed; a Sign-in provider's name has the same length.",
		Spec: site, Schema: "UserName", Owners: []string{"internal/kernel/validation"},
	},
	{
		Name: "EmailAddress", Doc: "is a stored email address.",
		Spec: site, Schema: "EmailAddress", Owners: []string{"internal/kernel/validation"},
	},
	{
		Name: "HoldReasonCode", Doc: "is the machine code of a Hold's reason.",
		Spec: service, Schema: "HoldInput", Property: "reasonCode", Owners: []string{"Hold"},
	},
	{
		Name: "HoldMessage", Doc: "is the message a Hold shows the Workspace's Owner.",
		Spec: service, Schema: "HoldInput", Property: "message", Owners: []string{"Hold"},
	},
	{
		Name: "HoldActionURL", Doc: "is where an Owner or Admin can resolve a Hold.",
		Spec: service, Schema: "HoldInput", Property: "actionUrl", Owners: []string{"Hold"},
	},
	{
		Name: "MaxPerDay", Doc: "is a rolling 24-hour limit: a Provider limit's daily value or a Send cap.",
		Spec: site, Schema: "MaxPerDay", Owners: []string{"Workspace", "InstanceState", "Integration"},
	},
	{
		Name: "MaxPerSecond", Doc: "is a Provider limit's value per second.",
		Spec: site, Schema: "MaxPerSecond", Owners: []string{"Integration"},
	},
	{
		Name: "AttributionWindowDays", Doc: "is how many days a Workspace's Attribution window lasts.",
		Spec: site, Schema: "AttributionWindowDays", Owners: []string{"Workspace"},
	},
	{
		Name: "RetentionDays", Doc: "is how many days a Workspace's Audit log is kept.",
		Spec: site, Schema: "RetentionDays", Owners: []string{"Workspace"},
	},
	{
		Name: "HTTPCallHeaders", Doc: "are the headers of an http Action or step.",
		Spec: site, Schema: "SiteRuleAction", Property: "headers", Owners: []string{"internal/infra/httpcall"},
	},
	{
		Name: "ContactCity", Doc: "is a Contact's city.",
		Spec: external, Schema: "UpsertContactInput", Property: "city", Owners: []string{"Contact"},
	},
	{
		Name: "LocaleTag", Doc: "is a BCP 47 language tag, a Contact's locale.",
		Spec: site, Schema: "LocaleTag", Owners: []string{"Contact"},
	},
	{
		Name: "ImageURL", Doc: "is the https URL of an image, a Contact's avatar.",
		Spec: site, Schema: "ImageUrl", Owners: []string{"Contact"},
	},
	{
		Name: "BrandKitLogoURL", Doc: "is the URL of a Brand kit's logo.",
		Spec: site, Schema: "SiteSaveBrandKitInput", Property: "logoUrl", Owners: []string{"BrandKit"},
	},
	{
		Name: "BrandKitButtonRadius", Doc: "is the corner radius of a Brand kit's buttons, in pixels.",
		Spec: site, Schema: "SiteSaveBrandKitInput", Property: "buttonRadius", Owners: []string{"BrandKit"},
	},
	{
		Name: "BrandKitFooterText", Doc: "is the footer text of a Brand kit.",
		Spec: site, Schema: "SiteSaveBrandKitInput", Property: "footerText", Owners: []string{"BrandKit"},
	},
	{
		Name: "SignupSurveyOther", Doc: "is the text of a Sign-up survey answer's `other` choice.",
		Spec: site, Schema: "SiteSignupSurveyInput", Property: "roleOther", Owners: []string{"SignupSurveyResponse"},
	},
	{
		Name: "TelegramButtons", Doc: "are the buttons of a Telegram message.",
		Spec: site, Schema: "TelegramBroadcastContentInput", Property: "buttons",
		Owners: []string{"internal/kernel/telegramrender"},
	},
	{
		Name: "TelegramUnsubscribeButton", Doc: "is the label of a Telegram message's unsubscribe button.",
		Spec: site, Schema: "TelegramBroadcastContentInput", Property: "unsubscribeButton",
		Owners: []string{"internal/kernel/telegramrender"},
	},
}
