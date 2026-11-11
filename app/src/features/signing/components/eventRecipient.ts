/**
 * The signer a recipient event is about, from the metadata the signing
 * timeline records ("Name (email)"), or null when it names no one.
 */
export function eventRecipient(
  metadata: Record<string, unknown> | undefined,
): string | null {
  const name =
    typeof metadata?.recipientName === 'string'
      ? metadata.recipientName.trim()
      : ''
  const email =
    typeof metadata?.recipientEmail === 'string'
      ? metadata.recipientEmail.trim()
      : ''
  if (name && email && name !== email) return `${name} (${email})`
  return name || email || null
}
