import React from 'react'
import { Avatar } from './Avatar'
import styles from './Avatar.module.css'
import groupStyles from './AvatarGroup.module.css'

type AvatarSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl'

interface AvatarGroupItem {
  src?: string
  initials?: string
  alt?: string
}

interface AvatarGroupProps {
  avatars: AvatarGroupItem[]
  max?: number
  size?: AvatarSize
  className?: string
}

const SIZE_PX: Record<AvatarSize, number> = {
  xs: 32, sm: 40, md: 48, lg: 64, xl: 80,
}

export function AvatarGroup({
  avatars,
  max = 4,
  size = 'md',
  className,
}: AvatarGroupProps) {
  const visible = avatars.slice(0, max)
  const overflow = avatars.length - max
  const px = SIZE_PX[size]
  const overlap = Math.round(px * 0.35)

  return (
    <div
      className={[groupStyles.group, className].filter(Boolean).join(' ')}
      style={{ paddingLeft: overlap }}
    >
      {visible.map((a, i) => (
        <span
          key={i}
          className={groupStyles.item}
          style={{ marginLeft: -overlap, zIndex: visible.length - i }}
        >
          <Avatar
            src={a.src}
            initials={a.initials}
            alt={a.alt}
            size={size}
            className={groupStyles.bordered}
          />
        </span>
      ))}
      {overflow > 0 && (
        <span
          className={[groupStyles.item, groupStyles.overflow].join(' ')}
          style={{
            marginLeft: -overlap,
            width: px,
            height: px,
            fontSize: size === 'xs' ? 11 : size === 'sm' ? 12 : size === 'md' ? 13 : size === 'lg' ? 15 : 18,
          }}
        >
          +{overflow}
        </span>
      )}
    </div>
  )
}
