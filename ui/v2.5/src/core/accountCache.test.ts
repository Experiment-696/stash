import assert from "node:assert/strict";
import test from "node:test";
import { InMemoryCache, gql } from "@apollo/client/core/index.js";
import { clearNormalizedAccountCache } from "./accountCache.js";

const meQuery = gql`
  query AccountCacheTestMe {
    me {
      id
      username
    }
  }
`;

test("account switch cannot retain the previous account's normalized data", () => {
  const cache = new InMemoryCache();
  cache.writeQuery({
    query: meQuery,
    data: { me: { __typename: "UserAccount", id: "1", username: "first" } },
  });
  assert.equal(cache.readQuery<{ me: { username: string } }>({ query: meQuery })?.me.username, "first");

  clearNormalizedAccountCache(cache);
  assert.deepEqual(cache.extract(), {});
  assert.equal(cache.readQuery({ query: meQuery }), null);

  cache.writeQuery({
    query: meQuery,
    data: { me: { __typename: "UserAccount", id: "2", username: "second" } },
  });
  const snapshot = cache.extract();
  assert.equal(cache.readQuery<{ me: { username: string } }>({ query: meQuery })?.me.username, "second");
  assert.equal(Object.values(snapshot).some((value) => JSON.stringify(value).includes("first")), false);
});
