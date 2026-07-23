export type HardDeleteOperationStatus = 'idle' | 'submitting' | 'succeeded' | 'failed'

export type HardDeleteOperationSnapshot<TResult> = {
  status: HardDeleteOperationStatus
  result: TResult | null
  error: unknown
}

type HardDeleteOperationListener<TResult> = (
  snapshot: HardDeleteOperationSnapshot<TResult>,
) => void

export class HardDeleteOperation<TResult> {
  private snapshot: HardDeleteOperationSnapshot<TResult> = {
    status: 'idle',
    result: null,
    error: null,
  }

  private commandPromise: Promise<TResult> | null = null

  private readonly listeners = new Set<HardDeleteOperationListener<TResult>>()

  current(): HardDeleteOperationSnapshot<TResult> {
    return this.snapshot
  }

  subscribe(listener: HardDeleteOperationListener<TResult>): () => void {
    this.listeners.add(listener)
    listener(this.snapshot)
    return () => {
      this.listeners.delete(listener)
    }
  }

  authorize(command: () => Promise<TResult>): Promise<TResult> {
    if (this.commandPromise) return this.commandPromise
    if (this.snapshot.status !== 'idle') {
      return this.snapshot.status === 'succeeded'
        ? Promise.resolve(this.snapshot.result as TResult)
        : Promise.reject(this.snapshot.error ?? new Error('Delete authorization is no longer active.'))
    }

    this.update({ status: 'submitting', result: null, error: null })
    this.commandPromise = Promise.resolve()
      .then(command)
      .then((result) => {
        this.update({ status: 'succeeded', result, error: null })
        return result
      })
      .catch((error: unknown) => {
        this.update({ status: 'failed', result: null, error })
        throw error
      })
    return this.commandPromise
  }

  prepareRetry(): boolean {
    if (this.snapshot.status !== 'failed') return false
    this.commandPromise = null
    this.update({ status: 'idle', result: null, error: null })
    return true
  }

  private update(snapshot: HardDeleteOperationSnapshot<TResult>) {
    this.snapshot = snapshot
    for (const listener of this.listeners) listener(snapshot)
  }
}

export class HardDeleteOperationRegistry<TResult> {
  private readonly operations = new Map<string, HardDeleteOperation<TResult>>()

  get(key: string): HardDeleteOperation<TResult> {
    const existing = this.operations.get(key)
    if (existing) return existing
    const operation = new HardDeleteOperation<TResult>()
    this.operations.set(key, operation)
    return operation
  }
}
