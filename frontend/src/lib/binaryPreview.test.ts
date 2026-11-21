import { expect, it } from 'vitest'
import { binaryPreview } from './binaryPreview'
it('decodes UTF-8 text while rejecting invalid, oversized and executable image formats', () => {
 expect(binaryPreview('0xD09FD180D0B8D0B2D0B5D182').text).toBe('Привет')
 expect(binaryPreview('0xDEADBEEF').text).toBe(null)
 expect(binaryPreview('0xxyz').text).toBe(null)
 expect(binaryPreview('0x3c7376673e').image).toBe(null)
 expect(binaryPreview('0x'+'61'.repeat(524289)).text).toBe(null)
})
it('recognizes safe raster signatures without enabling SVG or HTML', () => {
 expect(binaryPreview('0x89504E470D0A1A0A').image).toBe('data:image/png;base64,iVBORw0KGgo=')
 expect(binaryPreview('0x3C696D67207372633D783E').text).toBe('<img src=x>')
 expect(binaryPreview('0x3C696D67207372633D783E').image).toBe(null)
})
