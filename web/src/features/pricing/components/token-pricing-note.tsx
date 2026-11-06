import { useTranslation } from 'react-i18next'

/** Shared explanation of the existing token-expression contract. */
export function TokenPricingNote(props: { imageGeneration?: boolean }) {
  const { t } = useTranslation()
  if (props.imageGeneration) {
    return (
      <div className='text-muted-foreground my-2 space-y-1 text-xs'>
        <p>
          {t(
            'Text input is your prompt; image input is your reference image; image output is the generated image.'
          )}
        </p>
        <p>
          {t(
            'Cached text/image input: the provider reuses previously processed text or images. Cache rates apply to the cache usage reported by the provider.'
          )}
        </p>
      </div>
    )
  }
  return (
    <div className='text-muted-foreground my-2 space-y-1 text-xs'>
      <p>
        {t(
          'Input and output cover tokens not priced separately. Image input, image output and cache tokens use their own listed rates when separately priced.'
        )}
      </p>
      <p>
        {t(
          'Cache prices do not guarantee cache availability or a cache hit. Cache charges depend on the usage reported by the upstream provider.'
        )}
      </p>
    </div>
  )
}
