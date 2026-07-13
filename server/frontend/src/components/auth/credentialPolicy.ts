import type { CredentialKind } from '@/api/client'

class CredentialPolicy {
  isValid(kind: CredentialKind, value: string): boolean {
    if (kind === 'pin') return /^\d{4,}$/.test(value)
    return [...value].length >= 4
  }

  label(kind: CredentialKind): string {
    return kind === 'pin' ? 'New PIN (4+ digits)' : 'New password (4+ characters)'
  }
}

export const credentialPolicy = new CredentialPolicy()
