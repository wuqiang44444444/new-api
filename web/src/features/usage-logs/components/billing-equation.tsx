/** Wrap equations between operands without splitting a monetary amount. */
export function BillingEquation(props: {
  quantity: string
  price: string
  subtotal: string
}) {
  return (
    <span className='inline-flex max-w-full flex-wrap gap-x-1 break-normal'>
      <span className='whitespace-nowrap'>{props.quantity}</span>{' '}
      <span className='whitespace-nowrap'>× {props.price}</span>{' '}
      <span className='whitespace-nowrap'>= {props.subtotal}</span>
    </span>
  )
}
