// A Condition's tree, typed by the place it is used (ADR 0181): /site and /api show and take it the
// same way. One group template, ConditionOf, takes the union of the entries a place admits; each
// place declares that union once per form, the request's closed and the response's open (ADR
// 0148). TypeSpec cannot spread one union into another, so a place's union lists the shared leaves
// itself. A leaf or test whose fields hold no union has a twin (`model TagLeafInput is TagLeaf`);
// one that holds a union is a template instantiated once per form.

namespace Sphericon;

/** How a condition group combines its entries */
enum ConditionCombinator {
  and,
  or,
}

/** How a text test compares the value with its own */
enum TextOperator {
  eq,
  neq,
  contains,
  beginsWith,
}

/** A test of a text value. On a Contact attribute or an Event's property, contains is a
 * case-insensitive substring. On a Custom field, contains holds when the field's value is a list
 * that holds `value`, and a number or boolean field's eq and neq compare the text of its value. */
model TextTest {
  kind: "text";

  /** How the value compares */
  operator: TextOperator;

  /** The text compared with */
  value: string;
}

/** How a number test compares the value with its own: greater than, greater than or equal, less
 * than, or less than or equal */
enum NumberOperator {
  gt,
  gte,
  lt,
  lte,
}

/** A test of a number. A missing value, or one that is not a number (text, a boolean, null, a
 * list), fails it. */
model NumberTest {
  kind: "number";

  /** How the value compares */
  operator: NumberOperator;

  /** The number compared with */
  value: float64;
}

/** Which side of `at` a moment is on: before is strictly earlier, after is at or later, so the two
 * split time at `at` */
enum MomentOperator {
  before,
  after,
}

/** A test of a moment: before or after `at`. Only a date-time with a UTC offset is a moment: a
 * missing value, a date without a time, a date-time without an offset or a value that is not a
 * date-time fails it. */
model MomentTest {
  kind: "moment";

  /** Which side of `at` the moment is on */
  operator: MomentOperator;

  /** The moment compared with */
  at: Timestamp;
}

/** A test of a moment: within the last `days` days up to now, so a moment still to come is not
 * within. A moment is read as for the moment test. */
model WithinLastTest {
  kind: "withinLast";

  /** The window, in days back from now */
  @minValue(1)
  days: int32;
}

/** Whether a value is set */
enum PresenceOperator {
  set,
  notSet,
}

/** A test of whether a value is set at all */
model PresenceTest {
  kind: "presence";

  /** Set or not set */
  operator: PresenceOperator;
}

model TextTestInput is TextTest;
model NumberTestInput is NumberTest;
model MomentTestInput is MomentTest;
model WithinLastTestInput is WithinLastTest;
model PresenceTestInput is PresenceTest;

/** What a test of a Contact attribute asks of it, by `kind` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union AttributeTest {
  text: TextTest,
  presence: PresenceTest,
}

/** What a test of a Contact attribute asks of it, by `kind` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union AttributeTestInput {
  text: TextTestInput,
  presence: PresenceTestInput,
}

/** What a test of the Contact's birthday asks of it, by `kind` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union BirthdayTest {
  moment: MomentTest,
  withinLast: WithinLastTest,
}

/** What a test of the Contact's birthday asks of it, by `kind` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union BirthdayTestInput {
  moment: MomentTestInput,
  withinLast: WithinLastTestInput,
}

/** What a Custom field test asks of the field's value, by `kind`. Which tests and operators a
 * field takes depends on its type, which the Custom field lists as `conditionTests`; a test the
 * type does not take is refused when the Condition is saved. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union CustomFieldTest {
  text: TextTest,
  number: NumberTest,
  moment: MomentTest,
  withinLast: WithinLastTest,
  presence: PresenceTest,
}

/** What a Custom field test asks of the field's value, by `kind`. Which tests and operators a
 * field takes depends on its type, which the Custom field lists as `conditionTests`; a test the
 * type does not take is refused when the Condition is saved. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union CustomFieldTestInput {
  text: TextTestInput,
  number: NumberTestInput,
  moment: MomentTestInput,
  withinLast: WithinLastTestInput,
  presence: PresenceTestInput,
}

/** What a property test asks of the property's value, by `kind`. A key the Event lacks fails
 * every test; a text test fails on a value that is not text; set holds for a text value and notSet
 * for an explicit null. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union PropertyTest {
  text: TextTest,
  number: NumberTest,
  presence: PresenceTest,
}

/** What a property test asks of the property's value, by `kind`. A key the Event lacks fails
 * every test; a text test fails on a value that is not text; set holds for a text value and notSet
 * for an explicit null. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union PropertyTestInput {
  text: TextTestInput,
  number: NumberTestInput,
  presence: PresenceTestInput,
}

/** How a test of a Contact's Engagement tracking consent compares it with a state: it reads as
 * the state, or it does not */
enum StateOperator {
  eq,
  neq,
}

/** A test of a Contact's Engagement tracking consent against a state. eq holds where the consent
 * reads as `value`; neq holds everywhere else, where none is recorded included, so a test and its
 * negation split every Contact. */
model EngagementTrackingStateTest {
  kind: "text";

  /** Whether the consent reads as the state, or does not */
  operator: StateOperator;

  /** The state compared with */
  value: EngagementTrackingState;
}

model EngagementTrackingStateTestInput is EngagementTrackingStateTest;

/** What a test of a Contact's Engagement tracking consent on one Channel asks of it, by `kind`: a
 * comparison with `granted` or `denied`, or a presence test of whether any is recorded. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union EngagementTrackingTest {
  text: EngagementTrackingStateTest,
  presence: PresenceTest,
}

/** What a test of a Contact's Engagement tracking consent on one Channel asks of it, by `kind`: a
 * comparison with `granted` or `denied`, or a presence test of whether any is recorded. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union EngagementTrackingTestInput {
  text: EngagementTrackingStateTestInput,
  presence: PresenceTestInput,
}

/** A text attribute of a Contact */
enum ContactAttribute {
  email,
  phone,
  subject_id,
  first_name,
  last_name,
  time_zone,
  country,
  locale,
  city,
}

/** A test of a Contact's text attribute */
model AttributeLeafOf<Test> {
  kind: "attribute";

  /** The attribute tested */
  attribute: ContactAttribute;

  /** What it asks of the attribute */
  test: Test;
}

/** A test of the Contact's date of birth */
model BirthdayLeafOf<Test> {
  kind: "birthday";

  /** What it asks of the birthday */
  test: Test;
}

/** Whether a Contact has something: a Tag, a Telegram chat, a phone that can be texted */
enum HasOperator {
  has,
  doesNotHave,
}

/** A test of whether a Contact has the Tag called `name` */
model TagLeaf {
  kind: "tag";

  /** The Tag's name */
  name: string;

  /** Has it or does not */
  operator: HasOperator;
}

/** A test of whether a Contact has a Telegram chat bound, on its own or through a Visitor */
model TelegramLeaf {
  kind: "telegram";

  /** Has one or does not */
  operator: HasOperator;
}

/** A test of whether a Contact has a phone that can be texted: one with an SMS Confirmation and no
 * SMS opt-out or Suppression */
model SmsLeaf {
  kind: "sms";

  /** Has one or does not */
  operator: HasOperator;
}

/** A test of the Contact's Engagement tracking consent on one Channel (ADR 0120), read live as
 * recorded, whatever the Workspace's default: on email the consent of its email address, on sms of
 * its phone, on telegram of its own chats, which read `denied` when any is denied, `granted` when it
 * has one and all are granted, and as none recorded otherwise. A Contact without a Destination on
 * the Channel has none recorded. */
model EngagementTrackingLeafOf<Test> {
  kind: "engagementTracking";

  /** The Channel whose consent is tested */
  channel: EngagementTrackingChannel;

  /** What it asks of the consent */
  test: Test;
}

/** A test of a Custom field of the Workspace, its key in a field of its own */
model CustomFieldLeafOf<Test> {
  kind: "customField";

  /** The Custom field's key */
  key: string;

  /** What it asks of the field's value */
  test: Test;
}

/** How an event count is bounded: at least `times`, or at most `times` */
enum EventCountOperator {
  atLeast,
  atMost,
}

/** A count of how many times a Contact performed an Event action, over all time or within the
 * last `withinDays` days. Performed is atLeast 1, not performed is atMost 0; "exactly" is a group of
 * both. Engagement Events made by software (machine opens and clicks) are not counted. A window
 * longer than Event retention is capped by it: expired Events are no longer counted. */
model EventLeaf {
  kind: "event";

  /** The Event action counted */
  action: string;

  /** How the count is bounded */
  operator: EventCountOperator;

  /** The bound: atLeast takes 1 or more, atMost 0 or more */
  @minValue(0)
  times: int32;

  /** Count only the Events of the last this many days; absent counts all time */
  @minValue(1)
  withinDays?: int32;
}

/** The Event that fired a Rule, whose properties a property test reads */
model TriggerSource {
  kind: "trigger";
}

model TriggerSourceInput is TriggerSource;

/** Whose properties a Rule's property test reads, by `kind`: the triggering Event */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union RulePropertySource {
  trigger: TriggerSource,
}

/** Whose properties a Rule's property test reads, by `kind`: the triggering Event */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union RulePropertySourceInput {
  trigger: TriggerSourceInput,
}

/** Whose properties a Branch path's property test reads, by `kind`: the Event the Enrollment
 * began with */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union BranchPropertySource {
  trigger: TriggerSource,
}

/** Whose properties a Branch path's property test reads, by `kind`: the Event the Enrollment
 * began with */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union BranchPropertySourceInput {
  trigger: TriggerSourceInput,
}

/** A test of one property of an Event: whose it is in `of`, its key in a field of its own, so a
 * key holding a dot is one key */
model PropertyLeafOf<Of, Test> {
  kind: "property";

  /** Whose property it is */
  of: Of;

  /** The property's key */
  key: string;

  /** What it asks of the property's value */
  test: Test;
}

/** A test of one property of the Event or line a filter is about, its key in a field of its own,
 * so a key holding a dot is one key. It names no subject: a filter tests the one object it is
 * about. */
model FilterPropertyLeafOf<Test> {
  kind: "property";

  /** The property's key */
  key: string;

  /** What it asks of the property's value */
  test: Test;
}

model AttributeLeaf is AttributeLeafOf<AttributeTest>;
model AttributeLeafInput is AttributeLeafOf<AttributeTestInput>;
model BirthdayLeaf is BirthdayLeafOf<BirthdayTest>;
model BirthdayLeafInput is BirthdayLeafOf<BirthdayTestInput>;
model TagLeafInput is TagLeaf;
model TelegramLeafInput is TelegramLeaf;
model SmsLeafInput is SmsLeaf;
model EngagementTrackingLeaf
  is EngagementTrackingLeafOf<EngagementTrackingTest>;
model EngagementTrackingLeafInput
  is EngagementTrackingLeafOf<EngagementTrackingTestInput>;
model CustomFieldLeaf is CustomFieldLeafOf<CustomFieldTest>;
model CustomFieldLeafInput is CustomFieldLeafOf<CustomFieldTestInput>;
model EventLeafInput is EventLeaf;
model RulePropertyLeaf is PropertyLeafOf<RulePropertySource, PropertyTest>;
model BranchPropertyLeaf is PropertyLeafOf<BranchPropertySource, PropertyTest>;
model BranchPropertyLeafInput is PropertyLeafOf<
  BranchPropertySourceInput,
  PropertyTestInput
>;
model RulePropertyLeafInput is PropertyLeafOf<
  RulePropertySourceInput,
  PropertyTestInput
>;
model FilterPropertyLeaf is FilterPropertyLeafOf<PropertyTest>;
model FilterPropertyLeafInput is FilterPropertyLeafOf<PropertyTestInput>;

/** A group of a Condition: its entries combined with and/or, optionally negated. An empty group
 * matches every Contact (an empty negated group matches none). */
model ConditionOf<Node> {
  kind: "group";

  /** How the entries combine */
  combinator: ConditionCombinator;

  /** Negate the whole group */
  not?: boolean;

  /** The group's tests and nested groups */
  entries: Node[];
}

// A place's union is named …Node so that it sorts after its group (…Condition): the TS generator
// then breaks the recursion at the group's `entries`, and every union stays a discriminated one.

/** An entry of a Segment's Condition, by `kind`: a test of the Contact, an event count or a nested
 * group */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SegmentConditionNode {
  group: SegmentCondition,
  attribute: AttributeLeaf,
  birthday: BirthdayLeaf,
  tag: TagLeaf,
  telegram: TelegramLeaf,
  sms: SmsLeaf,
  engagementTracking: EngagementTrackingLeaf,
  customField: CustomFieldLeaf,
  event: EventLeaf,
}

/** An entry of a Segment's Condition, by `kind`: a test of the Contact, an event count or a nested
 * group */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SegmentConditionNodeInput {
  group: SegmentConditionInput,
  attribute: AttributeLeafInput,
  birthday: BirthdayLeafInput,
  tag: TagLeafInput,
  telegram: TelegramLeafInput,
  sms: SmsLeafInput,
  engagementTracking: EngagementTrackingLeafInput,
  customField: CustomFieldLeafInput,
  event: EventLeafInput,
}

/** A Segment's Condition: the tree of tests a Contact matches */
model SegmentCondition is ConditionOf<SegmentConditionNode>;

/** A Segment's Condition: the tree of tests a Contact matches */
model SegmentConditionInput is ConditionOf<SegmentConditionNodeInput>;

/** An entry of a Rule's Condition, by `kind`: a Segment's, or a test of a property of the Event
 * that fired the Rule */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union RuleConditionNode {
  group: RuleCondition,
  attribute: AttributeLeaf,
  birthday: BirthdayLeaf,
  tag: TagLeaf,
  telegram: TelegramLeaf,
  sms: SmsLeaf,
  engagementTracking: EngagementTrackingLeaf,
  customField: CustomFieldLeaf,
  event: EventLeaf,
  property: RulePropertyLeaf,
}

/** An entry of a Rule's Condition, by `kind`: a Segment's, or a test of a property of the Event
 * that fired the Rule */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union RuleConditionNodeInput {
  group: RuleConditionInput,
  attribute: AttributeLeafInput,
  birthday: BirthdayLeafInput,
  tag: TagLeafInput,
  telegram: TelegramLeafInput,
  sms: SmsLeafInput,
  engagementTracking: EngagementTrackingLeafInput,
  customField: CustomFieldLeafInput,
  event: EventLeafInput,
  property: RulePropertyLeafInput,
}

/** A Rule's Condition: the tree of tests the Contact and the Event that fired the Rule match */
model RuleCondition is ConditionOf<RuleConditionNode>;

/** A Rule's Condition: the tree of tests the Contact and the Event that fired the Rule match */
model RuleConditionInput is ConditionOf<RuleConditionNodeInput>;

/** An entry of a Branch path's Condition, by `kind`: a Rule's, the property test reading the
 * Event the Enrollment began with */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union BranchConditionNode {
  group: BranchCondition,
  attribute: AttributeLeaf,
  birthday: BirthdayLeaf,
  tag: TagLeaf,
  telegram: TelegramLeaf,
  sms: SmsLeaf,
  engagementTracking: EngagementTrackingLeaf,
  customField: CustomFieldLeaf,
  event: EventLeaf,
  property: BranchPropertyLeaf,
}

/** An entry of a Branch path's Condition, by `kind`: a Rule's, the property test reading the
 * Event the Enrollment began with */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union BranchConditionNodeInput {
  group: BranchConditionInput,
  attribute: AttributeLeafInput,
  birthday: BirthdayLeafInput,
  tag: TagLeafInput,
  telegram: TelegramLeafInput,
  sms: SmsLeafInput,
  engagementTracking: EngagementTrackingLeafInput,
  customField: CustomFieldLeafInput,
  event: EventLeafInput,
  property: BranchPropertyLeafInput,
}

/** A Branch path's Condition: the tree of tests the Contact, on its live data when the Enrollment
 * reaches the Branch, and the Event the Enrollment began with match */
model BranchCondition is ConditionOf<BranchConditionNode>;

/** A Branch path's Condition: the tree of tests the Contact, on its live data when the Enrollment
 * reaches the Branch, and the Event the Enrollment began with match */
model BranchConditionInput is ConditionOf<BranchConditionNodeInput>;

/** An entry of an Event filter, by `kind`: a test of a property of the one Event the filter is
 * about, or a nested group. A test of the Contact is a Rule's, never a filter's (ADR 0133). */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union EventFilterNode {
  group: EventFilter,
  property: FilterPropertyLeaf,
}

/** An entry of an Event filter, by `kind`: a test of a property of the one Event the filter is
 * about, or a nested group. A test of the Contact is a Rule's, never a filter's (ADR 0133). */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union EventFilterNodeInput {
  group: EventFilterInput,
  property: FilterPropertyLeafInput,
}

/** An Event matcher's filter (ADR 0133): the tests of the Event's own properties it holds. A
 * property the Event lacks, or holds with another type, fails its test, so a `not` over it holds. */
model EventFilter is ConditionOf<EventFilterNode>;

/** An Event matcher's filter (ADR 0133): the tests of the Event's own properties it holds. A
 * property the Event lacks, or holds with another type, fails its test, so a `not` over it holds. */
model EventFilterInput is ConditionOf<EventFilterNodeInput>;

/** The tests a Condition's Custom field test takes on a field of one type, each with the
 * operators it takes there; an empty list is a test the type does not take */
model CustomFieldConditionTests {
  /** The operators of a text test */
  text: TextOperator[];

  /** The operators of a number test */
  number: NumberOperator[];

  /** The operators of a moment test */
  moment: MomentOperator[];

  /** Whether it takes the test of the last days */
  withinLast: boolean;

  /** The operators of a presence test */
  presence: PresenceOperator[];
}
