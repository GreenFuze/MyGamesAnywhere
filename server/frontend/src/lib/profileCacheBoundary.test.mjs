import assert from 'node:assert/strict'
import test from 'node:test'
import { QueryClient } from '@tanstack/react-query'
import { clearProfileOwnedQueryCache } from './profileCacheBoundary.ts'

test('profile switch removes cached and delayed profile data before refetch', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const keys = [['games'], ['integrations'], ['notifications'], ['recent'], ['save-sync']]
  for (const key of keys) client.setQueryData(key, { owner: 'profile-a' })
  const pending = client.fetchQuery({
    queryKey: ['delayed-library'],
    queryFn: ({ signal }) => new Promise((_, reject) => signal.addEventListener('abort', () => reject(new Error('cancelled')), { once: true })),
  }).catch(() => undefined)

  await clearProfileOwnedQueryCache(client)
  await pending

  for (const key of [...keys, ['delayed-library']]) {
    assert.equal(client.getQueryData(key), undefined, `${key[0]} survived profile switch`)
  }
  assert.equal(client.getQueryCache().getAll().length, 0)
})
