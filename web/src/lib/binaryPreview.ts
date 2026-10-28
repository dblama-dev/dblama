export function binaryPreview(hex: string): {text: string | null; image: string | null} {
  const empty = { text: null, image: null }
  if (!/^0x(?:[0-9a-f]{2})*$/i.test(hex) || hex.length > 1048578) return empty
  const bytes = new Uint8Array((hex.length - 2) / 2)
  for (let i = 0; i < bytes.length; i++) bytes[i] = parseInt(hex.slice(2 + i * 2, 4 + i * 2), 16)
  let text: string | null = null
  try { text = new TextDecoder('utf-8', { fatal: true }).decode(bytes) } catch { /* binary data */ }
  const signature = hex.slice(2).toLowerCase()
  const mime = signature.startsWith('89504e470d0a1a0a') ? 'image/png'
    : signature.startsWith('ffd8ff') ? 'image/jpeg'
    : /^(474946383761|474946383961)/.test(signature) ? 'image/gif'
    : signature.startsWith('52494646') && signature.slice(16, 24) === '57454250' ? 'image/webp' : null
  let image: string | null = null
  if (mime) {
    let binary = ''
    for (const byte of bytes) binary += String.fromCharCode(byte)
    image = `data:${mime};base64,${btoa(binary)}`
  }
  return { text, image }
}
