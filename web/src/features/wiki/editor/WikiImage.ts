import Image from '@tiptap/extension-image'

/**
 * WikiImage is a block image of a campaign asset. The document stores assetId and alt only;
 * the address (/assets/<assetId>) is derived when the node is shown.
 */
export const WikiImage = Image.extend({
  name: 'image',

  addAttributes() {
    return {
      assetId: {
        default: '',
        isRequired: true,
        parseHTML: (el) => el.getAttribute('data-asset-id') ?? '',
        renderHTML: (attrs) => {
          const assetId = String(attrs.assetId)
          return assetId === '' ? {} : { 'data-asset-id': assetId, src: `/assets/${assetId}` }
        },
      },
      alt: {
        default: '',
        parseHTML: (el) => el.getAttribute('alt') ?? '',
        renderHTML: (attrs) => ({ alt: String(attrs.alt) }),
      },
    }
  },

  parseHTML() {
    // Only images of campaign assets: a pasted <img src="https://..."> is dropped.
    return [{ tag: 'img[data-asset-id]' }]
  },
})
