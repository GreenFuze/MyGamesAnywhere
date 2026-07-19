import type { QueryClient } from '@tanstack/react-query'

// Cancel in-flight profile requests and synchronously remove every cached
// projection before React observes a different selected profile.
export async function clearProfileOwnedQueryCache(queryClient: QueryClient): Promise<void> {
  const cancellation = queryClient.cancelQueries()
  queryClient.clear()
  await cancellation
}
