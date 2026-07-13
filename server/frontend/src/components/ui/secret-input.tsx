import { forwardRef, useState } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { Input, type InputProps } from '@/components/ui/input'

export type SecretInputProps = Omit<InputProps, 'type' | 'trailing'>

/** Password/PIN input with an accessible, reusable visibility toggle. */
export const SecretInput = forwardRef<HTMLInputElement, SecretInputProps>((props, ref) => {
  const [visible, setVisible] = useState(false)
  const action = visible ? 'Hide' : 'Show'
  const fieldName = props.label?.toLowerCase() || 'secret'

  return (
    <Input
      {...props}
      ref={ref}
      type={visible ? 'text' : 'password'}
      trailing={(
        <button
          type="button"
          onClick={() => setVisible((current) => !current)}
          className="grid h-8 w-8 place-items-center rounded-mga text-mga-muted transition-colors hover:bg-mga-elevated hover:text-mga-text focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
          aria-label={`${action} ${fieldName}`}
          title={`${action} ${fieldName}`}
        >
          {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      )}
    />
  )
})

SecretInput.displayName = 'SecretInput'
