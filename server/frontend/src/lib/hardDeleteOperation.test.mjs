import assert from 'node:assert/strict'
import test from 'node:test'
import {
  HardDeleteOperation,
  HardDeleteOperationRegistry,
} from './hardDeleteOperation.ts'

test('one authorization submits at most one destructive command across refocus-style reads', async () => {
  const registry = new HardDeleteOperationRegistry()
  const firstView = registry.get('game-1:source-1')
  let calls = 0
  let resolveCommand
  const command = () => {
    calls += 1
    return new Promise((resolve) => {
      resolveCommand = resolve
    })
  }

  const first = firstView.authorize(command)
  const refocusedView = registry.get('game-1:source-1')
  const second = refocusedView.authorize(command)

  assert.equal(firstView, refocusedView)
  assert.equal(calls, 0)
  await Promise.resolve()
  assert.equal(calls, 1)
  assert.equal(refocusedView.current().status, 'submitting')

  resolveCommand({ deleted: true })
  assert.deepEqual(await first, { deleted: true })
  assert.deepEqual(await second, { deleted: true })
  assert.equal(calls, 1)
  assert.equal(refocusedView.current().status, 'succeeded')
})

test('render retries and navigation remounts restore completed state without resubmission', async () => {
  const registry = new HardDeleteOperationRegistry()
  const operation = registry.get('game-2:source-2')
  const states = []
  const unsubscribe = operation.subscribe((snapshot) => states.push(snapshot.status))

  const result = await operation.authorize(async () => 'done')
  const remountedStates = []
  const remountedOperation = registry.get('game-2:source-2')
  remountedOperation.subscribe((snapshot) => remountedStates.push(snapshot.status))

  assert.equal(result, 'done')
  assert.equal(remountedOperation, operation)
  assert.deepEqual(states, ['idle', 'submitting', 'succeeded'])
  assert.deepEqual(remountedStates, ['succeeded'])
  unsubscribe()
})

test('a failed command needs an explicit retry reset before another authorization', async () => {
  const operation = new HardDeleteOperation()
  let calls = 0
  const failure = new Error('failed once')

  await assert.rejects(
    operation.authorize(async () => {
      calls += 1
      throw failure
    }),
    failure,
  )
  await assert.rejects(operation.authorize(async () => 'unexpected'), failure)
  assert.equal(calls, 1)

  assert.equal(operation.prepareRetry(), true)
  assert.equal(operation.current().status, 'idle')
  assert.equal(await operation.authorize(async () => {
    calls += 1
    return 'done'
  }), 'done')
  assert.equal(calls, 2)
})
