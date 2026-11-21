// SPIKE (#1674, ADR 0181): a Condition's tree per place. Throwaway: proves the shape through every
// generator; the move itself is #1676. One group template over the entries a place admits; each
// place declares its entry union once per form (the request's closed, the response's open, ADR
// 0148).

namespace Sphericon;

/** How a text test compares the value with its own */
enum TextOperator {
  eq,
  neq,
  contains,
  beginsWith,
}

/** A test of a text value. A number or boolean Custom field's eq and neq are text tests too,
 * compared with the stored value's text. */
model TextTest {
  kind: "text";

  /** How the value compares */
  operator: TextOperator;

  /** The text compared with */
  value: string;
}

/** How a number test compares the value with its own */
enum NumberOperator {
  gt,
  gte,
  lt,
  lte,
}

/** A test of a number. A missing value, or one that is not a number, fails it. */
model NumberTest {
  kind: "number";

  /** How the value compares */
  operator: NumberOperator;

  /** The number compared with */
  value: float64;
}

/** Which side of `at` a moment is on: before is strictly earlier, after is at or later */
enum MomentOperator {
  before,
  after,
}

/** A test of a moment: before or after `at`. Only a date-time with a UTC offset is a moment. */
model MomentTest {
  kind: "moment";

  /** Which side of `at` the moment is on */
  operator: MomentOperator;

  /** The moment compared with */
  at: Timestamp;
}

/** A test of a moment: within the last `days` days up to now, so a moment still to come is not
 * within */
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

/** What a Contact attribute test asks of a text attribute, by `kind` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union AttributeTest {
  text: TextTest,
  presence: PresenceTest,
}

/** What a Contact attribute test asks of a text attribute, by `kind` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union AttributeTestInput {
  text: TextTestInput,
  presence: PresenceTestInput,
}

/** What a Custom field test asks of the field's value, by `kind`; whether it fits the field's
 * type is checked at save */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union CustomFieldTest {
  text: TextTest,
  number: NumberTest,
  moment: MomentTest,
  withinLast: WithinLastTest,
  presence: PresenceTest,
}

/** What a Custom field test asks of the field's value, by `kind`; whether it fits the field's
 * type is checked at save */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union CustomFieldTestInput {
  text: TextTestInput,
  number: NumberTestInput,
  moment: MomentTestInput,
  withinLast: WithinLastTestInput,
  presence: PresenceTestInput,
}

/** What a property test asks of the property's value, by `kind` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union PropertyTest {
  text: TextTest,
  number: NumberTest,
  presence: PresenceTest,
}

/** What a property test asks of the property's value, by `kind` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union PropertyTestInput {
  text: TextTestInput,
  number: NumberTestInput,
  presence: PresenceTestInput,
}

/** A text attribute of a Contact */
enum ContactAttribute {
  email,
  phone,
  subjectId,
  firstName,
  lastName,
  timeZone,
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

/** A count of a Contact's Events of one action, over all time or within the last `withinDays`
 * days */
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

/** Whose properties a Rule's property test reads, by `kind`: only the triggering Event */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union RulePropertySource {
  trigger: TriggerSource,
}

/** Whose properties a Rule's property test reads, by `kind`: only the triggering Event */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union RulePropertySourceInput {
  trigger: TriggerSourceInput,
}

/** A test of one property of an Event: whose it is in `of`, its key in a field of its own, so a
 * key holding a dot needs no rule */
model PropertyLeafOf<Of, Test> {
  kind: "property";

  /** Whose property it is */
  of: Of;

  /** The property's key */
  key: string;

  /** What it asks of the property's value */
  test: Test;
}

model AttributeLeaf is AttributeLeafOf<AttributeTest>;
model AttributeLeafInput is AttributeLeafOf<AttributeTestInput>;
model TagLeafInput is TagLeaf;
model CustomFieldLeaf is CustomFieldLeafOf<CustomFieldTest>;
model CustomFieldLeafInput is CustomFieldLeafOf<CustomFieldTestInput>;
model EventLeafInput is EventLeaf;
model RulePropertyLeaf is PropertyLeafOf<RulePropertySource, PropertyTest>;
model RulePropertyLeafInput is PropertyLeafOf<
  RulePropertySourceInput,
  PropertyTestInput
>;

/** A group of a Condition: its entries combined with and/or, optionally negated. An empty group
 * matches every Contact (an empty negated group matches none). */
model ConditionOf<Entry> {
  kind: "group";

  /** How the entries combine */
  combinator: ConditionCombinator;

  /** Negate the whole group */
  not?: boolean;

  /** The group's tests and nested groups */
  entries: Entry[];
}

/** An entry of a Segment's Condition, by `kind`: a test of the Contact, an event count, or a nested group */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SegmentConditionNode {
  group: SegmentCondition,
  attribute: AttributeLeaf,
  tag: TagLeaf,
  customField: CustomFieldLeaf,
  event: EventLeaf,
}

/** An entry of a Segment's Condition, by `kind`: a test of the Contact, an event count, or a nested group */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SegmentConditionNodeInput {
  group: SegmentConditionInput,
  attribute: AttributeLeafInput,
  tag: TagLeafInput,
  customField: CustomFieldLeafInput,
  event: EventLeafInput,
}

/** A Segment's Condition: the tree of tests a Contact matches */
model SegmentCondition is ConditionOf<SegmentConditionNode>;

/** A Segment's Condition: the tree of tests a Contact matches */
model SegmentConditionInput is ConditionOf<SegmentConditionNodeInput>;

/** An entry of a Rule's Condition, by `kind`: a Segment's, or a test of the triggering Event's property */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union RuleConditionNode {
  group: RuleCondition,
  attribute: AttributeLeaf,
  tag: TagLeaf,
  customField: CustomFieldLeaf,
  event: EventLeaf,
  property: RulePropertyLeaf,
}

/** An entry of a Rule's Condition, by `kind`: a Segment's, or a test of the triggering Event's property */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union RuleConditionNodeInput {
  group: RuleConditionInput,
  attribute: AttributeLeafInput,
  tag: TagLeafInput,
  customField: CustomFieldLeafInput,
  event: EventLeafInput,
  property: RulePropertyLeafInput,
}

/** A Rule's Condition: the tree of tests the Contact and the triggering Event match */
model RuleCondition is ConditionOf<RuleConditionNode>;

/** A Rule's Condition: the tree of tests the Contact and the triggering Event match */
model RuleConditionInput is ConditionOf<RuleConditionNodeInput>;

/** SPIKE: a Segment's Condition sent to be stored and read back */
model SpikeSegmentConditionInput {
  condition: SegmentConditionInput;
}

/** SPIKE: a Segment's Condition as stored */
model SpikeSegmentConditionResult {
  condition: SegmentCondition;
}

/** SPIKE: a Rule's Condition sent to be stored and read back */
model SpikeRuleConditionInput {
  condition: RuleConditionInput;
}

/** SPIKE: a Rule's Condition as stored */
model SpikeRuleConditionResult {
  condition: RuleCondition;
}
