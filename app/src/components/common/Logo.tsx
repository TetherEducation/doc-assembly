import { BrandIcon } from '@/components/common/BrandIcon'
import { APP_NAME } from '@/lib/brand'
import { cn } from '@/lib/utils'

interface LogoProps {
  size?: 'sm' | 'md' | 'lg'
  showText?: boolean
  className?: string
}

const sizeClasses = {
  sm: {
    container: 'w-6 h-6',
    icon: 14,
    text: 'text-sm',
  },
  md: {
    container: 'w-8 h-8',
    icon: 16,
    text: 'text-lg',
  },
  lg: {
    container: 'w-10 h-10',
    icon: 20,
    text: 'text-xl',
  },
}

export function Logo({ size = 'md', showText = true, className }: LogoProps) {
  const sizes = sizeClasses[size]

  return (
    <div className={cn('flex items-center gap-3', className)}>
      <div
        className={cn(
          'flex items-center justify-center border-2 border-foreground',
          sizes.container
        )}
      >
        <BrandIcon size={sizes.icon} className="text-foreground" />
      </div>
      {showText && (
        <span
          className={cn(
            'font-display font-bold tracking-tight uppercase',
            sizes.text
          )}
        >
          {APP_NAME}
        </span>
      )}
    </div>
  )
}
