import { useTranslation } from 'react-i18next'

import type { EvidenceContent, EvidenceMessage } from '../evidence-api'

// Read-only source display: the editor's JSON.parse/stringify formatting would
// lose large numeric lexemes and duplicate keys. Formatting comes from the server.
export function EvidenceContentPreview(props: {
  content: EvidenceContent
  formatted: boolean
}) {
  const { t } = useTranslation()
  if (props.content.messages) {
    return (
      <ol className='max-h-[65vh] space-y-3 overflow-auto'>
        {props.content.messages.map((message, index) => (
          <li key={message.offset} className='space-y-1 rounded-md border p-3'>
            <p className='font-medium'>
              {t('Message {{number}}', { number: index + 1 })}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t('Message type')}: {message.message_type} ·{' '}
              {t('Original bytes')}: {message.original_bytes} ·{' '}
              {new Date(message.created_at_ms).toLocaleString()}
            </p>
            {message.error && (
              <pre className='break-all whitespace-pre-wrap'>
                {message.error}
              </pre>
            )}
            {message.message_type === 2 ? (
              <p>{t('Binary message: metadata only')}</p>
            ) : (
              <EvidenceTextPreview
                content={message}
                formatted={props.formatted}
              />
            )}
          </li>
        ))}
      </ol>
    )
  }
  return <EvidenceTextPreview {...props} />
}

function EvidenceTextPreview(props: {
  content: Pick<
    EvidenceContent | EvidenceMessage,
    'text' | 'base64' | 'formatted_text' | 'json_status' | 'display_limited'
  >
  formatted: boolean
}) {
  const { t } = useTranslation()
  const text =
    props.formatted && props.content.formatted_text != null
      ? props.content.formatted_text
      : props.content.text
  return (
    <>
      {props.formatted &&
        props.content.json_status &&
        props.content.json_status !== 'formatted' && (
          <p role='status' className='text-muted-foreground text-sm'>
            {t('JSON formatting is unavailable. Original text is shown.')}
          </p>
        )}
      {props.content.display_limited && (
        <p className='text-muted-foreground text-sm'>
          {t('Message preview is incomplete.')}
        </p>
      )}
      {typeof text === 'string' && (
        <pre className='max-h-[65vh] overflow-auto rounded-md border p-3 font-mono text-xs break-all whitespace-pre-wrap'>
          {text}
        </pre>
      )}
      {props.content.base64 && (
        <pre
          aria-label='Base64'
          className='max-h-48 overflow-auto break-all whitespace-pre-wrap'
        >
          {props.content.base64}
        </pre>
      )}
    </>
  )
}
