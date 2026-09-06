import type { ApolloCache, NormalizedCacheObject } from "@apollo/client";

export function clearNormalizedAccountCache(
  cache: ApolloCache<NormalizedCacheObject>
) {
  cache.restore({});
}
