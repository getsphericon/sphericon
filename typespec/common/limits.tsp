// The send limits a caller may set, shared by every contract that takes one: an Integration's
// Provider limit on /site and /api, a Workspace's Send cap on the Staff API (ADR 0067).

namespace Sphericon;

/** A rolling 24-hour limit the caller may set: a Provider limit's daily value or a Send cap */
@minValue(1)
@maxValue(100000000)
scalar MaxPerDay extends int32;
