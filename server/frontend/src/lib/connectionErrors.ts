/** Guidance for a connection failure the operator cannot act on from the raw
 * server sentence alone. */
export interface ConnectionFailureExplanation {
  title: string
  detail: string
  /** True when retrying the same action cannot succeed. */
  terminal: boolean
}

const EXPLANATIONS: Array<{ match: RegExp; explain: (providerName: string) => ConnectionFailureExplanation }> = [
  {
    // The server refuses to create a connection when a sign-in provider
    // authorizes from a cached session instead of asking the account holder.
    match: /did not start interactive account authorization/i,
    explain: (providerName) => ({
      title: `${providerName} did not ask you to sign in`,
      detail:
        `MGA requires a fresh sign-in for each new ${providerName} connection and will not reuse a session `
        + `left over from earlier. The provider plugin answered without starting one, so nothing was created `
        + `and no credentials were stored. This needs a fix in the ${providerName} plugin; retrying will fail `
        + `the same way.`,
      terminal: true,
    }),
  },
  {
    match: /OAuth sign-in state is missing or expired/i,
    explain: (providerName) => ({
      title: 'The sign-in expired',
      detail: `Start the ${providerName} sign-in again — the previous approval is no longer valid.`,
      terminal: false,
    }),
  },
  {
    match: /OAuth sign-in has not completed/i,
    explain: (providerName) => ({
      title: 'Sign-in is not finished yet',
      detail: `Approve access in the ${providerName} tab, then continue.`,
      terminal: false,
    }),
  },
  {
    match: /state does not belong to this profile/i,
    explain: () => ({
      title: 'That sign-in belongs to another profile',
      detail: 'Start the sign-in again from the profile that owns this connection.',
      terminal: false,
    }),
  },
]

/**
 * Turns a server rejection into something the operator can act on. Returns null
 * when the server's own message is already clear, so ordinary validation errors
 * are shown verbatim rather than being paraphrased.
 */
export function explainConnectionFailure(
  error: unknown,
  providerName: string,
): ConnectionFailureExplanation | null {
  const message = error instanceof Error ? error.message : typeof error === 'string' ? error : ''
  if (!message) return null
  const entry = EXPLANATIONS.find((candidate) => candidate.match.test(message))
  return entry ? entry.explain(providerName) : null
}
