// The choices of the Sign-up survey: /site takes the answers, the Staff console counts them.

namespace Sphericon;

/** What the User does (Sign-up survey, first question) */
enum SignupRole {
  founder,
  marketer,
  developer,
  product,

  /** None of the above; the answer may say what in a text of its own */
  other,
}

/** How many people work at the User's company (second question) */
enum SignupCompanySize {
  /** Only the User */
  one: "1",

  /** 2 to 10 people */
  twoToTen: "2_10",

  /** 11 to 50 people */
  elevenToFifty: "11_50",

  /** 51 to 200 people */
  fiftyOneToTwoHundred: "51_200",

  /** 201 to 1000 people */
  twoHundredOneToThousand: "201_1000",

  /** More than 1000 people */
  overThousand: "1000_plus",

  /** None of the above; the answer may say what in a text of its own */
  other,
}

/** What the User mainly wants to do with the product (third question) */
enum SignupGoal {
  broadcasts,
  automations,
  tracking,

  /** Moving over from another tool */
  migrating,

  /** None of the above; the answer may say what in a text of its own */
  other,
}

/** Where the User heard about the product (fourth question, optional) */
enum SignupSource {
  search,
  github,
  social,
  recommendation,

  /** None of the above; the answer may say what in a text of its own */
  other,
}
